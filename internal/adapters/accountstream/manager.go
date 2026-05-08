package accountstream

import (
	"fmt"
	"os"
	"strings"

	ws "github.com/SpaceCadetOG/VWAP-Scalper/internal/adapters/ws"
)

type ReadinessResult struct {
	Venue   string
	Ready   bool
	Missing []string
	Notes   string
}

func (r ReadinessResult) String() string {
	if r.Ready {
		if r.Notes != "" {
			return fmt.Sprintf("%s account_stream_ready notes=%s", r.Venue, r.Notes)
		}
		return fmt.Sprintf("%s account_stream_ready", r.Venue)
	}
	return fmt.Sprintf("%s account_stream_not_ready missing=%s", r.Venue, strings.Join(r.Missing, ","))
}

func ProbeReadiness(venue string) ReadinessResult {
	switch strings.ToLower(strings.TrimSpace(venue)) {
	case "hyperliquid":
		return probeHyperliquidReadiness()
	case "aster":
		return probeAsterReadiness()
	case "lighter":
		return probeLighterReadiness()
	default:
		return ReadinessResult{
			Venue:   venue,
			Ready:   false,
			Missing: []string{"unknown_venue"},
		}
	}
}

func ProbeConnectivity(venue string) ws.ConnectivityResult {
	switch strings.ToLower(strings.TrimSpace(venue)) {
	case "hyperliquid":
		return ws.ProbeConnectivity(HyperliquidAccountWSConfig())
	case "aster":
		return ws.ProbeConnectivity(AsterAccountWSConfig())
	case "lighter":
		return ws.ProbeConnectivity(LighterAccountWSConfig())
	default:
		return ws.ConnectivityResult{
			Venue: venue,
			Error: "unknown venue",
		}
	}
}

func getEnv(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

func missing(keys ...string) []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if getEnv(k) == "" {
			out = append(out, k)
		}
	}
	return out
}

func probeHyperliquidReadiness() ReadinessResult {
	m := missing("HYPERLIQUID_ACCOUNT_ADDRESS", "HYPERLIQUID_API_WALLET_PRIVATE_KEY")
	return ReadinessResult{
		Venue:   "hyperliquid",
		Ready:   len(m) == 0,
		Missing: m,
		Notes:   "subscriptions use user address on shared ws endpoint",
	}
}

func probeAsterReadiness() ReadinessResult {
	req := []string{"ASTER_USER", "ASTER_SIGNER", "ASTER_PRIVATE_KEY"}
	if getEnv("ASTER_SIGNER") == "" && getEnv("ASTER_SIGNER_ADDRESS") != "" {
		req = []string{"ASTER_USER", "ASTER_SIGNER_ADDRESS", "ASTER_PRIVATE_KEY"}
	}
	if getEnv("ASTER_PRIVATE_KEY") == "" && getEnv("ASTER_SIGNER_PRIVATE_KEY") != "" {
		// supported fallback path used in live command wiring
		req = make([]string, 0, 3)
		if getEnv("ASTER_USER") == "" {
			req = append(req, "ASTER_USER")
		}
		if getEnv("ASTER_SIGNER") == "" && getEnv("ASTER_SIGNER_ADDRESS") == "" {
			req = append(req, "ASTER_SIGNER or ASTER_SIGNER_ADDRESS")
		}
		if getEnv("ASTER_SIGNER_PRIVATE_KEY") == "" {
			req = append(req, "ASTER_SIGNER_PRIVATE_KEY")
		}
		return ReadinessResult{
			Venue:   "aster",
			Ready:   len(req) == 0,
			Missing: req,
			Notes:   "pro api wallet mode: signer-based auth + listenKey lifecycle",
		}
	}
	m := missing(req...)
	return ReadinessResult{
		Venue:   "aster",
		Ready:   len(m) == 0,
		Missing: m,
		Notes:   "pro api wallet mode: signer-based auth + listenKey lifecycle",
	}
}

func probeLighterReadiness() ReadinessResult {
	m := missing("LIGHTER_ACCOUNT_INDEX", "LIGHTER_API_KEY_INDEX", "LIGHTER_API_PRIVATE_KEY")
	if getEnv("LIGHTER_API_PRIVATE_KEY") == "" && getEnv("LIGHTER_API_SECRET") != "" {
		// supported fallback path in existing code
		m = make([]string, 0, 3)
		for _, key := range []string{"LIGHTER_ACCOUNT_INDEX", "LIGHTER_API_KEY_INDEX"} {
			if getEnv(key) == "" {
				m = append(m, key)
			}
		}
	}
	return ReadinessResult{
		Venue:   "lighter",
		Ready:   len(m) == 0,
		Missing: m,
		Notes:   "auth token/private stream subscription required for user channels",
	}
}
