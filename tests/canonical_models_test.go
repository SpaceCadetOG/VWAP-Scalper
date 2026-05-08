package tests

import (
	"testing"

	"github.com/SpaceCadetOG/VWAP-Scalper/internal/models"
)

func TestOrderRequestCanonicalFields(t *testing.T) {
	req := models.OrderRequest{
		Venue:         models.VenueHyperliquid,
		AccountID:     "acct-1",
		Symbol:        "BTC-USD-PERP",
		Side:          models.SideBuy,
		Type:          models.OrderTypeLimit,
		TIF:           models.TIFGTC,
		Qty:           "0.01",
		Px:            "60000.0",
		ClientOrderID: "coid-123",
		TTL: models.TTLConfig{
			SignalTsMs:        1000,
			MaxEntryDelayMs:   1500,
			NoFollowThroughMs: 120000,
			MaxHoldMs:         180000,
		},
	}

	if req.Venue != models.VenueHyperliquid {
		t.Fatalf("unexpected venue: %s", req.Venue)
	}
	if req.TIF != models.TIFGTC {
		t.Fatalf("unexpected tif: %s", req.TIF)
	}
	if req.TTL.MaxHoldMs != 180000 {
		t.Fatalf("unexpected max hold: %d", req.TTL.MaxHoldMs)
	}
}

func TestStateSignalEnumAndConfidence(t *testing.T) {
	s := models.StateSignal{
		State:           models.StateCompression,
		ConfidenceScore: 78,
		Invalidators:    []string{"spread_too_wide"},
		ExpiryMs:        5000,
	}
	if s.State != models.StateCompression {
		t.Fatalf("unexpected state: %s", s.State)
	}
	if s.ConfidenceScore < 0 || s.ConfidenceScore > 100 {
		t.Fatalf("unexpected confidence: %d", s.ConfidenceScore)
	}
}

func TestErrorCodeCoverage(t *testing.T) {
	err := models.NormalizedError{
		Code:      models.ErrInvalidNonce,
		Venue:     models.VenueAster,
		Message:   "nonce too low",
		Retryable: true,
		RawCode:   "-1021",
	}
	if err.Code != models.ErrInvalidNonce {
		t.Fatalf("unexpected code: %s", err.Code)
	}
	if !err.Retryable {
		t.Fatalf("expected retryable")
	}
}
