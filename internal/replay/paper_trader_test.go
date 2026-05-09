package replay

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/SpaceCadetOG/VWAP-Scalper/internal/router"
)

func TestPaperTraderTracksSetupsSeparately(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "paper_state.json")
	trader := NewPaperTrader(TraderConfig{StateFile: stateFile, StartBalance: 100, StopPct: 0.01, TakeProfitPct: 0.02, MaxHoldSeconds: 60})
	now := time.Unix(1_000, 0)

	opened, err := trader.OnSignal(router.Intent{CanonicalPair: "BTCUSDT", Setup: "VWAP_HYBRID_CONFLUENCE", Side: router.SideBuy, NotionalUSD: 10}, "hyperliquid", 100, now, 5, 3)
	if err != nil || !opened {
		t.Fatalf("expected first setup to open, opened=%t err=%v", opened, err)
	}
	opened, err = trader.OnSignal(router.Intent{CanonicalPair: "BTCUSDT", Setup: "VWAP_DOUBLE_TAP_REVERSAL", Side: router.SideBuy, NotionalUSD: 10}, "aster", 100, now, 5, 3)
	if err != nil || !opened {
		t.Fatalf("expected second setup to open, opened=%t err=%v", opened, err)
	}
	opened, err = trader.OnSignal(router.Intent{CanonicalPair: "BTCUSDT", Setup: "VWAP_DOUBLE_TAP_REVERSAL", Side: router.SideBuy, NotionalUSD: 10}, "aster", 100, now, 5, 3)
	if err != nil {
		t.Fatalf("unexpected duplicate err=%v", err)
	}
	if opened {
		t.Fatalf("expected duplicate setup position to be skipped")
	}

	positions := trader.OpenPositionsForSymbol("BTCUSDT")
	if len(positions) != 2 {
		t.Fatalf("expected 2 setup-specific positions, got %d", len(positions))
	}
}

func TestPaperTraderTracksVenuesSeparatelyForSameSetup(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "paper_state.json")
	trader := NewPaperTrader(TraderConfig{StateFile: stateFile, StartBalance: 100, StopPct: 0.01, TakeProfitPct: 0.02, MaxHoldSeconds: 60})
	now := time.Unix(2_000, 0)
	intent := router.Intent{CanonicalPair: "BTCUSDT", Setup: "VWAP_HYBRID_CONFLUENCE", Side: router.SideBuy, NotionalUSD: 10}

	opened, err := trader.OnSignal(intent, "hyperliquid", 100, now, 10, 3)
	if err != nil || !opened {
		t.Fatalf("expected hyperliquid position to open, opened=%t err=%v", opened, err)
	}
	opened, err = trader.OnSignal(intent, "aster", 100, now, 10, 3)
	if err != nil || !opened {
		t.Fatalf("expected aster position to open independently, opened=%t err=%v", opened, err)
	}
	opened, err = trader.OnSignal(intent, "hyperliquid", 100, now, 10, 3)
	if err != nil {
		t.Fatalf("unexpected duplicate err=%v", err)
	}
	if opened {
		t.Fatalf("expected duplicate same-venue position to be skipped")
	}

	positions := trader.OpenPositionsForSymbol("BTCUSDT")
	if len(positions) != 2 {
		t.Fatalf("expected 2 venue-specific positions, got %d", len(positions))
	}
}

func TestPaperTraderMarksAllSetupsForSymbol(t *testing.T) {
	stateFile := filepath.Join(t.TempDir(), "paper_state.json")
	trader := NewPaperTrader(TraderConfig{StateFile: stateFile, StartBalance: 100, StopPct: 0.01, TakeProfitPct: 0.02, MaxHoldSeconds: 60})
	openedAt := time.Unix(1_000, 0)
	_, _ = trader.OnSignal(router.Intent{CanonicalPair: "ETHUSDT", Setup: "VWAP_HYBRID_CONFLUENCE", Side: router.SideBuy, NotionalUSD: 10}, "hyperliquid", 100, openedAt, 5, 3)
	_, _ = trader.OnSignal(router.Intent{CanonicalPair: "ETHUSDT", Setup: "VWAP_NEWS_REACTION", Side: router.SideBuy, NotionalUSD: 10}, "aster", 100, openedAt, 5, 3)

	trades := trader.MarkSymbol("ETHUSDT", 103, openedAt.Add(61*time.Second))
	if len(trades) != 2 {
		t.Fatalf("expected 2 trades to close, got %d", len(trades))
	}
	if len(trader.OpenPositionsForSymbol("ETHUSDT")) != 0 {
		t.Fatalf("expected all ETHUSDT positions to be closed")
	}
}
