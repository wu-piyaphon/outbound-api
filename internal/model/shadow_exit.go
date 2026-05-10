package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ShadowExitDecision records what the shadow strategy would have done at a
// given bar tick for an open trade. Hold actions are inferred by absence;
// only non-trivial state changes are written to keep the table compact.
type ShadowExitDecision struct {
	ID           uuid.UUID        `db:"id"`
	TradeID      uuid.UUID        `db:"trade_id"`
	BarTime      time.Time        `db:"bar_time"`
	CurrentPrice decimal.Decimal  `db:"current_price"`
	PeakPrice    decimal.Decimal  `db:"peak_price"`
	CurrentStop  *decimal.Decimal `db:"current_stop"`
	// Action is one of: "stop_moved", "exit_stop", "exit_take_profit".
	Action    string  `db:"action"`
	Reasoning *string `db:"reasoning"`
	CreatedAt time.Time `db:"created_at"`
}
