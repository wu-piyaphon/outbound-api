package alpaca

import (
	"github.com/wu-piyaphon/outbound-api/internal/model"
)

// MapAlpacaEventToStatus converts an Alpaca trade event string to the
// corresponding internal Status. The second return value is false for
// transient lifecycle events (pending_new, pending_cancel, replaced, etc.)
// that do not change the persisted trade status.
func MapAlpacaEventToStatus(event string) (model.Status, bool) {
	switch event {
	case "new", "accepted_for_bidding":
		return model.StatusPending, true
	case "partial_fill":
		return model.StatusOpen, true
	case "fill":
		return model.StatusFilled, true
	case "rejected", "order_canceled_rejected":
		return model.StatusRejected, true
	case "canceled", "expired", "done_for_day":
		return model.StatusCancelled, true
	default:
		// This includes pending_new, pending_cancel, pending_replace, replaced, etc.
		return "", false
	}

}
