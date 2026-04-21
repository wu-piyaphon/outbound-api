package service

import (
	"context"
	"fmt"

	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/wu-piyaphon/outbound-api/internal/model"
	"github.com/wu-piyaphon/outbound-api/internal/repository"
)

type TradeService interface {
	ExecuteBuyTrade(ctx context.Context, signal *model.Signal, account *model.AccountTransfer) (*model.Trade, error)
	ExecuteSellTrade(ctx context.Context, signal *model.Signal, trade *model.Trade) (*model.Trade, error)
	CheckExitConditions(ctx context.Context, symbol string, currentPrice decimal.Decimal) ([]*model.ExitSignal, error)
}

type tradeService struct {
	tradeRepository           repository.TradeRepository
	accountTransferRepository repository.AccountTransferRepository
	transactor                repository.Transactor
	alpacaClient              *alpaca.Client
}

func NewTradeService(tradeRepository repository.TradeRepository, accountTransferRepository repository.AccountTransferRepository, transactor repository.Transactor, alpacaClient *alpaca.Client) TradeService {
	return &tradeService{tradeRepository: tradeRepository, accountTransferRepository: accountTransferRepository, transactor: transactor, alpacaClient: alpacaClient}
}

func (t *tradeService) ExecuteSellTrade(ctx context.Context, signal *model.Signal, trade *model.Trade) (*model.Trade, error) {
	sellTrade := &model.Trade{
		ID:                uuid.New(),
		ParentID:          &trade.ID,
		SignalID:          &signal.ID,
		AccountTransferID: nil,
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

func (t *tradeService) ExecuteBuyTrade(ctx context.Context, signal *model.Signal, account *model.AccountTransfer) (*model.Trade, error) {
	if account.RemainingTrades == nil || *account.RemainingTrades <= 0 {
		return nil, nil
	}

	limitPerTrade := account.AmountUSD.Div(decimal.NewFromInt(int64(account.TargetTrades)))
	quantity := limitPerTrade.Div(signal.PriceAtSignal)

	if quantity.LessThanOrEqual(decimal.NewFromFloat(0)) {
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
		Metadata:          nil,
		FilledAt:          nil,
		CreatedAt:         signal.CreatedAt,
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

func (t *tradeService) CheckExitConditions(ctx context.Context, symbol string, currentPrice decimal.Decimal) ([]*model.ExitSignal, error) {
	openTrades, err := t.tradeRepository.GetOpenBuyTradesBySymbol(ctx, symbol)
	if err != nil {
		return nil, fmt.Errorf("CheckExitConditions: %w", err)
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

	return exitSignals, nil
}
