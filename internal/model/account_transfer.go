package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type AccountTransfer struct {
	ID              uuid.UUID        `db:"id" json:"id"`
	Type            string           `db:"type" json:"type"`
	AmountTHB       decimal.Decimal  `db:"amount_thb" json:"amount_thb"`
	AmountUSD       decimal.Decimal  `db:"amount_usd" json:"amount_usd"`
	FeeTHB          decimal.Decimal  `db:"fee_thb" json:"fee_thb"`
	FeeUSD          *decimal.Decimal `db:"fee_usd" json:"fee_usd"`
	ExchangeRate    decimal.Decimal  `db:"exchange_rate" json:"exchange_rate"`
	TargetTrades    int              `db:"target_trades" json:"target_trades"`
	RemainingTrades *int             `db:"remaining_trades" json:"remaining_trades"`
	CreatedAt       time.Time        `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time        `db:"updated_at" json:"updated_at"`
}
