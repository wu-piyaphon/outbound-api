package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/wu-piyaphon/outbound-api/internal/model"
	"github.com/wu-piyaphon/outbound-api/internal/repository"
)

type AccountTransferService interface {
	CreateAccountTransfer(ctx context.Context, transfer *model.AccountTransfer) error
	GetAvailableBudget(ctx context.Context) (*model.AccountTransfer, error)
	DecrementRemainingTrades(ctx context.Context, transferID uuid.UUID) error
}

type accountTransferService struct {
	accountTransferRepository repository.AccountTransferRepository
}

func NewAccountTransferService(accountTransferRepository repository.AccountTransferRepository) AccountTransferService {
	return &accountTransferService{accountTransferRepository: accountTransferRepository}
}

func (a *accountTransferService) CreateAccountTransfer(ctx context.Context, transfer *model.AccountTransfer) error {
	return a.accountTransferRepository.Create(ctx, transfer)
}

func (a *accountTransferService) GetAvailableBudget(ctx context.Context) (*model.AccountTransfer, error) {
	return a.accountTransferRepository.GetAvailableBudget(ctx)
}

func (a *accountTransferService) DecrementRemainingTrades(ctx context.Context, transferID uuid.UUID) error {
	return a.accountTransferRepository.DecrementRemainingTrades(ctx, transferID)
}
