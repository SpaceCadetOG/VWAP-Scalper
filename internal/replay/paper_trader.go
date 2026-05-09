package replay

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/SpaceCadetOG/VWAP-Scalper/internal/router"
)

type PaperPosition struct {
	Setup       string    `json:"setup"`
	Symbol      string    `json:"symbol"`
	Venue       string    `json:"venue"`
	Side        string    `json:"side"`
	EntryPrice  float64   `json:"entry_price"`
	Leverage    int       `json:"leverage"`
	MarginUsed  float64   `json:"margin_used"`
	NotionalUSD float64   `json:"notional_usd"`
	Qty         float64   `json:"qty"`
	OpenedAt    time.Time `json:"opened_at"`
}

type PaperTrade struct {
	Setup       string    `json:"setup"`
	Symbol      string    `json:"symbol"`
	Venue       string    `json:"venue"`
	Side        string    `json:"side"`
	EntryPrice  float64   `json:"entry_price"`
	ExitPrice   float64   `json:"exit_price"`
	Leverage    int       `json:"leverage"`
	MarginUsed  float64   `json:"margin_used"`
	NotionalUSD float64   `json:"notional_usd"`
	Qty         float64   `json:"qty"`
	PnlUSD      float64   `json:"pnl_usd"`
	Reason      string    `json:"reason"`
	OpenedAt    time.Time `json:"opened_at"`
	ClosedAt    time.Time `json:"closed_at"`
}

type PaperState struct {
	BalanceUSD    float64                   `json:"balance_usd"`
	VenueBalances map[string]float64        `json:"venue_balances"`
	LastMarks     map[string]float64        `json:"last_marks"`
	Positions     map[string]*PaperPosition `json:"positions"`
	Trades        []PaperTrade              `json:"trades"`
}

type TraderConfig struct {
	StateFile      string
	StartBalance   float64
	StopPct        float64
	TakeProfitPct  float64
	MaxHoldSeconds int
}

type PaperTrader struct {
	cfg   TraderConfig
	state PaperState
}

func NewPaperTrader(cfg TraderConfig) *PaperTrader {
	if cfg.StateFile == "" {
		cfg.StateFile = "out/paper_state.json"
	}
	if cfg.StartBalance <= 0 {
		cfg.StartBalance = 100
	}
	if cfg.StopPct <= 0 {
		cfg.StopPct = 0.006
	}
	if cfg.TakeProfitPct <= 0 {
		cfg.TakeProfitPct = 0.009
	}
	if cfg.MaxHoldSeconds <= 0 {
		cfg.MaxHoldSeconds = 180
	}
	p := &PaperTrader{cfg: cfg}
	_ = p.load()
	if p.state.Positions == nil {
		p.state.Positions = map[string]*PaperPosition{}
	}
	p.normalizeState()
	p.ensureVenueBalances()
	p.recalculateBalance()
	return p
}

func (p *PaperTrader) State() PaperState { return p.state }

func (p *PaperTrader) UpdateMark(symbol string, price float64) {
	sym := strings.ToUpper(strings.TrimSpace(symbol))
	if sym == "" || price <= 0 {
		return
	}
	if p.state.LastMarks == nil {
		p.state.LastMarks = map[string]float64{}
	}
	p.state.LastMarks[sym] = price
}

func (p *PaperTrader) OnSignal(intent router.Intent, venue string, price float64, now time.Time, venueNotional float64, leverage int) (bool, error) {
	sym := strings.ToUpper(strings.TrimSpace(intent.CanonicalPair))
	if sym == "" || price <= 0 {
		return false, fmt.Errorf("invalid signal/price")
	}
	setup := normalizeSetup(intent.Setup)
	venue = normalizeVenue(venue)
	key := paperPositionKey(sym, setup, venue)
	if _, exists := p.state.Positions[key]; exists {
		return false, nil
	}
	notional := venueNotional
	if notional <= 0 {
		notional = intent.NotionalUSD
	}
	if notional <= 0 {
		return false, fmt.Errorf("no paper balance")
	}
	if leverage <= 0 {
		leverage = 1
	}
	marginRequired := notional / float64(leverage)
	available := p.state.VenueBalances[venue]
	if marginRequired > available {
		return false, fmt.Errorf("no paper balance for venue=%s available=%.4f required=%.4f", venue, available, marginRequired)
	}
	qty := notional / price
	pos := &PaperPosition{
		Setup:       setup,
		Symbol:      sym,
		Venue:       venue,
		Side:        strings.ToLower(string(intent.Side)),
		EntryPrice:  price,
		Leverage:    leverage,
		MarginUsed:  marginRequired,
		NotionalUSD: notional,
		Qty:         qty,
		OpenedAt:    now,
	}
	p.state.VenueBalances[venue] -= marginRequired
	p.state.Positions[key] = pos
	p.UpdateMark(sym, price)
	p.recalculateBalance()
	return true, p.save()
}

