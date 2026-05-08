package ws

import "time"

// VenueWSConfig configures one venue websocket target.
type VenueWSConfig struct {
	Venue        string
	URL          string
	ConnectWait  time.Duration
	ReadWait     time.Duration
	WriteWait    time.Duration
	PingInterval time.Duration
}

// HealthSnapshot captures current stream freshness for a venue.
type HealthSnapshot struct {
	Venue      string
	LastEvent  time.Time
	IsStale    bool
	StaleAfter time.Duration
}

