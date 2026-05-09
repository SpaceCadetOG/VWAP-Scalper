package strategycore

import (
	"fmt"
	"strings"

	"github.com/SpaceCadetOG/VWAP-Scalper/internal/models"
	"github.com/SpaceCadetOG/VWAP-Scalper/internal/router"
)

type CompileInput struct {
	SignalID      string
	CanonicalPair string
	SetupName     string
	State         models.StateSignal
	NotionalUSD   float64
	Delta         float64
}

type Compiler struct {
	MinConfidencePaper int
}

func NewCompiler(minConfidencePaper int) *Compiler {
	if minConfidencePaper <= 0 {
		minConfidencePaper = 70
	}
	return &Compiler{MinConfidencePaper: minConfidencePaper}
}

func (c *Compiler) Compile(in CompileInput) (router.Intent, error) {
	if strings.TrimSpace(in.SignalID) == "" {
		return router.Intent{}, fmt.Errorf("signal id is required")
	}
	if strings.TrimSpace(in.CanonicalPair) == "" {
		return router.Intent{}, fmt.Errorf("canonical pair is required")
	}
	if in.NotionalUSD <= 0 {
		return router.Intent{}, fmt.Errorf("notional must be > 0")
	}
	if in.State.ConfidenceScore < c.MinConfidencePaper {
		return router.Intent{}, fmt.Errorf("confidence %d below threshold %d", in.State.ConfidenceScore, c.MinConfidencePaper)
	}
	if in.State.State != models.StateCompression && in.State.State != models.StateExpansion {
		return router.Intent{}, fmt.Errorf("unsupported state for strategy intent: %s", in.State.State)
	}

	side := router.SideBuy
	if in.Delta < 0 {
		side = router.SideSell
	}

	setup := strings.TrimSpace(in.SetupName)
	if setup == "" {
		setup = "VWAP_HYBRID_CONFLUENCE"
	}

	return router.Intent{
		SignalID:      in.SignalID,
		Setup:         setup,
		CanonicalPair: strings.ToUpper(strings.TrimSpace(in.CanonicalPair)),
		Side:          side,
		NotionalUSD:   in.NotionalUSD,
		ReduceOnly:    false,
	}, nil
}
