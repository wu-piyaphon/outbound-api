package indicator

import (
	"errors"

	"github.com/shopspring/decimal"
)

// Bar is a single OHLC sample; only High, Low, and Close are needed for the
// indicators in this package (Open is not consumed).
type Bar struct {
	High  decimal.Decimal
	Low   decimal.Decimal
	Close decimal.Decimal
}

// CalculateATR returns the Average True Range over period bars. True Range is
// max(high-low, |high-prevClose|, |low-prevClose|) and is smoothed with the
// standard EMA (CalculateEMA) rather than Wilder's smoothing. RSI in this
// package uses Wilder's; the asymmetry is intentional — keeping ATR on a
// classic EMA produces a slightly more responsive stop distance for the
// risk-sizing formula in trade_service.
func CalculateATR(bars []Bar, period int) (decimal.Decimal, error) {
	if period <= 0 {
		return decimal.Zero, errors.New("period must be greater than zero")
	}

	if len(bars) == 0 {
		return decimal.Zero, errors.New("bars slice is empty")
	}

	if len(bars) < period+1 {
		return decimal.Zero, errors.New("not enough bars to calculate ATR")
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
