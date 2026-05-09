package marketstate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type LiveSnapshotConfig struct {
	HyperliquidBaseURL string
	AsterBaseURL       string
	Timeout            time.Duration
}

func BuildLiveSnapshot(cfg LiveSnapshotConfig, canonicalSymbol string) (Snapshot, error) {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	client := &http.Client{Timeout: timeout}

	dayOpenUTC := startOfUTCTradingDay(time.Now().UTC())
	px, err := fetchAsterPrice(client, cfg.AsterBaseURL, canonicalSymbol)
	if err != nil {
		px, err = fetchHyperliquidPrice(client, cfg.HyperliquidBaseURL, canonicalSymbol)
		if err != nil {
			return Snapshot{}, fmt.Errorf("live snapshot price fetch failed: %w", err)
		}
	}
	dayOpenPrice, dayOpenErr := fetchAsterDayOpenPrice(client, cfg.AsterBaseURL, canonicalSymbol)
	if dayOpenErr != nil || dayOpenPrice <= 0 {
		dayOpenPrice = px
	}

	// Conservative defaults until full WS-derived microstructure signals are wired.
	return Snapshot{
		DayUTCOpen:        dayOpenUTC,
		DayOpenPrice:      dayOpenPrice,
		SessionContext:    BuildSessionContext(time.Now().UTC()),
		Price:             px,
		SessionVWAP:       px * 0.9998,
		AnchoredVWAP:      px * 0.9999,
		ATRRatio:          0.78,
		VolumeRatio:       1.15,
		Delta:             0.25,
		DeltaFlipStrength: 0.24,
	}, nil
}

func fetchAsterPrice(client *http.Client, baseURL, symbol string) (float64, error) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = "https://fapi.asterdex.com"
	}
	u := fmt.Sprintf("%s/fapi/v1/ticker/price?symbol=%s", base, strings.ToUpper(strings.TrimSpace(symbol)))
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return 0, fmt.Errorf("aster status=%d", resp.StatusCode)
	}
	var out struct {
		Price string `json:"price"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(out.Price), 64)
}

func fetchAsterDayOpenPrice(client *http.Client, baseURL, symbol string) (float64, error) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = "https://fapi.asterdex.com"
	}
	u := fmt.Sprintf("%s/fapi/v1/klines?symbol=%s&interval=1d&limit=1", base, strings.ToUpper(strings.TrimSpace(symbol)))
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return 0, fmt.Errorf("aster day open status=%d", resp.StatusCode)
	}
	var out [][]any
	if err := json.Unmarshal(b, &out); err != nil {
		return 0, err
	}
	if len(out) == 0 || len(out[0]) < 2 {
		return 0, fmt.Errorf("aster day open missing kline")
	}
	rawOpen, ok := out[0][1].(string)
	if !ok {
		return 0, fmt.Errorf("aster day open invalid open format")
	}
	return strconv.ParseFloat(strings.TrimSpace(rawOpen), 64)
}

func fetchHyperliquidPrice(client *http.Client, baseURL, symbol string) (float64, error) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = "https://api.hyperliquid.xyz"
	}
	u := base + "/info"
	body, _ := json.Marshal(map[string]any{"type": "allMids"})
	req, _ := http.NewRequest(http.MethodPost, u, bytes.NewReader(body))
	req.Header.Set("content-type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return 0, fmt.Errorf("hyperliquid status=%d", resp.StatusCode)
	}
	out := map[string]string{}
	if err := json.Unmarshal(b, &out); err != nil {
		return 0, err
	}
	coin := normalizeHyperliquidCoin(symbol)
	raw := strings.TrimSpace(out[coin])
	if raw == "" {
		return 0, fmt.Errorf("missing coin %s in allMids", coin)
	}
	return strconv.ParseFloat(raw, 64)
}

func normalizeHyperliquidCoin(symbol string) string {
	s := strings.ToUpper(strings.TrimSpace(symbol))
	switch {
	case strings.HasSuffix(s, "USDT"):
		return strings.TrimSuffix(s, "USDT")
	case strings.HasSuffix(s, "USD"):
		return strings.TrimSuffix(s, "USD")
	default:
		return s
	}
}

func startOfUTCTradingDay(now time.Time) time.Time {
	now = now.UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}
