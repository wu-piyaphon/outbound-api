package service

import (
	"context"

	"github.com/wu-piyaphon/outbound-api/internal/repository"
)

// WatchlistService manages the set of symbols the bot subscribes to and
// evaluates each bar. Activation/deactivation is reflected on the next
// 30-second watchlist refresh in main.
type WatchlistService interface {
	// Create inserts symbol; existing symbols are left unchanged.
	Create(ctx context.Context, symbol string) error
	// GetAllActive returns the symbols currently flagged is_active.
	GetAllActive(ctx context.Context) ([]string, error)
	// Activate flips is_active to true for symbol.
	Activate(ctx context.Context, symbol string) error
	// Deactivate flips is_active to false for symbol.
	Deactivate(ctx context.Context, symbol string) error
}

type watchlistService struct {
	repo repository.WatchlistRepository
}

// NewWatchlistService constructs a WatchlistService backed by repo.
func NewWatchlistService(repo repository.WatchlistRepository) WatchlistService {
	return &watchlistService{repo: repo}
}

func (s *watchlistService) Create(ctx context.Context, symbol string) error {
	return s.repo.Create(ctx, symbol)
}

func (s *watchlistService) GetAllActive(ctx context.Context) ([]string, error) {
	watchlists, err := s.repo.GetAllActive(ctx)
	if err != nil {
		return nil, err
	}

	symbols := make([]string, len(watchlists))
	for i, w := range watchlists {
		symbols[i] = w.Symbol
	}
	return symbols, nil
}

func (s *watchlistService) Activate(ctx context.Context, symbol string) error {
	return s.repo.Activate(ctx, symbol)
}

func (s *watchlistService) Deactivate(ctx context.Context, symbol string) error {
	return s.repo.Deactivate(ctx, symbol)
}
