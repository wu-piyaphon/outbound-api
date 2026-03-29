package service

import (
	"context"

	"github.com/wu-piyaphon/outbound-api/internal/repository"
)

type WatchlistService interface {
	Create(ctx context.Context, symbol string) error
	GetAllActive(ctx context.Context) ([]string, error)
	Activate(ctx context.Context, symbol string) error
	Deactivate(ctx context.Context, symbol string) error
}

type watchlistService struct {
	repo repository.WatchlistRepository
}

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
