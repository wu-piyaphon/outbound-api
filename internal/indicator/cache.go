package indicator

import (
	"fmt"
	"sync"

	"github.com/shopspring/decimal"
)

// IndicatorState holds the most recently computed daily indicator values for
// a single symbol. All fields are computed from daily OHLC bars and remain
// constant until the next daily re-seed.
type IndicatorState struct {
	EMA decimal.Decimal
	RSI decimal.Decimal
	ATR decimal.Decimal
}

// IndicatorReader is the read-only view of the cache consumed by services.
type IndicatorReader interface {
	Get(symbol string) (IndicatorState, bool)
}

// IndicatorCache is a thread-safe store of per-symbol daily indicator state.
// It is seeded from historical daily bars at startup and re-seeded by a daily
// ticker. The hot trading path reads from it with zero network I/O.
type IndicatorCache struct {
	mu    sync.RWMutex
	state map[string]IndicatorState
}

// NewIndicatorCache returns an empty, ready-to-use cache.
func NewIndicatorCache() *IndicatorCache {
	return &IndicatorCache{state: make(map[string]IndicatorState)}
}

// Seed computes EMA, RSI, and ATR from the supplied historical bars and stores the result for symbol.
// It is safe to call concurrently and is idempotent. Calling it again replaces the prior state.
func (c *IndicatorCache) Seed(symbol string, bars []Bar, emaPeriod, rsiPeriod, atrPeriod int) error {
	if len(bars) == 0 {
		return fmt.Errorf("Seed %s: no bars supplied", symbol)
	}

	prices := make([]decimal.Decimal, len(bars))
	for i, b := range bars {
		prices[i] = b.Close
	}

	ema, err := CalculateEMA(prices, emaPeriod)
	if err != nil {
		return fmt.Errorf("Seed %s: EMA: %w", symbol, err)
	}

	rsi, err := CalculateRSI(prices, rsiPeriod)
	if err != nil {
		return fmt.Errorf("Seed %s: RSI: %w", symbol, err)
	}

	atr, err := CalculateATR(bars, atrPeriod)
	if err != nil {
		return fmt.Errorf("Seed %s: ATR: %w", symbol, err)
	}

	c.mu.Lock()
	c.state[symbol] = IndicatorState{EMA: ema, RSI: rsi, ATR: atr}
	c.mu.Unlock()

	return nil
}

// Get returns the cached indicator state for symbol. The second return value
// is false when the symbol has not yet been seeded.
func (c *IndicatorCache) Get(symbol string) (IndicatorState, bool) {
	c.mu.RLock()
	state, ok := c.state[symbol]
	c.mu.RUnlock()
	return state, ok
}
