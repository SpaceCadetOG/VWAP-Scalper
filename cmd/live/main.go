package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"math/big"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	accountstream "github.com/SpaceCadetOG/VWAP-Scalper/internal/adapters/accountstream"
	aster "github.com/SpaceCadetOG/VWAP-Scalper/internal/adapters/aster"
	coinbase "github.com/SpaceCadetOG/VWAP-Scalper/internal/adapters/coinbase"
	hyperliquid "github.com/SpaceCadetOG/VWAP-Scalper/internal/adapters/hyperliquid"
	lighter "github.com/SpaceCadetOG/VWAP-Scalper/internal/adapters/lighter"
	ws "github.com/SpaceCadetOG/VWAP-Scalper/internal/adapters/ws"
	"github.com/SpaceCadetOG/VWAP-Scalper/internal/marketstate"
	"github.com/SpaceCadetOG/VWAP-Scalper/internal/models"
	"github.com/SpaceCadetOG/VWAP-Scalper/internal/observability"
	"github.com/SpaceCadetOG/VWAP-Scalper/internal/replay"
	"github.com/SpaceCadetOG/VWAP-Scalper/internal/router"
	"github.com/SpaceCadetOG/VWAP-Scalper/internal/strategycore"
	lgclient "github.com/elliottech/lighter-go/client"
	lhttp "github.com/elliottech/lighter-go/client/http"
	ltypes "github.com/elliottech/lighter-go/types"
	ltxtypes "github.com/elliottech/lighter-go/types/txtypes"
	hlsdk "github.com/mdn0420/go_hyperliquid"
)

func main() {
	checkBalances := flag.Bool("check-balances", false, "run venue balance connectivity checks")
	checkWS := flag.Bool("check-ws", false, "run websocket connectivity checks (WS-first health)")
	checkAccountStreams := flag.Bool("check-account-streams", false, "run account stream readiness/connectivity checks (Step 7)")
	paperRoute := flag.Bool("paper-route", false, "run Step 9 paper/replay routing simulation (no live orders)")
	paperE2E := flag.Bool("paper-e2e", false, "run strategycore+marketstate+router+paper end-to-end simulation")
	paperDaemon := flag.Bool("paper-daemon", false, "run continuous paper e2e loop (for separate tab/window)")
	start := flag.Bool("start", false, "simple start mode (uses env only)")
	testTrade := flag.Bool("test-trade", false, "place a small live test trade and collect order/fill details")
	testOrderTypes := flag.Bool("test-order-types", false, "place/cancel small Aster order-type smoke tests")
	testBatch := flag.Bool("test-batch", false, "run live batch-order smoke tests")
	venue := flag.String("venue", "all", "venue to check (all|aster|hyperliquid|lighter|coinbase)")
	symbol := flag.String("symbol", "ETHUSDT", "trade symbol for --test-trade")
	symbols := flag.String("symbols", "", "comma-separated symbols for --paper-daemon (overrides --symbol)")
	notionalUSD := flag.Float64("notional-usd", 5.0, "target notional USD for --test-trade")
	flag.Parse()

	if *start {
		syms := resolveSymbols(envString("BOT_SYMBOLS", "BTCUSDT"), "BTCUSDT")
		runPaperDaemon(syms, envFloat("BOT_NOTIONAL_USD", 10))
		return
	}

	if *testBatch {
		runBatchSmoke(strings.ToLower(strings.TrimSpace(*venue)), strings.ToUpper(strings.TrimSpace(*symbol)), *notionalUSD)
		return
	}

	if *testOrderTypes {
		runAsterOrderTypeSmoke(strings.ToLower(strings.TrimSpace(*venue)), strings.ToUpper(strings.TrimSpace(*symbol)), *notionalUSD)
		return
	}

	if *testTrade {
		runTestTrade(strings.ToLower(strings.TrimSpace(*venue)), strings.ToUpper(strings.TrimSpace(*symbol)), *notionalUSD)
		return
	}
	if *paperRoute {
		runPaperRoute(strings.ToUpper(strings.TrimSpace(*symbol)), *notionalUSD)
		return
	}
	if *paperE2E {
		runPaperE2E(strings.ToUpper(strings.TrimSpace(*symbol)), *notionalUSD)
		return
	}
	if *paperDaemon {
		runPaperDaemon(resolveSymbols(*symbols, *symbol), *notionalUSD)
		return
	}

	if *checkWS {
		runWSChecks(strings.ToLower(strings.TrimSpace(*venue)))
		return
	}
	if *checkAccountStreams {
		runAccountStreamChecks(strings.ToLower(strings.TrimSpace(*venue)))
		return
	}

	if !*checkBalances {
		fmt.Println("vwap-multi-venue-bot bootstrap")
		return
	}

	v := strings.ToLower(strings.TrimSpace(*venue))
	switch v {
	case "all":
		runHyperliquidBalanceCheck(false)
		runAsterBalanceCheck(false)
		runLighterBalanceCheck(false)
		runCoinbaseBalanceCheck(false)
	case "hyperliquid":
		runHyperliquidBalanceCheck(true)
	case "aster":
		runAsterBalanceCheck(true)
	case "lighter":
		runLighterBalanceCheck(true)
	case "coinbase":
		runCoinbaseBalanceCheck(true)
	default:
		fmt.Printf("unknown venue %q\n", *venue)
		os.Exit(2)
	}
}

func runPaperRoute(symbol string, notionalUSD float64) {
	fmt.Println("=== STEP 9 PAPER ROUTE ===")
	cfg := loadRouterConfigFromEnv()
	statuses := collectVenueStatusForPaper()
	intent := router.Intent{
		SignalID:      fmt.Sprintf("paper-%d", time.Now().UnixMilli()),
		Setup:         "VWAP_HYBRID_CONFLUENCE",
		CanonicalPair: symbol,
		Side:          router.SideBuy,
		NotionalUSD:   notionalUSD,
	}
	plan := router.BuildPlan(intent, statuses, cfg)
	if !plan.Accepted {
		fmt.Printf("paper_plan_rejected reason=%s detail=%s\n", plan.Reason, plan.ReasonText)
		for _, r := range plan.Rejected {
			fmt.Printf("venue_reject venue=%s reason=%s detail=%s\n", r.Venue, r.Reason, r.Detail)
		}
		os.Exit(1)
	}

	fmt.Printf("paper_plan_accepted signal_id=%s allocations=%d\n", plan.Intent.SignalID, len(plan.Allocations))
	for _, a := range plan.Allocations {
		fmt.Printf("alloc venue=%s weight=%.4f notional_usd=%.4f\n", a.Venue, a.Weight, a.NotionalUSD)
	}
	for _, r := range plan.Rejected {
		fmt.Printf("venue_reject venue=%s reason=%s detail=%s\n", r.Venue, r.Reason, r.Detail)
	}

	engine := replay.NewEngine(replay.FillModel{
		SlippageBps: envFloat("PAPER_SLIPPAGE_BPS", 2.0),
		FeeBps:      envFloat("PAPER_FEE_BPS", 3.5),
		LatencyMs:   int64(envInt("PAPER_LATENCY_MS", 250)),
	})
	res := engine.ExecutePlan(plan)
	fmt.Printf("paper_exec accepted=%t total_notional_usd=%.4f total_net_cost=%.4f\n", res.Accepted, res.TotalNotional, res.TotalNetCost)
	for _, ex := range res.Executions {
		fmt.Printf("paper_fill venue=%s state=%s notional_usd=%.4f fee_cost=%.4f slippage_cost=%.4f latency_ms=%d\n",
			ex.Venue, ex.OrderState, ex.NotionalUSD, ex.FeeCost, ex.SlippageCost, ex.LatencyMs)
	}
}

func runPaperE2E(symbol string, notionalUSD float64) {
	if _, _, err := runPaperE2EOnce(symbol, notionalUSD); err != nil {
		fmt.Printf("paper_e2e_failed err=%v\n", err)
		os.Exit(1)
	}
}

func runPaperDaemon(symbols []string, notionalUSD float64) {
	intervalSec := envInt("PAPER_DAEMON_INTERVAL_SEC", 30)
	if intervalSec < 5 {
		intervalSec = 5
	}
	liveVenue := strings.ToLower(strings.TrimSpace(envString("PAPER_PROMOTE_LIVE_VENUE", "hyperliquid")))
	fmt.Printf("CONFIG symbols=%s notional=%.4f interval_sec=%d mode=paper live_venue=%s\n", strings.Join(symbols, ","), notionalUSD, intervalSec, liveVenue)
	fmt.Println("CONFIG controls: promote_next_live='y' + Enter")
	autoPromote := envBool("PAPER_AUTO_PROMOTE_LIVE", false)
	if autoPromote {
		fmt.Println("CONFIG auto_promote_live=true")
	}
	var promoteRequested int32
	go func() {
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			line := strings.TrimSpace(strings.ToLower(sc.Text()))
			if line == "y" {
				atomic.StoreInt32(&promoteRequested, 1)
				fmt.Println("ACTION promote_live_armed=true")
			}
		}
	}()
	ticker := time.NewTicker(time.Duration(intervalSec) * time.Second)
	defer ticker.Stop()
	for {
		for _, symbol := range symbols {
			intent, promoVenues, err := runPaperE2EOnce(symbol, notionalUSD)
			if err != nil {
				fmt.Printf("ACTION cycle_error symbol=%s live_venue=%s err=%v\n", symbol, liveVenue, err)
				continue
			}
			if autoPromote || atomic.LoadInt32(&promoteRequested) == 1 {
				atomic.StoreInt32(&promoteRequested, 0)
				fmt.Printf("ACTION promote_live signal_id=%s side=%s pair=%s live_venues=%s\n", intent.SignalID, intent.Side, intent.CanonicalPair, strings.Join(promoVenues, ","))
				runLivePromotion(intent, symbol, promoVenues)
			}
		}
		<-ticker.C
	}
}

func runPaperE2EOnce(symbol string, notionalUSD float64) (router.Intent, []string, error) {
	now := time.Now().UTC()
	liveVenue := strings.ToLower(strings.TrimSpace(envString("PAPER_PROMOTE_LIVE_VENUE", "hyperliquid")))
	fmt.Printf("\nCYCLE ts=%s symbol=%s live_venue=%s\n", now.Format(time.RFC3339), symbol, liveVenue)
	notifier := observability.NewNotifierFromEnv()
	if !envBool("SIM_USE_LIVE_SNAPSHOT", true) {
		return router.Intent{}, nil, fmt.Errorf("SIM_USE_LIVE_SNAPSHOT must be true (no placeholder mode)")
	}
	snap, err := marketstate.BuildLiveSnapshot(marketstate.LiveSnapshotConfig{
		HyperliquidBaseURL: envString("HYPERLIQUID_BASE_URL", "https://api.hyperliquid.xyz"),
		AsterBaseURL:       envString("ASTER_BASE_URL", "https://fapi.asterdex.com"),
		Timeout:            5 * time.Second,
	}, symbol)
	if err != nil {
		return router.Intent{}, nil, fmt.Errorf("live snapshot required: %w", err)
	}
	snap.EMA9 = envFloat("SIM_EMA9", snap.Price*1.0002)
	snap.EMA20 = envFloat("SIM_EMA20", snap.Price*0.9998)
	snap.HTFAligned = envBool("SIM_HTF_ALIGNED", true)
	snap.ProfileReady = envBool("SIM_PROFILE_READY", true)
	snap.TapeReady = envBool("SIM_TAPE_READY", true)
	fmt.Printf("ACCOUNT mark=%.2f vwap=%.2f avwap=%.2f\n", snap.Price, snap.SessionVWAP, snap.AnchoredVWAP)

	paper := replay.NewPaperTrader(replay.TraderConfig{
		StateFile:      envString("PAPER_STATE_FILE", "out/paper_state.json"),
		StartBalance:   envFloat("PAPER_START_BALANCE", 1000),
		StopPct:        envFloat("PAPER_STOP_PCT", 0.006),
		TakeProfitPct:  envFloat("PAPER_TP_PCT", 0.009),
		MaxHoldSeconds: envInt("PAPER_MAX_HOLD_SEC", 180),
	})
	if tr := paper.Mark(symbol, snap.Price, time.Now().UTC()); tr != nil {
		pct := 0.0
		if tr.NotionalUSD > 0 {
			pct = (tr.PnlUSD / tr.NotionalUSD) * 100.0
		}
		fmt.Printf("ACTION exit symbol=%s venue=%s side=%s reason=%s pnl=%s balance=%.4f\n",
			tr.Symbol, tr.Venue, colorSide(tr.Side), tr.Reason, colorPNL(tr.PnlUSD, pct), paper.State().BalanceUSD)
		notifyBestEffort(notifier, "paper_exit", fmt.Sprintf("symbol=%s reason=%s pnl=%.4f balance=%.4f", tr.Symbol, tr.Reason, tr.PnlUSD, paper.State().BalanceUSD))
	}

	detector := marketstate.NewDetector()
	state := detector.Detect(snap)
	fmt.Printf("PAPER state=%s conf=%d expiry_ms=%d\n", state.State, state.ConfidenceScore, state.ExpiryMs)
	notifyBestEffort(notifier, "market_state", fmt.Sprintf("symbol=%s state=%s confidence=%d", symbol, state.State, state.ConfidenceScore))

	comp := strategycore.NewCompiler(envInt("STRATEGY_MIN_CONFIDENCE_PAPER", 70))
	intent, err := comp.Compile(strategycore.CompileInput{
		SignalID:      fmt.Sprintf("e2e-%d", time.Now().UnixMilli()),
		CanonicalPair: symbol,
		SetupName:     "VWAP_HYBRID_CONFLUENCE",
		State:         state,
		NotionalUSD:   notionalUSD,
		Delta:         snap.Delta,
	})
	if err != nil {
		fmt.Printf("ACTION strategy_reject err=%v\n", err)
		notifyBestEffort(notifier, "strategy_reject", fmt.Sprintf("symbol=%s err=%v", symbol, err))
		return router.Intent{}, nil, err
	}
	fmt.Printf("PAPER signal id=%s setup=%s side=%s pair=%s notional=%.4f\n",
		intent.SignalID, intent.Setup, colorSide(string(intent.Side)), intent.CanonicalPair, intent.NotionalUSD)
	notifyBestEffort(notifier, "strategy_intent", fmt.Sprintf("signal_id=%s side=%s pair=%s notional=%.4f", intent.SignalID, intent.Side, intent.CanonicalPair, intent.NotionalUSD))

	cfg := loadRouterConfigFromEnv()
	statuses := collectVenueStatusForPaper()
	plan := router.BuildPlan(intent, statuses, cfg)
	if !plan.Accepted {
		fmt.Printf("ACTION route_reject reason=%s detail=%s\n", plan.Reason, plan.ReasonText)
		notifyBestEffort(notifier, "router_reject", fmt.Sprintf("reason=%s detail=%s", plan.Reason, plan.ReasonText))
		for _, r := range plan.Rejected {
			fmt.Printf("venue_reject venue=%s reason=%s detail=%s\n", r.Venue, r.Reason, r.Detail)
		}
		return intent, nil, fmt.Errorf("router rejected: %s", plan.Reason)
	}

	fmt.Printf("PAPER route allocations=%d\n", len(plan.Allocations))
	notifyBestEffort(notifier, "router_plan", fmt.Sprintf("accepted allocations=%d", len(plan.Allocations)))
	promoVenues := make([]string, 0, len(plan.Allocations))
	for _, a := range plan.Allocations {
		fmt.Printf("PAPER route_preview venue=%s weight=%.3f notional=%.4f\n", a.Venue, a.Weight, a.NotionalUSD)
		promoVenues = append(promoVenues, string(a.Venue))
	}

	engine := replay.NewEngine(replay.FillModel{
		SlippageBps: envFloat("PAPER_SLIPPAGE_BPS", 2.0),
		FeeBps:      envFloat("PAPER_FEE_BPS", 3.5),
		LatencyMs:   int64(envInt("PAPER_LATENCY_MS", 250)),
	})
	res := engine.ExecutePlan(plan)
	fmt.Printf("PAPER exec accepted=%t total_notional=%.4f est_cost=%.4f\n", res.Accepted, res.TotalNotional, res.TotalNetCost)
	notifyBestEffort(notifier, "paper_exec", fmt.Sprintf("accepted=%t total_notional=%.4f total_net_cost=%.4f", res.Accepted, res.TotalNotional, res.TotalNetCost))
	_ = res
	if err := paper.OnSignal(intent, liveVenue, snap.Price, time.Now().UTC()); err != nil {
		fmt.Printf("ACTION entry_skip err=%v\n", err)
	} else {
		st := paper.State()
		venue := liveVenue
		if pos := st.Positions[strings.ToUpper(strings.TrimSpace(symbol))]; pos != nil && strings.TrimSpace(pos.Venue) != "" {
			venue = pos.Venue
		}
		fmt.Printf("ACTION entry symbol=%s venue=%s side=%s px=%.2f open=%d balance=%.4f trades=%d\n", symbol, venue, colorSide(strings.ToLower(string(intent.Side))), snap.Price, len(st.Positions), st.BalanceUSD, len(st.Trades))
		notifyBestEffort(notifier, "paper_entry", fmt.Sprintf("symbol=%s side=%s price=%.4f open_positions=%d balance=%.4f", symbol, strings.ToLower(string(intent.Side)), snap.Price, len(st.Positions), st.BalanceUSD))
	}
	printPaperStatus(paper, symbol, snap.Price)
	return intent, promoVenues, nil
}

