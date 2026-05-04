package indicator

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestCalculateRSI(t *testing.T) {
	tests := []struct {
		name     string
		prices   []decimal.Decimal
		period   int
		expected decimal.Decimal
		wantErr  bool
	}{
		{
			// Wilder's RSI: seed avgGain=0.4, avgLoss=0 over first 5 changes,
			// then smooth two more bars (+2, -1) → RSI = 7200/97 ≈ 74.2268.
			name:   "normal calculation",
			prices: []decimal.Decimal{decimal.NewFromInt(10), decimal.NewFromInt(10), decimal.NewFromInt(10), decimal.NewFromInt(12), decimal.NewFromInt(12), decimal.NewFromInt(12), decimal.NewFromInt(14), decimal.NewFromInt(13)},
			period: 5,
			// 7200/97 rounded to 4 decimal places; comparison uses Round(4).
			expected: decimal.NewFromFloat(74.2268),
		},
		{
			name:    "negative period",
			prices:  []decimal.Decimal{decimal.NewFromInt(10), decimal.NewFromInt(11)},
			period:  -1,
			wantErr: true,
		},
		{
			name:    "period greater than prices length",
			prices:  []decimal.Decimal{decimal.NewFromInt(10), decimal.NewFromInt(11)},
			period:  3,
			wantErr: true,
		},
		{
			name:    "empty prices",
			prices:  []decimal.Decimal{},
			period:  5,
			wantErr: true,
		},
		{
			name:     "all gains no losses",
			prices:   []decimal.Decimal{decimal.NewFromInt(10), decimal.NewFromInt(11), decimal.NewFromInt(12), decimal.NewFromInt(13), decimal.NewFromInt(14), decimal.NewFromInt(15)},
			period:   4,
			expected: decimal.NewFromInt(100),
		},
		{
			name:     "all losses no gains",
			prices:   []decimal.Decimal{decimal.NewFromInt(15), decimal.NewFromInt(14), decimal.NewFromInt(13), decimal.NewFromInt(12), decimal.NewFromInt(11), decimal.NewFromInt(10)},
			period:   4,
			expected: decimal.Zero,
		},
		{
			name: "no gains no losses",
			prices: []decimal.Decimal{
				decimal.NewFromInt(10),
				decimal.NewFromInt(10),
				decimal.NewFromInt(10),
				decimal.NewFromInt(10),
				decimal.NewFromInt(10),
				decimal.NewFromInt(10),
			},
			period:   4,
			expected: decimal.NewFromInt(100),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CalculateRSI(tt.prices, tt.period)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !result.Round(4).Equal(tt.expected.Round(4)) {
			t.Fatalf("expected %v, got %v", tt.expected.Round(4), result.Round(4))
		}
		})
	}
}
