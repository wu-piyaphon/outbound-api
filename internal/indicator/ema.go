package indicator

import (
	"errors"

	"github.com/shopspring/decimal"
)

func CalculateEMA(values []decimal.Decimal, period int) (decimal.Decimal, error) {
	if period <= 0 {
		return decimal.Zero, errors.New("period must be greater than zero")
	}

	if len(values) == 0 {
		return decimal.Zero, errors.New("values slice is empty")
	}

	if len(values) < period {
		return decimal.Zero, errors.New("not enough values to calculate EMA")
	}

	multiplier := decimal.NewFromInt(2).Div(decimal.NewFromInt(int64(period + 1)))

	ema := calculateSMA(values[:period])

	for _, value := range values[period:] {
		ema = value.Sub(ema).Mul(multiplier).Add(ema)
	}

	return ema, nil
}

func calculateSMA(values []decimal.Decimal) decimal.Decimal {
	var sum decimal.Decimal
	for _, value := range values {
		sum = sum.Add(value)
	}
	return sum.Div(decimal.NewFromInt(int64(len(values))))
}