func printPaperStatus(paper *replay.PaperTrader, symbol string, mark float64) {
	st := paper.State()
	realized := st.BalanceUSD - envFloat("PAPER_START_BALANCE", 1000)
	realizedPct := 0.0
	start := envFloat("PAPER_START_BALANCE", 1000)
	if start > 0 {
		realizedPct = (realized / start) * 100.0
	}
	unreal := 0.0
	unrealPct := 0.0
	if p := st.Positions[strings.ToUpper(strings.TrimSpace(symbol))]; p != nil && mark > 0 {
		unreal = (mark - p.EntryPrice) * p.Qty
		if strings.EqualFold(p.Side, "sell") {
			unreal = -unreal
		}
		unrealPct = pctOf(unreal, p.NotionalUSD)
		fmt.Printf("ACTIVE_POSITION symbol=%s venue=%s side=%s entry=%.2f mark=%.2f notional=%.2f upnl=%s\n",
			p.Symbol, p.Venue, colorSide(p.Side), p.EntryPrice, mark, p.NotionalUSD, colorPNL(unreal, unrealPct))
	}
	fmt.Printf("status open=%d realized=%s unrealized=%s balance=%.4f\n",
		len(st.Positions),
		colorPNL(realized, realizedPct),
		colorPNL(unreal, unrealPct),
		st.BalanceUSD,
	)
}

