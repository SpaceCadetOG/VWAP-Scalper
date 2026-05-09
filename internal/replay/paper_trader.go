package replay

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SpaceCadetOG/VWAP-Scalper/internal/router"
)

type PaperPosition struct {
	Symbol      string    `json:"symbol"`
	Venue       string    `json:"venue"`
	Side        string    `json:"side"`
	EntryPrice  float64   `json:"entry_price"`
	NotionalUSD float64   `json:"notional_usd"`
	Qty         float64   `json:"qty"`
	OpenedAt    time.Time `json:"opened_at"`
}

type PaperTrade struct {
	Symbol      string    `json:"symbol"`
	Venue       string    `json:"venue"`
	Side        string    `json:"side"`
	EntryPrice  float64   `json:"entry_price"`
	ExitPrice   float64   `json:"exit_price"`
	NotionalUSD float64   `json:"notional_usd"`
	Qty         float64   `json:"qty"`
	PnlUSD      float64   `json:"pnl_usd"`
	Reason      string    `json:"reason"`
	OpenedAt    time.Time `json:"opened_at"`
	ClosedAt    time.Time `json:"closed_at"`
}

type PaperState struct {
	BalanceUSD float64                   `json:"balance_usd"`
	Positions  map[string]*PaperPosition `json:"positions"`
	Trades     []PaperTrade              `json:"trades"`
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
		cfg.StartBalance = 1000
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
	if p.state.BalanceUSD <= 0 {
		p.state.BalanceUSD = cfg.StartBalance
	}
	if p.state.Positions == nil {
		p.state.Positions = map[string]*PaperPosition{}
	}
	return p
}

func (p *PaperTrader) State() PaperState { return p.state }

func (p *PaperTrader) OnSignal(intent router.Intent, venue string, price float64, now time.Time) error {
	sym := strings.ToUpper(strings.TrimSpace(intent.CanonicalPair))
	if sym == "" || price <= 0 {
		return fmt.Errorf("invalid signal/price")
	}
	if _, exists := p.state.Positions[sym]; exists {
		return nil
	}
	notional := intent.NotionalUSD
	if notional > p.state.BalanceUSD {
		notional = p.state.BalanceUSD
	}
	if notional <= 0 {
		return fmt.Errorf("no paper balance")
	}
	qty := notional / price
	pos := &PaperPosition{
		Symbol:      sym,
		Venue:       strings.ToLower(strings.TrimSpace(venue)),
		Side:        strings.ToLower(string(intent.Side)),
		EntryPrice:  price,
		NotionalUSD: notional,
		Qty:         qty,
		OpenedAt:    now,
	}
	p.state.Positions[sym] = pos
	return p.save()
}

func (p *PaperTrader) Mark(symbol string, price float64, now time.Time) *PaperTrade {
	sym := strings.ToUpper(strings.TrimSpace(symbol))
	pos := p.state.Positions[sym]
	if pos == nil || price <= 0 {
		return nil
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
		return nil
	}
	pnl := (price - pos.EntryPrice) * pos.Qty
	if pos.Side == "sell" {
		pnl = -pnl
	}
	p.state.BalanceUSD += pnl
	tr := PaperTrade{
		Symbol:      pos.Symbol,
		Venue:       pos.Venue,
		Side:        pos.Side,
		EntryPrice:  pos.EntryPrice,
		ExitPrice:   price,
		NotionalUSD: pos.NotionalUSD,
		Qty:         pos.Qty,
		PnlUSD:      pnl,
		Reason:      reason,
		OpenedAt:    pos.OpenedAt,
		ClosedAt:    now,
	}
	p.state.Trades = append(p.state.Trades, tr)
	delete(p.state.Positions, sym)
	_ = p.save()
	return &tr
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
