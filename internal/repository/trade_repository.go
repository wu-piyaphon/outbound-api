package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/wu-piyaphon/outbound-api/internal/model"
)

// TradeRepository persists Trade rows and the queries that drive the trading
// hot path (open-position checks, exit evaluation, broker reconciliation).
type TradeRepository interface {
	// Create inserts a new trade row.
	Create(ctx context.Context, trade model.Trade) error
	// GetOpenBuyTradesBySymbol returns active buy trades for symbol that have
	// no sell child, locking the rows with FOR UPDATE SKIP LOCKED so concurrent
	// workers do not double-process the same exit.
	GetOpenBuyTradesBySymbol(ctx context.Context, symbol string) ([]*model.Trade, error)
	// GetByAlpacaOrderID looks up a trade by its broker-side order ID.
	// Returns nil, nil when no matching row exists.
	GetByAlpacaOrderID(ctx context.Context, alpacaOrderID string) (*model.Trade, error)
	// Update overwrites all mutable columns for the trade with the given ID.
	Update(ctx context.Context, trade model.Trade) error
	// Delete removes a trade row by ID.
	Delete(ctx context.Context, id uuid.UUID) error
	// HasOpenPosition reports whether any non-terminal buy trade exists for
	// symbol with no sell child. Used to enforce one-position-per-symbol.
	HasOpenPosition(ctx context.Context, symbol string) (bool, error)
}

type tradeRepository struct {
	pool DBTX
}

// NewTradeRepository constructs a TradeRepository backed by pool.
func NewTradeRepository(pool DBTX) TradeRepository {
	return &tradeRepository{pool: pool}
}

const insertTradeQuery = `
	INSERT INTO trades (id, parent_id, signal_id, account_transfer_id, alpaca_order_id, symbol, side, quantity, price_per_unit, avg_fill_price, commission_fee, fx_fee_amortized, stop_loss, take_profit, peak_price, entry_atr, status, metadata, filled_at, created_at) 
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)`

func (t *tradeRepository) Create(ctx context.Context, trade model.Trade) error {
	args := []any{
		trade.ID,
		trade.ParentID,
		trade.SignalID,
		trade.AccountTransferID,
		trade.AlpacaOrderID,
		trade.Symbol,
		trade.Side,
		trade.Quantity,
		trade.PricePerUnit,
		trade.AvgFillPrice,
		trade.CommissionFee,
		trade.FXFeeAmortized,
		trade.StopLoss,
		trade.TakeProfit,
		trade.PeakPrice,
		trade.EntryATR,
		trade.Status,
		trade.Metadata,
		trade.FilledAt,
		trade.CreatedAt,
	}

	_, err := GetDB(ctx, t.pool).Exec(ctx, insertTradeQuery, args...)
	if err != nil {
		return fmt.Errorf("Create: %w", err)
	}

	return nil
}

// getOpenBuyTradesBySymbolQuery fetches all active buy trades for a symbol that
// have no sell child. Both 'filled' and 'open' (partial-fill) statuses are
// included so that stop-loss and take-profit levels are enforced as soon as
// shares are held, not only after the order is completely filled.
//
// For partially-filled buys the sell quantity will be trade.Quantity (the
// originally ordered amount). Alpaca will reject the sell if fewer shares are
// held; the cleanup path in EvaluateAndExecuteExits retries on the next bar.
const getOpenBuyTradesBySymbolQuery = `
	SELECT id, parent_id, signal_id, account_transfer_id, alpaca_order_id, symbol, side, quantity, price_per_unit, avg_fill_price, commission_fee, fx_fee_amortized, stop_loss, take_profit, peak_price, entry_atr, status, metadata, filled_at, created_at
	FROM trades
	WHERE symbol = $1 AND side = 'buy' AND status IN ('open', 'filled') AND NOT EXISTS (
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
			&trade.PeakPrice,
			&trade.EntryATR,
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

const getByAlpacaOrderIDQuery = `
	SELECT id, parent_id, signal_id, account_transfer_id, alpaca_order_id, symbol, side, quantity, price_per_unit, avg_fill_price, commission_fee, fx_fee_amortized, stop_loss, take_profit, peak_price, entry_atr, status, metadata, filled_at, created_at
	FROM trades
	WHERE alpaca_order_id = $1`

func (t *tradeRepository) GetByAlpacaOrderID(ctx context.Context, alpacaOrderID string) (*model.Trade, error) {
	row := GetDB(ctx, t.pool).QueryRow(ctx, getByAlpacaOrderIDQuery, alpacaOrderID)

	var trade model.Trade
	err := row.Scan(
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
		&trade.PeakPrice,
		&trade.EntryATR,
		&trade.Status,
		&trade.Metadata,
		&trade.FilledAt,
		&trade.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("GetByAlpacaOrderID: %w", err)
	}

	return &trade, nil
}

const deleteTradeQuery = `DELETE FROM trades WHERE id = $1`

// Delete removes a trade record by ID. Used to clean up a pre-inserted sell
// record when the subsequent broker call fails, so the next evaluation cycle
// can retry without hitting the unique-sell-per-buy constraint.
func (t *tradeRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := GetDB(ctx, t.pool).Exec(ctx, deleteTradeQuery, id)
	if err != nil {
		return fmt.Errorf("Delete: %w", err)
	}
	return nil
}

// hasOpenPositionQuery checks for any active buy trade (pending, open, or filled)
// that has no corresponding sell child. Checking all non-terminal statuses
// prevents a second buy being opened while an earlier one is still pending or
// open with the broker, which would violate the one-position-per-symbol rule.
const hasOpenPositionQuery = `
	SELECT EXISTS (
		SELECT 1 FROM trades
		WHERE symbol = $1
		AND side = $2
		AND status IN ('pending', 'open', 'filled')
		AND NOT EXISTS (
			SELECT 1 FROM trades sell
			WHERE sell.side = $3 AND sell.parent_id = trades.id
		)
	)`

func (t *tradeRepository) HasOpenPosition(ctx context.Context, symbol string) (bool, error) {
	var exists bool
	err := GetDB(ctx, t.pool).QueryRow(ctx, hasOpenPositionQuery, symbol, model.SideBuy, model.SideSell).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("HasOpenPosition: %w", err)
	}
	return exists, nil
}

const updateTradeQuery = `
	UPDATE trades
	SET parent_id = $2, signal_id = $3, account_transfer_id = $4, alpaca_order_id = $5, symbol = $6, side = $7, quantity = $8, price_per_unit = $9, avg_fill_price = $10, commission_fee = $11, fx_fee_amortized = $12, stop_loss = $13, take_profit = $14, peak_price = $15, entry_atr = $16, status = $17, metadata = $18, filled_at = $19
	WHERE id = $1`

func (t *tradeRepository) Update(ctx context.Context, trade model.Trade) error {
	args := []any{
		trade.ID,
		trade.ParentID,
		trade.SignalID,
		trade.AccountTransferID,
		trade.AlpacaOrderID,
		trade.Symbol,
		trade.Side,
		trade.Quantity,
		trade.PricePerUnit,
		trade.AvgFillPrice,
		trade.CommissionFee,
		trade.FXFeeAmortized,
		trade.StopLoss,
		trade.TakeProfit,
		trade.PeakPrice,
		trade.EntryATR,
		trade.Status,
		trade.Metadata,
		trade.FilledAt,
	}

	_, err := GetDB(ctx, t.pool).Exec(ctx, updateTradeQuery, args...)
	if err != nil {
		return fmt.Errorf("Update: %w", err)
	}

	return nil
}
