package main

import (
	"flag"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"

	aster "github.com/SpaceCadetOG/VWAP-Scalper/internal/adapters/aster"
	hyperliquid "github.com/SpaceCadetOG/VWAP-Scalper/internal/adapters/hyperliquid"
	lighter "github.com/SpaceCadetOG/VWAP-Scalper/internal/adapters/lighter"
)

func main() {
	checkBalances := flag.Bool("check-balances", false, "run venue balance connectivity checks")
	venue := flag.String("venue", "all", "venue to check (all|aster|hyperliquid|lighter)")
	flag.Parse()

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
	case "hyperliquid":
		runHyperliquidBalanceCheck(true)
	case "aster":
		runAsterBalanceCheck(true)
	case "lighter":
		runLighterBalanceCheck(true)
	default:
		fmt.Printf("unknown venue %q\n", *venue)
		os.Exit(2)
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
	fmt.Printf("account_value=%v total_ntl_pos=%v total_raw_usd=%v positions=%d\n", ms["accountValue"], ms["totalNtlPos"], cs["totalRawUsd"], len(positions))
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
	for _, r := range bals {
		bal := fmt.Sprint(r["balance"])
		if rat, ok := new(big.Rat).SetString(strings.TrimSpace(bal)); ok && rat.Sign() != 0 {
			nonZero++
		}
	}
	fmt.Printf("account_keys=%d balances=%d nonzero=%d\n", len(acct), len(bals), nonZero)
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
