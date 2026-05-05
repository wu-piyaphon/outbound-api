package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
	"github.com/wu-piyaphon/outbound-api/internal/model"
	"github.com/wu-piyaphon/outbound-api/internal/repository"
)

// OrderPlacer is the broker integration point for submitting market orders.
// *alpaca.Client from the Alpaca SDK satisfies this interface.
type OrderPlacer interface {
	PlaceOrder(req alpaca.PlaceOrderRequest) (*alpaca.Order, error)
}

// TradeService manages the full lifecycle of a trade: sizing, order placement,
// exit evaluation, and reconciliation of broker status updates.
type TradeService interface {
	ExecuteBuyTrade(ctx context.Context, signal *model.Signal, account *model.AccountTransfer) (*model.Trade, error)
	ExecuteSellTrade(ctx context.Context, signal *model.Signal, trade *model.Trade) (*model.Trade, error)
	EvaluateAndExecuteExits(ctx context.Context, symbol string, currentPrice decimal.Decimal) error
	ApplyTradeUpdates(ctx context.Context, update alpaca.TradeUpdate, updateStatus model.Status) error
}

type tradeService struct {
	tradeRepository           repository.TradeRepository
	accountTransferRepository repository.AccountTransferRepository
	signalRepository          repository.SignalRepository
	transactor                repository.Transactor
	orderPlacer               OrderPlacer
	riskPerTradePct           decimal.Decimal
	atrRiskMultiplier         decimal.Decimal
	takeProfitMultiplier      decimal.Decimal
}

// NewTradeService constructs a TradeService with the supplied repositories,
// broker client, and risk parameters read from configuration.
func NewTradeService(
	tradeRepository repository.TradeRepository,
	accountTransferRepository repository.AccountTransferRepository,
	signalRepository repository.SignalRepository,
	transactor repository.Transactor,
	orderPlacer OrderPlacer,
	riskPerTradePct decimal.Decimal,
	atrRiskMultiplier decimal.Decimal,
	takeProfitMultiplier decimal.Decimal,
) TradeService {
	return &tradeService{
		tradeRepository:           tradeRepository,
		accountTransferRepository: accountTransferRepository,
		signalRepository:          signalRepository,
		transactor:                transactor,
		orderPlacer:               orderPlacer,
		riskPerTradePct:           riskPerTradePct,
		atrRiskMultiplier:         atrRiskMultiplier,
		takeProfitMultiplier:      takeProfitMultiplier,
	}
}

// ExecuteSellTrade places a market sell order for the full quantity of the
// parent buy trade and persists the resulting sell trade record. The parent
// trade's AccountTransferID is required to maintain the budget link.
func (t *tradeService) ExecuteSellTrade(ctx context.Context, signal *model.Signal, trade *model.Trade) (*model.Trade, error) {
	if trade.AccountTransferID == nil {
		return nil, fmt.Errorf("ExecuteSellTrade: parent trade %s has nil account_transfer_id", trade.ID)
	}

	sellTrade := &model.Trade{
		ID:                uuid.New(),
		ParentID:          &trade.ID,
		SignalID:          &signal.ID,
		AccountTransferID: trade.AccountTransferID,
		Symbol:            signal.Symbol,
		Side:              string(model.SideSell),
		Quantity:          trade.Quantity,
		PricePerUnit:      &signal.PriceAtSignal,
		Status:            model.StatusPending,
		CreatedAt:         signal.CreatedAt,
	}

	// Pre-insert before broker call so a DB failure never leaves an untracked live order.
	if err := t.tradeRepository.Create(ctx, *sellTrade); err != nil {
		return nil, fmt.Errorf("ExecuteSellTrade: %w", err)
	}

	placedOrder, err := t.orderPlacer.PlaceOrder(alpaca.PlaceOrderRequest{
		Symbol:      signal.Symbol,
		Qty:         &sellTrade.Quantity,
		Side:        alpaca.Sell,
		Type:        alpaca.Market,
		TimeInForce: alpaca.Day,
	})
	if err != nil {
		// Remove the pre-inserted record so the next evaluation tick can retry.
		cleanCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if delErr := t.tradeRepository.Delete(cleanCtx, sellTrade.ID); delErr != nil {
			slog.Error("ExecuteSellTrade: cleanup: delete trade failed", "trade_id", sellTrade.ID, "error", delErr)
		}
		return nil, fmt.Errorf("ExecuteSellTrade: %w", err)
	}

	sellTrade.AlpacaOrderID = &placedOrder.ID
	if err := t.tradeRepository.Update(ctx, *sellTrade); err != nil {
		slog.Error("ExecuteSellTrade: CRITICAL: order placed but alpaca_order_id not persisted",
			"order_id", placedOrder.ID, "trade_id", sellTrade.ID, "error", err)
	}

	return sellTrade, nil
}

