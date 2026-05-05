package repository

import (
	"context"
	"fmt"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/google/uuid"
	"github.com/wu-piyaphon/outbound-api/internal/model"
)

type SignalRepository interface {
	GetAll(ctx context.Context) ([]model.Signal, error)
	Create(ctx context.Context, signal *model.Signal) error
	// Delete removes a signal by ID. Used to clean up a signal record that was
	// orphaned when the subsequent broker call failed, so the next evaluation
	// cycle can retry cleanly.
	Delete(ctx context.Context, id uuid.UUID) error
	// MarkExecuted flips is_executed to true once the corresponding broker order
	// has been successfully placed.
	MarkExecuted(ctx context.Context, id uuid.UUID) error
}

type signalRepository struct {
	pool DBTX
}

func NewSignalRepository(pool DBTX) SignalRepository {
	return &signalRepository{pool: pool}
}

func (r *signalRepository) GetAll(ctx context.Context) ([]model.Signal, error) {
	var signals []model.Signal

	err := pgxscan.Select(ctx, GetDB(ctx, r.pool), &signals, "SELECT id, symbol, side, price_at_signal, indicators, is_executed, reasoning, created_at FROM signals")
	if err != nil {
		return nil, fmt.Errorf("GetAll scan: %w", err)
	}

	return signals, nil
}

func (r *signalRepository) Create(ctx context.Context, signal *model.Signal) error {
	_, err := GetDB(ctx, r.pool).Exec(ctx, "INSERT INTO signals (id, symbol, side, price_at_signal, indicators, is_executed, reasoning) VALUES ($1, $2, $3, $4, $5, $6, $7)",
		signal.ID, signal.Symbol, signal.Side, signal.PriceAtSignal, signal.Indicators, signal.IsExecuted, signal.Reasoning)
	if err != nil {
		return fmt.Errorf("Create: %w", err)
	}

	return nil
}

func (r *signalRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := GetDB(ctx, r.pool).Exec(ctx, "DELETE FROM signals WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("Delete: %w", err)
	}
	return nil
}

func (r *signalRepository) MarkExecuted(ctx context.Context, id uuid.UUID) error {
	_, err := GetDB(ctx, r.pool).Exec(ctx, "UPDATE signals SET is_executed = true WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("MarkExecuted: %w", err)
	}
	return nil
}
