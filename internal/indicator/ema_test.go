package indicator

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestCalculateEMA(t *testing.T) {
	tests := []struct {
		name     string
		prices   []decimal.Decimal
		period   int
		expected decimal.Decimal
		wantErr  bool
	}{
		{
			name:     "normal calculation",
			prices:   []decimal.Decimal{decimal.NewFromInt(10), decimal.NewFromInt(11), decimal.NewFromInt(12), decimal.NewFromInt(13), decimal.NewFromInt(14), decimal.NewFromInt(15), decimal.NewFromInt(16)},
			period:   5,
			expected: decimal.NewFromInt(14),
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
			name:     "same length of prices and period",
			prices:   []decimal.Decimal{decimal.NewFromInt(11), decimal.NewFromInt(12), decimal.NewFromInt(13)},
			period:   3,
			expected: decimal.NewFromInt(12),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CalculateEMA(tt.prices, tt.period)

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
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
