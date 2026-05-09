package tests

import (
	"os"
	"strconv"
	"strings"
	"testing"

	aster "github.com/SpaceCadetOG/VWAP-Scalper/internal/adapters/aster"
	hyperliquid "github.com/SpaceCadetOG/VWAP-Scalper/internal/adapters/hyperliquid"
	lighter "github.com/SpaceCadetOG/VWAP-Scalper/internal/adapters/lighter"
)

func TestAcceptanceContracts(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping acceptance contracts in short mode")
	}
	if !envBool("ACCEPTANCE_LIVE", false) {
		t.Skip("set ACCEPTANCE_LIVE=true to run live contract acceptance tests")
	}

	t.Run("hyperliquid_perps_read_contract", testHyperliquidAcceptance)
	t.Run("aster_pro_api_wallet_contract", testAsterAcceptance)
	t.Run("lighter_account_contract", testLighterAcceptance)
}

func testHyperliquidAcceptance(t *testing.T) {
	user := strings.ToLower(strings.TrimSpace(os.Getenv("HYPERLIQUID_ACCOUNT_ADDRESS")))
	if user == "" {
		t.Skip("HYPERLIQUID_ACCOUNT_ADDRESS is required")
	}
	cli := hyperliquid.NewClient(strings.TrimSpace(os.Getenv("HYPERLIQUID_BASE_URL")))
	state, err := cli.ClearinghouseState(user)
	if err != nil {
		t.Fatalf("clearinghouseState failed: %v", err)
	}
	if _, ok := state["marginSummary"]; !ok {
		t.Fatalf("missing marginSummary in response")
	}
}

func testAsterAcceptance(t *testing.T) {
	chainID := int64(1666)
	if raw := strings.TrimSpace(os.Getenv("ASTER_CHAIN_ID")); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
			chainID = n
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
	cfg := aster.Config{
		BaseURL:    strings.TrimSpace(os.Getenv("ASTER_BASE_URL")),
		User:       strings.TrimSpace(os.Getenv("ASTER_USER")),
		Signer:     signer,
		PrivateKey: priv,
		ChainID:    chainID,
		APIKey:     strings.TrimSpace(os.Getenv("ASTER_API_KEY")),
		APISecret:  strings.TrimSpace(os.Getenv("ASTER_API_SECRET")),
	}
	if strings.TrimSpace(cfg.User) == "" || strings.TrimSpace(cfg.Signer) == "" || strings.TrimSpace(cfg.PrivateKey) == "" {
		t.Skip("ASTER_USER + ASTER_SIGNER(ADDRESS) + ASTER_PRIVATE_KEY(SIGNER_PRIVATE_KEY) required")
	}
	cli, err := aster.NewClient(cfg)
	if err != nil {
		t.Fatalf("aster client init failed: %v", err)
	}
	if err := cli.Ping(); err != nil {
		t.Fatalf("aster ping failed: %v", err)
	}
	if _, err := cli.ServerTime(); err != nil {
		t.Fatalf("aster server time failed: %v", err)
	}
	lk, err := cli.StartUserDataStream()
	if err != nil {
		t.Fatalf("aster listenKey start failed: %v", err)
	}
	if err := cli.KeepaliveUserDataStream(lk); err != nil {
		t.Fatalf("aster listenKey keepalive failed: %v", err)
	}
	if err := cli.CloseUserDataStream(lk); err != nil {
		t.Fatalf("aster listenKey close failed: %v", err)
	}
}

func testLighterAcceptance(t *testing.T) {
	cli := lighter.NewClient(strings.TrimSpace(os.Getenv("LIGHTER_BASE_URL")))
	if err := cli.Health(); err != nil {
		t.Fatalf("lighter health failed: %v", err)
	}

	idx := strings.TrimSpace(os.Getenv("LIGHTER_ACCOUNT_INDEX"))
	if idx == "" {
		t.Skip("LIGHTER_ACCOUNT_INDEX not set; skipping account-level contract check")
	}
	out, err := cli.AccountByIndex(idx)
	if err != nil {
		t.Fatalf("lighter account by index failed: %v", err)
	}
	if out == nil {
		t.Fatalf("lighter account response nil")
	}
}

func envBool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return strings.EqualFold(v, "true") || v == "1" || strings.EqualFold(v, "yes")
}
