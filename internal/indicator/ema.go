package indicator

import (
	"errors"

	"github.com/shopspring/decimal"
)

func CalculateEMA(prices []decimal.Decimal, period int) (decimal.Decimal, error) {

	if period <= 0 {
		return decimal.Zero, errors.New("Period is less than zero. Unable to calculate EMA.")
	}

	if len(prices) == 0 {
		return decimal.Zero, errors.New("Prices array is empty. Unable to calculate EMA.")
	}

	if len(prices) < period {
		return decimal.Zero, errors.New("Not enough price data to calculate EMA.")
	}

	multiplier := decimal.NewFromInt(2).Div(decimal.NewFromInt(int64(period + 1)))

	ema := calculateSMA(prices[:period])

	for _, price := range prices[period:] {
		ema = price.Sub(ema).Mul(multiplier).Add(ema)
	}

	return ema, nil
}

func calculateSMA(prices []decimal.Decimal) decimal.Decimal {
	var sum decimal.Decimal
	for _, price := range prices {
		sum = sum.Add(price)
	}
	return sum.Div(decimal.NewFromInt(int64(len(prices))))
}