// isUniqueConstraintError reports whether err is a PostgreSQL unique-constraint
// violation (SQLSTATE 23505). Used to swallow duplicate sell orders that arise
// when a bar triggers both stop-loss and take-profit in the same tick.
func isUniqueConstraintError(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// ExecuteBuyTrade sizes a position using ATR-based risk, places a market buy
// order, and persists the trade while atomically decrementing the account's
// remaining trade slot. Returns nil, nil when the account is exhausted or when
// sizing produces a zero quantity.
func (t *tradeService) ExecuteBuyTrade(ctx context.Context, signal *model.Signal, account *model.AccountTransfer) (*model.Trade, error) {
	if account == nil {
		return nil, fmt.Errorf("ExecuteBuyTrade: account is nil")
	}
	if account.RemainingTrades == nil || *account.RemainingTrades <= 0 {
		return nil, nil
	}

	// Layer 4: Dynamic risk-based position sizing.
	// Risk amount = available budget × risk percentage per trade.
	riskAmount := account.AmountUSD.Mul(t.riskPerTradePct)

	// Stop distance = ATR × ATRRiskMultiplier.
	// This matches the actual stop-loss placement below so risk is consistent.
	atrStopDistance := signal.Indicators.ATR.Mul(t.atrRiskMultiplier)

	var quantity decimal.Decimal
	if !atrStopDistance.IsZero() && signal.PriceAtSignal.GreaterThan(decimal.Zero) {
		// Shares at risk = risk amount / $ stop distance per share.
		quantity = riskAmount.Div(atrStopDistance)

		// Cap at the per-slot maximum to respect transfer constraints.
		maxPerTrade := account.AmountUSD.Div(decimal.NewFromInt(int64(account.TargetTrades)))
		maxQuantity := maxPerTrade.Div(signal.PriceAtSignal)
		if quantity.GreaterThan(maxQuantity) {
			quantity = maxQuantity
		}
	} else {
		// Fallback: equal-weight sizing when ATR is unavailable.
		limitPerTrade := account.AmountUSD.Div(decimal.NewFromInt(int64(account.TargetTrades)))
		quantity = limitPerTrade.Div(signal.PriceAtSignal)
	}

	if quantity.LessThanOrEqual(decimal.Zero) {
		return nil, nil
	}

	// Stop-loss and take-profit use the same ATR multipliers as the sizing
	// calculation to keep risk assumptions internally consistent.
	stopLoss := signal.PriceAtSignal.Sub(signal.Indicators.ATR.Mul(t.atrRiskMultiplier))
	takeProfit := signal.PriceAtSignal.Add(signal.Indicators.ATR.Mul(t.takeProfitMultiplier))

	trade := &model.Trade{
		ID:                uuid.New(),
		SignalID:          &signal.ID,
		AccountTransferID: &account.ID,
		Symbol:            signal.Symbol,
		Side:              string(model.SideBuy),
		Quantity:          quantity,
		PricePerUnit:      &signal.PriceAtSignal,
		StopLoss:          &stopLoss,
		TakeProfit:        &takeProfit,
		Status:            model.StatusPending,
		Metadata: map[string]any{
			"risk_amount":            riskAmount.String(),
			"atr_stop_distance":      atrStopDistance.String(),
			"risk_per_trade_pct":     t.riskPerTradePct.String(),
			"atr_risk_multiplier":    t.atrRiskMultiplier.String(),
			"take_profit_multiplier": t.takeProfitMultiplier.String(),
		},
		CreatedAt: signal.CreatedAt,
	}

	// Position check, trade insert, and slot decrement are all committed in one
	// transaction. Re-checking HasOpenPosition here closes the TOCTOU window
	// between the earlier check in EvaluateBuySignal and this commit: two workers
	// can both pass the initial gate before either writes, but only one can win
	// this inner check and proceed to place a broker order.
	err := t.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		hasPosition, err := t.tradeRepository.HasOpenPosition(txCtx, signal.Symbol)
		if err != nil {
			return fmt.Errorf("HasOpenPosition: %w", err)
		}
		if hasPosition {
			return repository.ErrPositionAlreadyOpen
		}
		if err := t.tradeRepository.Create(txCtx, *trade); err != nil {
			return err
		}
		return t.accountTransferRepository.DecrementRemainingTrades(txCtx, *trade.AccountTransferID)
	})
	if err != nil {
		if errors.Is(err, repository.ErrNoRemainingSlots) || errors.Is(err, repository.ErrPositionAlreadyOpen) {
			return nil, nil
		}
		return nil, fmt.Errorf("ExecuteBuyTrade: %w", err)
	}

	placedOrder, err := t.orderPlacer.PlaceOrder(alpaca.PlaceOrderRequest{
		Symbol:      signal.Symbol,
		Qty:         &quantity,
		Side:        alpaca.Buy,
		Type:        alpaca.Market,
		TimeInForce: alpaca.Day,
	})
	if err != nil {
		// Best-effort: cancel the pre-inserted trade and restore the slot so
		// the budget remains available for future signals.
		t.cancelTradeAndRestoreSlot(trade)
		return nil, fmt.Errorf("ExecuteBuyTrade: place order: %w", err)
	}

	// Store the order ID so ApplyTradeUpdates can reconcile broker events.
	trade.AlpacaOrderID = &placedOrder.ID
	if err := t.tradeRepository.Update(ctx, *trade); err != nil {
		slog.Error("ExecuteBuyTrade: CRITICAL: order placed but alpaca_order_id not persisted",
			"order_id", placedOrder.ID, "trade_id", trade.ID, "error", err)
	}

	if err := t.signalRepository.MarkExecuted(ctx, signal.ID); err != nil {
		slog.Warn("ExecuteBuyTrade: could not mark signal as executed",
			"signal_id", signal.ID, "trade_id", trade.ID, "error", err)
	}

	return trade, nil
}

