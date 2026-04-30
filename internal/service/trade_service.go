package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
	"github.com/wu-piyaphon/outbound-api/internal/model"
	"github.com/wu-piyaphon/outbound-api/internal/repository"
)

var decimalZero = decimal.NewFromInt(0)

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
	alpacaClient              *alpaca.Client
	riskPerTradePct           decimal.Decimal
	atrRiskMultiplier         decimal.Decimal
}

func NewTradeService(
	tradeRepository repository.TradeRepository,
	accountTransferRepository repository.AccountTransferRepository,
	signalRepository repository.SignalRepository,
	transactor repository.Transactor,
	alpacaClient *alpaca.Client,
	riskPerTradePct decimal.Decimal,
	atrRiskMultiplier decimal.Decimal,
) TradeService {
	return &tradeService{
		tradeRepository:           tradeRepository,
		accountTransferRepository: accountTransferRepository,
		signalRepository:          signalRepository,
		transactor:                transactor,
		alpacaClient:              alpacaClient,
		riskPerTradePct:           riskPerTradePct,
		atrRiskMultiplier:         atrRiskMultiplier,
	}
}

func (t *tradeService) ExecuteSellTrade(ctx context.Context, signal *model.Signal, trade *model.Trade) (*model.Trade, error) {
	if trade.AccountTransferID == nil {
		return nil, fmt.Errorf("ExecuteSellTrade: parent trade %s has nil account_transfer_id", trade.ID)
	}

	sellTrade := &model.Trade{
		ID:                uuid.New(),
		ParentID:          &trade.ID,
		SignalID:          &signal.ID,
		AccountTransferID: trade.AccountTransferID,
		AlpacaOrderID:     nil,
		Symbol:            signal.Symbol,
		Side:              string(model.SideSell),
		Quantity:          trade.Quantity,
		PricePerUnit:      &signal.PriceAtSignal,
		AvgFillPrice:      nil,
		CommissionFee:     nil,
		FXFeeAmortized:    nil,
		Status:            model.StatusPending,
		Metadata:          nil,
		FilledAt:          nil,
		CreatedAt:         signal.CreatedAt,
	}

	alpacaOrder := alpaca.PlaceOrderRequest{
		Symbol:      signal.Symbol,
		Qty:         &sellTrade.Quantity,
		Side:        alpaca.Sell,
		Type:        alpaca.Market,
		TimeInForce: alpaca.Day,
	}

	placedOrder, err := t.alpacaClient.PlaceOrder(alpacaOrder)
	if err != nil {
		return nil, fmt.Errorf("ExecuteSellTrade: %w", err)
	}

	sellTrade.Status = model.StatusPending
	sellTrade.AlpacaOrderID = &placedOrder.ID

	err = t.tradeRepository.Create(ctx, *sellTrade)
	if err != nil {
		return nil, fmt.Errorf("ExecuteSellTrade: %w", err)
	}

	return sellTrade, nil
}

