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
