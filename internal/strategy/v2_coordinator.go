package strategy

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/wu-piyaphon/outbound-api/internal/indicator"
	"github.com/wu-piyaphon/outbound-api/internal/model"
	"github.com/wu-piyaphon/outbound-api/internal/repository"
	"github.com/wu-piyaphon/outbound-api/internal/service"
)

// V2Coordinator dual-executes bar events: the live v1 path runs unchanged,
// and a shadow path records what the strategy would have done for offline
// comparison.
//
// The shadow buy gate applies the regime filter (SPY > EMA-50) before running
// the signal evaluation. When the regime is off a shadow signal is logged with
// reasoning="regime_off" so the suppression is visible in the signals table.
//
// Shadow exits apply adaptive ATR trailing / break-even in a DB transaction,
// update peak_price on open buys, and record ShadowExitDecision rows whenever
// v2 would tighten stops or exit while v1 still holds (static stop / TP).
//
// shadowSignalSvc uses a different sentiment provider (LLM) than the v1
// signalService, so the two paths are independently observable.
//
// Shadow writes are best-effort at the coordinator boundary: transaction
// failures are logged but never abort the live path.
type V2Coordinator struct {
	v1              *V1Coordinator
	shadowSignalSvc service.SignalService
	tradeRepo       repository.TradeRepository
	shadowRepo      repository.ShadowRepository
	regime          indicator.RegimeReader
	transactor      repository.Transactor
	adaptive        AdaptiveExitParams
}

// NewV2Coordinator constructs a V2Coordinator. The v1 coordinator handles the
// live path; shadowSignalSvc (wired with LLM sentiment), tradeRepo, shadowRepo,
// regime, transactor, and adaptive params power the shadow path.
func NewV2Coordinator(
	v1 *V1Coordinator,
	shadowSignalSvc service.SignalService,
	tradeRepo repository.TradeRepository,
	shadowRepo repository.ShadowRepository,
	regime indicator.RegimeReader,
	transactor repository.Transactor,
	adaptive AdaptiveExitParams,
) *V2Coordinator {
	return &V2Coordinator{
		v1:              v1,
		shadowSignalSvc: shadowSignalSvc,
		tradeRepo:       tradeRepo,
		shadowRepo:      shadowRepo,
		regime:          regime,
		transactor:      transactor,
		adaptive:        adaptive,
	}
}

// EvaluateBar runs the shadow observation pass first (so it sees the same DB
// state as v1), then delegates to the live v1 path.
func (c *V2Coordinator) EvaluateBar(ctx context.Context, event BarEvent) error {
	c.shadowEvaluateExits(ctx, event)
	c.shadowEvaluateBuySignal(ctx, event)

	return c.v1.EvaluateBar(ctx, event)
}

func tradeEntryPrice(trade *model.Trade) *decimal.Decimal {
	if trade.AvgFillPrice != nil {
		return trade.AvgFillPrice
	}
	return trade.PricePerUnit
}

// shadowEvaluateExits applies adaptive peak tracking and stop tightening in a
// single transaction, then logs comparison rows when v2 diverges from v1
// while v1 still holds.
func (c *V2Coordinator) shadowEvaluateExits(ctx context.Context, event BarEvent) {
	err := c.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		openTrades, err := c.tradeRepo.GetOpenBuyTradesBySymbol(txCtx, event.Symbol)
		if err != nil {
			return fmt.Errorf("GetOpenBuyTradesBySymbol: %w", err)
		}
		for _, trade := range openTrades {
			if err := c.shadowEvaluateOneTrade(txCtx, trade, event); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		slog.Warn("shadow: exit evaluation transaction failed", "symbol", event.Symbol, "error", err)
	}
}

func (c *V2Coordinator) shadowEvaluateOneTrade(txCtx context.Context, trade *model.Trade, event BarEvent) error {
	entryPx := tradeEntryPrice(trade)

	if trade.EntryATR != nil && !trade.EntryATR.IsZero() {
		if entryPx == nil || trade.StopLoss == nil {
			return nil
		}
		return c.shadowAdaptiveExit(txCtx, trade, event, *entryPx)
	}

	return c.shadowLegacyExit(txCtx, trade, event)
}

