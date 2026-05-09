package replay

import (
	"testing"

	"github.com/SpaceCadetOG/VWAP-Scalper/internal/models"
	"github.com/SpaceCadetOG/VWAP-Scalper/internal/router"
)

func TestExecutePlanAccepted(t *testing.T) {
	plan := router.Plan{
		Intent:   router.Intent{SignalID: "sig-1"},
		Accepted: true,
		Allocations: []router.Allocation{
			{Venue: models.VenueAster, NotionalUSD: 20},
			{Venue: models.VenueHyperliquid, NotionalUSD: 10},
		},
	}
	e := NewEngine(FillModel{SlippageBps: 2, FeeBps: 4, LatencyMs: 100})
	res := e.ExecutePlan(plan)
	if !res.Accepted {
		t.Fatalf("expected accepted result")
	}
	if len(res.Executions) != 2 {
		t.Fatalf("expected 2 executions, got %d", len(res.Executions))
	}
	if res.TotalNotional != 30 {
		t.Fatalf("unexpected total notional: %f", res.TotalNotional)
	}
	if res.TotalNetCost <= 0 {
		t.Fatalf("expected positive net cost, got %f", res.TotalNetCost)
	}
}

func TestExecutePlanRejected(t *testing.T) {
	plan := router.Plan{
		Intent:     router.Intent{SignalID: "sig-2"},
		Accepted:   false,
		Reason:     router.RejectNoEligibleVenues,
		ReasonText: "no venues",
	}
	e := NewEngine(FillModel{})
	res := e.ExecutePlan(plan)
	if res.Accepted {
		t.Fatalf("expected rejected result")
	}
	if res.RejectReason == "" {
		t.Fatalf("expected reject reason")
	}
}
