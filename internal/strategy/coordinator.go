// Package strategy provides the Coordinator abstraction that fans bar events
// to one or more strategy paths. The live (v1) path drives real orders; the
// shadow (v2) path runs in parallel and writes observations for comparison
// without affecting broker state.
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
