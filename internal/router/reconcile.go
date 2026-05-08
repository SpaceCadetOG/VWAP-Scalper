package router

import "strings"

type ExecState string

const (
	ExecStateSubmitted ExecState = "submitted"
	ExecStateAccepted  ExecState = "accepted"
	ExecStatePartial   ExecState = "partially_filled"
	ExecStateFilled    ExecState = "filled"
	ExecStateCanceled  ExecState = "canceled"
	ExecStateRejected  ExecState = "rejected"
	ExecStateExpired   ExecState = "expired"
	ExecStateUnknown   ExecState = "unknown"
)

func NormalizeVenueOrderState(raw string) ExecState {
	s := strings.ToLower(strings.TrimSpace(raw))
	switch s {
	case "new", "submitted", "open":
		return ExecStateSubmitted
	case "accepted", "acknowledged", "resting":
		return ExecStateAccepted
	case "partially_filled", "partial_fill", "partial":
		return ExecStatePartial
	case "filled", "done":
		return ExecStateFilled
	case "canceled", "cancelled":
		return ExecStateCanceled
	case "rejected", "error":
		return ExecStateRejected
	case "expired":
		return ExecStateExpired
	default:
		return ExecStateUnknown
	}
}

