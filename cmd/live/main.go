package main

import (
	"flag"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"strconv"
	"strings"

	aster "github.com/SpaceCadetOG/VWAP-Scalper/internal/adapters/aster"
	coinbase "github.com/SpaceCadetOG/VWAP-Scalper/internal/adapters/coinbase"
	hyperliquid "github.com/SpaceCadetOG/VWAP-Scalper/internal/adapters/hyperliquid"
	lighter "github.com/SpaceCadetOG/VWAP-Scalper/internal/adapters/lighter"
	ws "github.com/SpaceCadetOG/VWAP-Scalper/internal/adapters/ws"
)

func main() {
	checkBalances := flag.Bool("check-balances", false, "run venue balance connectivity checks")
	checkWS := flag.Bool("check-ws", false, "run websocket connectivity checks (WS-first health)")
	testTrade := flag.Bool("test-trade", false, "place a small live test trade and collect order/fill details")
	venue := flag.String("venue", "all", "venue to check (all|aster|hyperliquid|lighter|coinbase)")
	symbol := flag.String("symbol", "ETHUSDT", "trade symbol for --test-trade")
	notionalUSD := flag.Float64("notional-usd", 5.0, "target notional USD for --test-trade")
	flag.Parse()

	if *testTrade {
		runTestTrade(strings.ToLower(strings.TrimSpace(*venue)), strings.ToUpper(strings.TrimSpace(*symbol)), *notionalUSD)
		return
	}

	if *checkWS {
		runWSChecks(strings.ToLower(strings.TrimSpace(*venue)))
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

func runTestTrade(venue, symbol string, notionalUSD float64) {
	switch venue {
	case "aster":
		runAsterTestTrade(symbol, notionalUSD)
	default:
		fmt.Printf("test-trade currently implemented for venue=aster only; requested=%s\n", venue)
		os.Exit(2)
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
	// conservative precision for test path; exchange will enforce final precision constraints.
	qty := fmt.Sprintf("%.4f", rawQty)
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
	limitPrice := price * 0.5
	limitVals := map[string]string{
		"symbol":           symbol,
		"side":             "BUY",
		"type":             "LIMIT",
		"timeInForce":      "GTC",
		"positionSide":     "BOTH",
		"quantity":         qty,
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
		if exitOnErr { os.Exit(1) }
		return
	}
	cli := hyperliquid.NewClient(strings.TrimSpace(os.Getenv("HYPERLIQUID_BASE_URL")))
	state, err := cli.ClearinghouseState(addr)
	if err != nil {
		fmt.Printf("HYPERLIQUID check failed: %v\n", err)
		if exitOnErr { os.Exit(1) }
		return
	}
	ms, _ := state["marginSummary"].(map[string]any)
	cs, _ := state["crossMarginSummary"].(map[string]any)
	positions, _ := state["assetPositions"].([]any)
	fmt.Printf("perp account_value=%v total_ntl_pos=%v total_raw_usd=%v positions=%d\n", ms["accountValue"], ms["totalNtlPos"], cs["totalRawUsd"], len(positions))

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
			fmt.Printf("spot %s total=%s hold=%s\n", coin, total, hold)
		}
	}
	fmt.Printf("spot balances=%d nonzero=%d\n", len(bals), nonZero)
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
		if exitOnErr { os.Exit(1) }
		return
	}

	if err := cli.Ping(); err != nil {
		fmt.Printf("ASTER ping failed: %v\n", err)
		if exitOnErr { os.Exit(1) }
		return
	}

	acct, err := cli.GetAccountSummaryRaw()
	if err != nil {
		fmt.Printf("ASTER account failed: %v\n", err)
		if exitOnErr { os.Exit(1) }
		return
	}
	bals, err := cli.GetBalanceRaw()
	if err != nil {
		fmt.Printf("ASTER balance failed: %v\n", err)
		if exitOnErr { os.Exit(1) }
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
		if exitOnErr { os.Exit(1) }
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
			if exitOnErr { os.Exit(1) }
			return
		}
		acc, err = cli.AccountByL1(addr)
		if err == nil {
			fmt.Printf("lookup=by:l1_address value=%s\n", addr)
		}
	}
	if err != nil {
		fmt.Printf("LIGHTER account failed: %v\n", err)
		if exitOnErr { os.Exit(1) }
		return
	}
	fmt.Printf("accounts=%d total=%d\n", len(acc.Accounts), acc.Total)
	for i, a := range acc.Accounts {
		if i >= 3 {
			break
		}
		fmt.Printf("account_index=%d collateral=%s available=%s positions=%d\n", a.AccountIndex, a.Collateral, a.AvailableBalance, len(a.Positions))
	}
}
