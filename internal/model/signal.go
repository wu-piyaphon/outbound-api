package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type Signal struct {
	ID            uuid.UUID       `json:"id"`
	Symbol        string          `json:"symbol"`
	Side          string          `json:"side"`
	PriceAtSignal decimal.Decimal `json:"price_at_signal"`
	Indicators    map[string]any  `json:"indicators"`
	IsExecuted    bool            `json:"is_executed"`
	Reasoning     *string         `json:"reasoning"`
	CreatedAt     time.Time       `json:"created_at"`
}
