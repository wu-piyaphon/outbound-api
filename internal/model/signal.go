package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type Signal struct {
	ID            uuid.UUID        `db:"id" json:"id"`
	Symbol        string           `db:"symbol" json:"symbol"`
	Side          string           `db:"side" json:"side"`
	PriceAtSignal decimal.Decimal  `db:"price_at_signal" json:"price_at_signal"`
	Indicators    SignalIndicators `db:"indicators" json:"indicators"`
	IsExecuted    bool             `db:"is_executed" json:"is_executed"`
	Reasoning     *string          `db:"reasoning" json:"reasoning"`
	CreatedAt     time.Time        `db:"created_at" json:"created_at"`
}

type SignalIndicators struct {
	EMA decimal.Decimal `json:"ema"`
	RSI decimal.Decimal `json:"rsi"`
}
