package strategy

import (
	"context"
	"fmt"

	"github.com/wu-piyaphon/outbound-api/internal/service"
)

// LiveCoordinator runs the broker-backed path: exit evaluation then buy signal
// evaluation with real order placement via the service layer.
type LiveCoordinator struct {
	signalService          service.SignalService
	tradeService           service.TradeService
	accountTransferService service.AccountTransferService
}

// NewLiveCoordinator constructs a LiveCoordinator backed by the supplied services.
func NewLiveCoordinator(
	signalService service.SignalService,
	tradeService service.TradeService,
	accountTransferService service.AccountTransferService,
) *LiveCoordinator {
	return &LiveCoordinator{
		signalService:          signalService,
		tradeService:           tradeService,
		accountTransferService: accountTransferService,
	}
}

// EvaluateBar evaluates exit conditions for open trades then checks for a buy
// signal. Order is significant: exits are evaluated first so a stop-loss tick
// is processed before the same bar could trigger a new entry.
func (c *LiveCoordinator) EvaluateBar(ctx context.Context, event BarEvent) error {
	if err := c.tradeService.EvaluateAndExecuteExits(ctx, event.Symbol, event.Price); err != nil {
		return fmt.Errorf("live: EvaluateAndExecuteExits: %w", err)
	}

	entrySignal, err := c.signalService.EvaluateBuySignal(ctx, event.Symbol, event.Price)
	if err != nil {
		return fmt.Errorf("live: EvaluateBuySignal: %w", err)
	}

	if entrySignal == nil {
		return nil
	}

	budget, err := c.accountTransferService.GetAvailableBudget(ctx)
	if err != nil {
		return fmt.Errorf("live: GetAvailableBudget: %w", err)
	}
	if budget == nil {
		return nil
	}

	if _, err := c.tradeService.ExecuteBuyTrade(ctx, entrySignal, budget); err != nil {
		return fmt.Errorf("live: ExecuteBuyTrade: %w", err)
	}

	return nil
}
