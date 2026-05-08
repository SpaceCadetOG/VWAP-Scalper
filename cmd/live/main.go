package main

import (
	"flag"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"

	aster "github.com/SpaceCadetOG/VWAP-Scalper/internal/adapters/aster"
)

func main() {
	checkBalances := flag.Bool("check-balances", false, "run venue balance connectivity checks")
	venue := flag.String("venue", "aster", "venue to check (aster|hyperliquid|lighter)")
	flag.Parse()

	if !*checkBalances {
		fmt.Println("vwap-multi-venue-bot bootstrap")
		return
	}

	switch strings.ToLower(strings.TrimSpace(*venue)) {
	case "aster":
		runAsterBalanceCheck()
	default:
		fmt.Printf("balance check for venue %q not implemented yet\n", *venue)
		os.Exit(2)
	}
}

func runAsterBalanceCheck() {
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
		os.Exit(1)
	}

	if err := cli.Ping(); err != nil {
		fmt.Printf("ASTER ping failed: %v\n", err)
		os.Exit(1)
	}
	ts, err := cli.ServerTime()
	if err != nil {
		fmt.Printf("ASTER server time failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("ASTER ping ok, serverTime=%d\n", ts)

	acct, err := cli.GetAccountSummaryRaw()
	if err != nil {
		fmt.Printf("ASTER account failed: %v\n", err)
		os.Exit(1)
	}
	bals, err := cli.GetBalanceRaw()
	if err != nil {
		fmt.Printf("ASTER balance failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("ASTER account fetched: keys=%d\n", len(acct))
	fmt.Printf("ASTER balances fetched: rows=%d\n", len(bals))

	// Print compact balance preview.
	nonZero := 0
	for _, r := range bals {
		asset := fmt.Sprint(r["asset"])
		bal := fmt.Sprint(r["balance"])
		avail := fmt.Sprint(r["availableBalance"])
		if rat, ok := new(big.Rat).SetString(strings.TrimSpace(bal)); ok && rat.Sign() != 0 {
			nonZero++
		}
		fmt.Printf("ASTER balance asset=%s balance=%s available=%s\n", asset, bal, avail)
	}
	fmt.Printf("ASTER nonzero balance rows=%d\n", nonZero)
}
