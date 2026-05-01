package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/wu-piyaphon/outbound-api/internal/indicator"
	"github.com/wu-piyaphon/outbound-api/internal/sentiment"
)

type mockIndicatorReader struct {
	state indicator.IndicatorState
	ready bool
}

func (m *mockIndicatorReader) Get(_ string) (indicator.IndicatorState, bool) {
	return m.state, m.ready
}

type mockSentimentProvider struct {
	result *sentiment.Result
	err    error
}

func (m *mockSentimentProvider) Analyze(_ context.Context, _ string) (*sentiment.Result, error) {
	return m.result, m.err
}

func newTestSignalService(hasOpenPos bool, indicators indicator.IndicatorState, indicatorsReady bool, sentimentPositive bool) SignalService {
	return NewSignalService(
		&mockSignalRepo{},
		&mockTradeRepo{hasOpenPos: hasOpenPos},
		&mockIndicatorReader{state: indicators, ready: indicatorsReady},
		&mockSentimentProvider{
			result: &sentiment.Result{Positive: sentimentPositive, Score: 0.6, Reasoning: "test"},
		},
	)
}

// conditionsMetState returns an indicator state where all layers would pass
// given a price of 100: EMA < 100 (trend OK), RSI < 35 (momentum OK), ATR set.
func conditionsMetState() indicator.IndicatorState {
	return indicator.IndicatorState{
		EMA: decimal.NewFromFloat(90), // price 100 > EMA 90 ✓
		RSI: decimal.NewFromFloat(30), // RSI 30 < 35 ✓
		ATR: decimal.NewFromFloat(2),
	}
}

var testPrice = decimal.NewFromFloat(100)

func TestEvaluateBuySignal_AlreadyInPosition(t *testing.T) {
	svc := newTestSignalService(true, conditionsMetState(), true, true)

	signal, err := svc.EvaluateBuySignal(context.Background(), "AAPL", testPrice)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if signal != nil {
		t.Fatal("expected nil signal when already in position")
	}
}

func TestEvaluateBuySignal_CacheNotReady(t *testing.T) {
	svc := newTestSignalService(false, indicator.IndicatorState{}, false, true)

	signal, err := svc.EvaluateBuySignal(context.Background(), "AAPL", testPrice)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if signal != nil {
		t.Fatal("expected nil signal when cache not seeded")
	}
}

func TestEvaluateBuySignal_TrendLayerBlocks(t *testing.T) {
	// price 100 is NOT above EMA 110 — Layer 1 fails
	state := indicator.IndicatorState{
		EMA: decimal.NewFromFloat(110),
		RSI: decimal.NewFromFloat(30),
		ATR: decimal.NewFromFloat(2),
	}
	svc := newTestSignalService(false, state, true, true)

	signal, err := svc.EvaluateBuySignal(context.Background(), "AAPL", testPrice)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if signal != nil {
		t.Fatalf("expected nil signal when price below EMA, got %+v", signal)
	}
}

func TestEvaluateBuySignal_MomentumLayerBlocks(t *testing.T) {
	// RSI 60 is NOT below 35 — Layer 2 fails
	state := indicator.IndicatorState{
		EMA: decimal.NewFromFloat(90),
		RSI: decimal.NewFromFloat(60),
		ATR: decimal.NewFromFloat(2),
	}
	svc := newTestSignalService(false, state, true, true)

	signal, err := svc.EvaluateBuySignal(context.Background(), "AAPL", testPrice)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if signal != nil {
		t.Fatalf("expected nil signal when RSI above threshold, got %+v", signal)
	}
}

func TestEvaluateBuySignal_SentimentLayerBlocks(t *testing.T) {
	// All technical layers pass but sentiment is negative — Layer 3 blocks.
	svc := newTestSignalService(false, conditionsMetState(), true, false)

	signal, err := svc.EvaluateBuySignal(context.Background(), "AAPL", testPrice)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if signal != nil {
		t.Fatal("expected nil signal when sentiment is negative")
	}
}