func resolveSymbols(rawSymbols, fallback string) []string {
	if strings.TrimSpace(rawSymbols) == "" {
		return []string{strings.ToUpper(strings.TrimSpace(fallback))}
	}
	parts := strings.Split(rawSymbols, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, p := range parts {
		s := strings.ToUpper(strings.TrimSpace(p))
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	if len(out) == 0 {
		return []string{strings.ToUpper(strings.TrimSpace(fallback))}
	}
	return out
}

func pctOf(v, base float64) float64 {
	if base == 0 {
		return 0
	}
	return (v / base) * 100.0
}

func colorSide(side string) string {
	s := strings.ToLower(strings.TrimSpace(side))
	if s == "buy" || s == "long" {
		return "\033[32m" + side + "\033[0m"
	}
	if s == "sell" || s == "short" {
		return "\033[31m" + side + "\033[0m"
	}
	return side
}

func colorPNL(pnl, pct float64) string {
	val := fmt.Sprintf("%.4f (%.2f%%)", pnl, pct)
	if pnl > 0 {
		return "\033[32m+" + val + "\033[0m"
	}
	if pnl < 0 {
		return "\033[31m" + val + "\033[0m"
	}
	return val
}

func runLivePromotion(intent router.Intent, symbol string, promoVenues []string) {
	liveNotional := envFloat("PAPER_PROMOTE_LIVE_NOTIONAL_USD", intent.NotionalUSD)
	venues := promoVenues
	if len(venues) == 0 {
		venues = []string{strings.ToLower(strings.TrimSpace(envString("PAPER_PROMOTE_LIVE_VENUE", "hyperliquid")))}
	}
	for _, venue := range venues {
		v := strings.ToLower(strings.TrimSpace(venue))
		fmt.Printf("ACTION live_promotion_execute venue=%s symbol=%s notional_usd=%.4f\n", v, symbol, liveNotional)
		switch v {
		case "hyperliquid", "aster":
			runTestTrade(v, symbol, liveNotional)
		default:
			fmt.Printf("ACTION live_promotion_skip venue=%s reason=live_path_not_enabled\n", v)
		}
	}
}

func notifyBestEffort(n *observability.Notifier, event, msg string) {
	if n == nil || !n.Enabled() {
		return
	}
	if err := n.Send(event, msg); err != nil {
		fmt.Printf("alert_send_failed event=%s err=%v\n", event, err)
	}
}

func loadRouterConfigFromEnv() router.Config {
	cfg := router.DefaultConfig()
	cfg.MultiVenueEnable = envBool("ROUTER_MULTI_VENUE_ENABLE", cfg.MultiVenueEnable)
	cfg.MaxVenuesPerSignal = envInt("ROUTER_MAX_VENUES_PER_SIGNAL", cfg.MaxVenuesPerSignal)
	cfg.GlobalRiskPerSignalUSD = envFloat("ROUTER_GLOBAL_RISK_PER_SIGNAL_USD", cfg.GlobalRiskPerSignalUSD)
	cfg.VenueRiskSplitMode = envString("ROUTER_VENUE_RISK_SPLIT_MODE", cfg.VenueRiskSplitMode)
	cfg.RequireIsolated = envBool("ROUTER_REQUIRE_ISOLATED", cfg.RequireIsolated)
	return cfg
}

func collectVenueStatusForPaper() []router.VenueStatus {
	type v struct {
		name  string
		venue models.Venue
	}
	all := []v{
		{name: "hyperliquid", venue: models.VenueHyperliquid},
		{name: "aster", venue: models.VenueAster},
		{name: "lighter", venue: models.VenueLighter},
	}
	out := make([]router.VenueStatus, 0, len(all))
	for _, it := range all {
		rd := accountstream.ProbeReadiness(it.name)
		wsr := accountstream.ProbeConnectivity(it.name)
		s := router.VenueStatus{
			Venue:                 it.venue,
			Healthy:               rd.Ready && wsr.OK,
			SupportsPerpExecution: true,
			IsolatedConfirmed:     venueIsolatedConfirmed(it.name),
			Score:                 venueScore(it.name),
		}
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

func venueIsolatedConfirmed(venue string) bool {
	switch venue {
	case "lighter":
		return strings.EqualFold(strings.TrimSpace(os.Getenv("LIGHTER_MARGIN_MODE")), "isolated") &&
			strings.EqualFold(strings.TrimSpace(os.Getenv("LIGHTER_ISOLATED_CONFIRMED")), "true")
	case "hyperliquid":
		return strings.TrimSpace(os.Getenv("HYPERLIQUID_ACCOUNT_ADDRESS")) != "" &&
			strings.TrimSpace(os.Getenv("HYPERLIQUID_API_WALLET_PRIVATE_KEY")) != ""
	case "aster":
		return strings.TrimSpace(os.Getenv("ASTER_USER")) != "" &&
			(strings.TrimSpace(os.Getenv("ASTER_SIGNER")) != "" || strings.TrimSpace(os.Getenv("ASTER_SIGNER_ADDRESS")) != "") &&
			(strings.TrimSpace(os.Getenv("ASTER_PRIVATE_KEY")) != "" || strings.TrimSpace(os.Getenv("ASTER_SIGNER_PRIVATE_KEY")) != "")
	default:
		return false
	}
}

func venueScore(venue string) float64 {
	switch venue {
	case "hyperliquid":
		return envFloat("ROUTER_SCORE_HYPERLIQUID", 1.0)
	case "aster":
		return envFloat("ROUTER_SCORE_ASTER", 1.0)
	case "lighter":
		return envFloat("ROUTER_SCORE_LIGHTER", 1.0)
	default:
		return 1.0
	}
}

func envString(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v
}

func envInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func envFloat(key string, def float64) float64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return n
}

func envBool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return strings.EqualFold(v, "true") || v == "1" || strings.EqualFold(v, "yes")
}

func runTestTrade(venue, symbol string, notionalUSD float64) {
	switch venue {
	case "aster":
		runAsterTestTrade(symbol, notionalUSD)
	case "hyperliquid":
		runHyperliquidTestTrade(symbol, notionalUSD)
	default:
		fmt.Printf("test-trade currently implemented for venue=aster|hyperliquid only; requested=%s\n", venue)
		os.Exit(2)
	}
}

func runBatchSmoke(venue, symbol string, notionalUSD float64) {
	switch venue {
	case "aster":
		runAsterBatchSmoke(symbol, notionalUSD)
	case "lighter":
		runLighterBatchSmoke(symbol, notionalUSD)
	case "hyperliquid":
		runHyperliquidBatchSmoke(symbol, notionalUSD)
	default:
		fmt.Printf("test-batch currently implemented for venue=aster|lighter|hyperliquid only; requested=%s\n", venue)
		os.Exit(2)
	}
}

func runHyperliquidTestTrade(symbol string, notionalUSD float64) {
	fmt.Println("=== HYPERLIQUID TEST TRADE ===")
	account := strings.ToLower(strings.TrimSpace(os.Getenv("HYPERLIQUID_ACCOUNT_ADDRESS")))
	privateKey := strings.TrimSpace(os.Getenv("HYPERLIQUID_API_WALLET_PRIVATE_KEY"))
	if account == "" || privateKey == "" {
		fmt.Println("HYPERLIQUID_ACCOUNT_ADDRESS and HYPERLIQUID_API_WALLET_PRIVATE_KEY are required")
		os.Exit(1)
	}
	coin := normalizeHyperliquidCoin(symbol)
	hl := hlsdk.NewHyperliquid(&hlsdk.HyperliquidClientConfig{
		IsMainnet:      true,
		AccountAddress: account,
		PrivateKey:     privateKey,
	})

	if err := ensureHyperliquidIsolated(hl, account, coin); err != nil {
		fmt.Printf("isolated_preflight_failed: %v\n", err)
		os.Exit(1)
	}

	px, err := hl.InfoAPI.GetMartketPx(coin)
	if err != nil || px <= 0 {
		fmt.Printf("market price failed coin=%s err=%v px=%.8f\n", coin, err, px)
		os.Exit(1)
	}
	effectiveNotional := notionalUSD
	if effectiveNotional < 10.5 {
		effectiveNotional = 10.5
	}
	sz := effectiveNotional / px
	if sz < 0.0001 {
		sz = 0.0001
	}
	sz = roundTo(sz, 6)
	fmt.Printf("coin=%s mark_price=%.6f target_notional=%.2f effective_notional=%.2f sz=%.6f\n", coin, px, notionalUSD, effectiveNotional, sz)

	openResp, err := hl.ExchangeAPI.MarketOrder(coin, sz, nil)
	if err != nil {
		fmt.Printf("open_market_order_failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("open_market_order_resp=%+v\n", openResp)

	time.Sleep(1500 * time.Millisecond)
	fills, err := hl.InfoAPI.GetAccountFills()
	if err == nil && fills != nil {
		fmt.Printf("recent_fills_count=%d\n", len(*fills))
		for i := 0; i < len(*fills) && i < 3; i++ {
			f := (*fills)[i]
			fmt.Printf("fill[%d] coin=%s side=%s sz=%.6f px=%.6f oid=%d fee=%.6f\n", i, f.Coin, f.Side, f.Sz, f.Px, f.Oid, f.Fee)
		}
	}

	closeResp, err := hl.ExchangeAPI.ClosePosition(coin)
	if err != nil {
		fmt.Printf("close_position_note: %v\n", err)
	} else {
		fmt.Printf("close_position_resp=%+v\n", closeResp)
	}

	time.Sleep(1200 * time.Millisecond)
	state, err := hl.InfoAPI.GetAccountState()
	if err == nil && state != nil {
		openPos := 0
		for _, ap := range state.AssetPositions {
			if math.Abs(ap.Position.Szi) > 0 {
				openPos++
			}
		}
		fmt.Printf("post_close account_value=%.6f total_ntl_pos=%.6f open_positions=%d\n", state.MarginSummary.AccountValue, state.MarginSummary.TotalNtlPos, openPos)
	}
	fmt.Println("HYPERLIQUID test trade complete")
}

func runHyperliquidBatchSmoke(symbol string, notionalUSD float64) {
	fmt.Println("=== HYPERLIQUID BATCH SMOKE ===")
	account := strings.ToLower(strings.TrimSpace(os.Getenv("HYPERLIQUID_ACCOUNT_ADDRESS")))
	privateKey := strings.TrimSpace(os.Getenv("HYPERLIQUID_API_WALLET_PRIVATE_KEY"))
	if account == "" || privateKey == "" {
		fmt.Println("HYPERLIQUID_ACCOUNT_ADDRESS and HYPERLIQUID_API_WALLET_PRIVATE_KEY are required")
		os.Exit(1)
	}
	coin := normalizeHyperliquidCoin(symbol)
	hl := hlsdk.NewHyperliquid(&hlsdk.HyperliquidClientConfig{
		IsMainnet:      true,
		AccountAddress: account,
		PrivateKey:     privateKey,
	})

	if err := ensureHyperliquidIsolated(hl, account, coin); err != nil {
		fmt.Printf("isolated_preflight_failed: %v\n", err)
		os.Exit(1)
	}

	px, err := hl.InfoAPI.GetMartketPx(coin)
	if err != nil || px <= 0 {
		fmt.Printf("market price failed coin=%s err=%v px=%.8f\n", coin, err, px)
		os.Exit(1)
	}
	effectiveNotional := notionalUSD
	if effectiveNotional < 10.5 {
		effectiveNotional = 10.5
	}
	sz := roundTo(math.Max(effectiveNotional/px, 0.0001), 6)
	buyPx := roundTo(px*0.96, 4)
	sellPx := roundTo(px*1.04, 4)
	orders := []hlsdk.OrderRequest{
		{
			Coin:    coin,
			IsBuy:   true,
			Sz:      sz,
			LimitPx: buyPx,
			OrderType: hlsdk.OrderType{
				Limit: &hlsdk.LimitOrderType{Tif: hlsdk.TifGtc},
			},
			ReduceOnly: false,
		},
		{
			Coin:    coin,
			IsBuy:   false,
			Sz:      sz,
			LimitPx: sellPx,
			OrderType: hlsdk.OrderType{
				Limit: &hlsdk.LimitOrderType{Tif: hlsdk.TifGtc},
			},
			ReduceOnly: false,
		},
	}
	fmt.Printf("batch coin=%s mid=%.6f target_notional=%.2f effective_notional=%.2f sz=%.6f buy_px=%.4f sell_px=%.4f\n", coin, px, notionalUSD, effectiveNotional, sz, buyPx, sellPx)
	batchResp, err := hl.ExchangeAPI.BulkOrders(orders, hlsdk.GroupingNa, false)
	if err != nil {
		fmt.Printf("batch_place_failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("batch_place_resp=%+v\n", batchResp)

	time.Sleep(1200 * time.Millisecond)
	openOrders, err := hl.InfoAPI.GetAccountOpenOrders()
	if err != nil || openOrders == nil {
		fmt.Printf("open_orders_read_failed: %v\n", err)
		os.Exit(1)
	}
	openForCoin := 0
	for _, o := range *openOrders {
		if strings.EqualFold(o.Coin, coin) {
			openForCoin++
		}
	}
	fmt.Printf("open_orders_for_%s=%d\n", coin, openForCoin)
	allResp, err := hl.ExchangeAPI.CancelAllOrdersByCoin(coin)
	if err != nil {
		fmt.Printf("cancel_all_by_coin_failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("cancel_all_by_coin_resp=%+v\n", allResp)
	fmt.Println("HYPERLIQUID batch smoke complete")
}

func normalizeHyperliquidCoin(symbol string) string {
	s := strings.ToUpper(strings.TrimSpace(symbol))
	s = strings.TrimSuffix(s, "-USDC")
	s = strings.TrimSuffix(s, "USDC")
	s = strings.TrimSuffix(s, "-USD")
	s = strings.TrimSuffix(s, "USD")
	s = strings.TrimSuffix(s, "-USDT")
	s = strings.TrimSuffix(s, "USDT")
	if s == "" {
		return "BTC"
	}
	return s
}

func ensureHyperliquidIsolated(hl *hlsdk.Hyperliquid, account, coin string) error {
	state, err := hl.InfoAPI.GetUserState(account)
	if err != nil {
		return fmt.Errorf("get user state: %w", err)
	}
	for _, ap := range state.AssetPositions {
		if !strings.EqualFold(ap.Position.Coin, coin) {
			continue
		}
		mode := strings.ToLower(strings.TrimSpace(ap.Position.Leverage.Type))
		if mode == "isolated" {
			fmt.Printf("margin_mode_check coin=%s mode=isolated action=none\n", coin)
			return nil
		}
		fmt.Printf("margin_mode_check coin=%s mode=%s action=switch_to_isolated\n", coin, mode)
	}
	lev := 3
	if len(state.AssetPositions) > 0 {
		for _, ap := range state.AssetPositions {
			if strings.EqualFold(ap.Position.Coin, coin) && ap.Position.Leverage.Value > 0 {
				lev = ap.Position.Leverage.Value
				break
			}
		}
	}
	resp, err := hl.ExchangeAPI.UpdateLeverage(coin, false, lev)
	if err != nil {
		return fmt.Errorf("switch isolated failed: %w", err)
	}
	fmt.Printf("margin_mode_switch_resp=%+v\n", resp)
	state2, err := hl.InfoAPI.GetUserState(account)
	if err != nil {
		return fmt.Errorf("isolated recheck failed: %w", err)
	}
	for _, ap := range state2.AssetPositions {
		if !strings.EqualFold(ap.Position.Coin, coin) {
			continue
		}
		mode := strings.ToLower(strings.TrimSpace(ap.Position.Leverage.Type))
		fmt.Printf("margin_mode_recheck coin=%s mode=%s\n", coin, mode)
		if mode == "isolated" {
			return nil
		}
		return fmt.Errorf("margin mode still not isolated (mode=%s)", mode)
	}
	return nil
}

func ensureLighterIsolatedStrict() error {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("LIGHTER_MARGIN_MODE")))
	confirmed := strings.EqualFold(strings.TrimSpace(os.Getenv("LIGHTER_ISOLATED_CONFIRMED")), "true")
	if mode == "isolated" && confirmed {
		fmt.Println("margin_mode_check venue=lighter mode=isolated action=none source=env_guard")
		return nil
	}
	if mode == "cross" {
		return fmt.Errorf("lighter guard blocked: LIGHTER_MARGIN_MODE=cross is forbidden")
	}
	return fmt.Errorf("lighter guard blocked: set LIGHTER_MARGIN_MODE=isolated and LIGHTER_ISOLATED_CONFIRMED=true before trading")
}

func roundTo(v float64, decimals int) float64 {
	p := math.Pow10(decimals)
	return math.Round(v*p) / p
}

func hlNormalizePx(px float64, szDecimals int) float64 {
	// Match Hyperliquid reference normalization: 5 significant figures, then cap decimals.
	sigRounded, _ := strconv.ParseFloat(fmt.Sprintf("%.5g", px), 64)
	decimals := 6 - szDecimals
	if decimals < 0 {
		decimals = 0
	}
	return roundTo(sigRounded, decimals)
}

func hlFormatTriggerPx(px float64, szDecimals int) string {
	n := hlNormalizePx(px, szDecimals)
	return strconv.FormatFloat(n, 'f', -1, 64)
}

func sendHyperliquidRawAction(hl *hlsdk.Hyperliquid, action any) (map[string]any, error) {
	nonce := uint64(time.Now().UnixMilli())
	v, r, s, err := hl.ExchangeAPI.SignL1Action(action, nonce)
	if err != nil {
		return nil, fmt.Errorf("sign action: %w", err)
	}
	req := hlsdk.ExchangeRequest{
		Action:    action,
		Nonce:     nonce,
		Signature: hlsdk.ToTypedSig(r, s, v),
	}
	out, err := hlsdk.MakeUniversalRequest[map[string]any](hl.ExchangeAPI, req)
	if err != nil {
		return nil, err
	}
	return *out, nil
}

type hlTwapWire struct {
	A int    `msgpack:"a" json:"a"`
	B bool   `msgpack:"b" json:"b"`
	S string `msgpack:"s" json:"s"`
	R bool   `msgpack:"r" json:"r"`
	M int    `msgpack:"m" json:"m"`
	T bool   `msgpack:"t" json:"t"`
}

type hlTwapOrderAction struct {
	Type string     `msgpack:"type" json:"type"`
	Twap hlTwapWire `msgpack:"twap" json:"twap"`
}

type hlTwapCancelAction struct {
	Type string `msgpack:"type" json:"type"`
	A    int    `msgpack:"a" json:"a"`
	T    int64  `msgpack:"t" json:"t"`
}

func extractTwapID(resp map[string]any) int64 {
	response, ok := resp["response"].(map[string]any)
	if !ok {
		return 0
	}
	data, ok := response["data"].(map[string]any)
	if !ok {
		return 0
	}
	status, ok := data["status"].(map[string]any)
	if !ok {
		return 0
	}
	running, ok := status["running"].(map[string]any)
	if !ok {
		return 0
	}
	switch x := running["twapId"].(type) {
	case float64:
		return int64(x)
	case int64:
		return x
	case json.Number:
		v, _ := x.Int64()
		return v
	default:
		return 0
	}
}

func runAsterTestTrade(symbol string, notionalUSD float64) {
	fmt.Println("=== ASTER TEST TRADE ===")
	chainID := int64(1666)
	if raw := strings.TrimSpace(os.Getenv("ASTER_CHAIN_ID")); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil {
			chainID = parsed
		}
	}
	signer := strings.TrimSpace(os.Getenv("ASTER_SIGNER"))
	if signer == "" {
		signer = strings.TrimSpace(os.Getenv("ASTER_SIGNER_ADDRESS"))
	}
	priv := strings.TrimSpace(os.Getenv("ASTER_PRIVATE_KEY"))
	if priv == "" {
		priv = strings.TrimSpace(os.Getenv("ASTER_SIGNER_PRIVATE_KEY"))
	}
	cli, err := aster.NewClient(aster.Config{
		BaseURL:    strings.TrimSpace(os.Getenv("ASTER_BASE_URL")),
		User:       strings.TrimSpace(os.Getenv("ASTER_USER")),
		Signer:     signer,
		PrivateKey: priv,
		ChainID:    chainID,
	})
	if err != nil {
		fmt.Printf("ASTER init failed: %v\n", err)
		os.Exit(1)
	}
	if err := ensureAsterIsolated(cli, symbol); err != nil {
		fmt.Printf("isolated_preflight_failed: %v\n", err)
		os.Exit(1)
	}

	price, err := cli.MarkPrice(symbol)
	if err != nil {
		fmt.Printf("mark price failed: %v\n", err)
		os.Exit(1)
	}
	if price <= 0 {
		fmt.Println("invalid mark price")
		os.Exit(1)
	}
	rawQty := notionalUSD / price
	// Aster enforces symbol step/precision; round UP to 0.001 step so notional stays >= target.
	stepped := math.Ceil(rawQty*1000.0) / 1000.0
	qty := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", stepped), "0"), ".")
	if qty == "" || qty == "0" {
		qty = "0.001"
	}
	fmt.Printf("symbol=%s mark_price=%.6f target_notional=%.2f qty=%s\n", symbol, price, notionalUSD, qty)

	openVals := map[string]string{
		"symbol":           symbol,
		"side":             "BUY",
		"type":             "MARKET",
		"positionSide":     "BOTH",
		"quantity":         qty,
		"newOrderRespType": "RESULT",
	}
	openQ := makeURLValues(openVals)
	openResp, err := cli.PlaceOrder(openQ)
	if err != nil {
		fmt.Printf("open order failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("open_order_resp=%v\n", openResp)

	orderID := int64FromAny(openResp["orderId"])
	executedQty := strings.TrimSpace(fmt.Sprint(openResp["executedQty"]))
	if executedQty == "" || executedQty == "0" || executedQty == "0.0" {
		fmt.Println("open executedQty is zero; stopping test flow")
		os.Exit(1)
	}

	if orderID > 0 {
		detail, err := cli.GetOrder(symbol, orderID)
		if err == nil {
			fmt.Printf("open_order_detail=%v\n", detail)
		} else {
			fmt.Printf("open_order_detail_error=%v\n", err)
		}
	}

	posBeforeClose, _ := cli.PositionRisk(symbol)
	fmt.Printf("position_risk_after_open=%v\n", posBeforeClose)

	closeVals := map[string]string{
		"symbol":           symbol,
		"side":             "SELL",
		"type":             "MARKET",
		"positionSide":     "BOTH",
		"reduceOnly":       "true",
		"quantity":         executedQty,
		"newOrderRespType": "RESULT",
	}
	closeResp, err := cli.PlaceOrder(makeURLValues(closeVals))
	if err != nil {
		fmt.Printf("close order failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("close_order_resp=%v\n", closeResp)

	closeOrderID := int64FromAny(closeResp["orderId"])
	if closeOrderID > 0 {
		detail, err := cli.GetOrder(symbol, closeOrderID)
		if err == nil {
			fmt.Printf("close_order_detail=%v\n", detail)
		} else {
			fmt.Printf("close_order_detail_error=%v\n", err)
		}
	}

	posAfterClose, _ := cli.PositionRisk(symbol)
	fmt.Printf("position_risk_after_close=%v\n", posAfterClose)

	// Cancel-order test: create a far-away LIMIT order then cancel it.
	limitPrice := price * 0.95
	limitQtyRaw := notionalUSD / limitPrice
	limitQty := math.Ceil(limitQtyRaw*1000.0) / 1000.0
	limitVals := map[string]string{
		"symbol":           symbol,
		"side":             "BUY",
		"type":             "LIMIT",
		"timeInForce":      "GTC",
		"positionSide":     "BOTH",
		"quantity":         strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", limitQty), "0"), "."),
		"price":            fmt.Sprintf("%.2f", limitPrice),
		"newOrderRespType": "RESULT",
	}
	limitResp, err := cli.PlaceOrder(makeURLValues(limitVals))
	if err != nil {
		fmt.Printf("limit order (cancel test) failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("limit_order_resp=%v\n", limitResp)
	limitOrderID := int64FromAny(limitResp["orderId"])
	if limitOrderID > 0 {
		cancelResp, err := cli.CancelOrder(symbol, limitOrderID)
		if err != nil {
			fmt.Printf("cancel_order_error=%v\n", err)
			os.Exit(1)
		}
		fmt.Printf("cancel_order_resp=%v\n", cancelResp)
		cancelDetail, err := cli.GetOrder(symbol, limitOrderID)
		if err != nil {
			fmt.Printf("cancel_order_detail_error=%v\n", err)
		} else {
			fmt.Printf("cancel_order_detail=%v\n", cancelDetail)
		}
	}
	fmt.Println("ASTER test trade complete")
}

func runAsterBatchSmoke(symbol string, notionalUSD float64) {
	fmt.Println("=== ASTER BATCH SMOKE ===")
	chainID := int64(1666)
	if raw := strings.TrimSpace(os.Getenv("ASTER_CHAIN_ID")); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil {
			chainID = parsed
		}
	}
	signer := strings.TrimSpace(os.Getenv("ASTER_SIGNER"))
	if signer == "" {
		signer = strings.TrimSpace(os.Getenv("ASTER_SIGNER_ADDRESS"))
	}
	priv := strings.TrimSpace(os.Getenv("ASTER_PRIVATE_KEY"))
	if priv == "" {
		priv = strings.TrimSpace(os.Getenv("ASTER_SIGNER_PRIVATE_KEY"))
	}
	cli, err := aster.NewClient(aster.Config{
		BaseURL:    strings.TrimSpace(os.Getenv("ASTER_BASE_URL")),
		User:       strings.TrimSpace(os.Getenv("ASTER_USER")),
		Signer:     signer,
		PrivateKey: priv,
		ChainID:    chainID,
	})
	if err != nil {
		fmt.Printf("ASTER init failed: %v\n", err)
		os.Exit(1)
	}
	if err := ensureAsterIsolated(cli, symbol); err != nil {
		fmt.Printf("isolated_preflight_failed: %v\n", err)
		os.Exit(1)
	}
	price, err := cli.MarkPrice(symbol)
	if err != nil || price <= 0 {
		fmt.Printf("mark price failed: %v\n", err)
		os.Exit(1)
	}
	formatPrice := func(p float64) string {
		if strings.EqualFold(symbol, "BTCUSDT") {
			return strconv.FormatFloat(p, 'f', 1, 64)
		}
		return strconv.FormatFloat(p, 'f', 2, 64)
	}
	qtyBase := math.Ceil((notionalUSD/price)*1000.0) / 1000.0
	if qtyBase < 0.001 {
		qtyBase = 0.001
	}
	qty := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", qtyBase), "0"), ".")
	if qty == "" {
		qty = "0.001"
	}
	orders := []url.Values{
		makeURLValues(map[string]string{
			"symbol":           symbol,
			"side":             "BUY",
			"type":             "LIMIT",
			"timeInForce":      "GTC",
			"positionSide":     "BOTH",
			"quantity":         qty,
			"price":            formatPrice(price * 0.95),
			"newOrderRespType": "RESULT",
		}),
		makeURLValues(map[string]string{
			"symbol":           symbol,
			"side":             "BUY",
			"type":             "LIMIT",
			"timeInForce":      "GTC",
			"positionSide":     "BOTH",
			"quantity":         qty,
			"price":            formatPrice(price * 0.94),
			"newOrderRespType": "RESULT",
		}),
	}
	resp, err := cli.PlaceBatchOrders(orders)
	if err != nil {
		fmt.Printf("aster batch place failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("aster_batch_resp=%v\n", resp)
	ids := make([]int64, 0, len(resp))
	for _, r := range resp {
		id := int64FromAny(r["orderId"])
		if id > 0 {
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		c, err := cli.CancelOrder(symbol, id)
		if err != nil {
			fmt.Printf("aster_batch_cancel orderId=%d err=%v\n", id, err)
			continue
		}
		fmt.Printf("aster_batch_cancel orderId=%d resp=%v\n", id, c)
	}
	if allResp, err := cli.CancelAllOpenOrders(symbol); err == nil {
		fmt.Printf("aster_cancel_all_resp=%v\n", allResp)
	} else {
		fmt.Printf("aster_cancel_all_err=%v\n", err)
	}
	pos, _ := cli.PositionRisk(symbol)
	fmt.Printf("aster_batch_position_risk=%v\n", pos)
	fmt.Println("ASTER batch smoke complete")
}

func runAsterOrderTypeSmoke(venue, symbol string, notionalUSD float64) {
	if venue != "aster" {
		if venue == "lighter" {
			runLighterOrderTypeSmoke(symbol, notionalUSD)
			return
		}
		if venue == "hyperliquid" {
			runHyperliquidOrderTypeSmoke(symbol, notionalUSD)
			return
		}
		fmt.Printf("test-order-types currently implemented for venue=aster|lighter|hyperliquid only; requested=%s\n", venue)
		os.Exit(2)
	}
	fmt.Println("=== ASTER ORDER TYPE SMOKE ===")
	chainID := int64(1666)
	if raw := strings.TrimSpace(os.Getenv("ASTER_CHAIN_ID")); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil {
			chainID = parsed
		}
	}
	signer := strings.TrimSpace(os.Getenv("ASTER_SIGNER"))
	if signer == "" {
		signer = strings.TrimSpace(os.Getenv("ASTER_SIGNER_ADDRESS"))
	}
	priv := strings.TrimSpace(os.Getenv("ASTER_PRIVATE_KEY"))
	if priv == "" {
		priv = strings.TrimSpace(os.Getenv("ASTER_SIGNER_PRIVATE_KEY"))
	}
	cli, err := aster.NewClient(aster.Config{
		BaseURL:    strings.TrimSpace(os.Getenv("ASTER_BASE_URL")),
		User:       strings.TrimSpace(os.Getenv("ASTER_USER")),
		Signer:     signer,
		PrivateKey: priv,
		ChainID:    chainID,
	})
	if err != nil {
		fmt.Printf("ASTER init failed: %v\n", err)
		os.Exit(1)
	}
	if err := ensureAsterIsolated(cli, symbol); err != nil {
		fmt.Printf("isolated_preflight_failed: %v\n", err)
		os.Exit(1)
	}
	price, err := cli.MarkPrice(symbol)
	if err != nil || price <= 0 {
		fmt.Printf("mark price failed: %v\n", err)
		os.Exit(1)
	}

	qtyRaw := notionalUSD / price
	qtyStep := math.Ceil(qtyRaw*1000.0) / 1000.0
	qty := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", qtyStep), "0"), ".")
	if qty == "" || qty == "0" {
		qty = "0.001"
	}
	limitAway := price * 0.95
	stopPrice := price * 1.15
	stopLimitPrice := price * 1.16
	// For trailing stop:
	// BUY requires activationPrice < latest, SELL requires activationPrice > latest.
	trailingActivationSell := price * 1.20
	formatPrice := func(p float64) string {
		upper := strings.ToUpper(strings.TrimSpace(symbol))
		decimals := 2
		switch upper {
		case "BTCUSDT":
			decimals = 1
		}
		return strconv.FormatFloat(p, 'f', decimals, 64)
	}

	type smokeCase struct {
		name   string
		params map[string]string
	}
	cases := []smokeCase{
		{
			name: "limit_gtc",
			params: map[string]string{
				"symbol":           symbol,
				"side":             "BUY",
				"type":             "LIMIT",
				"timeInForce":      "GTC",
				"positionSide":     "BOTH",
				"quantity":         qty,
				"price":            formatPrice(limitAway),
				"newOrderRespType": "RESULT",
			},
		},
		{
			name: "stop_limit",
			params: map[string]string{
				"symbol":           symbol,
				"side":             "BUY",
				"type":             "STOP",
				"timeInForce":      "GTC",
				"positionSide":     "BOTH",
				"quantity":         qty,
				"price":            formatPrice(stopLimitPrice),
				"stopPrice":        formatPrice(stopPrice),
				"workingType":      "MARK_PRICE",
				"newOrderRespType": "RESULT",
			},
		},
		{
			name: "stop_market",
			params: map[string]string{
				"symbol":           symbol,
				"side":             "BUY",
				"type":             "STOP_MARKET",
				"positionSide":     "BOTH",
				"quantity":         qty,
				"stopPrice":        formatPrice(stopPrice),
				"workingType":      "MARK_PRICE",
				"newOrderRespType": "RESULT",
			},
		},
		{
			name: "trailing_stop_market",
			params: map[string]string{
				"symbol":           symbol,
				"side":             "SELL",
				"type":             "TRAILING_STOP_MARKET",
				"positionSide":     "BOTH",
				"quantity":         qty,
				"callbackRate":     "0.3",
				"activationPrice":  formatPrice(trailingActivationSell),
				"workingType":      "MARK_PRICE",
				"newOrderRespType": "RESULT",
			},
		},
		{
			name: "post_only_gtx",
			params: map[string]string{
				"symbol":           symbol,
				"side":             "BUY",
				"type":             "LIMIT",
				"timeInForce":      "GTX",
				"positionSide":     "BOTH",
				"quantity":         qty,
				"price":            formatPrice(limitAway),
				"newOrderRespType": "RESULT",
			},
		},
		{
			name: "twap_probe",
			params: map[string]string{
				"symbol":           symbol,
				"side":             "BUY",
				"type":             "TWAP",
				"positionSide":     "BOTH",
				"quantity":         qty,
				"price":            formatPrice(limitAway),
				"newOrderRespType": "RESULT",
			},
		},
		{
			name: "scaled_probe",
			params: map[string]string{
				"symbol":           symbol,
				"side":             "BUY",
				"type":             "SCALED",
				"positionSide":     "BOTH",
				"quantity":         qty,
				"price":            formatPrice(limitAway),
				"newOrderRespType": "RESULT",
			},
		},
	}

	created := make([]int64, 0, len(cases))
	for _, tc := range cases {
		resp, err := cli.PlaceOrder(makeURLValues(tc.params))
		if err != nil {
			fmt.Printf("order_type=%s result=ERROR err=%v\n", tc.name, err)
			continue
		}
		id := int64FromAny(resp["orderId"])
		status := strings.TrimSpace(fmt.Sprint(resp["status"]))
		fmt.Printf("order_type=%s result=OK orderId=%d status=%s resp=%v\n", tc.name, id, status, resp)
		if id > 0 && status != "FILLED" && status != "CANCELED" && status != "EXPIRED" {
			created = append(created, id)
		}
	}

	for _, orderID := range created {
		cancelResp, err := cli.CancelOrder(symbol, orderID)
		if err != nil {
			fmt.Printf("cleanup_cancel orderId=%d result=ERROR err=%v\n", orderID, err)
			continue
		}
		fmt.Printf("cleanup_cancel orderId=%d result=OK resp=%v\n", orderID, cancelResp)
		detail, err := cli.GetOrder(symbol, orderID)
		if err != nil {
			fmt.Printf("cleanup_detail orderId=%d result=ERROR err=%v\n", orderID, err)
			continue
		}
		fmt.Printf("cleanup_detail orderId=%d result=OK detail=%v\n", orderID, detail)
	}

	pos, err := cli.PositionRisk(symbol)
	if err != nil {
		fmt.Printf("position_risk_error=%v\n", err)
	} else {
		fmt.Printf("position_risk=%v\n", pos)
	}
	fmt.Println("ASTER order type smoke complete")
}

func runHyperliquidOrderTypeSmoke(symbol string, notionalUSD float64) {
	fmt.Println("=== HYPERLIQUID ORDER TYPE SMOKE ===")
	account := strings.ToLower(strings.TrimSpace(os.Getenv("HYPERLIQUID_ACCOUNT_ADDRESS")))
	privateKey := strings.TrimSpace(os.Getenv("HYPERLIQUID_API_WALLET_PRIVATE_KEY"))
	if account == "" || privateKey == "" {
		fmt.Println("HYPERLIQUID_ACCOUNT_ADDRESS and HYPERLIQUID_API_WALLET_PRIVATE_KEY are required")
		os.Exit(1)
	}
	coin := normalizeHyperliquidCoin(symbol)
	hl := hlsdk.NewHyperliquid(&hlsdk.HyperliquidClientConfig{
		IsMainnet:      true,
		AccountAddress: account,
		PrivateKey:     privateKey,
	})
	if err := ensureHyperliquidIsolated(hl, account, coin); err != nil {
		fmt.Printf("isolated_preflight_failed: %v\n", err)
		os.Exit(1)
	}
	px, err := hl.InfoAPI.GetMartketPx(coin)
	if err != nil || px <= 0 {
		fmt.Printf("market price failed coin=%s err=%v px=%.8f\n", coin, err, px)
		os.Exit(1)
	}
	effectiveNotional := notionalUSD
	if effectiveNotional < 10.5 {
		effectiveNotional = 10.5
	}
	sz := roundTo(math.Max(effectiveNotional/px, 0.0001), 6)
	szDecimals := 0
	if metaMap, err := hl.InfoAPI.BuildMetaMap(); err == nil {
		if m, ok := metaMap[coin]; ok {
			szDecimals = m.SzDecimals
		}
	}
	fmt.Printf("coin=%s mark_price=%.6f target_notional=%.2f effective_notional=%.2f sz=%.6f\n", coin, px, notionalUSD, effectiveNotional, sz)

	type smokeCase struct {
		name    string
		request hlsdk.OrderRequest
	}
	cases := []smokeCase{
		{
			name: "limit_gtc",
			request: hlsdk.OrderRequest{
				Coin:       coin,
				IsBuy:      true,
				Sz:         sz,
				LimitPx:    roundTo(px*0.96, 4),
				OrderType:  hlsdk.OrderType{Limit: &hlsdk.LimitOrderType{Tif: hlsdk.TifGtc}},
				ReduceOnly: false,
			},
		},
		{
			name: "limit_post_only_alo",
			request: hlsdk.OrderRequest{
				Coin:       coin,
				IsBuy:      true,
				Sz:         sz,
				LimitPx:    roundTo(px*0.95, 4),
				OrderType:  hlsdk.OrderType{Limit: &hlsdk.LimitOrderType{Tif: hlsdk.TifAlo}},
				ReduceOnly: false,
			},
		},
		{
			name: "stop_market_sl",
			request: hlsdk.OrderRequest{
				Coin:    coin,
				IsBuy:   true,
				Sz:      sz,
				LimitPx: hlNormalizePx(px*0.82, szDecimals),
				OrderType: hlsdk.OrderType{
					Trigger: &hlsdk.TriggerOrderType{
						IsMarket:  true,
						TriggerPx: hlFormatTriggerPx(px*0.85, szDecimals),
						TpSl:      hlsdk.TriggerSl,
					},
				},
				ReduceOnly: false,
			},
		},
		{
			name: "stop_limit_sl",
			request: hlsdk.OrderRequest{
				Coin:    coin,
				IsBuy:   true,
				Sz:      sz,
				LimitPx: hlNormalizePx(px*0.84, szDecimals),
				OrderType: hlsdk.OrderType{
					Trigger: &hlsdk.TriggerOrderType{
						IsMarket:  false,
						TriggerPx: hlFormatTriggerPx(px*0.85, szDecimals),
						TpSl:      hlsdk.TriggerSl,
					},
				},
				ReduceOnly: false,
			},
		},
		{
			name: "take_profit_market_tp",
			request: hlsdk.OrderRequest{
				Coin:    coin,
				IsBuy:   true,
				Sz:      sz,
				LimitPx: hlNormalizePx(px*1.18, szDecimals),
				OrderType: hlsdk.OrderType{
					Trigger: &hlsdk.TriggerOrderType{
						IsMarket:  true,
						TriggerPx: hlFormatTriggerPx(px*1.15, szDecimals),
						TpSl:      hlsdk.TriggerTp,
					},
				},
				ReduceOnly: false,
			},
		},
		{
			name: "take_profit_limit_tp",
			request: hlsdk.OrderRequest{
				Coin:    coin,
				IsBuy:   true,
				Sz:      sz,
				LimitPx: hlNormalizePx(px*1.16, szDecimals),
				OrderType: hlsdk.OrderType{
					Trigger: &hlsdk.TriggerOrderType{
						IsMarket:  false,
						TriggerPx: hlFormatTriggerPx(px*1.15, szDecimals),
						TpSl:      hlsdk.TriggerTp,
					},
				},
				ReduceOnly: false,
			},
		},
	}

	for _, tc := range cases {
		resp, err := hl.ExchangeAPI.Order(tc.request, hlsdk.GroupingNa)
		if err != nil {
			fmt.Printf("order_type=%s result=ERROR err=%v\n", tc.name, err)
			continue
		}
		fmt.Printf("order_type=%s result=OK resp=%+v\n", tc.name, resp)
	}

	// Position-backed TP/SL validation flow:
	// open tiny long -> place reduce-only sell TP/SL -> cleanup -> close position.
	fmt.Println("position_tpsl_lifecycle begin")
	openResp, err := hl.ExchangeAPI.MarketOrder(coin, sz, nil)
	if err != nil {
		fmt.Printf("position_tpsl_open result=ERROR err=%v\n", err)
	} else {
		fmt.Printf("position_tpsl_open result=OK resp=%+v\n", openResp)
		time.Sleep(900 * time.Millisecond)
		filledSz := sz
		if len(openResp.Response.Data.Statuses) > 0 {
			fs := openResp.Response.Data.Statuses[0].Filled.TotalSz
			if fs > 0 {
				filledSz = fs
			}
		}

		tpLimit := hlNormalizePx(px*1.075, szDecimals)
		slLimit := hlNormalizePx(px*0.925, szDecimals)
		parent := hlsdk.OrderRequest{
			Coin:       coin,
			IsBuy:      true,
			Sz:         sz,
			LimitPx:    hlNormalizePx(px*1.08, szDecimals),
			OrderType:  hlsdk.OrderType{Limit: &hlsdk.LimitOrderType{Tif: hlsdk.TifIoc}},
			ReduceOnly: false,
		}
		tpChild := hlsdk.OrderRequest{
			Coin:    coin,
			IsBuy:   false,
			Sz:      filledSz,
			LimitPx: tpLimit,
			OrderType: hlsdk.OrderType{
				Trigger: &hlsdk.TriggerOrderType{
					IsMarket:  true,
					TriggerPx: hlFormatTriggerPx(px*1.08, szDecimals),
					TpSl:      hlsdk.TriggerTp,
				},
			},
			ReduceOnly: true,
		}
		slChild := hlsdk.OrderRequest{
			Coin:    coin,
			IsBuy:   false,
			Sz:      filledSz,
			LimitPx: slLimit,
			OrderType: hlsdk.OrderType{
				Trigger: &hlsdk.TriggerOrderType{
					IsMarket:  true,
					TriggerPx: hlFormatTriggerPx(px*0.92, szDecimals),
					TpSl:      hlsdk.TriggerSl,
				},
			},
			ReduceOnly: true,
		}
		resp, err := hl.ExchangeAPI.BulkOrders([]hlsdk.OrderRequest{parent, tpChild, slChild}, hlsdk.Grouping("normalTpsl"), false)
		if err != nil {
			fmt.Printf("order_type=normal_tpsl_bundle result=ERROR err=%v\n", err)
		} else {
			fmt.Printf("order_type=normal_tpsl_bundle result=OK resp=%+v\n", resp)
		}
	}

	metaMap, metaErr := hl.InfoAPI.BuildMetaMap()
	if metaErr != nil {
		fmt.Printf("order_type=twap_probe result=ERROR err=meta_map:%v\n", metaErr)
	} else if m, ok := metaMap[coin]; !ok {
		fmt.Printf("order_type=twap_probe result=ERROR err=asset_not_found coin=%s\n", coin)
	} else {
		asset := m.AssetId
		twapAction := hlTwapOrderAction{
			Type: "twapOrder",
			Twap: hlTwapWire{
				A: asset,
				B: true,
				S: strconv.FormatFloat(sz, 'f', -1, 64),
				R: false,
				M: 5,
				T: false,
			},
		}
		twapResp, err := sendHyperliquidRawAction(hl, twapAction)
		if err != nil {
			fmt.Printf("order_type=twap_probe result=ERROR err=%v\n", err)
		} else {
			fmt.Printf("order_type=twap_probe result=OK resp=%v\n", twapResp)
			if twapID := extractTwapID(twapResp); twapID > 0 {
				cancelAction := hlTwapCancelAction{Type: "twapCancel", A: asset, T: twapID}
				cancelResp, cerr := sendHyperliquidRawAction(hl, cancelAction)
				if cerr != nil {
					fmt.Printf("order_type=twap_cancel result=ERROR twap_id=%d err=%v\n", twapID, cerr)
				} else {
					fmt.Printf("order_type=twap_cancel result=OK twap_id=%d resp=%v\n", twapID, cancelResp)
				}
			}
		}
	}
	allResp, err := hl.ExchangeAPI.CancelAllOrdersByCoin(coin)
	if err != nil {
		fmt.Printf("cleanup_cancel_all_by_coin result=ERROR err=%v\n", err)
		os.Exit(1)
	}
	fmt.Printf("cleanup_cancel_all_by_coin result=OK resp=%+v\n", allResp)
	openOrders, err := hl.InfoAPI.GetAccountOpenOrders()
	if err == nil && openOrders != nil {
		openForCoin := 0
		for _, o := range *openOrders {
			if strings.EqualFold(o.Coin, coin) {
				openForCoin++
			}
		}
		fmt.Printf("post_cleanup_open_orders coin=%s count=%d\n", coin, openForCoin)
	}
	closeResp, cerr := hl.ExchangeAPI.ClosePosition(coin)
	if cerr != nil {
		fmt.Printf("position_tpsl_close note=%v\n", cerr)
	} else {
		fmt.Printf("position_tpsl_close result=OK resp=%+v\n", closeResp)
	}
	fmt.Println("HYPERLIQUID order type smoke complete")
}

func runLighterOrderTypeSmoke(symbol string, notionalUSD float64) {
	fmt.Println("=== LIGHTER ORDER TYPE SMOKE ===")
	if err := ensureLighterIsolatedStrict(); err != nil {
		fmt.Printf("isolated_preflight_failed: %v\n", err)
		os.Exit(1)
	}
	baseURL := strings.TrimSpace(os.Getenv("LIGHTER_BASE_URL"))
	if baseURL == "" {
		baseURL = "https://mainnet.zklighter.elliot.ai"
	}
	cli := lighter.NewClient(baseURL)

	accountIndex, err := resolveLighterAccountIndex(cli)
	if err != nil {
		fmt.Printf("lighter account resolution failed: %v\n", err)
		os.Exit(1)
	}

	apiIdx, err := parseUint8Env("LIGHTER_API_KEY_INDEX")
	if err != nil {
		fmt.Printf("lighter api key index parse failed: %v\n", err)
		os.Exit(1)
	}

	privateKey := strings.TrimSpace(os.Getenv("LIGHTER_API_PRIVATE_KEY"))
	if privateKey == "" {
		privateKey = strings.TrimSpace(os.Getenv("LIGHTER_API_SECRET"))
	}
	if privateKey == "" {
		fmt.Println("missing LIGHTER_API_PRIVATE_KEY (or fallback LIGHTER_API_SECRET)")
		os.Exit(1)
	}
	chainID := uint32(304)
	if raw := strings.TrimSpace(os.Getenv("LIGHTER_CHAIN_ID")); raw != "" {
		if parsed, err := strconv.ParseUint(raw, 10, 32); err == nil {
			chainID = uint32(parsed)
		}
	}
	txClient, err := lgclient.CreateClient(lhttp.NewClient(baseURL), privateKey, chainID, apiIdx, accountIndex)
	if err != nil {
		fmt.Printf("lighter signer init failed: %v\n", err)
		os.Exit(1)
	}
	if err := txClient.Check(); err != nil {
		fmt.Printf("lighter signer check failed: %v\n", err)
		fmt.Println("hint: API key index/private key pair may not match registered key")
		os.Exit(1)
	}
	fmt.Printf("lighter signer check OK account_index=%d api_key_index=%d\n", accountIndex, apiIdx)

	authToken, err := txClient.GetAuthToken(time.Now().Add(7 * time.Hour))
	if err != nil {
		fmt.Printf("lighter auth token failed: %v\n", err)
		os.Exit(1)
	}

	meta, err := lighterFindPerpMeta(cli, symbol)
	if err != nil {
		fmt.Printf("lighter symbol metadata failed: %v\n", err)
		os.Exit(1)
	}
	book, err := cli.OrderBookOrders(meta.MarketID, 1)
	if err != nil {
		fmt.Printf("lighter top of book failed: %v\n", err)
		os.Exit(1)
	}
	bestAsk := parsePriceOrZero(firstPrice(book.Asks))
	bestBid := parsePriceOrZero(firstPrice(book.Bids))
	refPrice := bestAsk
	if refPrice <= 0 {
		refPrice = bestBid
	}
	if refPrice <= 0 {
		fmt.Println("could not determine reference price from order book")
		os.Exit(1)
	}

	sizeStep := math.Pow10(meta.SupportedSizeDecimals)
	priceStep := math.Pow10(meta.SupportedPriceDecimals)
	minBase := parsePriceOrZero(meta.MinBaseAmount)
	minQuote := parsePriceOrZero(meta.MinQuoteAmount)

	rawQty := notionalUSD / refPrice
	qty := math.Ceil(rawQty*sizeStep) / sizeStep
	if qty < minBase {
		qty = minBase
	}
	limitPrice := bestBid * 0.95
	if limitPrice <= 0 {
		limitPrice = refPrice * 0.95
	}
	quote := qty * limitPrice
	if quote < minQuote {
		qty = math.Ceil((minQuote/limitPrice)*sizeStep) / sizeStep
		quote = qty * limitPrice
	}
	baseAmount := int64(math.Round(qty * sizeStep))
	clientOrderIndex := time.Now().UnixMilli() % ((1 << 47) - 1)
	if clientOrderIndex <= 0 {
		clientOrderIndex = 1
	}
	fmt.Printf("lighter symbol=%s market_id=%d ref=%.4f bid=%.4f ask=%.4f qty=%.8f price=%.8f quote=%.4f\n",
		meta.Symbol, meta.MarketID, refPrice, bestBid, bestAsk, qty, limitPrice, quote)

	type advCase struct {
		name         string
		orderType    uint8
		tif          uint8
		isAsk        uint8
		reduceOnly   uint8
		price        uint32
		triggerPrice uint32
		orderExpiry  int64
	}
	buyPriceInt := uint32(math.Round((bestBid * 0.95) * priceStep))
	sellPriceInt := uint32(math.Round((bestAsk * 1.05) * priceStep))
	triggerHigh := uint32(math.Round((refPrice * 1.10) * priceStep))
	triggerLow := uint32(math.Round((refPrice * 0.90) * priceStep))
	now := time.Now()
	cases := []advCase{
		{name: "limit_gtt_buy", orderType: ltxtypes.LimitOrder, tif: ltxtypes.GoodTillTime, isAsk: 0, price: buyPriceInt, triggerPrice: 0, orderExpiry: now.Add(10 * time.Minute).UnixMilli()},
		{name: "post_only_buy", orderType: ltxtypes.LimitOrder, tif: ltxtypes.PostOnly, isAsk: 0, price: buyPriceInt, triggerPrice: 0, orderExpiry: now.Add(10 * time.Minute).UnixMilli()},
		{name: "stop_market_buy", orderType: ltxtypes.StopLossOrder, tif: ltxtypes.ImmediateOrCancel, isAsk: 0, price: buyPriceInt, triggerPrice: triggerHigh, orderExpiry: now.Add(10 * time.Minute).UnixMilli()},
		{name: "stop_limit_buy", orderType: ltxtypes.StopLossLimitOrder, tif: ltxtypes.GoodTillTime, isAsk: 0, price: buyPriceInt, triggerPrice: triggerHigh, orderExpiry: now.Add(10 * time.Minute).UnixMilli()},
		{name: "take_profit_market_sell", orderType: ltxtypes.TakeProfitOrder, tif: ltxtypes.ImmediateOrCancel, isAsk: 1, price: sellPriceInt, triggerPrice: triggerHigh, orderExpiry: now.Add(10 * time.Minute).UnixMilli()},
		{name: "take_profit_limit_sell", orderType: ltxtypes.TakeProfitLimitOrder, tif: ltxtypes.GoodTillTime, isAsk: 1, price: sellPriceInt, triggerPrice: triggerHigh, orderExpiry: now.Add(10 * time.Minute).UnixMilli()},
		{name: "twap_buy", orderType: ltxtypes.TWAPOrder, tif: ltxtypes.GoodTillTime, isAsk: 0, price: buyPriceInt, triggerPrice: 0, orderExpiry: now.Add(12 * time.Minute).UnixMilli()},
		{name: "stop_market_sell_probe", orderType: ltxtypes.StopLossOrder, tif: ltxtypes.ImmediateOrCancel, isAsk: 1, price: sellPriceInt, triggerPrice: triggerLow, orderExpiry: now.Add(10 * time.Minute).UnixMilli()},
	}
	_ = triggerLow

	for i, tc := range cases {
		coi := clientOrderIndex + int64(i)
		req := &ltypes.CreateOrderTxReq{
			MarketIndex:      meta.MarketID,
			ClientOrderIndex: coi,
			BaseAmount:       baseAmount,
			Price:            tc.price,
			IsAsk:            tc.isAsk,
			Type:             tc.orderType,
			TimeInForce:      tc.tif,
			ReduceOnly:       tc.reduceOnly,
			TriggerPrice:     tc.triggerPrice,
			OrderExpiry:      tc.orderExpiry,
		}
		tx, err := txClient.GetCreateOrderTransaction(req, nil)
		if err != nil {
			fmt.Printf("lighter_case=%s build_error=%v\n", tc.name, err)
			continue
		}
		info, err := tx.GetTxInfo()
		if err != nil {
			fmt.Printf("lighter_case=%s txinfo_error=%v\n", tc.name, err)
			continue
		}
		ack, err := cli.SendTx(tx.GetTxType(), info, true)
		if err != nil {
			fmt.Printf("lighter_case=%s send_error=%v\n", tc.name, err)
			continue
		}
		fmt.Printf("lighter_case=%s create_ack=%+v\n", tc.name, *ack)
		txState, err := waitLighterTxFinal(cli, ack.TxHash, 8, 500*time.Millisecond)
		if err != nil {
			fmt.Printf("lighter_case=%s tx_final_error=%v\n", tc.name, err)
			continue
		}
		fmt.Printf("lighter_case=%s tx_final status=%d type=%d hash=%s\n", tc.name, txState.Status, txState.Type, txState.Hash)

		active, err := cli.AccountActiveOrders(accountIndex, meta.MarketID, authToken)
		if err != nil {
			fmt.Printf("lighter_case=%s active_read_error=%v\n", tc.name, err)
			continue
		}
		cancelIndex := int64(0)
		for _, o := range active.Orders {
			if o.ClientOrderIndex == coi {
				cancelIndex = o.OrderIndex
				fmt.Printf("lighter_case=%s order_open order_index=%d status=%s\n", tc.name, o.OrderIndex, o.Status)
				break
			}
		}
		if cancelIndex == 0 {
			inactive, ierr := cli.AccountInactiveOrders(accountIndex, meta.MarketID, 20, authToken)
			if ierr == nil {
				for _, o := range inactive.Orders {
					if o.ClientOrderIndex == coi {
						fmt.Printf("lighter_case=%s inactive_status order_index=%d status=%s\n", tc.name, o.OrderIndex, o.Status)
						cancelIndex = -1
						break
					}
				}
			}
		}
		if cancelIndex <= 0 {
			continue
		}
		cancelReq := &ltypes.CancelOrderTxReq{MarketIndex: meta.MarketID, Index: cancelIndex}
		cancelTx, err := txClient.GetCancelOrderTransaction(cancelReq, nil)
		if err != nil {
			fmt.Printf("lighter_case=%s cancel_build_error=%v\n", tc.name, err)
			continue
		}
		cancelInfo, err := cancelTx.GetTxInfo()
		if err != nil {
			fmt.Printf("lighter_case=%s cancel_txinfo_error=%v\n", tc.name, err)
			continue
		}
		cancelAck, err := cli.SendTx(cancelTx.GetTxType(), cancelInfo, true)
		if err != nil {
			fmt.Printf("lighter_case=%s cancel_send_error=%v\n", tc.name, err)
			continue
		}
		fmt.Printf("lighter_case=%s cancel_ack=%+v\n", tc.name, *cancelAck)
		cancelState, err := waitLighterTxFinal(cli, cancelAck.TxHash, 8, 500*time.Millisecond)
		if err != nil {
			fmt.Printf("lighter_case=%s cancel_tx_final_error=%v\n", tc.name, err)
			continue
		}
		fmt.Printf("lighter_case=%s cancel_tx_final status=%d hash=%s\n", tc.name, cancelState.Status, cancelState.Hash)
	}

	// Grouped-order smoke (scale-like staged logic): OTO and OTOCO, then cancel-all safety.
	now2 := time.Now()
	groupParentPrice := uint32(math.Round((bestBid * 0.96) * priceStep))
	groupSLTrigger := uint32(math.Round((refPrice * 0.90) * priceStep))
	groupTPTrigger := uint32(math.Round((refPrice * 1.10) * priceStep))
	groupSLExecPrice := uint32(math.Round((bestBid * 0.94) * priceStep))
	groupTPExecPrice := uint32(math.Round((bestAsk * 1.06) * priceStep))
	childExpiry := now2.Add(15 * time.Minute).UnixMilli()

	oto := &ltypes.CreateGroupedOrdersTxReq{
		GroupingType: ltxtypes.GroupingType_OneTriggersTheOther,
		Orders: []*ltypes.CreateOrderTxReq{
			{
				MarketIndex:      meta.MarketID,
				ClientOrderIndex: 0,
				BaseAmount:       baseAmount,
				Price:            groupParentPrice,
				IsAsk:            0, // buy parent
				Type:             ltxtypes.LimitOrder,
				TimeInForce:      ltxtypes.GoodTillTime,
				ReduceOnly:       0,
				TriggerPrice:     0,
				OrderExpiry:      childExpiry,
			},
			{
				MarketIndex:      meta.MarketID,
				ClientOrderIndex: 0,
				BaseAmount:       0, // required by OTO child
				Price:            groupSLExecPrice,
				IsAsk:            1, // sell child
				Type:             ltxtypes.StopLossOrder,
				TimeInForce:      ltxtypes.ImmediateOrCancel,
				ReduceOnly:       1,
				TriggerPrice:     groupSLTrigger,
				OrderExpiry:      childExpiry,
			},
		},
	}
	if err := runLighterGroupedCase(cli, txClient, accountIndex, meta.MarketID, authToken, "group_oto", oto); err != nil {
		fmt.Printf("lighter_case=group_oto error=%v\n", err)
	}
	if err := lighterCancelAllSafety(cli, txClient); err != nil {
		fmt.Printf("lighter_cancel_all_safety_error after=group_oto err=%v\n", err)
	}

	otoco := &ltypes.CreateGroupedOrdersTxReq{
		GroupingType: ltxtypes.GroupingType_OneTriggersAOneCancelsTheOther,
		Orders: []*ltypes.CreateOrderTxReq{
			{
				MarketIndex:      meta.MarketID,
				ClientOrderIndex: 0,
				BaseAmount:       baseAmount,
				Price:            groupParentPrice,
				IsAsk:            0, // buy parent
				Type:             ltxtypes.LimitOrder,
				TimeInForce:      ltxtypes.GoodTillTime,
				ReduceOnly:       0,
				TriggerPrice:     0,
				OrderExpiry:      childExpiry,
			},
			{
				MarketIndex:      meta.MarketID,
				ClientOrderIndex: 0,
				BaseAmount:       0, // required by OTOCO child
				Price:            groupSLExecPrice,
				IsAsk:            1, // sell child
				Type:             ltxtypes.StopLossLimitOrder,
				TimeInForce:      ltxtypes.GoodTillTime,
				ReduceOnly:       1,
				TriggerPrice:     groupSLTrigger,
				OrderExpiry:      childExpiry,
			},
			{
				MarketIndex:      meta.MarketID,
				ClientOrderIndex: 0,
				BaseAmount:       0, // required by OTOCO child
				Price:            groupTPExecPrice,
				IsAsk:            1, // sell child
				Type:             ltxtypes.TakeProfitLimitOrder,
				TimeInForce:      ltxtypes.GoodTillTime,
				ReduceOnly:       1,
				TriggerPrice:     groupTPTrigger,
				OrderExpiry:      childExpiry,
			},
		},
	}
	if err := runLighterGroupedCase(cli, txClient, accountIndex, meta.MarketID, authToken, "group_otoco", otoco); err != nil {
		fmt.Printf("lighter_case=group_otoco error=%v\n", err)
	}
	if err := lighterCancelAllSafety(cli, txClient); err != nil {
		fmt.Printf("lighter_cancel_all_safety_error after=group_otoco err=%v\n", err)
	}

	// Final cleanup/status snapshot for this symbol.
	activeEnd, _ := cli.AccountActiveOrders(accountIndex, meta.MarketID, authToken)
	fmt.Printf("lighter_final_active_orders=%d\n", len(activeEnd.Orders))
	fmt.Println("LIGHTER order type smoke complete")
}

func runLighterBatchSmoke(symbol string, notionalUSD float64) {
	fmt.Println("=== LIGHTER BATCH SMOKE ===")
	if err := ensureLighterIsolatedStrict(); err != nil {
		fmt.Printf("isolated_preflight_failed: %v\n", err)
		os.Exit(1)
	}
	baseURL := strings.TrimSpace(os.Getenv("LIGHTER_BASE_URL"))
	if baseURL == "" {
		baseURL = "https://mainnet.zklighter.elliot.ai"
	}
	cli := lighter.NewClient(baseURL)
	accountIndex, err := resolveLighterAccountIndex(cli)
	if err != nil {
		fmt.Printf("lighter account resolution failed: %v\n", err)
		os.Exit(1)
	}
	apiIdx, err := parseUint8Env("LIGHTER_API_KEY_INDEX")
	if err != nil {
		fmt.Printf("lighter api key index parse failed: %v\n", err)
		os.Exit(1)
	}
	privateKey := strings.TrimSpace(os.Getenv("LIGHTER_API_PRIVATE_KEY"))
	if privateKey == "" {
		privateKey = strings.TrimSpace(os.Getenv("LIGHTER_API_SECRET"))
	}
	if privateKey == "" {
		fmt.Println("missing LIGHTER_API_PRIVATE_KEY (or fallback LIGHTER_API_SECRET)")
		os.Exit(1)
	}
	chainID := uint32(304)
	if raw := strings.TrimSpace(os.Getenv("LIGHTER_CHAIN_ID")); raw != "" {
		if parsed, err := strconv.ParseUint(raw, 10, 32); err == nil {
			chainID = uint32(parsed)
		}
	}
	txClient, err := lgclient.CreateClient(lhttp.NewClient(baseURL), privateKey, chainID, apiIdx, accountIndex)
	if err != nil {
		fmt.Printf("lighter signer init failed: %v\n", err)
		os.Exit(1)
	}
	if err := txClient.Check(); err != nil {
		fmt.Printf("lighter signer check failed: %v\n", err)
		os.Exit(1)
	}
	authToken, err := txClient.GetAuthToken(time.Now().Add(7 * time.Hour))
	if err != nil {
		fmt.Printf("lighter auth token failed: %v\n", err)
		os.Exit(1)
	}
	meta, err := lighterFindPerpMeta(cli, symbol)
	if err != nil {
		fmt.Printf("lighter symbol metadata failed: %v\n", err)
		os.Exit(1)
	}
	book, err := cli.OrderBookOrders(meta.MarketID, 1)
	if err != nil {
		fmt.Printf("lighter top of book failed: %v\n", err)
		os.Exit(1)
	}
	bestBid := parsePriceOrZero(firstPrice(book.Bids))
	bestAsk := parsePriceOrZero(firstPrice(book.Asks))
	ref := bestAsk
	if ref <= 0 {
		ref = bestBid
	}
	if ref <= 0 {
		fmt.Println("lighter could not derive reference price")
		os.Exit(1)
	}
	sizeStep := math.Pow10(meta.SupportedSizeDecimals)
	priceStep := math.Pow10(meta.SupportedPriceDecimals)
	qty := math.Ceil((notionalUSD/ref)*sizeStep) / sizeStep
	minBase := parsePriceOrZero(meta.MinBaseAmount)
	if qty < minBase {
		qty = minBase
	}
	baseAmount := int64(math.Round(qty * sizeStep))
	price1 := uint32(math.Round((bestBid * 0.95) * priceStep))
	price2 := uint32(math.Round((bestBid * 0.94) * priceStep))
	exp := time.Now().Add(10 * time.Minute).UnixMilli()

	req1 := &ltypes.CreateOrderTxReq{
		MarketIndex:      meta.MarketID,
		ClientOrderIndex: time.Now().UnixMilli(),
		BaseAmount:       baseAmount,
		Price:            price1,
		IsAsk:            0,
		Type:             ltxtypes.LimitOrder,
		TimeInForce:      ltxtypes.GoodTillTime,
		ReduceOnly:       0,
		TriggerPrice:     0,
		OrderExpiry:      exp,
	}
	req2 := &ltypes.CreateOrderTxReq{
		MarketIndex:      meta.MarketID,
		ClientOrderIndex: time.Now().UnixMilli() + 1,
		BaseAmount:       baseAmount,
		Price:            price2,
		IsAsk:            0,
		Type:             ltxtypes.LimitOrder,
		TimeInForce:      ltxtypes.GoodTillTime,
		ReduceOnly:       0,
		TriggerPrice:     0,
		OrderExpiry:      exp,
	}
	minHTTP := lhttp.NewClient(baseURL)
	startNonce, err := minHTTP.GetNextNonce(accountIndex, apiIdx)
	if err != nil {
		fmt.Printf("lighter batch nextNonce failed: %v\n", err)
		os.Exit(1)
	}
	nonce1 := startNonce
	nonce2 := startNonce + 1
	acct := accountIndex
	keyIdx := apiIdx
	ops1 := &ltypes.TransactOpts{FromAccountIndex: &acct, ApiKeyIndex: &keyIdx, Nonce: &nonce1}
	ops2 := &ltypes.TransactOpts{FromAccountIndex: &acct, ApiKeyIndex: &keyIdx, Nonce: &nonce2}

	tx1, err := txClient.GetCreateOrderTransaction(req1, ops1)
	if err != nil {
		fmt.Printf("lighter batch tx1 build failed: %v\n", err)
		os.Exit(1)
	}
	tx2, err := txClient.GetCreateOrderTransaction(req2, ops2)
	if err != nil {
		fmt.Printf("lighter batch tx2 build failed: %v\n", err)
		os.Exit(1)
	}
	info1, _ := tx1.GetTxInfo()
	info2, _ := tx2.GetTxInfo()
	batchAck, err := cli.SendTxBatch([]uint8{tx1.GetTxType(), tx2.GetTxType()}, []string{info1, info2})
	if err != nil {
		fmt.Printf("lighter sendTxBatch failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("lighter_batch_ack=%+v\n", *batchAck)
	for _, h := range batchAck.TxHash {
		final, ferr := waitLighterTxFinal(cli, h, 8, 500*time.Millisecond)
		if ferr != nil {
			fmt.Printf("lighter_batch_tx_final hash=%s err=%v\n", h, ferr)
			continue
		}
		fmt.Printf("lighter_batch_tx_final hash=%s status=%d type=%d\n", h, final.Status, final.Type)
	}
	if err := lighterCancelAllSafety(cli, txClient); err != nil {
		fmt.Printf("lighter batch cancel-all safety failed: %v\n", err)
	}
	time.Sleep(1200 * time.Millisecond)
	activeEnd, _ := cli.AccountActiveOrders(accountIndex, meta.MarketID, authToken)
	fmt.Printf("lighter_batch_final_active_orders=%d\n", len(activeEnd.Orders))
	fmt.Println("LIGHTER batch smoke complete")
}

func runLighterGroupedCase(cli *lighter.Client, txClient *lgclient.TxClient, accountIndex int64, marketID int16, authToken, caseName string, req *ltypes.CreateGroupedOrdersTxReq) error {
	tx, err := txClient.GetCreateGroupedOrdersTransaction(req, nil)
	if err != nil {
		return fmt.Errorf("build grouped tx: %w", err)
	}
	info, err := tx.GetTxInfo()
	if err != nil {
		return fmt.Errorf("grouped tx info: %w", err)
	}
	ack, err := cli.SendTx(tx.GetTxType(), info, true)
	if err != nil {
		return fmt.Errorf("send grouped tx: %w", err)
	}
	fmt.Printf("lighter_case=%s create_ack=%+v\n", caseName, *ack)
	final, err := waitLighterTxFinal(cli, ack.TxHash, 8, 500*time.Millisecond)
	if err != nil {
		return fmt.Errorf("grouped final tx: %w", err)
	}
	fmt.Printf("lighter_case=%s tx_final status=%d type=%d hash=%s\n", caseName, final.Status, final.Type, final.Hash)
	active, err := cli.AccountActiveOrders(accountIndex, marketID, authToken)
	if err != nil {
		return fmt.Errorf("active orders after grouped: %w", err)
	}
	fmt.Printf("lighter_case=%s active_orders=%d\n", caseName, len(active.Orders))
	return nil
}

func lighterCancelAllSafety(cli *lighter.Client, txClient *lgclient.TxClient) error {
	req := &ltypes.CancelAllOrdersTxReq{
		TimeInForce: ltxtypes.ImmediateCancelAll,
		Time:        0,
	}
	tx, err := txClient.GetCancelAllOrdersTransaction(req, nil)
	if err != nil {
		return fmt.Errorf("build cancel-all tx: %w", err)
	}
	info, err := tx.GetTxInfo()
	if err != nil {
		return fmt.Errorf("cancel-all tx info: %w", err)
	}
	ack, err := cli.SendTx(tx.GetTxType(), info, true)
	if err != nil {
		return fmt.Errorf("send cancel-all: %w", err)
	}
	fmt.Printf("lighter_cancel_all_ack=%+v\n", *ack)
	final, err := waitLighterTxFinal(cli, ack.TxHash, 8, 500*time.Millisecond)
	if err != nil {
		return fmt.Errorf("cancel-all final tx: %w", err)
	}
	fmt.Printf("lighter_cancel_all_final status=%d hash=%s\n", final.Status, final.Hash)
	return nil
}

func resolveLighterAccountIndex(cli *lighter.Client) (int64, error) {
	if raw := strings.TrimSpace(os.Getenv("LIGHTER_ACCOUNT_INDEX")); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err == nil && n > 0 {
			return n, nil
		}
	}
	addr := strings.TrimSpace(os.Getenv("LIGHTER_L1_ADDRESS"))
	if addr == "" {
		addr = strings.TrimSpace(os.Getenv("HYPERLIQUID_ACCOUNT_ADDRESS"))
	}
	if addr == "" {
		return 0, fmt.Errorf("missing LIGHTER_ACCOUNT_INDEX and LIGHTER_L1_ADDRESS/HYPERLIQUID_ACCOUNT_ADDRESS")
	}
	acc, err := cli.AccountByL1(addr)
	if err != nil {
		return 0, err
	}
	if len(acc.Accounts) == 0 {
		return 0, fmt.Errorf("no lighter account for l1 address")
	}
	return acc.Accounts[0].AccountIndex, nil
}

func parseUint8Env(key string) (uint8, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0, fmt.Errorf("%s missing", key)
	}
	n, err := strconv.ParseUint(raw, 10, 8)
	if err != nil {
		return 0, err
	}
	return uint8(n), nil
}

func lighterFindPerpMeta(cli *lighter.Client, symbol string) (*lighter.OrderBookMeta, error) {
	obs, err := cli.OrderBooks("perp")
	if err != nil {
		return nil, err
	}
	want := strings.ToUpper(strings.TrimSpace(symbol))
	for _, ob := range obs.OrderBooks {
		if strings.EqualFold(strings.TrimSpace(ob.Symbol), want) {
			cp := ob
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("symbol %s not found in lighter perp metadata", want)
}

func firstPrice(rows []lighter.BookOrder) string {
	if len(rows) == 0 {
		return ""
	}
	return strings.TrimSpace(rows[0].Price)
}

func parsePriceOrZero(s string) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return f
}

func waitLighterTxFinal(cli *lighter.Client, txHash string, maxAttempts int, sleep time.Duration) (*lighter.EnrichedTx, error) {
	var last *lighter.EnrichedTx
	for i := 0; i < maxAttempts; i++ {
		tx, err := cli.TxByHash(txHash)
		if err == nil && tx != nil {
			last = tx
			if tx.Status != 0 {
				return tx, nil
			}
		}
		time.Sleep(sleep)
	}
	if last != nil {
		return last, nil
	}
	return nil, fmt.Errorf("no tx result for hash %s", txHash)
}

func ensureAsterIsolated(cli *aster.Client, symbol string) error {
	pos, err := cli.PositionRisk(symbol)
	if err != nil {
		return fmt.Errorf("position risk read failed: %w", err)
	}
	current := ""
	for _, p := range pos {
		if strings.EqualFold(strings.TrimSpace(fmt.Sprint(p["symbol"])), symbol) {
			current = strings.ToLower(strings.TrimSpace(fmt.Sprint(p["marginType"])))
			break
		}
	}
	if current == "isolated" {
		fmt.Printf("margin_mode_check symbol=%s mode=isolated action=none\n", symbol)
		return nil
	}
	fmt.Printf("margin_mode_check symbol=%s mode=%s action=switch_to_isolated\n", symbol, current)
	resp, err := cli.ChangeMarginType(symbol, "ISOLATED")
	if err != nil {
		return fmt.Errorf("switch margin type failed: %w", err)
	}
	fmt.Printf("margin_mode_switch_resp=%v\n", resp)
	pos2, err := cli.PositionRisk(symbol)
	if err != nil {
		return fmt.Errorf("position risk recheck failed: %w", err)
	}
	for _, p := range pos2 {
		if strings.EqualFold(strings.TrimSpace(fmt.Sprint(p["symbol"])), symbol) {
			mode := strings.ToLower(strings.TrimSpace(fmt.Sprint(p["marginType"])))
			fmt.Printf("margin_mode_recheck symbol=%s mode=%s\n", symbol, mode)
			if mode == "isolated" {
				return nil
			}
			return fmt.Errorf("margin mode still not isolated (mode=%s)", mode)
		}
	}
	return fmt.Errorf("symbol not found in position risk recheck")
}

func makeURLValues(m map[string]string) url.Values {
	v := url.Values{}
	for k, val := range m {
		v.Set(k, val)
	}
	return v
}

func int64FromAny(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return int64(x)
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(x), 10, 64)
		return n
	default:
		s := strings.TrimSpace(fmt.Sprint(v))
		n, _ := strconv.ParseInt(s, 10, 64)
		return n
	}
}

func runWSChecks(venue string) {
	results := make([]ws.ConnectivityResult, 0)
	add := func(r ws.ConnectivityResult) { results = append(results, r) }
	switch venue {
	case "all":
		add(ws.ProbeConnectivity(hyperliquid.DefaultWSConfig()))
		add(ws.ProbeConnectivity(aster.DefaultWSConfig()))
		add(ws.ProbeConnectivity(lighter.DefaultWSConfig()))
	case "hyperliquid":
		add(ws.ProbeConnectivity(hyperliquid.DefaultWSConfig()))
	case "aster":
		add(ws.ProbeConnectivity(aster.DefaultWSConfig()))
	case "lighter":
		add(ws.ProbeConnectivity(lighter.DefaultWSConfig()))
	default:
		fmt.Printf("unknown venue %q for ws check\n", venue)
		os.Exit(2)
	}
	okCount := 0
	for _, r := range results {
		fmt.Println(ws.FormatConnectivity(r))
		if r.OK {
			okCount++
		}
	}
	fmt.Printf("ws_summary ok=%d total=%d\n", okCount, len(results))
	if okCount != len(results) {
		os.Exit(1)
	}
}

func runAccountStreamChecks(venue string) {
	results := make([]ws.ConnectivityResult, 0)
	readiness := make([]accountstream.ReadinessResult, 0)
	asterSelected := false
	add := func(v string) {
		readiness = append(readiness, accountstream.ProbeReadiness(v))
		results = append(results, accountstream.ProbeConnectivity(v))
		if v == "aster" {
			asterSelected = true
		}
	}
	switch venue {
	case "all":
		add("hyperliquid")
		add("aster")
		add("lighter")
	case "hyperliquid", "aster", "lighter":
		add(venue)
	default:
		fmt.Printf("unknown venue %q for account-stream check\n", venue)
		os.Exit(2)
	}

	readyCount := 0
	for _, r := range readiness {
		fmt.Println(r.String())
		if r.Ready {
			readyCount++
		}
	}
	okCount := 0
	for _, r := range results {
		fmt.Println(ws.FormatConnectivity(r))
		if r.OK {
			okCount++
		}
	}
	fmt.Printf("account_stream_summary ready=%d/%d ws_ok=%d/%d\n", readyCount, len(readiness), okCount, len(results))
	if readyCount != len(readiness) || okCount != len(results) {
		os.Exit(1)
	}
	if asterSelected {
		if err := runAsterListenKeyLifecycleSmoke(); err != nil {
			fmt.Printf("aster_listenkey_lifecycle_failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("aster_listenkey_lifecycle_ok")
	}
}

func runAsterListenKeyLifecycleSmoke() error {
	chainID := int64(1666)
	if raw := strings.TrimSpace(os.Getenv("ASTER_CHAIN_ID")); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil {
			chainID = parsed
		}
	}
	signer := strings.TrimSpace(os.Getenv("ASTER_SIGNER"))
	if signer == "" {
		signer = strings.TrimSpace(os.Getenv("ASTER_SIGNER_ADDRESS"))
	}
	priv := strings.TrimSpace(os.Getenv("ASTER_PRIVATE_KEY"))
	if priv == "" {
		priv = strings.TrimSpace(os.Getenv("ASTER_SIGNER_PRIVATE_KEY"))
	}
	cli, err := aster.NewClient(aster.Config{
		BaseURL:    strings.TrimSpace(os.Getenv("ASTER_BASE_URL")),
		User:       strings.TrimSpace(os.Getenv("ASTER_USER")),
		Signer:     signer,
		PrivateKey: priv,
		ChainID:    chainID,
		APIKey:     strings.TrimSpace(os.Getenv("ASTER_API_KEY")),
		APISecret:  strings.TrimSpace(os.Getenv("ASTER_API_SECRET")),
	})
	if err != nil {
		return fmt.Errorf("init: %w", err)
	}
	lk, err := cli.StartUserDataStream()
	if err != nil {
		return fmt.Errorf("start_listenkey: %w", err)
	}
	if err := cli.KeepaliveUserDataStream(lk); err != nil {
		return fmt.Errorf("keepalive_listenkey: %w", err)
	}
	if err := cli.CloseUserDataStream(lk); err != nil {
		return fmt.Errorf("close_listenkey: %w", err)
	}
	return nil
}

func runCoinbaseBalanceCheck(exitOnErr bool) {
	fmt.Println("=== COINBASE (SPOT ONLY) ===")
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("COINBASE_SPOT_ONLY")), "true") {
		fmt.Println("warning: COINBASE_SPOT_ONLY not set to true")
	}
	baseURL := strings.TrimSpace(os.Getenv("COINBASE_BASE_URL"))
	cli := coinbase.NewClient(baseURL)
	timePayload, err := cli.Time()
	if err != nil {
		fmt.Printf("COINBASE connectivity failed: %v\n", err)
		if exitOnErr {
			os.Exit(1)
		}
		return
	}
	fmt.Printf("connectivity_ok time=%s\n", strings.TrimSpace(timePayload))

	key := strings.TrimSpace(os.Getenv("COINBASE_API_KEY"))
	sec := strings.TrimSpace(os.Getenv("COINBASE_API_SECRET"))
	pass := strings.TrimSpace(os.Getenv("COINBASE_API_PASSPHRASE"))
	if key == "" || sec == "" || pass == "" {
		fmt.Println("spot_balance=not_checked (missing COINBASE_API_KEY/SECRET/PASSPHRASE)")
		return
	}

	authCli := coinbase.NewClientWithAuth(baseURL, key, sec, pass)
	accounts, err := authCli.ListAccounts()
	if err != nil {
		fmt.Printf("spot_balance_error=%v\n", err)
		if exitOnErr {
			os.Exit(1)
		}
		return
	}

	priceCache := map[string]float64{
		"USD":  1.0,
		"USDC": 1.0,
		"USDT": 1.0,
	}
	getUSDPrice := func(cur string) (float64, bool) {
		cur = strings.ToUpper(strings.TrimSpace(cur))
		if p, ok := priceCache[cur]; ok {
			return p, true
		}
		pairs := []string{cur + "-USD", cur + "-USDC", cur + "-USDT"}
		for _, p := range pairs {
			px, err := authCli.ProductTicker(p)
			if err == nil && px > 0 {
				priceCache[cur] = px
				return px, true
			}
		}
		return 0, false
	}

	type row struct {
		currency string
		balance  string
		avail    string
		estUSD   float64
	}
	rows := make([]row, 0)
	unpriced := 0
	for _, a := range accounts {
		rat, ok := new(big.Rat).SetString(strings.TrimSpace(a.Balance))
		if !ok || rat.Sign() == 0 {
			continue
		}
		f, _ := rat.Float64()
		price, ok := getUSDPrice(a.Currency)
		if !ok {
			unpriced++
			continue
		}
		est := f * price
		if est >= 0.01 {
			rows = append(rows, row{currency: a.Currency, balance: a.Balance, avail: a.Available, estUSD: est})
		}
	}

	fmt.Printf("spot_accounts=%d tokens_ge_0.01_usd=%d unpriced_nonzero=%d\n", len(accounts), len(rows), unpriced)
	for _, r := range rows {
		fmt.Printf("%s balance=%s available=%s est_usd=%.6f\n", r.currency, r.balance, r.avail, r.estUSD)
	}
}

func runHyperliquidBalanceCheck(exitOnErr bool) {
	fmt.Println("=== HYPERLIQUID ===")
	addr := strings.TrimSpace(os.Getenv("HYPERLIQUID_ACCOUNT_ADDRESS"))
	if addr == "" {
		fmt.Println("HYPERLIQUID_ACCOUNT_ADDRESS missing")
		if exitOnErr {
			os.Exit(1)
		}
		return
	}
	cli := hyperliquid.NewClient(strings.TrimSpace(os.Getenv("HYPERLIQUID_BASE_URL")))
	state, err := cli.ClearinghouseState(addr)
	if err != nil {
		fmt.Printf("HYPERLIQUID check failed: %v\n", err)
		if exitOnErr {
			os.Exit(1)
		}
		return
	}
	ms, _ := state["marginSummary"].(map[string]any)
	cs, _ := state["crossMarginSummary"].(map[string]any)
	positions, _ := state["assetPositions"].([]any)
	fmt.Printf("perp_only account_value=%v total_ntl_pos=%v total_raw_usd=%v positions=%d\n", ms["accountValue"], ms["totalNtlPos"], cs["totalRawUsd"], len(positions))

	spot, err := cli.SpotClearinghouseState(addr)
	if err != nil {
		fmt.Printf("spot_state_error=%v\n", err)
		if exitOnErr {
			os.Exit(1)
		}
		return
	}
	bals, _ := spot["balances"].([]any)
	nonZero := 0
	spotUSDEst := 0.0
	for _, row := range bals {
		m, ok := row.(map[string]any)
		if !ok {
			continue
		}
		coin := fmt.Sprint(m["coin"])
		total := strings.TrimSpace(fmt.Sprint(m["total"]))
		hold := strings.TrimSpace(fmt.Sprint(m["hold"]))
		if rat, ok := new(big.Rat).SetString(total); ok && rat.Sign() != 0 {
			nonZero++
			f, _ := rat.Float64()
			if strings.EqualFold(coin, "USDC") || strings.EqualFold(coin, "USDT") || strings.EqualFold(coin, "USD") {
				spotUSDEst += f
			}
			fmt.Printf("spot %s total=%s hold=%s\n", coin, total, hold)
		}
	}
	fmt.Printf("spot balances=%d nonzero=%d\n", len(bals), nonZero)
	fmt.Printf("portfolio_est_usd spot_stables=%.6f note=perp_only_account_value_excludes_spot\n", spotUSDEst)
}

func runAsterBalanceCheck(exitOnErr bool) {
	fmt.Println("=== ASTER ===")
	chainID := int64(1666)
	if raw := strings.TrimSpace(os.Getenv("ASTER_CHAIN_ID")); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil {
			chainID = parsed
		}
	}

	signer := strings.TrimSpace(os.Getenv("ASTER_SIGNER"))
	if signer == "" {
		signer = strings.TrimSpace(os.Getenv("ASTER_SIGNER_ADDRESS"))
	}
	priv := strings.TrimSpace(os.Getenv("ASTER_PRIVATE_KEY"))
	if priv == "" {
		priv = strings.TrimSpace(os.Getenv("ASTER_SIGNER_PRIVATE_KEY"))
	}

	cli, err := aster.NewClient(aster.Config{
		BaseURL:    strings.TrimSpace(os.Getenv("ASTER_BASE_URL")),
		User:       strings.TrimSpace(os.Getenv("ASTER_USER")),
		Signer:     signer,
		PrivateKey: priv,
		ChainID:    chainID,
		APIKey:     strings.TrimSpace(os.Getenv("ASTER_API_KEY")),
		APISecret:  strings.TrimSpace(os.Getenv("ASTER_API_SECRET")),
	})
	if err != nil {
		fmt.Printf("ASTER init failed: %v\n", err)
		if exitOnErr {
			os.Exit(1)
		}
		return
	}

	if err := cli.Ping(); err != nil {
		fmt.Printf("ASTER ping failed: %v\n", err)
		if exitOnErr {
			os.Exit(1)
		}
		return
	}

	acct, err := cli.GetAccountSummaryRaw()
	if err != nil {
		fmt.Printf("ASTER account failed: %v\n", err)
		if exitOnErr {
			os.Exit(1)
		}
		return
	}
	bals, err := cli.GetBalanceRaw()
	if err != nil {
		fmt.Printf("ASTER balance failed: %v\n", err)
		if exitOnErr {
			os.Exit(1)
		}
		return
	}

	nonZero := 0
	usdtBal, usdtAvail := "not_found", "not_found"
	usdcBal, usdcAvail := "not_found", "not_found"
	for _, r := range bals {
		asset := fmt.Sprint(r["asset"])
		bal := fmt.Sprint(r["balance"])
		avail := fmt.Sprint(r["availableBalance"])
		if rat, ok := new(big.Rat).SetString(strings.TrimSpace(bal)); ok && rat.Sign() != 0 {
			nonZero++
		}
		if asset == "USDT" {
			usdtBal, usdtAvail = bal, avail
		}
		if asset == "USDC" {
			usdcBal, usdcAvail = bal, avail
		}
	}
	positions := 0
	openPositions := 0
	if p, ok := acct["positions"].([]any); ok {
		positions = len(p)
		for _, row := range p {
			m, ok := row.(map[string]any)
			if !ok {
				continue
			}
			pos := strings.TrimSpace(fmt.Sprint(m["position"]))
			if rat, ok := new(big.Rat).SetString(pos); ok && rat.Sign() != 0 {
				openPositions++
			}
		}
	}
	fmt.Printf("account_keys=%d balances=%d nonzero=%d positions=%d open_positions=%d\n", len(acct), len(bals), nonZero, positions, openPositions)
	fmt.Printf("USDT balance=%s available=%s\n", usdtBal, usdtAvail)
	fmt.Printf("USDC balance=%s available=%s\n", usdcBal, usdcAvail)
}

func runLighterBalanceCheck(exitOnErr bool) {
	fmt.Println("=== LIGHTER ===")
	index := strings.TrimSpace(os.Getenv("LIGHTER_ACCOUNT_INDEX"))
	addr := strings.TrimSpace(os.Getenv("LIGHTER_L1_ADDRESS"))
	if addr == "" {
		addr = strings.TrimSpace(os.Getenv("HYPERLIQUID_ACCOUNT_ADDRESS"))
	}
	cli := lighter.NewClient(strings.TrimSpace(os.Getenv("LIGHTER_BASE_URL")))
	if err := cli.Health(); err != nil {
		fmt.Printf("LIGHTER health failed: %v\n", err)
		if exitOnErr {
			os.Exit(1)
		}
		return
	}
	var acc *lighter.AccountByL1Response
	var err error
	if index != "" {
		acc, err = cli.AccountByIndex(index)
		if err == nil {
			fmt.Printf("lookup=by:index value=%s\n", index)
		}
	}
	if err != nil || acc == nil {
		if addr == "" {
			fmt.Println("LIGHTER_ACCOUNT_INDEX missing and no LIGHTER_L1_ADDRESS/HYPERLIQUID_ACCOUNT_ADDRESS fallback")
			if exitOnErr {
				os.Exit(1)
			}
			return
		}
		acc, err = cli.AccountByL1(addr)
		if err == nil {
			fmt.Printf("lookup=by:l1_address value=%s\n", addr)
		}
	}
	if err != nil {
		fmt.Printf("LIGHTER account failed: %v\n", err)
		if exitOnErr {
			os.Exit(1)
		}
		return
	}
	fmt.Printf("accounts=%d total=%d\n", len(acc.Accounts), acc.Total)
	for i, a := range acc.Accounts {
		if i >= 3 {
			break
		}
		nonZero := 0
		for _, p := range a.Positions {
			pos := strings.TrimSpace(p.Position)
			if rat, ok := new(big.Rat).SetString(pos); ok && rat.Sign() != 0 {
				nonZero++
			}
		}
		fmt.Printf("account_index=%d collateral=%s available=%s positions=%d nonzero_positions=%d\n", a.AccountIndex, a.Collateral, a.AvailableBalance, len(a.Positions), nonZero)
		for j, p := range a.Positions {
			if j >= 5 {
				break
			}
			fmt.Printf("  lighter_pos symbol=%s position=%s unrealized_pnl=%s\n", p.Symbol, p.Position, p.UnrealizedPnl)
		}
	}
}
