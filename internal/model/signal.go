package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Signal records a buy-or-sell evaluation outcome. Live signals drive real
// orders (IsExecuted flips to true after broker placement). Shadow signals are
// recorded for strategy comparison only and never trigger orders.
type Signal struct {
	ID            uuid.UUID       `db:"id" json:"id"`
	Symbol        string          `db:"symbol" json:"symbol"`
	Side          Side            `db:"side" json:"side"`
	PriceAtSignal decimal.Decimal `db:"price_at_signal" json:"price_at_signal"`
	Indicators    SignalIndicators `db:"indicators" json:"indicators"`
	IsExecuted    bool            `db:"is_executed" json:"is_executed"`
	Mode          SignalMode      `db:"mode" json:"mode"`
	Reasoning     *string         `db:"reasoning" json:"reasoning"`
	CreatedAt     time.Time       `db:"created_at" json:"created_at"`
}

// SignalIndicators captures the daily indicator values used at signal time so
// the decision can be reconstructed from the row alone.
type SignalIndicators struct {
	EMA decimal.Decimal `json:"ema"`
	RSI decimal.Decimal `json:"rsi"`
	ATR decimal.Decimal `json:"atr"`
}
