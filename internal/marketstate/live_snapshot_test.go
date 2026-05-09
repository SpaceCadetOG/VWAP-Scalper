package marketstate

import (
	"testing"
	"time"
)

func TestStartOfUTCTradingDay(t *testing.T) {
	input := time.Date(2026, time.May, 9, 19, 5, 44, 123, time.UTC)
	got := startOfUTCTradingDay(input)
	want := time.Date(2026, time.May, 9, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("unexpected UTC day start: got=%s want=%s", got, want)
	}
}
