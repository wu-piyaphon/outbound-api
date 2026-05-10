package model

type Side string

const (
	SideBuy  Side = "buy"
	SideSell Side = "sell"
)

type Status string

const (
	StatusPending   Status = "pending"   // placed locally, awaiting first broker acknowledgement
	StatusOpen      Status = "open"      // accepted by broker, possibly partial-filled
	StatusFilled    Status = "filled"    // fully filled (terminal)
	StatusCancelled Status = "cancelled" // cancelled or expired (terminal)
	StatusRejected  Status = "rejected"  // rejected (terminal)
)

// SignalMode distinguishes live signals that drive real orders from shadow
// signals recorded for strategy comparison only.
type SignalMode string

const (
	SignalModeLive   SignalMode = "live"
	SignalModeShadow SignalMode = "shadow"
)
