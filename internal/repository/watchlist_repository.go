package repository

import (
	"context"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/wu-piyaphon/outbound-api/internal/model"
)

type WatchlistRepository interface {
	Create(ctx context.Context, symbol string) error
	GetAllActive(ctx context.Context) ([]model.Watchlist, error)
	Activate(ctx context.Context, symbol string) error
	Deactivate(ctx context.Context, symbol string) error
}

type watchlistRepository struct {
	pool DBTX
}

func NewWatchlistRepository(pool DBTX) WatchlistRepository {
	return &watchlistRepository{pool: pool}
}

func (r *watchlistRepository) Create(ctx context.Context, symbol string) error {
	_, err := GetDB(ctx, r.pool).Exec(ctx, "INSERT INTO watchlists (symbol) VALUES ($1) ON CONFLICT DO NOTHING", symbol)
	return err
}

func (r *watchlistRepository) GetAllActive(ctx context.Context) ([]model.Watchlist, error) {
	var watchlists []model.Watchlist
	err := pgxscan.Select(ctx, GetDB(ctx, r.pool), &watchlists, "SELECT symbol, is_active FROM watchlists WHERE is_active = TRUE")
	if err != nil {
		return nil, err
	}
	return watchlists, nil
}

func (r *watchlistRepository) Activate(ctx context.Context, symbol string) error {
	_, err := GetDB(ctx, r.pool).Exec(ctx, "UPDATE watchlists SET is_active = TRUE WHERE symbol = $1", symbol)
	return err
}

func (r *watchlistRepository) Deactivate(ctx context.Context, symbol string) error {
	_, err := GetDB(ctx, r.pool).Exec(ctx, "UPDATE watchlists SET is_active = FALSE WHERE symbol = $1", symbol)
	return err
}
