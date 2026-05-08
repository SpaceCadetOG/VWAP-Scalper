package router

import (
	"testing"

	"github.com/SpaceCadetOG/VWAP-Scalper/internal/models"
)

func TestBuildPlan_MultiVenueScoreWeighted(t *testing.T) {
	intent := Intent{
		SignalID:      "sig-1",
		CanonicalPair: "BTCUSDT",
		Side:          SideBuy,
		NotionalUSD:   90,
	}
	cfg := DefaultConfig()
	cfg.GlobalRiskPerSignalUSD = 90
	cfg.MaxVenuesPerSignal = 3
	cfg.VenueRiskSplitMode = "score_weighted"

	status := []VenueStatus{
		{Venue: models.VenueHyperliquid, Healthy: true, IsolatedConfirmed: true, SupportsPerpExecution: true, Score: 60},
		{Venue: models.VenueAster, Healthy: true, IsolatedConfirmed: true, SupportsPerpExecution: true, Score: 30},
		{Venue: models.VenueLighter, Healthy: true, IsolatedConfirmed: true, SupportsPerpExecution: true, Score: 10},
	}

	plan := BuildPlan(intent, status, cfg)
	if !plan.Accepted {
		t.Fatalf("expected accepted plan, got reject=%s", plan.ReasonText)
	}
	if len(plan.Allocations) != 3 {
		t.Fatalf("expected 3 allocations, got %d", len(plan.Allocations))
	}

	if plan.Allocations[0].Venue != models.VenueHyperliquid || plan.Allocations[0].NotionalUSD != 54 {
		t.Fatalf("unexpected first allocation: %+v", plan.Allocations[0])
	}
	if plan.Allocations[1].Venue != models.VenueAster || plan.Allocations[1].NotionalUSD != 27 {
		t.Fatalf("unexpected second allocation: %+v", plan.Allocations[1])
	}
	if plan.Allocations[2].Venue != models.VenueLighter || plan.Allocations[2].NotionalUSD != 9 {
		t.Fatalf("unexpected third allocation: %+v", plan.Allocations[2])
	}
}

func TestBuildPlan_RequireIsolatedRejectsCross(t *testing.T) {
	intent := Intent{
		SignalID:      "sig-2",
		CanonicalPair: "ETHUSDT",
		Side:          SideSell,
		NotionalUSD:   50,
	}
	cfg := DefaultConfig()
	cfg.GlobalRiskPerSignalUSD = 50
	cfg.RequireIsolated = true

	status := []VenueStatus{
		{Venue: models.VenueHyperliquid, Healthy: true, IsolatedConfirmed: false, SupportsPerpExecution: true, Score: 100},
	}

	plan := BuildPlan(intent, status, cfg)
	if plan.Accepted {
		t.Fatalf("expected rejected plan")
	}
	if plan.Reason != RejectNoEligibleVenues {
		t.Fatalf("unexpected reject reason: %s", plan.Reason)
	}
	if len(plan.Rejected) != 1 || plan.Rejected[0].Reason != RejectIsolatedRequired {
		t.Fatalf("expected isolated rejection, got %+v", plan.Rejected)
	}
}

func TestBuildPlan_SingleVenueMode(t *testing.T) {
	intent := Intent{
		SignalID:      "sig-3",
		CanonicalPair: "SOLUSDT",
		Side:          SideBuy,
		NotionalUSD:   60,
	}
	cfg := DefaultConfig()
	cfg.MultiVenueEnable = false
	cfg.MaxVenuesPerSignal = 3
	cfg.GlobalRiskPerSignalUSD = 60

	status := []VenueStatus{
		{Venue: models.VenueLighter, Healthy: true, IsolatedConfirmed: true, SupportsPerpExecution: true, Score: 10},
		{Venue: models.VenueAster, Healthy: true, IsolatedConfirmed: true, SupportsPerpExecution: true, Score: 50},
	}

	plan := BuildPlan(intent, status, cfg)
	if !plan.Accepted {
		t.Fatalf("expected accepted plan")
	}
	if len(plan.Allocations) != 1 {
		t.Fatalf("expected 1 allocation, got %d", len(plan.Allocations))
	}
	if plan.Allocations[0].Venue != models.VenueAster {
		t.Fatalf("expected top-scored venue, got %s", plan.Allocations[0].Venue)
	}
	if plan.Allocations[0].NotionalUSD != 60 {
		t.Fatalf("expected full budget to single venue, got %f", plan.Allocations[0].NotionalUSD)
	}
}

func TestNormalizeVenueOrderState(t *testing.T) {
	cases := map[string]ExecState{
		"new":              ExecStateSubmitted,
		"accepted":         ExecStateAccepted,
		"partial_fill":     ExecStatePartial,
		"filled":           ExecStateFilled,
		"cancelled":        ExecStateCanceled,
		"rejected":         ExecStateRejected,
		"expired":          ExecStateExpired,
		"something_weird":  ExecStateUnknown,
	}
	for raw, want := range cases {
		got := NormalizeVenueOrderState(raw)
		if got != want {
			t.Fatalf("state %q => %q, want %q", raw, got, want)
		}
	}
}

