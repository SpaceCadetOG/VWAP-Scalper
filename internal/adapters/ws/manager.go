package ws

import (
	"fmt"
	"time"

	"github.com/gorilla/websocket"
)

// ConnectivityResult is a lightweight connection probe result.
type ConnectivityResult struct {
	Venue   string
	URL     string
	OK      bool
	Latency time.Duration
	Error   string
}

// ProbeConnectivity dials a websocket endpoint and closes immediately.
func ProbeConnectivity(cfg VenueWSConfig) ConnectivityResult {
	start := time.Now()
	res := ConnectivityResult{
		Venue: cfg.Venue,
		URL:   cfg.URL,
	}
	if cfg.URL == "" {
		res.Error = "missing ws url"
		return res
	}
	timeout := cfg.ConnectWait
	if timeout <= 0 {
		timeout = 6 * time.Second
	}
	dialer := websocket.Dialer{
		HandshakeTimeout: timeout,
	}
	conn, _, err := dialer.Dial(cfg.URL, nil)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	_ = conn.Close()
	res.OK = true
	res.Latency = time.Since(start)
	return res
}

// FormatConnectivity prints a compact status line for logs/CLI.
func FormatConnectivity(r ConnectivityResult) string {
	if r.OK {
		return fmt.Sprintf("%s ws_ok latency=%s url=%s", r.Venue, r.Latency.Round(time.Millisecond), r.URL)
	}
	return fmt.Sprintf("%s ws_fail err=%s url=%s", r.Venue, r.Error, r.URL)
}