// cancelTradeAndRestoreSlot marks trade cancelled and restores the account slot
// after a broker failure. Uses a background context so a cancelled caller
// context does not prevent the rollback.
func (t *tradeService) cancelTradeAndRestoreSlot(trade *model.Trade) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	trade.Status = model.StatusCancelled
	if err := t.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := t.tradeRepository.Update(txCtx, *trade); err != nil {
			return err
		}
		if trade.AccountTransferID != nil {
			return t.accountTransferRepository.IncrementRemainingTrades(txCtx, *trade.AccountTransferID)
		}
		return nil
	}); err != nil {
		slog.Error("cancelTradeAndRestoreSlot: CRITICAL: trade not cancelled and slot not restored",
			"trade_id", trade.ID, "error", err)
	}
}

// EvaluateAndExecuteExits checks all open buy trades for symbol against their
// stop-loss and take-profit levels and fires a sell order for each that is
// triggered.
func (t *tradeService) EvaluateAndExecuteExits(ctx context.Context, symbol string, currentPrice decimal.Decimal) error {
	type exitPair struct {
		signal    *model.Signal
		sellTrade *model.Trade
	}

	var pending []exitPair

	err := t.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		openTrades, err := t.tradeRepository.GetOpenBuyTradesBySymbol(txCtx, symbol)
		if err != nil {
			return fmt.Errorf("EvaluateAndExecuteExits: %w", err)
		}

		for _, trade := range openTrades {
			var reason string
			switch {
			case trade.StopLoss != nil && currentPrice.LessThanOrEqual(*trade.StopLoss):
				reason = "stop_loss"
			case trade.TakeProfit != nil && currentPrice.GreaterThanOrEqual(*trade.TakeProfit):
				reason = "take_profit"
			default:
				continue
			}

			sellSignal := &model.Signal{
				ID:            uuid.New(),
				Reasoning:     &reason,
				Symbol:        trade.Symbol,
				Side:          model.SideSell,
				PriceAtSignal: currentPrice,
				IsExecuted:    false,
				CreatedAt:     time.Now().UTC(),
			}

			if err := t.signalRepository.Create(txCtx, sellSignal); err != nil {
				return fmt.Errorf("EvaluateAndExecuteExits: create signal: %w", err)
			}

			sellTrade := &model.Trade{
				ID:                uuid.New(),
				ParentID:          &trade.ID,
				SignalID:          &sellSignal.ID,
				AccountTransferID: trade.AccountTransferID,
				Symbol:            trade.Symbol,
				Side:              string(model.SideSell),
				Quantity:          trade.Quantity,
				PricePerUnit:      &currentPrice,
				Status:            model.StatusPending,
				CreatedAt:         sellSignal.CreatedAt,
			}

			if err := t.tradeRepository.Create(txCtx, *sellTrade); err != nil {
				if isUniqueConstraintError(err) {
					// GetOpenBuyTradesBySymbol already filters out buys that have a sell
					// (NOT EXISTS) and FOR UPDATE SKIP LOCKED prevents concurrent writes
					// to the same row, so 23505 here indicates an unexpected state.
					// Return an error (aborting this tick) rather than continuing in an
					// aborted PostgreSQL transaction, which would corrupt subsequent inserts.
					return fmt.Errorf("EvaluateAndExecuteExits: unexpected duplicate sell for buy %s: %w",
						trade.ID, err)
				}
				return fmt.Errorf("EvaluateAndExecuteExits: create sell trade: %w", err)
			}

			pending = append(pending, exitPair{signal: sellSignal, sellTrade: sellTrade})
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("EvaluateAndExecuteExits: %w", err)
	}

	for _, p := range pending {
		placedOrder, err := t.orderPlacer.PlaceOrder(alpaca.PlaceOrderRequest{
			Symbol:      p.signal.Symbol,
			Qty:         &p.sellTrade.Quantity,
			Side:        alpaca.Sell,
			Type:        alpaca.Market,
			TimeInForce: alpaca.Day,
		})
		if err != nil {
			// Delete both the pre-inserted trade and signal so the next evaluation
			// cycle can retry without hitting the unique-sell-per-buy constraint
			// and without accumulating orphan signal rows.
			cleanCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if delErr := t.tradeRepository.Delete(cleanCtx, p.sellTrade.ID); delErr != nil {
				slog.Error("EvaluateAndExecuteExits: cleanup: delete sell trade failed",
					"trade_id", p.sellTrade.ID, "error", delErr)
			}
			if delErr := t.signalRepository.Delete(cleanCtx, p.signal.ID); delErr != nil {
				slog.Error("EvaluateAndExecuteExits: cleanup: delete sell signal failed",
					"signal_id", p.signal.ID, "error", delErr)
			}
			cancel()
			slog.Error("EvaluateAndExecuteExits: place sell order failed",
				"parent_trade_id", *p.sellTrade.ParentID, "error", err)
			continue
		}

		p.sellTrade.AlpacaOrderID = &placedOrder.ID
		if err := t.tradeRepository.Update(ctx, *p.sellTrade); err != nil {
			slog.Error("EvaluateAndExecuteExits: CRITICAL: sell order placed but order ID not persisted",
				"order_id", placedOrder.ID, "trade_id", p.sellTrade.ID, "error", err)
		}

		if err := t.signalRepository.MarkExecuted(ctx, p.signal.ID); err != nil {
			slog.Warn("EvaluateAndExecuteExits: could not mark sell signal as executed",
				"signal_id", p.signal.ID, "trade_id", p.sellTrade.ID, "error", err)
		}
	}

	return nil
}