func (c *V2Coordinator) shadowAdaptiveExit(txCtx context.Context, trade *model.Trade, event BarEvent, entry decimal.Decimal) error {
	oldPeak := entry
	if trade.PeakPrice != nil {
		oldPeak = *trade.PeakPrice
	}
	newPeak := oldPeak
	if event.Price.GreaterThan(newPeak) {
		newPeak = event.Price
	}

	oldEff := ComputeAdaptiveEffectiveStop(oldPeak, entry, *trade.EntryATR, *trade.StopLoss, c.adaptive)
	newEff := ComputeAdaptiveEffectiveStop(newPeak, entry, *trade.EntryATR, *trade.StopLoss, c.adaptive)

	v1ExitStop := event.Price.LessThanOrEqual(*trade.StopLoss)
	v1ExitTP := trade.TakeProfit != nil && event.Price.GreaterThanOrEqual(*trade.TakeProfit)
	v1Holds := !v1ExitStop && !v1ExitTP

	if !newPeak.Equal(oldPeak) {
		p := newPeak
		trade.PeakPrice = &p
		if err := c.tradeRepo.Update(txCtx, *trade); err != nil {
			return fmt.Errorf("shadow peak update trade %s: %w", trade.ID, err)
		}
	}

	if !v1Holds {
		return nil
	}

	now := time.Now().UTC()
	stopSnap := newEff

	if event.Price.LessThanOrEqual(newEff) {
		reason := "adaptive_exit"
		dec := &model.ShadowExitDecision{
			ID:           uuid.New(),
			TradeID:      trade.ID,
			BarTime:      event.BarTime,
			CurrentPrice: event.Price,
			PeakPrice:    newPeak,
			CurrentStop:  &stopSnap,
			Action:       "exit_stop",
			Reasoning:    &reason,
			CreatedAt:    now,
		}
		return c.shadowRepo.LogShadowExitDecision(txCtx, dec)
	}

	if newEff.GreaterThan(oldEff) {
		reason := "adaptive_stop_moved"
		dec := &model.ShadowExitDecision{
			ID:           uuid.New(),
			TradeID:      trade.ID,
			BarTime:      event.BarTime,
			CurrentPrice: event.Price,
			PeakPrice:    newPeak,
			CurrentStop:  &stopSnap,
			Action:       "stop_moved",
			Reasoning:    &reason,
			CreatedAt:    now,
		}
		return c.shadowRepo.LogShadowExitDecision(txCtx, dec)
	}

	return nil
}

func (c *V2Coordinator) shadowLegacyExit(txCtx context.Context, trade *model.Trade, event BarEvent) error {
	var action string
	switch {
	case trade.StopLoss != nil && event.Price.LessThanOrEqual(*trade.StopLoss):
		action = "exit_stop"
	case trade.TakeProfit != nil && event.Price.GreaterThanOrEqual(*trade.TakeProfit):
		action = "exit_take_profit"
	default:
		return nil
	}

	now := time.Now().UTC()
	dec := &model.ShadowExitDecision{
		ID:           uuid.New(),
		TradeID:      trade.ID,
		BarTime:      event.BarTime,
		CurrentPrice: event.Price,
		PeakPrice:    event.Price,
		CurrentStop:  trade.StopLoss,
		Action:       action,
		CreatedAt:    now,
	}
	return c.shadowRepo.LogShadowExitDecision(txCtx, dec)
}

// shadowEvaluateBuySignal previews whether a buy signal would fire for the
// bar and, if so, logs it to the signals table with mode='shadow'.
//
// Regime gate: when the market regime is off (SPY below EMA-50) the full
// signal evaluation is skipped and a shadow signal with reasoning="regime_off"
// is logged instead, making the suppression observable.
func (c *V2Coordinator) shadowEvaluateBuySignal(ctx context.Context, event BarEvent) {
	if !c.regime.IsRiskOn() {
		reason := "regime_off"
		sig := &model.Signal{
			ID:            uuid.New(),
			Symbol:        event.Symbol,
			Side:          model.SideBuy,
			PriceAtSignal: event.Price,
			IsExecuted:    false,
			Mode:          model.SignalModeShadow,
			Reasoning:     &reason,
			CreatedAt:     event.BarTime,
		}
		if err := c.shadowRepo.LogShadowSignal(ctx, sig); err != nil {
			slog.Warn("shadow: failed to log regime_off signal",
				"symbol", event.Symbol, "error", err)
		}
		return
	}

	sig, err := c.shadowSignalSvc.PreviewBuySignal(ctx, event.Symbol, event.Price)
	if err != nil {
		slog.Warn("shadow: failed to preview buy signal",
			"symbol", event.Symbol, "error", err)
		return
	}
	if sig == nil {
		return
	}

	if err := c.shadowRepo.LogShadowSignal(ctx, sig); err != nil {
		slog.Warn("shadow: failed to log shadow signal",
			"symbol", event.Symbol, "error", err)
	}
}
