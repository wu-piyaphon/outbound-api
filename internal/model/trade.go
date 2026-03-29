package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type Trade struct {
	ID                uuid.UUID        `db:"id" json:"id"`
	ParentID          *uuid.UUID       `db:"parent_id" json:"parent_id"`
	SignalID          *uuid.UUID       `db:"signal_id" json:"signal_id"`
	AccountTransferID *uuid.UUID       `db:"account_transfer_id" json:"account_transfer_id"`
	AlpacaOrderID     *string          `db:"alpaca_order_id" json:"alpaca_order_id"`
	Symbol            string           `db:"symbol" json:"symbol"`
	Side              string           `db:"side" json:"side"`
	Quantity          decimal.Decimal  `db:"quantity" json:"quantity"`
	PricePerUnit      *decimal.Decimal `db:"price_per_unit" json:"price_per_unit"`
	AvgFillPrice      *decimal.Decimal `db:"avg_fill_price" json:"avg_fill_price"`
	CommissionFee     *decimal.Decimal `db:"commission_fee" json:"commission_fee"`
	FXFeeAmortized    *decimal.Decimal `db:"fx_fee_amortized" json:"fx_fee_amortized"`
	Status            string           `db:"status" json:"status"`
	Metadata          map[string]any   `db:"metadata" json:"metadata"`
	FilledAt          *time.Time       `db:"filled_at" json:"filled_at"`
	CreatedAt         time.Time        `db:"created_at" json:"created_at"`
}
