package router

import (
	"sort"
)

func selectEligible(status []VenueStatus, cfg Config) (eligible []VenueStatus, rejected []VenueReject) {
	eligible = make([]VenueStatus, 0, len(status))
	rejected = make([]VenueReject, 0, len(status))

	for _, s := range status {
		if !s.Healthy {
			rejected = append(rejected, VenueReject{
				Venue:  s.Venue,
				Reason: RejectNoHealthyVenues,
				Detail: "venue not healthy",
			})
			continue
		}
		if !s.SupportsPerpExecution {
			rejected = append(rejected, VenueReject{
				Venue:  s.Venue,
				Reason: RejectPerpNotSupported,
				Detail: "perp execution not supported",
			})
			continue
		}
		if cfg.RequireIsolated && !s.IsolatedConfirmed {
			rejected = append(rejected, VenueReject{
				Venue:  s.Venue,
				Reason: RejectIsolatedRequired,
				Detail: "isolated mode not confirmed",
			})
			continue
		}
		eligible = append(eligible, s)
	}

	sort.SliceStable(eligible, func(i, j int) bool {
		return eligible[i].Score > eligible[j].Score
	})

	return eligible, rejected
}
