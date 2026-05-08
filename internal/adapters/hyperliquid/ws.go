package hyperliquid

import (
	"os"
	"strings"
	"time"

	ws "github.com/SpaceCadetOG/VWAP-Scalper/internal/adapters/ws"
)

func DefaultWSConfig() ws.VenueWSConfig {
	url := strings.TrimSpace(os.Getenv("HYPERLIQUID_WS_URL"))
	if url == "" {
		url = "wss://api.hyperliquid.xyz/ws"
	}
	return ws.VenueWSConfig{
		Venue:       "hyperliquid",
		URL:         url,
		ConnectWait: 8 * time.Second,
	}
}

