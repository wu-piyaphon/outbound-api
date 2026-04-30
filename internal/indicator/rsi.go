package indicator

import (
	"errors"

	"github.com/shopspring/decimal"
)

func CalculateRSI(prices []decimal.Decimal, period int) (decimal.Decimal, error) {
	if period <= 0 {
		return decimal.Zero, errors.New("period must be greater than zero")
	}

	if len(prices) <= period {
		return decimal.Zero, errors.New("not enough price data to calculate RSI")
	}

	var gains []decimal.Decimal
	var losses []decimal.Decimal

	recentPrices := prices[len(prices)-period-1:]

	for idx, price := range recentPrices[1:] {
		result := price.Sub(recentPrices[idx])

		if result.GreaterThan(decimal.Zero) {
			gains = append(gains, result)
		}
		if result.LessThan(decimal.Zero) {
			losses = append(losses, result.Abs())
		}
	}

	sumGains := decimal.Zero
	sumLosses := decimal.Zero

	for _, gain := range gains {
		sumGains = sumGains.Add(gain)
	}

	for _, loss := range losses {
		sumLosses = sumLosses.Add(loss)
	}

	avgGain := sumGains.Div(decimal.NewFromInt(int64(period)))
	avgLoss := sumLosses.Div(decimal.NewFromInt(int64(period)))

	if avgLoss.IsZero() {
		return decimal.NewFromInt(100), nil
	}

	rs := avgGain.Div(avgLoss)
	rsi := decimal.NewFromInt(100).Sub(decimal.NewFromInt(100).Div(decimal.NewFromInt(1).Add(rs)))

	return rsi, nil
}
