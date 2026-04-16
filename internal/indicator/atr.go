package indicator

import (
	"errors"

	"github.com/shopspring/decimal"
)

type Bar struct {
	High  decimal.Decimal
	Low   decimal.Decimal
	Close decimal.Decimal
}

func CalculateATR(bars []Bar, period int) (decimal.Decimal, error) {
	if period <= 0 {
		return decimal.Zero, errors.New("Period is less than zero. Unable to calculate ATR.")
	}

	if len(bars) == 0 {
		return decimal.Zero, errors.New("Bars array is empty. Unable to calculate ATR.")
	}

	if len(bars) < period+1 {
		return decimal.Zero, errors.New("Not enough bars to calculate ATR.")
	}

	trs := make([]decimal.Decimal, 0, len(bars)-1)
	for i := 1; i < len(bars); i++ {
		tr := calculateTrueRange(bars[i], bars[i-1].Close)
		trs = append(trs, tr)
	}

	atr, err := CalculateEMA(trs, period)
	if err != nil {
		return decimal.Zero, err
	}

	return atr, nil
}

func calculateTrueRange(bar Bar, prevClose decimal.Decimal) decimal.Decimal {
	highLow := bar.High.Sub(bar.Low).Abs()
	highPrevClose := bar.High.Sub(prevClose).Abs()
	lowPrevClose := bar.Low.Sub(prevClose).Abs()

	return decimal.Max(highLow, decimal.Max(highPrevClose, lowPrevClose))
}
