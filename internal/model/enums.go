// Package model holds the domain types shared by the repository, service, and
// strategy layers. Types are plain structs with db/json tags; no behaviour.
package model

// Side is the direction of a trade or signal.
type Side string

const (
	SideBuy  Side = "buy"
	SideSell Side = "sell"
)

// Status tracks a trade's lifecycle from local placement to a terminal
// broker outcome. Filled, Cancelled, and Rejected are terminal — broker
// events for trades already in those states are ignored as replays.
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
