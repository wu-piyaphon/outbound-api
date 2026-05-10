package indicator

import (
	"fmt"
	"sync"

	"github.com/shopspring/decimal"
)

// RegimeReader is the read-only view of the regime cache consumed by strategy
// coordinators. Fail-closed: returns false until the cache is seeded.
type RegimeReader interface {
	IsRiskOn() bool
}

// RegimeCache stores whether the market is in a risk-on regime based on a
// single index (typically SPY) relative to its EMA. It is seeded at startup
// and refreshed by the daily re-seed ticker, independently of IndicatorCache.
//
// Fail-closed design: IsRiskOn returns false until Seed succeeds at least once,
// so a startup failure suppresses all shadow buy signals rather than letting
// them through with an uncertain regime.
type RegimeCache struct {
	mu     sync.RWMutex
	seeded bool
	riskOn bool // last close > EMA(emaPeriod)
}

// NewRegimeCache returns an unseeded cache. IsRiskOn returns false until Seed
// is called successfully.
func NewRegimeCache() *RegimeCache {
	return &RegimeCache{}
}

// Seed computes EMA(emaPeriod) from the supplied bars and stores whether the
// final close is above it. Safe for concurrent use; replaces any prior state.
func (c *RegimeCache) Seed(bars []Bar, emaPeriod int) error {
	if len(bars) == 0 {
		return fmt.Errorf("RegimeCache.Seed: no bars supplied")
	}

	closes := make([]decimal.Decimal, len(bars))
	for i, b := range bars {
		closes[i] = b.Close
	}

	ema, err := CalculateEMA(closes, emaPeriod)
	if err != nil {
		return fmt.Errorf("RegimeCache.Seed: EMA: %w", err)
	}

	lastClose := bars[len(bars)-1].Close

	c.mu.Lock()
	c.seeded = true
	c.riskOn = lastClose.GreaterThan(ema)
	c.mu.Unlock()

	return nil
}

// IsRiskOn returns true when the regime index's last close is above its EMA.
// Returns false if the cache has not been seeded (fail-closed).
func (c *RegimeCache) IsRiskOn() bool {
	c.mu.RLock()
	v := c.seeded && c.riskOn
	c.mu.RUnlock()
	return v
}
