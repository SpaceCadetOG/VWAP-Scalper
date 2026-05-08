package accountstream

import (
	"os"
	"strings"
	"time"

	ws "github.com/SpaceCadetOG/VWAP-Scalper/internal/adapters/ws"
)

func HyperliquidAccountWSConfig() ws.VenueWSConfig {
	url := strings.TrimSpace(os.Getenv("HYPERLIQUID_ACCOUNT_WS_URL"))
	if url == "" {
		url = strings.TrimSpace(os.Getenv("HYPERLIQUID_WS_URL"))
	}
	if url == "" {
		url = "wss://api.hyperliquid.xyz/ws"
	}
	return ws.VenueWSConfig{
		Venue:       "hyperliquid",
		URL:         url,
		ConnectWait: 8 * time.Second,
	}
}

func AsterAccountWSConfig() ws.VenueWSConfig {
	url := strings.TrimSpace(os.Getenv("ASTER_ACCOUNT_WS_URL"))
	if url == "" {
		url = strings.TrimSpace(os.Getenv("ASTER_WS_URL"))
	}
	if url == "" {
		url = "wss://fstream.asterdex.com/ws"
	}
	return ws.VenueWSConfig{
		Venue:       "aster",
		URL:         url,
		ConnectWait: 8 * time.Second,
	}
}

func LighterAccountWSConfig() ws.VenueWSConfig {
	url := strings.TrimSpace(os.Getenv("LIGHTER_ACCOUNT_WS_URL"))
	if url == "" {
		url = strings.TrimSpace(os.Getenv("LIGHTER_WS_URL"))
	}
	if url == "" {
		url = "wss://mainnet.zklighter.elliot.ai/stream?readonly=true"
	}
	return ws.VenueWSConfig{
		Venue:       "lighter",
		URL:         url,
		ConnectWait: 8 * time.Second,
	}
}
