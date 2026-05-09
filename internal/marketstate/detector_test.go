package marketstate

import (
	"testing"

	"github.com/SpaceCadetOG/VWAP-Scalper/internal/models"
)

func TestDetectCompressionConfluence(t *testing.T) {
	d := NewDetector()
	s := Snapshot{
		Price:             100,
		SessionVWAP:       100.02,
		AnchoredVWAP:      100.01,
		EMA9:              100.03,
		EMA20:             99.97,
		HTFAligned:        true,
		ProfileReady:      true,
		TapeReady:         true,
		ATRRatio:          0.7,
		VolumeRatio:       1.3,
		DeltaFlipStrength: 0.3,
	}
	got := d.Detect(s)
	if got.State != models.StateCompression {
		t.Fatalf("expected compression state, got %s", got.State)
	}
	if got.ConfidenceScore < 80 {
		t.Fatalf("expected high confidence, got %d", got.ConfidenceScore)
	}
}

func TestDetectChapter3ToolGate(t *testing.T) {
	d := NewDetector()
	s := Snapshot{
		Price:             100,
		SessionVWAP:       100,
		AnchoredVWAP:      100,
		EMA9:              0,
		EMA20:             0,
		HTFAligned:        false,
		ProfileReady:      false,
		TapeReady:         false,
		ATRRatio:          0.7,
		VolumeRatio:       1.2,
		DeltaFlipStrength: 0.3,
	}
	got := d.Detect(s)
	if got.ConfidenceScore <= 0 {
		t.Fatalf("expected non-zero advisory confidence, got %d", got.ConfidenceScore)
	}
	if len(got.Invalidators) == 0 {
		t.Fatalf("expected chapter3 invalidator, got %+v", got.Invalidators)
	}
}
