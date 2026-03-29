package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type AccountTransfer struct {
	ID              uuid.UUID        `json:"id"`
	Type            string           `json:"type"`
	AmountTHB       decimal.Decimal  `json:"amount_thb"`
	AmountUSD       decimal.Decimal  `json:"amount_usd"`
	FeeTHB          decimal.Decimal  `json:"fee_thb"`
	FeeUSD          *decimal.Decimal `json:"fee_usd"`
	ExchangeRate    decimal.Decimal  `json:"exchange_rate"`
	TargetTrades    int              `json:"target_trades"`
	RemainingTrades *int             `json:"remaining_trades"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}
