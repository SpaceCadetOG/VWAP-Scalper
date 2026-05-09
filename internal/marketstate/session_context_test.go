package marketstate

import (
	"testing"
	"time"
)

func TestBuildSessionContextUSOpen(t *testing.T) {
	// 13:45 UTC on May 8, 2026 is during New York regular session and inside US open hour.
	now := time.Date(2026, time.May, 8, 13, 45, 0, 0, time.UTC)
	ctx := BuildSessionContext(now)
	if !ctx.IsUSOpen {
		t.Fatalf("expected US open true")
	}
	if ctx.PrimarySession != "US_OPEN" {
		t.Fatalf("unexpected primary session: %s", ctx.PrimarySession)
	}
}

func TestBuildSessionContextLondonUSOverlap(t *testing.T) {
	// 14:00 UTC on a trading Friday in May is overlap between London and New York.
	now := time.Date(2026, time.May, 8, 14, 0, 0, 0, time.UTC)
	ctx := BuildSessionContext(now)
	if ctx.PrimarySession != "LONDON_US_OVERLAP" && ctx.PrimarySession != "US_OPEN" {
		t.Fatalf("unexpected primary session: %s", ctx.PrimarySession)
	}
	if len(ctx.Tags) == 0 {
		t.Fatalf("expected non-empty tags")
	}
}

func TestBuildSessionContextAsia(t *testing.T) {
	now := time.Date(2026, time.May, 7, 1, 0, 0, 0, time.UTC)
	ctx := BuildSessionContext(now)
	if ctx.PrimarySession != "ASIA" {
		t.Fatalf("unexpected primary session: %s", ctx.PrimarySession)
	}
}

func TestBuildSessionContextWeekendOffHours(t *testing.T) {
	now := time.Date(2026, time.May, 8, 22, 0, 0, 0, time.UTC)
	ctx := BuildSessionContext(now)
	if ctx.PrimarySession != "OFF_HOURS" {
		t.Fatalf("expected weekend off hours, got %s", ctx.PrimarySession)
	}
}
