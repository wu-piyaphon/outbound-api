package model

type Side string

const (
	SideBuy  Side = "buy"
	SideSell Side = "sell"
)

type Status string

const (
	StatusPending  Status = "pending"  // placed locally, awaiting first broker acknowledgement
	StatusOpen     Status = "open"     // accepted by broker, possibly partial-filled
	StatusFilled   Status = "filled"   // fully filled (terminal)
	StatusCanceled Status = "canceled" // canceled or expired (terminal)
	StatusRejected Status = "rejected" // rejected (terminal)
)
