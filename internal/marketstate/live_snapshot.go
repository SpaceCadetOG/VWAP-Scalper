package marketstate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
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
	stats, statsErr := fetchAsterRecentStats(client, cfg.AsterBaseURL, canonicalSymbol)
	if statsErr != nil {
		stats = recentStats{
			EMA9:              px,
			EMA20:             px,
			SessionVWAP:       px,
			AnchoredVWAP:      px,
			ATRRatio:          1.0,
			VolumeRatio:       1.0,
			Delta:             deltaProxy(px, dayOpenPrice),
			DeltaFlipStrength: priceDriftBps(px, dayOpenPrice) / 10000.0,
			HTFAligned:        px >= dayOpenPrice,
			ProfileReady:      false,
			TapeReady:         false,
		}
	}

	return Snapshot{
		DayUTCOpen:        dayOpenUTC,
		DayOpenPrice:      dayOpenPrice,
		SessionContext:    BuildSessionContext(time.Now().UTC()),
		Price:             px,
		EMA9:              stats.EMA9,
		EMA20:             stats.EMA20,
		SessionVWAP:       stats.SessionVWAP,
		AnchoredVWAP:      stats.AnchoredVWAP,
		ATRRatio:          stats.ATRRatio,
		VolumeRatio:       stats.VolumeRatio,
		Delta:             stats.Delta,
		DeltaFlipStrength: stats.DeltaFlipStrength,
		HTFAligned:        stats.HTFAligned,
		ProfileReady:      stats.ProfileReady,
		TapeReady:         stats.TapeReady,
	}, nil
}

type recentStats struct {
	EMA9              float64
	EMA20             float64
	SessionVWAP       float64
	AnchoredVWAP      float64
	ATRRatio          float64
	VolumeRatio       float64
	Delta             float64
	DeltaFlipStrength float64
	HTFAligned        bool
	ProfileReady      bool
	TapeReady         bool
}

type asterCandle struct {
	open   float64
	high   float64
	low    float64
	close  float64
	volume float64
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

func fetchAsterRecentStats(client *http.Client, baseURL, symbol string) (recentStats, error) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = "https://fapi.asterdex.com"
	}
	u := fmt.Sprintf("%s/fapi/v1/klines?symbol=%s&interval=1m&limit=30", base, strings.ToUpper(strings.TrimSpace(symbol)))
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	resp, err := client.Do(req)
	if err != nil {
		return recentStats{}, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return recentStats{}, fmt.Errorf("aster recent klines status=%d", resp.StatusCode)
	}
	var raw [][]any
	if err := json.Unmarshal(b, &raw); err != nil {
		return recentStats{}, err
	}
	if len(raw) < 5 {
		return recentStats{}, fmt.Errorf("aster recent klines insufficient rows")
	}
	candles := make([]asterCandle, 0, len(raw))
	for _, row := range raw {
		if len(row) < 6 {
			continue
		}
		o, err1 := parseKlineFloat(row[1])
		h, err2 := parseKlineFloat(row[2])
		l, err3 := parseKlineFloat(row[3])
		c, err4 := parseKlineFloat(row[4])
		v, err5 := parseKlineFloat(row[5])
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil {
			continue
		}
		candles = append(candles, asterCandle{open: o, high: h, low: l, close: c, volume: v})
	}
	if len(candles) < 5 {
		return recentStats{}, fmt.Errorf("aster recent klines parse insufficient rows")
	}
	last := candles[len(candles)-1]
	prev := candles[len(candles)-2]
	ema9 := ema(candles, 9)
	ema20 := ema(candles, 20)
	sessionVWAP := weightedVWAP(candles)
	anchorStart := len(candles) - 8
	if anchorStart < 0 {
		anchorStart = 0
	}
	anchoredVWAP := weightedVWAP(candles[anchorStart:])
	if sessionVWAP <= 0 {
		sessionVWAP = last.close
	}
	if anchoredVWAP <= 0 {
		anchoredVWAP = sessionVWAP
	}
	avgVol := avgVolume(candles[:len(candles)-1])
	volumeRatio := 1.0
	if avgVol > 0 {
		volumeRatio = last.volume / avgVol
	}
	avgRangePct := avgRangePct(candles[:len(candles)-1])
	lastRangePct := safeRangePct(last.high, last.low, last.close)
	atrRatio := 1.0
	if avgRangePct > 0 {
		atrRatio = lastRangePct / avgRangePct
	}
	delta := deltaProxy(last.close, sessionVWAP)
	if last.close < sessionVWAP && prev.close < prev.open {
		delta = -math.Abs(delta)
	} else if last.close > sessionVWAP && prev.close > prev.open {
		delta = math.Abs(delta)
	}
	deltaFlipStrength := priceDriftBps(last.close, prev.close) / 10000.0
	if last.close < prev.close {
		deltaFlipStrength = -math.Abs(deltaFlipStrength)
	}
	return recentStats{
		EMA9:              ema9,
		EMA20:             ema20,
		SessionVWAP:       sessionVWAP,
		AnchoredVWAP:      anchoredVWAP,
		ATRRatio:          clampFloat(atrRatio, 0.2, 3.0),
		VolumeRatio:       clampFloat(volumeRatio, 0.2, 4.0),
		Delta:             clampFloat(delta, -1.0, 1.0),
		DeltaFlipStrength: clampFloat(deltaFlipStrength, -1.0, 1.0),
		HTFAligned:        last.close >= sessionVWAP,
		ProfileReady:      true,
		TapeReady:         true,
	}, nil
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

