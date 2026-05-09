package router

import (
	"fmt"
	"strings"
)

func validateConfig(cfg Config) error {
	if cfg.MaxVenuesPerSignal <= 0 {
		return fmt.Errorf("max venues per signal must be > 0")
	}
	if cfg.GlobalRiskPerSignalUSD <= 0 {
		return fmt.Errorf("global risk per signal must be > 0")
	}
	mode := strings.TrimSpace(strings.ToLower(cfg.VenueRiskSplitMode))
	switch mode {
	case "score_weighted", "equal":
		return nil
	default:
		return fmt.Errorf("unsupported venue split mode: %s", cfg.VenueRiskSplitMode)
	}
}
