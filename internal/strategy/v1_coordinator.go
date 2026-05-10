package strategy

import (
	"context"
	"fmt"

	"github.com/wu-piyaphon/outbound-api/internal/service"
)

// V1Coordinator runs the current live strategy: exit evaluation then buy
// signal evaluation, with real order placement via the service layer.
// It is a direct replacement for the inline bar-worker logic in main.go.
type V1Coordinator struct {
	signalService          service.SignalService
	tradeService           service.TradeService
	accountTransferService service.AccountTransferService
}

// NewV1Coordinator constructs a V1Coordinator backed by the supplied services.
func NewV1Coordinator(
	signalService service.SignalService,
	tradeService service.TradeService,
	accountTransferService service.AccountTransferService,
) *V1Coordinator {
	return &V1Coordinator{
		signalService:          signalService,
		tradeService:           tradeService,
		accountTransferService: accountTransferService,
	}
}

// EvaluateBar evaluates exit conditions for open trades then checks for a buy
// signal. Order is significant: exits are evaluated first so a stop-loss tick
// is processed before the same bar could trigger a new entry.
func (c *V1Coordinator) EvaluateBar(ctx context.Context, event BarEvent) error {
	if err := c.tradeService.EvaluateAndExecuteExits(ctx, event.Symbol, event.Price); err != nil {
		return fmt.Errorf("v1: EvaluateAndExecuteExits: %w", err)
	}

	entrySignal, err := c.signalService.EvaluateBuySignal(ctx, event.Symbol, event.Price)
	if err != nil {
		return fmt.Errorf("v1: EvaluateBuySignal: %w", err)
	}

	if entrySignal == nil {
		return nil
	}

	budget, err := c.accountTransferService.GetAvailableBudget(ctx)
	if err != nil {
		return fmt.Errorf("v1: GetAvailableBudget: %w", err)
	}
	if budget == nil {
		return nil
	}

	if _, err := c.tradeService.ExecuteBuyTrade(ctx, entrySignal, budget); err != nil {
		return fmt.Errorf("v1: ExecuteBuyTrade: %w", err)
	}

	return nil
}
