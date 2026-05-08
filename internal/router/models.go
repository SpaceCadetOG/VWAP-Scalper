package router

import (
	"fmt"
	"strings"

	"github.com/SpaceCadetOG/VWAP-Scalper/internal/models"
)

type Side string

const (
	SideBuy  Side = "buy"
	SideSell Side = "sell"
)

type Intent struct {
	SignalID      string
	Setup         string
	CanonicalPair string
	Side          Side
	NotionalUSD   float64
	ReduceOnly    bool
}

func (i Intent) Validate() error {
	if strings.TrimSpace(i.SignalID) == "" {
		return fmt.Errorf("signal_id is required")
	}
	if strings.TrimSpace(i.CanonicalPair) == "" {
		return fmt.Errorf("canonical_pair is required")
	}
	if i.Side != SideBuy && i.Side != SideSell {
		return fmt.Errorf("invalid side: %s", i.Side)
	}
	if i.NotionalUSD <= 0 {
		return fmt.Errorf("notional_usd must be > 0")
	}
	return nil
}

type VenueStatus struct {
	Venue                 models.Venue
	Healthy               bool
	IsolatedConfirmed     bool
	SupportsPerpExecution bool
	Score                 float64
}

type Config struct {
	MultiVenueEnable       bool
	MaxVenuesPerSignal     int
	GlobalRiskPerSignalUSD float64
	VenueRiskSplitMode     string
	RequireIsolated        bool
}

func DefaultConfig() Config {
	return Config{
		MultiVenueEnable:       true,
		MaxVenuesPerSignal:     3,
		GlobalRiskPerSignalUSD: 30.0,
		VenueRiskSplitMode:     "score_weighted",
		RequireIsolated:        true,
	}
}

type RejectReason string

const (
	RejectInvalidIntent     RejectReason = "invalid_intent"
	RejectNoHealthyVenues   RejectReason = "no_healthy_venues"
	RejectNoEligibleVenues  RejectReason = "no_eligible_venues"
	RejectIsolatedRequired  RejectReason = "isolated_required"
	RejectPerpNotSupported  RejectReason = "perp_not_supported"
	RejectRiskBudgetInvalid RejectReason = "risk_budget_invalid"
)

type VenueReject struct {
	Venue  models.Venue
	Reason RejectReason
	Detail string
}

type Allocation struct {
	Venue       models.Venue
	Weight      float64
	NotionalUSD float64
}

type Plan struct {
	Intent      Intent
	Accepted    bool
	Reason      RejectReason
	ReasonText  string
	Allocations []Allocation
	Rejected    []VenueReject
}