func parseKlineFloat(v any) (float64, error) {
	switch t := v.(type) {
	case string:
		return strconv.ParseFloat(strings.TrimSpace(t), 64)
	case float64:
		return t, nil
	default:
		return 0, fmt.Errorf("unsupported kline numeric type %T", v)
	}
}

func weightedVWAP(candles []asterCandle) float64 {
	totalPV := 0.0
	totalVol := 0.0
	for _, c := range candles {
		typical := (c.high + c.low + c.close) / 3.0
		if c.volume <= 0 || typical <= 0 {
			continue
		}
		totalPV += typical * c.volume
		totalVol += c.volume
	}
	if totalVol <= 0 {
		return 0
	}
	return totalPV / totalVol
}

func avgVolume(candles []asterCandle) float64 {
	if len(candles) == 0 {
		return 0
	}
	sum := 0.0
	n := 0.0
	for _, c := range candles {
		if c.volume <= 0 {
			continue
		}
		sum += c.volume
		n++
	}
	if n == 0 {
		return 0
	}
	return sum / n
}

func avgRangePct(candles []asterCandle) float64 {
	if len(candles) == 0 {
		return 0
	}
	sum := 0.0
	n := 0.0
	for _, c := range candles {
		r := safeRangePct(c.high, c.low, c.close)
		if r <= 0 {
			continue
		}
		sum += r
		n++
	}
	if n == 0 {
		return 0
	}
	return sum / n
}

func safeRangePct(high, low, close float64) float64 {
	if close <= 0 || high <= 0 || low < 0 || high < low {
		return 0
	}
	return (high - low) / close
}

func deltaProxy(price, reference float64) float64 {
	if price <= 0 || reference <= 0 {
		return 0
	}
	return clampFloat((price-reference)/reference*20.0, -1.0, 1.0)
}

func priceDriftBps(a, b float64) float64 {
	if a <= 0 || b <= 0 {
		return 0
	}
	return ((a - b) / b) * 10000.0
}

func clampFloat(v, minV, maxV float64) float64 {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

func ema(candles []asterCandle, period int) float64 {
	if len(candles) == 0 {
		return 0
	}
	if period <= 1 {
		return candles[len(candles)-1].close
	}
	mult := 2.0 / float64(period+1)
	val := candles[0].close
	for i := 1; i < len(candles); i++ {
		val = ((candles[i].close - val) * mult) + val
	}
	return val
}

func startOfUTCTradingDay(now time.Time) time.Time {
	now = now.UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}