func (p *PaperTrader) MarkSymbol(symbol string, price float64, now time.Time) []PaperTrade {
	sym := strings.ToUpper(strings.TrimSpace(symbol))
	if sym == "" || price <= 0 {
		return nil
	}
	matched := make([]string, 0)
	for key, pos := range p.state.Positions {
		if pos == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(pos.Symbol), sym) {
			matched = append(matched, key)
		}
	}
	if len(matched) == 0 {
		return nil
	}
	sort.Strings(matched)
	trades := make([]PaperTrade, 0)
	changed := false
	for _, key := range matched {
		pos := p.state.Positions[key]
		if pos == nil {
			continue
		}
		stop := pos.EntryPrice * (1 - p.cfg.StopPct)
		take := pos.EntryPrice * (1 + p.cfg.TakeProfitPct)
		if pos.Side == "sell" {
			stop = pos.EntryPrice * (1 + p.cfg.StopPct)
			take = pos.EntryPrice * (1 - p.cfg.TakeProfitPct)
		}
		reason := ""
		if pos.Side == "buy" {
			if price <= stop {
				reason = "stop"
			} else if price >= take {
				reason = "take_profit"
			}
		} else {
			if price >= stop {
				reason = "stop"
			} else if price <= take {
				reason = "take_profit"
			}
		}
		if reason == "" && now.Sub(pos.OpenedAt) >= time.Duration(p.cfg.MaxHoldSeconds)*time.Second {
			reason = "max_hold"
		}
		if reason == "" {
			continue
		}
		pnl := (price - pos.EntryPrice) * pos.Qty
		if pos.Side == "sell" {
			pnl = -pnl
		}
		p.state.VenueBalances[normalizeVenue(pos.Venue)] += pos.MarginUsed + pnl
		tr := PaperTrade{
			Setup:       normalizeSetup(pos.Setup),
			Symbol:      pos.Symbol,
			Venue:       pos.Venue,
			Side:        pos.Side,
			EntryPrice:  pos.EntryPrice,
			ExitPrice:   price,
			Leverage:    pos.Leverage,
			MarginUsed:  pos.MarginUsed,
			NotionalUSD: pos.NotionalUSD,
			Qty:         pos.Qty,
			PnlUSD:      pnl,
			Reason:      reason,
			OpenedAt:    pos.OpenedAt,
			ClosedAt:    now,
		}
		p.state.Trades = append(p.state.Trades, tr)
		delete(p.state.Positions, key)
		trades = append(trades, tr)
		changed = true
	}
	if changed {
		p.recalculateBalance()
		_ = p.save()
	}
	return trades
}

func (p *PaperTrader) OpenPositionsForSymbol(symbol string) []*PaperPosition {
	sym := strings.ToUpper(strings.TrimSpace(symbol))
	if sym == "" {
		return nil
	}
	out := make([]*PaperPosition, 0)
	for _, pos := range p.state.Positions {
		if pos == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(pos.Symbol), sym) {
			cp := *pos
			cp.Setup = normalizeSetup(cp.Setup)
			out = append(out, &cp)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Setup < out[j].Setup
	})
	return out
}

func (p *PaperTrader) OpenPositions() []*PaperPosition {
	out := make([]*PaperPosition, 0, len(p.state.Positions))
	for _, pos := range p.state.Positions {
		if pos == nil {
			continue
		}
		cp := *pos
		cp.Symbol = strings.ToUpper(strings.TrimSpace(cp.Symbol))
		cp.Setup = normalizeSetup(cp.Setup)
		out = append(out, &cp)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Symbol == out[j].Symbol {
			if out[i].Venue == out[j].Venue {
				return out[i].Setup < out[j].Setup
			}
			return out[i].Venue < out[j].Venue
		}
		return out[i].Symbol < out[j].Symbol
	})
	return out
}

