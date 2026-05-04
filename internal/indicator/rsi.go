package indicator

import (
	"errors"

	"github.com/shopspring/decimal"
)

// CalculateRSI computes the Relative Strength Index using Wilder's smoothed
// moving average, which matches the formula used by standard charting platforms
// (TradingView, Bloomberg). A minimum of period+1 prices is required to seed
// the initial simple average; all subsequent bars apply Wilder's smoothing:
//
//	avgGain = (prevAvgGain × (period−1) + currentGain) / period
func CalculateRSI(prices []decimal.Decimal, period int) (decimal.Decimal, error) {
	if period <= 0 {
		return decimal.Zero, errors.New("period must be greater than zero")
	}

	if len(prices) <= period {
		return decimal.Zero, errors.New("not enough price data to calculate RSI")
	}

	periodD := decimal.NewFromInt(int64(period))
	periodMinus1 := decimal.NewFromInt(int64(period - 1))

	// Seed: simple average of the first `period` price changes.
	var sumGain, sumLoss decimal.Decimal
	for i := 1; i <= period; i++ {
		change := prices[i].Sub(prices[i-1])
		if change.GreaterThan(decimal.Zero) {
			sumGain = sumGain.Add(change)
		} else if change.LessThan(decimal.Zero) {
			sumLoss = sumLoss.Add(change.Abs())
		}
	}

	avgGain := sumGain.Div(periodD)
	avgLoss := sumLoss.Div(periodD)

	// Wilder's smoothed moving average for all remaining bars.
	for i := period + 1; i < len(prices); i++ {
		change := prices[i].Sub(prices[i-1])
		var gain, loss decimal.Decimal
		if change.GreaterThan(decimal.Zero) {
			gain = change
		} else if change.LessThan(decimal.Zero) {
			loss = change.Abs()
		}
		avgGain = avgGain.Mul(periodMinus1).Add(gain).Div(periodD)
		avgLoss = avgLoss.Mul(periodMinus1).Add(loss).Div(periodD)
	}

	if avgLoss.IsZero() {
		return decimal.NewFromInt(100), nil
	}

	rs := avgGain.Div(avgLoss)
	rsi := decimal.NewFromInt(100).Sub(decimal.NewFromInt(100).Div(decimal.NewFromInt(1).Add(rs)))

	return rsi, nil
}
