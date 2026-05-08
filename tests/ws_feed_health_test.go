package tests

import (
	"testing"
	"time"

	ws "github.com/SpaceCadetOG/VWAP-Scalper/internal/adapters/ws"
)

func TestFeedHealthTrackerStaleAndFresh(t *testing.T) {
	tr := ws.NewFeedHealthTracker(2 * time.Second)
	now := time.Unix(1000, 0)

	// No event yet => stale.
	s0 := tr.Snapshot("aster", now)
	if !s0.IsStale {
		t.Fatalf("expected stale without any events")
	}

	// Recent event => fresh.
	tr.MarkEvent("aster", now.Add(-1*time.Second))
	s1 := tr.Snapshot("aster", now)
	if s1.IsStale {
		t.Fatalf("expected fresh after recent event")
	}

	// Old event => stale.
	s2 := tr.Snapshot("aster", now.Add(3*time.Second))
	if !s2.IsStale {
		t.Fatalf("expected stale after staleAfter window")
	}
}