func TestEvaluateBuySignal_AllLayersPass(t *testing.T) {
	svc := newTestSignalService(false, conditionsMetState(), true, true)

	signal, err := svc.EvaluateBuySignal(context.Background(), "AAPL", testPrice)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if signal == nil {
		t.Fatal("expected a signal when all layers pass")
	}
	if signal.Symbol != "AAPL" {
		t.Errorf("expected symbol AAPL, got %s", signal.Symbol)
	}
	if !signal.PriceAtSignal.Equal(testPrice) {
		t.Errorf("expected price %s, got %s", testPrice, signal.PriceAtSignal)
	}
	if signal.Indicators.EMA.IsZero() || signal.Indicators.RSI.IsZero() || signal.Indicators.ATR.IsZero() {
		t.Error("expected non-zero indicators on the signal")
	}
}

// ---------------------------------------------------------------------------
// Sentiment cache (integration between cachedProvider and signal evaluation)
// ---------------------------------------------------------------------------

// countingSentimentProvider counts how many times Analyze is called.
type countingSentimentProvider struct {
	calls  atomic.Int64
	result *sentiment.Result
}

func (c *countingSentimentProvider) Analyze(_ context.Context, _ string) (*sentiment.Result, error) {
	c.calls.Add(1)
	return c.result, nil
}

// TestSentimentCache_HitAvoidsNetworkCall verifies that NewCachedProvider does
// not call the inner provider a second time within the TTL window.
func TestSentimentCache_HitAvoidsNetworkCall(t *testing.T) {
	inner := &countingSentimentProvider{
		result: &sentiment.Result{Positive: true, Score: 0.8, Reasoning: "test"},
	}
	cached := sentiment.NewCachedProvider(inner, 5*time.Minute)

	// First call — should hit the inner provider.
	if _, err := cached.Analyze(context.Background(), "AAPL"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Second call within TTL — should be served from cache.
	if _, err := cached.Analyze(context.Background(), "AAPL"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := inner.calls.Load(); got != 1 {
		t.Errorf("expected inner provider called once (cache hit on second call), got %d", got)
	}
}

// TestSentimentCache_MissAfterExpiry verifies that the inner provider is called
// again once the TTL expires.
func TestSentimentCache_MissAfterExpiry(t *testing.T) {
	inner := &countingSentimentProvider{
		result: &sentiment.Result{Positive: true, Score: 0.8, Reasoning: "test"},
	}
	// Very short TTL so the entry expires quickly.
	cached := sentiment.NewCachedProvider(inner, 1*time.Millisecond)

	if _, err := cached.Analyze(context.Background(), "TSLA"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	time.Sleep(5 * time.Millisecond) // let the TTL expire

	if _, err := cached.Analyze(context.Background(), "TSLA"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := inner.calls.Load(); got != 2 {
		t.Errorf("expected inner provider called twice after TTL expiry, got %d", got)
	}
}

// TestSentimentCache_IndependentPerSymbol verifies that each symbol has its own
// cache entry and does not share state with other symbols.
func TestSentimentCache_IndependentPerSymbol(t *testing.T) {
	inner := &countingSentimentProvider{
		result: &sentiment.Result{Positive: true, Score: 0.7, Reasoning: "test"},
	}
	cached := sentiment.NewCachedProvider(inner, 5*time.Minute)

	for _, sym := range []string{"AAPL", "MSFT", "TSLA"} {
		if _, err := cached.Analyze(context.Background(), sym); err != nil {
			t.Fatalf("unexpected error for %s: %v", sym, err)
		}
	}

	if got := inner.calls.Load(); got != 3 {
		t.Errorf("expected 3 inner calls (one per distinct symbol), got %d", got)
	}

	// Second round — all should hit cache.
	for _, sym := range []string{"AAPL", "MSFT", "TSLA"} {
		if _, err := cached.Analyze(context.Background(), sym); err != nil {
			t.Fatalf("unexpected error for %s: %v", sym, err)
		}
	}
	if got := inner.calls.Load(); got != 3 {
		t.Errorf("expected still 3 inner calls after cache hits, got %d", got)
	}
}
