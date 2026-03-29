package service

import (
	"context"

	"github.com/wu-piyaphon/outbound-api/internal/model"
	"github.com/wu-piyaphon/outbound-api/internal/repository"
)

type SignalService interface {
	GetAllSignals(ctx context.Context) ([]model.Signal, error)
}

type signalService struct {
	signalRepo repository.SignalRepository
}

func NewSignalService(signalRepo repository.SignalRepository) SignalService {
	return &signalService{signalRepo: signalRepo}
}

func (s *signalService) GetAllSignals(ctx context.Context) ([]model.Signal, error) {
	rows, err := s.signalRepo.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	return rows, nil
}
