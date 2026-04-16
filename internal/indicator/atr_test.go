package indicator

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestCalculateATR(t *testing.T) {
	tests := []struct {
		name     string
		bars     []Bar
		period   int
		expected decimal.Decimal
		wantErr  bool
	}{
		{
			name: "normal calculation",
			bars: []Bar{
				{High: decimal.NewFromInt(50), Low: decimal.NewFromInt(25), Close: decimal.NewFromInt(25)},
				// TR = 30
				{High: decimal.NewFromInt(55), Low: decimal.NewFromInt(30), Close: decimal.NewFromInt(20)},
				// TR = 40
				{High: decimal.NewFromInt(60), Low: decimal.NewFromInt(30), Close: decimal.NewFromInt(60)},
				// TR = 30
				{High: decimal.NewFromInt(70), Low: decimal.NewFromInt(40), Close: decimal.NewFromInt(45)},
				// TR = 45
				{High: decimal.NewFromInt(65), Low: decimal.NewFromInt(20), Close: decimal.NewFromInt(50)},
				// TR = 25
				{High: decimal.NewFromInt(75), Low: decimal.NewFromInt(50), Close: decimal.NewFromInt(55)},
			},
			// SMA = (30 + 40 + 30) / 3 = 33.3334
			// EMA(1) = (45 - 33.3334) * 2/(3+1) + 33.3334 = 39.1667
			// EMA(2) = (25 - 39.1667) * 2/(3+1) + 39.1667 = 32.0834
			// ATR = 32.0834
			period:   3,
			expected: decimal.NewFromFloat(32.0834),
		},
		{
			name: "negative period",
			bars: []Bar{
				{High: decimal.NewFromInt(50), Low: decimal.NewFromInt(25), Close: decimal.NewFromInt(25)},
				{High: decimal.NewFromInt(55), Low: decimal.NewFromInt(30), Close: decimal.NewFromInt(20)},
			},
			period:  -1,
			wantErr: true,
		},
		{
			name: "period greater than bars length",
			bars: []Bar{
				{High: decimal.NewFromInt(50), Low: decimal.NewFromInt(25), Close: decimal.NewFromInt(25)},
				{High: decimal.NewFromInt(55), Low: decimal.NewFromInt(30), Close: decimal.NewFromInt(20)},
			},
			period:  3,
			wantErr: true,
		},
		{
			name: "not enough bars to calculate ATR",
			bars: []Bar{
				{High: decimal.NewFromInt(50), Low: decimal.NewFromInt(25), Close: decimal.NewFromInt(25)},
				{High: decimal.NewFromInt(55), Low: decimal.NewFromInt(30), Close: decimal.NewFromInt(20)},
				{High: decimal.NewFromInt(60), Low: decimal.NewFromInt(30), Close: decimal.NewFromInt(60)},
			},
			period:  3,
			wantErr: true,
		},
		{
			name: "period equal to bars length minus one",
			bars: []Bar{
				{High: decimal.NewFromInt(50), Low: decimal.NewFromInt(25), Close: decimal.NewFromInt(25)},
				{High: decimal.NewFromInt(55), Low: decimal.NewFromInt(30), Close: decimal.NewFromInt(20)},
				{High: decimal.NewFromInt(60), Low: decimal.NewFromInt(30), Close: decimal.NewFromInt(60)},
				{High: decimal.NewFromInt(70), Low: decimal.NewFromInt(40), Close: decimal.NewFromInt(45)},
			},
			// SMA = (30 + 40 + 30) / 3 = 33.3334
			period:   3,
			expected: decimal.NewFromFloat(33.3334),
		},
		{
			name:    "empty bars",
			bars:    []Bar{},
			period:  3,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CalculateATR(tt.bars, tt.period)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			diff := result.Sub(tt.expected).Abs()
			if diff.GreaterThan(decimal.NewFromFloat(0.0001)) {
				t.Fatalf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
