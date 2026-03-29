package repository

import (
	"context"
	"fmt"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wu-piyaphon/outbound-api/internal/model"
)

type SignalRepository interface {
	GetAll(ctx context.Context) ([]model.Signal, error)
}

type signalRepository struct {
	pool *pgxpool.Pool
}

func NewSignalRepository(pool *pgxpool.Pool) SignalRepository {
	return &signalRepository{pool: pool}
}

func (r *signalRepository) GetAll(ctx context.Context) ([]model.Signal, error) {
	var signals []model.Signal

	err := pgxscan.Select(ctx, r.pool, &signals, "SELECT id, symbol, side, price_at_signal, indicators, is_executed, reasoning, created_at FROM signals")
	if err != nil {
		return nil, fmt.Errorf("GetAll scan: %w", err)
	}

	return signals, nil
}
