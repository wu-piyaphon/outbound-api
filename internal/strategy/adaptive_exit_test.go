package strategy

import (
	"testing"

	"github.com/shopspring/decimal"
)

func defaultAdaptiveParams() AdaptiveExitParams {
	return AdaptiveExitParams{
		BreakEvenATRTrigger: decimal.NewFromFloat(1.0),
		TrailATRTrigger:     decimal.NewFromFloat(1.5),
		TrailATRDistance:    decimal.NewFromFloat(2.0),
	}
}

func TestComputeAdaptiveEffectiveStop_NoProfitUsesDbStop(t *testing.T) {
	entry := decimal.NewFromInt(100)
	peak := decimal.NewFromInt(100)
	atr := decimal.NewFromInt(2)
	dbStop := decimal.NewFromInt(95)

	got := ComputeAdaptiveEffectiveStop(peak, entry, atr, dbStop, defaultAdaptiveParams())
	if !got.Equal(dbStop) {
		t.Fatalf("want dbStop %v, got %v", dbStop, got)
	}
}

func TestComputeAdaptiveEffectiveStop_TrailTierOverridesBreakEven(t *testing.T) {
	entry := decimal.NewFromInt(100)
	peak := decimal.NewFromInt(103) // (103-100)/2 = 1.5 ATR — trail tier
	atr := decimal.NewFromInt(2)
	dbStop := decimal.NewFromInt(95)

	got := ComputeAdaptiveEffectiveStop(peak, entry, atr, dbStop, defaultAdaptiveParams())
	// peak - 2*atr = 103 - 4 = 99
	want := decimal.NewFromInt(99)
	if !got.Equal(want) {
		t.Fatalf("trail tier: want %v, got %v", want, got)
	}
}

func TestComputeAdaptiveEffectiveStop_BreakEvOnlyBelowTrailThreshold(t *testing.T) {
	p := defaultAdaptiveParams()
	entry := decimal.NewFromInt(100)
	peak := decimal.NewFromInt(101)
	atr := decimal.NewFromInt(2)
	dbStop := decimal.NewFromInt(95)
	if got := ComputeAdaptiveEffectiveStop(peak, entry, atr, dbStop, p); !got.Equal(dbStop) {
		t.Fatalf("want %v, got %v", dbStop, got)
	}

	peak = decimal.NewFromInt(102) // profit = 1.0 -> break-even
	want := entry
	if got := ComputeAdaptiveEffectiveStop(peak, entry, atr, dbStop, p); !got.Equal(want) {
		t.Fatalf("break-even: want %v, got %v", want, got)
	}
}

func TestComputeAdaptiveEffectiveStop_TrailTierFarPeak(t *testing.T) {
	entry := decimal.NewFromInt(100)
	peak := decimal.NewFromInt(106)
	atr := decimal.NewFromInt(2)
	dbStop := decimal.NewFromInt(95)
	want := decimal.NewFromInt(102)
	got := ComputeAdaptiveEffectiveStop(peak, entry, atr, dbStop, defaultAdaptiveParams())
	if !got.Equal(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestComputeAdaptiveEffectiveStop_TrailDoesNotLoosenBelowDbStop(t *testing.T) {
	entry := decimal.NewFromInt(100)
	peak := decimal.NewFromInt(103)
	atr := decimal.NewFromInt(2)
	dbStop := decimal.NewFromInt(98)
	got := ComputeAdaptiveEffectiveStop(peak, entry, atr, dbStop, defaultAdaptiveParams())
	want := decimal.NewFromInt(99)
	if !got.Equal(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestComputeAdaptiveEffectiveStop_ZeroEntryATR(t *testing.T) {
	db := decimal.NewFromInt(90)
	got := ComputeAdaptiveEffectiveStop(
		decimal.NewFromInt(200),
		decimal.NewFromInt(100),
		decimal.Zero,
		db,
		defaultAdaptiveParams(),
	)
	if !got.Equal(db) {
		t.Fatalf("want unchanged db stop, got %v", got)
	}
}
