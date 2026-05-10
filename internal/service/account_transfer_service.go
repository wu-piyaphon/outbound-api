package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/wu-piyaphon/outbound-api/internal/model"
	"github.com/wu-piyaphon/outbound-api/internal/repository"
)

// AccountTransferService manages funding transfers and the per-transfer trade
// slot counter that bounds how many positions a single deposit can finance.
type AccountTransferService interface {
	// CreateAccountTransfer persists a new transfer record.
	CreateAccountTransfer(ctx context.Context, transfer *model.AccountTransfer) error
	// GetAvailableBudget returns the oldest transfer with remaining trade slots
	// (FIFO), or nil when no transfer has slots left.
	GetAvailableBudget(ctx context.Context) (*model.AccountTransfer, error)
	// DecrementRemainingTrades reduces the slot counter by one for transferID.
	// Returns repository.ErrNoRemainingSlots when the counter is already zero.
	DecrementRemainingTrades(ctx context.Context, transferID uuid.UUID) error
}

type accountTransferService struct {
	accountTransferRepository repository.AccountTransferRepository
}

// NewAccountTransferService constructs an AccountTransferService backed by repo.
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
