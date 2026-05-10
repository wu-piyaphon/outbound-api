// Package strategy provides the Coordinator abstraction for bar evaluation.
// TradingCoordinator is the default implementation: shadow observation (LLM,
// regime, adaptive exits) plus LiveCoordinator for broker-backed execution with
// keyword sentiment on buys.
package strategy

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

// BarEvent carries market data for a single closed bar tick.
type BarEvent struct {
	Symbol  string
	Price   decimal.Decimal
	BarTime time.Time
}

// Coordinator evaluates a bar event against all active strategy paths.
// Implementations must be safe for concurrent use from multiple goroutines.
type Coordinator interface {
	EvaluateBar(ctx context.Context, event BarEvent) error
}
