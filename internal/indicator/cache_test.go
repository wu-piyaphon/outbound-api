package indicator

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestSeed_RSIPrevMatchesTruncatedSeries(t *testing.T) {
	c := NewIndicatorCache()

	// Enough bars for EMA(5), RSI(3)+cross state, ATR(3).
	n := 12
	bars := make([]Bar, n)
	base := decimal.NewFromInt(100)
	for i := range bars {
		d := base.Add(decimal.NewFromInt(int64(i)))
		bars[i] = Bar{High: d, Low: d, Close: d}
	}

	const (
		emaPeriod = 5
		rsiPeriod = 3
		atrPeriod = 3
	)

	prices := make([]decimal.Decimal, n)
	for i, b := range bars {
		prices[i] = b.Close
	}

	wantPrev, err := CalculateRSI(prices[:n-1], rsiPeriod)
	if err != nil {
		t.Fatalf("CalculateRSI truncated: %v", err)
	}
	wantCurr, err := CalculateRSI(prices, rsiPeriod)
	if err != nil {
		t.Fatalf("CalculateRSI full: %v", err)
	}

	if err := c.Seed("TEST", bars, emaPeriod, rsiPeriod, atrPeriod); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	state, ok := c.Get("TEST")
	if !ok {
		t.Fatal("expected seeded symbol")
	}
	if !state.RSIPrev.Equal(wantPrev) {
		t.Errorf("RSIPrev: want %v got %v", wantPrev, state.RSIPrev)
	}
	if !state.RSI.Equal(wantCurr) {
		t.Errorf("RSI: want %v got %v", wantCurr, state.RSI)
	}
}

func TestSeed_TooFewBarsForRSIPrev_ReturnsError(t *testing.T) {
	c := NewIndicatorCache()

	// rsiPeriod+1 bars: enough for single RSI on full series, not for RSIPrev.
	const rsiPeriod = 14
	n := rsiPeriod + 1
	bars := make([]Bar, n)
	for i := range bars {
		d := decimal.NewFromInt(int64(100 + i))
		bars[i] = Bar{High: d, Low: d, Close: d}
	}

	err := c.Seed("X", bars, 10, rsiPeriod, 10)
	if err == nil {
		t.Fatal("expected error when len(bars) < rsiPeriod+2")
	}
}
