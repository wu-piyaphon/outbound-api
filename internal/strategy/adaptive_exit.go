package strategy

import (
	"github.com/shopspring/decimal"
)

// AdaptiveExitParams configures ATR-based shadow exits compared against live
// static stop-loss / take-profit levels. Values are loaded from config and
// expressed in ATR multiples at entry time.
type AdaptiveExitParams struct {
	// BreakEvenATRTrigger is the profit (in ATR units) at which the stop is
	// lifted to entry price, locking out the loss. Default 1.0.
	BreakEvenATRTrigger decimal.Decimal
	// TrailATRTrigger is the profit (in ATR units) at which the trailing stop
	// activates and supersedes the break-even tier. Default 1.5.
	TrailATRTrigger     decimal.Decimal
	// TrailATRDistance is the distance (in ATR units) the trailing stop sits
	// below the running peak price once trailing is active. Default 2.0.
	TrailATRDistance    decimal.Decimal
}

// ComputeAdaptiveEffectiveStop returns the tightened stop implied by peak price,
// entry, entry-time ATR, and profit tiers. Long-only: higher stop prices are
// tighter (closer to locking profit). Starts from dbStop (live static stop).
func ComputeAdaptiveEffectiveStop(peak, entry, entryATR, dbStop decimal.Decimal, p AdaptiveExitParams) decimal.Decimal {
	eff := dbStop
	if entryATR.IsZero() {
		return eff
	}

	profitATR := peak.Sub(entry).Div(entryATR)

	if profitATR.GreaterThanOrEqual(p.TrailATRTrigger) {
		trail := peak.Sub(entryATR.Mul(p.TrailATRDistance))
		if trail.GreaterThan(eff) {
			eff = trail
		}
	} else if profitATR.GreaterThanOrEqual(p.BreakEvenATRTrigger) {
		if entry.GreaterThan(eff) {
			eff = entry
		}
	}

	return eff
}