func (p *PaperTrader) load() error {
	b, err := os.ReadFile(p.cfg.StateFile)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, &p.state)
}

func (p *PaperTrader) save() error {
	dir := filepath.Dir(p.cfg.StateFile)
	if dir != "." && dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	b, err := json.MarshalIndent(p.state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p.cfg.StateFile, b, 0o644)
}

func (p *PaperTrader) normalizeState() {
	if p.state.Positions == nil {
		p.state.Positions = map[string]*PaperPosition{}
	}
	if p.state.VenueBalances == nil {
		p.state.VenueBalances = map[string]float64{}
	}
	if p.state.LastMarks == nil {
		p.state.LastMarks = map[string]float64{}
	}
	normalized := make(map[string]*PaperPosition, len(p.state.Positions))
	for key, pos := range p.state.Positions {
		if pos == nil {
			continue
		}
		pos.Symbol = strings.ToUpper(strings.TrimSpace(pos.Symbol))
		pos.Setup = normalizeSetup(pos.Setup)
		pos.Venue = normalizeVenue(pos.Venue)
		if pos.Leverage <= 0 {
			pos.Leverage = 1
		}
		if pos.MarginUsed <= 0 && pos.NotionalUSD > 0 {
			pos.MarginUsed = pos.NotionalUSD / float64(pos.Leverage)
		}
		normKey := paperPositionKey(pos.Symbol, pos.Setup, pos.Venue)
		if strings.TrimSpace(key) == "" {
			key = normKey
		}
		normalized[normKey] = pos
	}
	p.state.Positions = normalized
	normalizedMarks := make(map[string]float64, len(p.state.LastMarks))
	for symbol, px := range p.state.LastMarks {
		normalizedMarks[strings.ToUpper(strings.TrimSpace(symbol))] = px
	}
	p.state.LastMarks = normalizedMarks
	for i := range p.state.Trades {
		p.state.Trades[i].Symbol = strings.ToUpper(strings.TrimSpace(p.state.Trades[i].Symbol))
		p.state.Trades[i].Setup = normalizeSetup(p.state.Trades[i].Setup)
		p.state.Trades[i].Venue = normalizeVenue(p.state.Trades[i].Venue)
		if p.state.Trades[i].Leverage <= 0 {
			p.state.Trades[i].Leverage = 1
		}
		if p.state.Trades[i].MarginUsed <= 0 && p.state.Trades[i].NotionalUSD > 0 {
			p.state.Trades[i].MarginUsed = p.state.Trades[i].NotionalUSD / float64(p.state.Trades[i].Leverage)
		}
	}
}

func normalizeSetup(setup string) string {
	setup = strings.ToUpper(strings.TrimSpace(setup))
	if setup == "" {
		return "VWAP_HYBRID_CONFLUENCE"
	}
	return setup
}

func paperPositionKey(symbol, setup, venue string) string {
	return strings.ToUpper(strings.TrimSpace(symbol)) + "|" + normalizeSetup(setup) + "|" + normalizeVenue(venue)
}

func normalizeVenue(venue string) string {
	venue = strings.ToLower(strings.TrimSpace(venue))
	if venue == "" {
		return "paper"
	}
	return venue
}

func (p *PaperTrader) ensureVenueBalances() {
	start := p.cfg.StartBalance
	if start <= 0 {
		start = 100
	}
	for _, venue := range []string{"hyperliquid", "aster", "lighter"} {
		if _, ok := p.state.VenueBalances[venue]; !ok {
			p.state.VenueBalances[venue] = start
		}
	}
}

func (p *PaperTrader) recalculateBalance() {
	total := 0.0
	for _, v := range p.state.VenueBalances {
		total += v
	}
	for _, pos := range p.state.Positions {
		if pos == nil {
			continue
		}
		total += pos.MarginUsed
	}
	p.state.BalanceUSD = total
}
