package aster

import (
	"os"
	"strings"
	"time"

	ws "github.com/SpaceCadetOG/VWAP-Scalper/internal/adapters/ws"
)

func DefaultWSConfig() ws.VenueWSConfig {
	url := strings.TrimSpace(os.Getenv("ASTER_WS_URL"))
	if url == "" {
		url = "wss://fstream.asterdex.com/ws"
	}
	return ws.VenueWSConfig{
		Venue:       "aster",
		URL:         url,
		ConnectWait: 8 * time.Second,
	}
}

