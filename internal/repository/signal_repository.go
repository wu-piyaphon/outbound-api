package repository

import (
	"context"
	"fmt"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/wu-piyaphon/outbound-api/internal/model"
)

type SignalRepository interface {
	GetAll(ctx context.Context) ([]model.Signal, error)
	Create(ctx context.Context, signal *model.Signal) error
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
