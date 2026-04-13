package service

import (
	"context"

	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/wu-piyaphon/outbound-api/internal/model"
	"github.com/wu-piyaphon/outbound-api/internal/repository"
)

type TradeService interface {
	ExecuteTrade(ctx context.Context, signal *model.Signal) (*model.Trade, error)
}

type tradeService struct {
	tradeRepository repository.TradeRepository
	alpacaClient    *alpaca.Client
}

func NewTradeService(tradeRepository repository.TradeRepository, alpacaClient *alpaca.Client) TradeService {
	return &tradeService{tradeRepository: tradeRepository, alpacaClient: alpacaClient}
}

func (t *tradeService) ExecuteTrade(ctx context.Context, signal *model.Signal) (*model.Trade, error) {
	quantity := signal.PriceAtSignal.Div(decimal.NewFromInt(100))

	trade := &model.Trade{
		ID:                uuid.New(),
		ParentID:          nil,
		SignalID:          &signal.ID,
		AccountTransferID: nil,
		AlpacaOrderID:     nil,
		Symbol:            signal.Symbol,
		Side:              string(model.SideBuy),
		Quantity:          quantity,
		PricePerUnit:      &signal.PriceAtSignal,
		AvgFillPrice:      nil,
		CommissionFee:     nil,
		FXFeeAmortized:    nil,
		Status:            model.StatusPending,
		Metadata:          nil,
		FilledAt:          nil,
		CreatedAt:         signal.CreatedAt,
	}

	if signal.Side == model.SideSell {
		trade.Side = string(model.SideSell)

		trades, err := t.tradeRepository.GetOpenBuyTradesBySymbol(ctx, signal.Symbol)
		if err != nil {
			return nil, err
		}

		if len(trades) == 0 {
			return nil, nil
		}

		for _, openBuyTrade := range trades {
			trade.ID = uuid.New()
			trade.ParentID = &openBuyTrade.ID
			trade.Quantity = openBuyTrade.Quantity
			err := t.tradeRepository.Create(ctx, *trade)
			if err != nil {
				return nil, err
			}
		}
	}

	if signal.Side == model.SideBuy {
		quantity := decimal.NewFromFloat(1.0) // TODO: Calculate quantity based on signal price and risk management rules

		alpacaOrder := alpaca.PlaceOrderRequest{
			Symbol:      signal.Symbol,
			Qty:         &quantity,
			Side:        alpaca.Buy,
			Type:        alpaca.Market,
			TimeInForce: alpaca.Day,
		}

		placedOrder, err := t.alpacaClient.PlaceOrder(alpacaOrder)
		if err != nil {
			return nil, err
		}

		trade.AlpacaOrderID = &placedOrder.ID
		trade.Status = model.StatusFilled

		err = t.tradeRepository.Create(ctx, *trade)
		if err != nil {
			return nil, err
		}

	}

	return trade, nil
}