func isUniqueConstraintError(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func (t *tradeService) ExecuteBuyTrade(ctx context.Context, signal *model.Signal, account *model.AccountTransfer) (*model.Trade, error) {
	if account == nil {
		return nil, fmt.Errorf("ExecuteBuyTrade: account is nil")
	}
	if account.RemainingTrades == nil || *account.RemainingTrades <= 0 {
		return nil, nil
	}

	riskAmount := account.AmountUSD.Mul(t.riskPerTradePct)

	atrStopDistance := signal.Indicators.ATR.Mul(t.atrRiskMultiplier)

	var quantity decimal.Decimal
	if !atrStopDistance.IsZero() && signal.PriceAtSignal.GreaterThan(decimalZero) {
		quantity = riskAmount.Div(atrStopDistance)

		maxPerTrade := account.AmountUSD.Div(decimal.NewFromInt(int64(account.TargetTrades)))
		maxQuantity := maxPerTrade.Div(signal.PriceAtSignal)
		if quantity.GreaterThan(maxQuantity) {
			quantity = maxQuantity
		}
	} else {
		limitPerTrade := account.AmountUSD.Div(decimal.NewFromInt(int64(account.TargetTrades)))
		quantity = limitPerTrade.Div(signal.PriceAtSignal)
	}

	if quantity.LessThanOrEqual(decimalZero) {
		return nil, nil
	}

	alpacaOrder := alpaca.PlaceOrderRequest{
		Symbol:      signal.Symbol,
		Qty:         &quantity,
		Side:        alpaca.Buy,
		Type:        alpaca.Market,
		TimeInForce: alpaca.Day,
	}

	placedOrder, err := t.alpacaClient.PlaceOrder(alpacaOrder)
	if err != nil {
		return nil, fmt.Errorf("ExecuteBuyTrade: %w", err)
	}

	stopLoss := signal.PriceAtSignal.Sub(signal.Indicators.ATR.Mul(decimal.NewFromFloat(2)))
	takeProfit := signal.PriceAtSignal.Add(signal.Indicators.ATR.Mul(decimal.NewFromFloat(3)))

	trade := &model.Trade{
		ID:                uuid.New(),
		ParentID:          nil,
		SignalID:          &signal.ID,
		AccountTransferID: &account.ID,
		AlpacaOrderID:     &placedOrder.ID,
		Symbol:            signal.Symbol,
		Side:              string(model.SideBuy),
		Quantity:          quantity,
		PricePerUnit:      &signal.PriceAtSignal,
		AvgFillPrice:      nil,
		CommissionFee:     nil,
		FXFeeAmortized:    nil,
		StopLoss:          &stopLoss,
		TakeProfit:        &takeProfit,
		Status:            model.StatusPending,
		Metadata: map[string]any{
			"risk_amount":         riskAmount.String(),
			"atr_stop_distance":   atrStopDistance.String(),
			"risk_per_trade_pct":  t.riskPerTradePct.String(),
			"atr_risk_multiplier": t.atrRiskMultiplier.String(),
		},
		FilledAt:  nil,
		CreatedAt: signal.CreatedAt,
	}

	err = t.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		err := t.tradeRepository.Create(txCtx, *trade)
		if err != nil {
			return err
		}

		err = t.accountTransferRepository.DecrementRemainingTrades(txCtx, *trade.AccountTransferID)
		if err != nil {
			return err
		}

		return nil

	})
	if err != nil {
		return nil, fmt.Errorf("ExecuteBuyTrade: %w", err)
	}

	return trade, nil
}

func (t *tradeService) EvaluateAndExecuteExits(ctx context.Context, symbol string, currentPrice decimal.Decimal) error {
	err := t.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		openTrades, err := t.tradeRepository.GetOpenBuyTradesBySymbol(txCtx, symbol)
		if err != nil {
			return fmt.Errorf("EvaluateAndExecuteExits: %w", err)
		}

		var exitSignals []*model.ExitSignal
		for _, trade := range openTrades {
			if trade.StopLoss != nil && currentPrice.LessThanOrEqual(*trade.StopLoss) {
				exitSignals = append(exitSignals, &model.ExitSignal{
					Trade:  trade,
					Reason: "stop_loss",
				})
			} else if trade.TakeProfit != nil && currentPrice.GreaterThanOrEqual(*trade.TakeProfit) {
				exitSignals = append(exitSignals, &model.ExitSignal{
					Trade:  trade,
					Reason: "take_profit",
				})
			}
		}

		for _, signal := range exitSignals {
			sellSignal := &model.Signal{
				ID:            uuid.New(),
				Reasoning:     &signal.Reason,
				Symbol:        signal.Trade.Symbol,
				Side:          model.SideSell,
				PriceAtSignal: currentPrice,
				IsExecuted:    false,
				CreatedAt:     time.Now().UTC(),
			}

			err := t.signalRepository.Create(txCtx, sellSignal)
			if err != nil {
				return fmt.Errorf("EvaluateAndExecuteExits: %w", err)
			}

			_, err = t.ExecuteSellTrade(txCtx, sellSignal, signal.Trade)
			if err != nil {
				if isUniqueConstraintError(err) {
					continue
				}

				return fmt.Errorf("EvaluateAndExecuteExits: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("EvaluateAndExecuteExits: %w", err)
	}

	return nil
}

func (t *tradeService) ApplyTradeUpdates(ctx context.Context, update alpaca.TradeUpdate, updateStatus model.Status) error {
	trade, err := t.tradeRepository.GetByAlpacaOrderID(ctx, update.Order.ID)
	if err != nil {
		return fmt.Errorf("ApplyTradeUpdates: %w", err)
	}
	if trade == nil {
		return fmt.Errorf("ApplyTradeUpdates: trade not found")
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

		err = t.tradeRepository.Update(txCtx, *trade)
		if err != nil {
			return fmt.Errorf("ApplyTradeUpdates: %w", err)
		}

		if updateStatus == model.StatusRejected || updateStatus == model.StatusCancelled {
			if trade.Side == string(model.SideBuy) && trade.AccountTransferID != nil {
				err = t.accountTransferRepository.IncrementRemainingTrades(txCtx, *trade.AccountTransferID)
				if err != nil {
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
