package lighter

import (
	"os"
	"strings"
	"time"

	ws "github.com/SpaceCadetOG/VWAP-Scalper/internal/adapters/ws"
)

func DefaultWSConfig() ws.VenueWSConfig {
	url := strings.TrimSpace(os.Getenv("LIGHTER_WS_URL"))
	if url == "" {
		url = "wss://mainnet.zklighter.elliot.ai/stream?readonly=true"
	}
	return ws.VenueWSConfig{
		Venue:       "lighter",
		URL:         url,
		ConnectWait: 8 * time.Second,
	}
}
