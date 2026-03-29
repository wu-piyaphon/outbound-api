package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type Trade struct {
	ID                uuid.UUID        `json:"id"`
	ParentID          *uuid.UUID       `json:"parent_id"`
	SignalID          *uuid.UUID       `json:"signal_id"`
	AccountTransferID *uuid.UUID       `json:"account_transfer_id"`
	AlpacaOrderID     *string          `json:"alpaca_order_id"`
	Symbol            string           `json:"symbol"`
	Side              string           `json:"side"`
	Quantity          decimal.Decimal  `json:"quantity"`
	PricePerUnit      *decimal.Decimal `json:"price_per_unit"`
	AvgFillPrice      *decimal.Decimal `json:"avg_fill_price"`
	CommissionFee     *decimal.Decimal `json:"commission_fee"`
	FXFeeAmortized    *decimal.Decimal `json:"fx_fee_amortized"`
	Status            string           `json:"status"`
	Metadata          map[string]any   `json:"metadata"`
	FilledAt          *time.Time       `json:"filled_at"`
	CreatedAt         time.Time        `json:"created_at"`
}
