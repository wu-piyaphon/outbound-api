package repository

import (
	"context"
	"fmt"

	"github.com/wu-piyaphon/outbound-api/internal/model"
)

type TradeRepository interface {
	Create(ctx context.Context, trade model.Trade) error
	GetOpenBuyTradesBySymbol(ctx context.Context, symbol string) ([]*model.Trade, error)
}

type tradeRepository struct {
	pool DBTX
}

func NewTradeRepository(pool DBTX) TradeRepository {
	return &tradeRepository{pool: pool}
}

const insertTradeQuery = `
	INSERT INTO trades (id, parent_id, signal_id, account_transfer_id, alpaca_order_id, symbol, side, quantity, price_per_unit, avg_fill_price, commission_fee, fx_fee_amortized, stop_loss, take_profit, status, metadata, filled_at, created_at) 
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)`

func (t *tradeRepository) Create(ctx context.Context, trade model.Trade) error {
	args := []any{
		trade.ID,                // $1
		trade.ParentID,          // $2
		trade.SignalID,          // $3
		trade.AccountTransferID, // $4
		trade.AlpacaOrderID,     // $5
		trade.Symbol,            // $6
		trade.Side,              // $7
		trade.Quantity,          // $8
		trade.PricePerUnit,      // $9
		trade.AvgFillPrice,      // $10
		trade.CommissionFee,     // $11
		trade.FXFeeAmortized,    // $12
		trade.StopLoss,          // $13
		trade.TakeProfit,        // $14
		trade.Status,            // $15
		trade.Metadata,          // $16
		trade.FilledAt,          // $17
		trade.CreatedAt,         // $18
	}

	_, err := GetDB(ctx, t.pool).Exec(ctx, insertTradeQuery, args...)
	if err != nil {
		return fmt.Errorf("Create: %w", err)
	}

	return nil
}

const getOpenBuyTradesBySymbolQuery = `
	SELECT id, parent_id, signal_id, account_transfer_id, alpaca_order_id, symbol, side, quantity, price_per_unit, avg_fill_price, commission_fee, fx_fee_amortized, stop_loss, take_profit, status, metadata, filled_at, created_at
	FROM trades
	WHERE symbol = $1 AND side = 'buy' AND status = 'filled' AND NOT EXISTS (
		SELECT 1 FROM trades sell
		WHERE sell.side = 'sell' AND sell.parent_id = trades.id 
	)
	FOR UPDATE SKIP LOCKED`

func (t *tradeRepository) GetOpenBuyTradesBySymbol(ctx context.Context, symbol string) ([]*model.Trade, error) {
	rows, err := GetDB(ctx, t.pool).Query(ctx, getOpenBuyTradesBySymbolQuery, symbol)
	if err != nil {
		return nil, fmt.Errorf("GetOpenBuyTradesBySymbol: %w", err)
	}
	defer rows.Close()

	var trades []*model.Trade
	for rows.Next() {
		var trade model.Trade
		err := rows.Scan(
			&trade.ID,
			&trade.ParentID,
			&trade.SignalID,
			&trade.AccountTransferID,
			&trade.AlpacaOrderID,
			&trade.Symbol,
			&trade.Side,
			&trade.Quantity,
			&trade.PricePerUnit,
			&trade.AvgFillPrice,
			&trade.CommissionFee,
			&trade.FXFeeAmortized,
			&trade.StopLoss,
			&trade.TakeProfit,
			&trade.Status,
			&trade.Metadata,
			&trade.FilledAt,
			&trade.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("GetOpenBuyTradesBySymbol: %w", err)
		}
		trades = append(trades, &trade)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetOpenBuyTradesBySymbol: %w", err)
	}

	return trades, nil
}
