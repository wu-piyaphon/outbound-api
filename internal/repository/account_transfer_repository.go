package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/wu-piyaphon/outbound-api/internal/model"
)

// AccountTransferRepository manages account transfer records and their
// remaining trade slot counters. Slot adjustments are always performed inside
// a transaction together with the corresponding trade write.
type AccountTransferRepository interface {
	Create(ctx context.Context, transfer *model.AccountTransfer) error
	GetAvailableBudget(ctx context.Context) (*model.AccountTransfer, error)
	DecrementRemainingTrades(ctx context.Context, transferID uuid.UUID) error
	IncrementRemainingTrades(ctx context.Context, transferID uuid.UUID) error
}

type accountTransferRepository struct {
	pool DBTX
}

func NewAccountTransferRepository(pool DBTX) AccountTransferRepository {
	return &accountTransferRepository{pool: pool}
}

const insertAccountTransferQuery = `
	INSERT INTO account_transfers (id, type, amount_thb, amount_usd, fee_thb, fee_usd, exchange_rate, target_trades, remaining_trades, created_at, updated_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
`

func (a *accountTransferRepository) Create(ctx context.Context, transfer *model.AccountTransfer) error {
	args := []any{
		transfer.ID,
		transfer.Type,
		transfer.AmountTHB,
		transfer.AmountUSD,
		transfer.FeeTHB,
		transfer.FeeUSD,
		transfer.ExchangeRate,
		transfer.TargetTrades,
		transfer.RemainingTrades,
		transfer.CreatedAt,
		transfer.UpdatedAt,
	}

	_, err := GetDB(ctx, a.pool).Exec(ctx, insertAccountTransferQuery, args...)
	if err != nil {
		return fmt.Errorf("Create: %w", err)
	}

	return nil
}

const getActiveAccountTransfersQuery = `
	SELECT id, type, amount_thb, amount_usd, fee_thb, fee_usd, exchange_rate, target_trades, remaining_trades, created_at, updated_at
	FROM account_transfers
	WHERE remaining_trades > 0 
	ORDER BY created_at
	LIMIT 1
`

// GetAvailableBudget returns the oldest transfer that still has remaining trade
// slots (FIFO), or nil if no eligible transfer exists. The caller uses the
// returned record's AmountUSD and TargetTrades for position sizing.
func (a *accountTransferRepository) GetAvailableBudget(ctx context.Context) (*model.AccountTransfer, error) {
	var t model.AccountTransfer
	err := pgxscan.Get(ctx, GetDB(ctx, a.pool), &t, getActiveAccountTransfersQuery)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("GetAvailableBudget: %w", err)
	}

	return &t, nil
}

const decrementRemainingTradesQuery = `
	UPDATE account_transfers
	SET remaining_trades = remaining_trades - 1, updated_at = $2
	WHERE id = $1 AND remaining_trades > 0
`

// DecrementRemainingTrades reduces the slot counter by one. It returns
// ErrNoRemainingSlots when no row is updated (counter already zero), so
// concurrent buy workers racing on the last slot are guaranteed to see an
// explicit error rather than silently over-committing.
func (a *accountTransferRepository) DecrementRemainingTrades(ctx context.Context, transferID uuid.UUID) error {
	args := []any{
		transferID,
		time.Now().UTC(),
	}

	tag, err := GetDB(ctx, a.pool).Exec(ctx, decrementRemainingTradesQuery, args...)
	if err != nil {
		return fmt.Errorf("DecrementRemainingTrades: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("DecrementRemainingTrades: %w", ErrNoRemainingSlots)
	}

	return nil
}

const incrementRemainingTradesQuery = `
	UPDATE account_transfers
	SET remaining_trades = remaining_trades + 1, updated_at = $2
	WHERE id = $1
`

func (a *accountTransferRepository) IncrementRemainingTrades(ctx context.Context, transferID uuid.UUID) error {
	args := []any{
		transferID,
		time.Now().UTC(),
	}

	tag, err := GetDB(ctx, a.pool).Exec(ctx, incrementRemainingTradesQuery, args...)
	if err != nil {
		return fmt.Errorf("IncrementRemainingTrades: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("IncrementRemainingTrades: no account transfer found for id %s", transferID)
	}

	return nil
}
