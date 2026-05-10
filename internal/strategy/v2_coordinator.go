package strategy

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/wu-piyaphon/outbound-api/internal/model"
	"github.com/wu-piyaphon/outbound-api/internal/repository"
	"github.com/wu-piyaphon/outbound-api/internal/service"
)

// V2Coordinator dual-executes bar events: the live v1 path runs unchanged,
// and a shadow path records what the strategy would have done for offline
// comparison. In PR 0 the shadow path mirrors v1 exactly; future PRs will
// replace the shadow evaluation with an independent strategy implementation.
//
// shadowSignalSvc uses a different sentiment provider (LLM) than the v1
// signalService, so the two paths are independently observable.
//
// Shadow writes are best-effort: failures are logged but never propagate to
// the caller so the live path is always unaffected.
type V2Coordinator struct {
	v1               *V1Coordinator
	shadowSignalSvc  service.SignalService
	tradeRepo        repository.TradeRepository
	shadowRepo       repository.ShadowRepository
}

// NewV2Coordinator constructs a V2Coordinator. The v1 coordinator handles the
// live path; shadowSignalSvc (which should be wired with the LLM sentiment
// provider), tradeRepo, and shadowRepo power the shadow path.
func NewV2Coordinator(
	v1 *V1Coordinator,
	shadowSignalSvc service.SignalService,
	tradeRepo repository.TradeRepository,
	shadowRepo repository.ShadowRepository,
) *V2Coordinator {
	return &V2Coordinator{
		v1:              v1,
		shadowSignalSvc: shadowSignalSvc,
		tradeRepo:       tradeRepo,
		shadowRepo:      shadowRepo,
	}
}

// EvaluateBar runs the shadow observation pass first (so it sees the same DB
// state as v1), then delegates to the live v1 path.
func (c *V2Coordinator) EvaluateBar(ctx context.Context, event BarEvent) error {
	// Shadow pass: observe what the strategy would do — no real orders.
	// Failures are best-effort; the live path always runs regardless.
	c.shadowEvaluateExits(ctx, event)
	c.shadowEvaluateBuySignal(ctx, event)

	// Live path: unchanged v1 execution.
	return c.v1.EvaluateBar(ctx, event)
}

// shadowEvaluateExits checks open trades against their stop-loss and
// take-profit levels and logs a ShadowExitDecision for each triggered trade.
// Hold decisions are inferred by absence to keep the table compact.
func (c *V2Coordinator) shadowEvaluateExits(ctx context.Context, event BarEvent) {
	openTrades, err := c.tradeRepo.GetOpenBuyTradesBySymbol(ctx, event.Symbol)
	if err != nil {
		slog.Warn("shadow: failed to get open trades for exit evaluation",
			"symbol", event.Symbol, "error", err)
		return
	}

	now := time.Now().UTC()
	for _, trade := range openTrades {
		var action string
		switch {
		case trade.StopLoss != nil && event.Price.LessThanOrEqual(*trade.StopLoss):
			action = "exit_stop"
		case trade.TakeProfit != nil && event.Price.GreaterThanOrEqual(*trade.TakeProfit):
			action = "exit_take_profit"
		default:
			// Hold — inferred by absence; no row written.
			continue
		}

		dec := &model.ShadowExitDecision{
			ID:           uuid.New(),
			TradeID:      trade.ID,
			BarTime:      event.BarTime,
			CurrentPrice: event.Price,
			// v1 does not track a running peak; use current price as a
			// conservative proxy. Future PRs will maintain peak state.
			PeakPrice:   event.Price,
			CurrentStop: trade.StopLoss,
			Action:      action,
			CreatedAt:   now,
		}

		if err := c.shadowRepo.LogShadowExitDecision(ctx, dec); err != nil {
			slog.Warn("shadow: failed to log exit decision",
				"trade_id", trade.ID, "action", action, "error", err)
		}
	}
}

// shadowEvaluateBuySignal previews whether a buy signal would fire for the
// bar and, if so, logs it to the signals table with mode='shadow'.
func (c *V2Coordinator) shadowEvaluateBuySignal(ctx context.Context, event BarEvent) {
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
