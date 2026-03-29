package indicator

import "github.com/shopspring/decimal"

func CalculateEMA(prices []decimal.Decimal, period int) decimal.Decimal {
	var k decimal.Decimal = decimal.NewFromInt(2).Div(decimal.NewFromInt(int64(period + 1)))

	var ema decimal.Decimal
	for i, price := range prices {
		if i == 0 {
			ema = price
		} else {
			ema = price.Mul(k).Add(ema.Mul(decimal.NewFromInt(1).Sub(k)))
		}
	}

	return ema
}