// ApplyTradeUpdates reconciles an Alpaca broker event with the persisted trade
// record. Events for trades already in a terminal status are ignored to handle
// replay. When a buy order is rejected or cancelled the account slot is
// restored so the budget remains available for future trades.
func (t *tradeService) ApplyTradeUpdates(ctx context.Context, update alpaca.TradeUpdate, updateStatus model.Status) error {
	trade, err := t.tradeRepository.GetByAlpacaOrderID(ctx, update.Order.ID)
	if err != nil {
		return fmt.Errorf("ApplyTradeUpdates: %w", err)
	}
	if trade == nil {
		slog.Warn("ApplyTradeUpdates: no trade found for order — skipping",
			"order_id", update.Order.ID, "event", update.Event)
		return nil
	}

	if trade.Status == model.StatusFilled ||
		trade.Status == model.StatusRejected ||
		trade.Status == model.StatusCancelled {
		return nil // already terminal, ignore replay
	}

	err = t.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		trade.Status = updateStatus
		trade.FilledAt = update.Order.FilledAt
		trade.AvgFillPrice = update.Order.FilledAvgPrice

		if updateStatus == model.StatusFilled && update.Order.FilledAvgPrice != nil {
			commissionFee := update.Order.FilledQty.Mul(*update.Order.FilledAvgPrice).Mul(decimal.NewFromFloat(0.0005))
			fxFeeAmortized := update.Order.FilledQty.Mul(*update.Order.FilledAvgPrice).Mul(decimal.NewFromFloat(0.0001))
			trade.CommissionFee = &commissionFee
			trade.FXFeeAmortized = &fxFeeAmortized
		}

		if err := t.tradeRepository.Update(txCtx, *trade); err != nil {
			return fmt.Errorf("ApplyTradeUpdates: %w", err)
		}

		if updateStatus == model.StatusRejected || updateStatus == model.StatusCancelled {
			if trade.Side == string(model.SideBuy) && trade.AccountTransferID != nil {
				if err := t.accountTransferRepository.IncrementRemainingTrades(txCtx, *trade.AccountTransferID); err != nil {
					return fmt.Errorf("ApplyTradeUpdates: %w", err)
				}
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("ApplyTradeUpdates: %w", err)
	}

	return nil
}
