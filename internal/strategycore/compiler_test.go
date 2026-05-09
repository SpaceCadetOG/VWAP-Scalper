package strategycore

import (
	"testing"

	"github.com/SpaceCadetOG/VWAP-Scalper/internal/models"
	"github.com/SpaceCadetOG/VWAP-Scalper/internal/router"
)

func TestCompileIntentLong(t *testing.T) {
	c := NewCompiler(70)
	intent, err := c.Compile(CompileInput{
		SignalID:      "s1",
		CanonicalPair: "BTCUSDT",
		State: models.StateSignal{
			State:           models.StateCompression,
			ConfidenceScore: 85,
		},
		NotionalUSD: 20,
		Delta:       0.4,
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if intent.Side != router.SideBuy {
		t.Fatalf("expected buy side, got %s", intent.Side)
	}
}

func TestCompileRejectLowConfidence(t *testing.T) {
	c := NewCompiler(70)
	_, err := c.Compile(CompileInput{
		SignalID:      "s2",
		CanonicalPair: "BTCUSDT",
		State: models.StateSignal{
			State:           models.StateCompression,
			ConfidenceScore: 65,
		},
		NotionalUSD: 20,
		Delta:       -0.2,
	})
	if err == nil {
		t.Fatalf("expected low confidence rejection")
	}
}
