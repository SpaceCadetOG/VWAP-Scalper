package marketstate

import (
	"math"
	"os"
	"strings"

	"github.com/SpaceCadetOG/VWAP-Scalper/internal/models"
)

// Snapshot is a compact normalized market input for regime detection.
type Snapshot struct {
	Price             float64
	SessionVWAP       float64
	AnchoredVWAP      float64
	EMA9              float64
	EMA20             float64
	HTFAligned        bool
	ProfileReady      bool
	TapeReady         bool
	ATRRatio          float64
	VolumeRatio       float64
	Delta             float64
	DeltaFlipStrength float64
}

// Detector converts snapshots into market state signals.
type Detector struct{}

func NewDetector() *Detector { return &Detector{} }

func (d *Detector) Detect(s Snapshot) models.StateSignal {
	toolingReady := s.HTFAligned && s.ProfileReady && s.TapeReady && s.EMA9 > 0 && s.EMA20 > 0
	strictGate := strings.EqualFold(strings.TrimSpace(os.Getenv("CHAPTER3_STRICT_GATE")), "true")
	if strictGate && !toolingReady {
		return models.StateSignal{
			State:           models.StateChop,
			ConfidenceScore: 0,
			Invalidators:    []string{"chapter3_toolstack_not_ready"},
			ExpiryMs:        1000,
		}
	}

	if s.SessionVWAP <= 0 || s.Price <= 0 {
		return models.StateSignal{
			State:           models.StateChop,
			ConfidenceScore: 0,
			Invalidators:    []string{"missing_price_or_vwap"},
			ExpiryMs:        1000,
		}
	}

	vwapDistBps := math.Abs((s.Price-s.SessionVWAP)/s.SessionVWAP) * 10000.0
	anchorAligned := s.AnchoredVWAP > 0 && math.Abs((s.SessionVWAP-s.AnchoredVWAP)/s.SessionVWAP)*10000.0 <= 20.0
	lowVol := s.ATRRatio > 0 && s.ATRRatio <= 0.8
	volSupport := s.VolumeRatio >= 1.0
	deltaFlip := math.Abs(s.DeltaFlipStrength) >= 0.2

	// Premium selectivity setup for Chapter 28 hybrid confluence.
	if vwapDistBps <= 25.0 && anchorAligned && volSupport && deltaFlip {
		score := 85
		if lowVol {
			score += 5
		}
		if !toolingReady {
			score -= 12
		}
		if score > 100 {
			score = 100
		}
		invalidators := []string{}
		if !toolingReady {
			invalidators = append(invalidators, "chapter3_toolstack_not_ready")
		}
		return models.StateSignal{
			State:           models.StateCompression,
			ConfidenceScore: score,
			Invalidators:    invalidators,
			ExpiryMs:        180000,
		}
	}

	if vwapDistBps > 60.0 && s.VolumeRatio >= 1.3 {
		return models.StateSignal{
			State:           models.StateExpansion,
			ConfidenceScore: 70,
			Invalidators:    []string{"too_extended_for_confluence"},
			ExpiryMs:        45000,
		}
	}

	return models.StateSignal{
		State:           models.StateChop,
		ConfidenceScore: 40,
		Invalidators:    []string{"no_clean_state_alignment"},
		ExpiryMs:        30000,
	}
}
