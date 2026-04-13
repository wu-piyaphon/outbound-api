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

type AccountTransferRepository interface {
	Create(ctx context.Context, transfer *model.AccountTransfer) error
	GetAvailableBudget(ctx context.Context) (*model.AccountTransfer, error)
	DecrementRemainingTrades(ctx context.Context, transferID uuid.UUID) error
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

func (a *accountTransferRepository) DecrementRemainingTrades(ctx context.Context, transferID uuid.UUID) error {
	args := []any{
		transferID,
		time.Now().UTC(),
	}

	_, err := GetDB(ctx, a.pool).Exec(ctx, decrementRemainingTradesQuery, args...)
	if err != nil {
		return fmt.Errorf("DecrementRemainingTrades: %w", err)
	}

	return nil
}
