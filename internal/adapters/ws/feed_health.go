package ws

import (
	"sync"
	"time"
)

// FeedHealthTracker tracks last event timestamps and stale state by venue.
type FeedHealthTracker struct {
	mu         sync.RWMutex
	lastEvent  map[string]time.Time
	staleAfter time.Duration
}

func NewFeedHealthTracker(staleAfter time.Duration) *FeedHealthTracker {
	if staleAfter <= 0 {
		staleAfter = 10 * time.Second
	}
	return &FeedHealthTracker{
		lastEvent:  make(map[string]time.Time),
		staleAfter: staleAfter,
	}
}

func (t *FeedHealthTracker) MarkEvent(venue string, ts time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lastEvent[venue] = ts
}

func (t *FeedHealthTracker) Snapshot(venue string, now time.Time) HealthSnapshot {
	t.mu.RLock()
	defer t.mu.RUnlock()
	last := t.lastEvent[venue]
	isStale := last.IsZero() || now.Sub(last) > t.staleAfter
	return HealthSnapshot{
		Venue:      venue,
		LastEvent:  last,
		IsStale:    isStale,
		StaleAfter: t.staleAfter,
	}
}

