package strategy

import (
	"github.com/shopspring/decimal"
)

// AdaptiveExitParams configures ATR-based shadow exits compared against live
// static stop-loss / take-profit levels.
type AdaptiveExitParams struct {
	BreakEvenATRTrigger decimal.Decimal
	TrailATRTrigger     decimal.Decimal
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
