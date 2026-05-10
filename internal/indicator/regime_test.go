package indicator

import (
	"testing"

	"github.com/shopspring/decimal"
)

// makeBars builds a slice of Bar with the given close prices (high=close,
// low=close for simplicity — regime only uses Close).
func makeBars(closes ...float64) []Bar {
	bars := make([]Bar, len(closes))
	for i, c := range closes {
		d := decimal.NewFromFloat(c)
		bars[i] = Bar{High: d, Low: d, Close: d}
	}
	return bars
}

func TestRegimeCache_NotSeeded_ReturnsFalse(t *testing.T) {
	c := NewRegimeCache()
	if c.IsRiskOn() {
		t.Error("unseeded cache must return false (fail-closed)")
	}
}

func TestRegimeCache_CloseAboveEMA_RiskOn(t *testing.T) {
	c := NewRegimeCache()
	// Rising prices: last close (20) will be above EMA(3).
	bars := makeBars(10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20)
	if err := c.Seed(bars, 3); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !c.IsRiskOn() {
		t.Error("expected risk-on when close is above EMA")
	}
}

func TestRegimeCache_CloseBelowEMA_RiskOff(t *testing.T) {
	c := NewRegimeCache()
	// Falling prices: last close (1) will be below EMA(3).
	bars := makeBars(20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 1)
	if err := c.Seed(bars, 3); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.IsRiskOn() {
		t.Error("expected risk-off when close is below EMA")
	}
}

func TestRegimeCache_CloseEqualsEMA_RiskOff(t *testing.T) {
	// Flat prices: close == EMA. GreaterThan is strict so this should be off.
	c := NewRegimeCache()
	bars := makeBars(10, 10, 10, 10, 10)
	if err := c.Seed(bars, 3); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.IsRiskOn() {
		t.Error("expected risk-off when close equals EMA (strict greater-than required)")
	}
}

func TestRegimeCache_Seed_NoBars_ReturnsError(t *testing.T) {
	c := NewRegimeCache()
	if err := c.Seed([]Bar{}, 3); err == nil {
		t.Error("expected error when no bars are supplied")
	}
	if c.IsRiskOn() {
		t.Error("failed seed must not set risk-on")
	}
}

func TestRegimeCache_Seed_TooFewBarsForEMA_ReturnsError(t *testing.T) {
	c := NewRegimeCache()
	// Only 2 bars but period=50: EMA requires at least 50 bars.
	bars := makeBars(100, 101)
	if err := c.Seed(bars, 50); err == nil {
		t.Error("expected error when bars < EMA period")
	}
	if c.IsRiskOn() {
		t.Error("failed seed must not set risk-on")
	}
}

func TestRegimeCache_ReseedUpdatesState(t *testing.T) {
	c := NewRegimeCache()

	// First seed: risk-on (rising prices).
	if err := c.Seed(makeBars(10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20), 3); err != nil {
		t.Fatalf("first seed failed: %v", err)
	}
	if !c.IsRiskOn() {
		t.Error("expected risk-on after first seed")
	}

	// Re-seed: risk-off (falling prices).
	if err := c.Seed(makeBars(20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 1), 3); err != nil {
		t.Fatalf("re-seed failed: %v", err)
	}
	if c.IsRiskOn() {
		t.Error("expected risk-off after re-seed with falling prices")
	}
}

func TestRegimeCache_ImplementsRegimeReader(t *testing.T) {
	// Compile-time check that *RegimeCache satisfies the RegimeReader interface.
	var _ RegimeReader = (*RegimeCache)(nil)
}
