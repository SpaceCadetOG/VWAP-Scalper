package router

import (
	"strings"
)

func BuildPlan(intent Intent, statuses []VenueStatus, cfg Config) Plan {
	plan := Plan{Intent: intent}

	if err := intent.Validate(); err != nil {
		plan.Reason = RejectInvalidIntent
		plan.ReasonText = err.Error()
		return plan
	}
	if err := validateConfig(cfg); err != nil {
		plan.Reason = RejectRiskBudgetInvalid
		plan.ReasonText = err.Error()
		return plan
	}

	eligible, rejected := selectEligible(statuses, cfg)
	plan.Rejected = append(plan.Rejected, rejected...)

	if len(eligible) == 0 {
		plan.Reason = RejectNoEligibleVenues
		plan.ReasonText = "no venue passed health/isolated/capability gates"
		return plan
	}

	selected := capSelectedVenues(eligible, cfg)
	if len(selected) == 0 {
		plan.Reason = RejectNoEligibleVenues
		plan.ReasonText = "no venue selected after cap"
		return plan
	}

	budget := minPositive(cfg.GlobalRiskPerSignalUSD, intent.NotionalUSD)
	weights := computeWeights(selected, strings.ToLower(strings.TrimSpace(cfg.VenueRiskSplitMode)))

	allocs := make([]Allocation, 0, len(selected))
	for i, s := range selected {
		w := weights[i]
		allocs = append(allocs, Allocation{
			Venue:       s.Venue,
			Weight:      w,
			NotionalUSD: round4(budget * w),
		})
	}

	plan.Allocations = allocs
	plan.Accepted = len(allocs) > 0
	if !plan.Accepted {
		plan.Reason = RejectNoEligibleVenues
		plan.ReasonText = "allocation generation returned empty"
	}
	return plan
}

func capSelectedVenues(eligible []VenueStatus, cfg Config) []VenueStatus {
	maxN := cfg.MaxVenuesPerSignal
	if !cfg.MultiVenueEnable && maxN > 1 {
		maxN = 1
	}
	if maxN > len(eligible) {
		maxN = len(eligible)
	}
	return eligible[:maxN]
}

func computeWeights(selected []VenueStatus, mode string) []float64 {
	n := len(selected)
	out := make([]float64, n)
	if n == 0 {
		return out
	}
	if mode == "equal" {
		w := 1.0 / float64(n)
		for i := range out {
			out[i] = w
		}
		return out
	}

	total := 0.0
	for _, s := range selected {
		score := s.Score
		if score <= 0 {
			score = 1
		}
		total += score
	}
	if total <= 0 {
		w := 1.0 / float64(n)
		for i := range out {
			out[i] = w
		}
		return out
	}
	for i, s := range selected {
		score := s.Score
		if score <= 0 {
			score = 1
		}
		out[i] = score / total
	}
	return out
}

func minPositive(a, b float64) float64 {
	if a <= 0 {
		return b
	}
	if b <= 0 {
		return a
	}
	if a < b {
		return a
	}
	return b
}

func round4(v float64) float64 {
	const m = 10000.0
	if v >= 0 {
		return float64(int(v*m+0.5)) / m
	}
	return float64(int(v*m-0.5)) / m
}

