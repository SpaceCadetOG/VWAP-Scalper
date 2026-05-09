package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestAsterSymbolEligible(t *testing.T) {
	if !asterSymbolEligible("TRADING", "PERPETUAL") {
		t.Fatalf("expected trading perpetual symbol to be eligible")
	}
	if asterSymbolEligible("BREAK", "PERPETUAL") {
		t.Fatalf("expected non-trading symbol to be rejected")
	}
	if asterSymbolEligible("TRADING", "CURRENT_QUARTER") {
		t.Fatalf("expected non-perpetual contract to be rejected")
	}
}

func TestDiscoverAsterSymbolsFiltersToTradingPerps(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fapi/v1/exchangeInfo" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"symbols":[{"symbol":"BTCUSDT","status":"TRADING","contractType":"PERPETUAL"},{"symbol":"ETHUSDT","status":"BREAK","contractType":"PERPETUAL"},{"symbol":"SOLUSD","status":"TRADING","contractType":"PERPETUAL"},{"symbol":"XRPUSDT","status":"TRADING","contractType":"CURRENT_QUARTER"}]}`)
	}))
	defer ts.Close()

	got, err := discoverAsterSymbols(ts.URL)
	if err != nil {
		t.Fatalf("discoverAsterSymbols err=%v", err)
	}
	want := []string{"BTCUSDT"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected symbols: got=%v want=%v", got, want)
	}
}

func TestDiscoverLighterSymbolsFiltersStatuses(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/orderBooks" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"code":200,"message":"ok","order_books":[{"symbol":"BTCUSDT","market_id":1,"market_type":"perp","status":"active"},{"symbol":"ETHUSDT","market_id":2,"market_type":"perp","status":"halted"},{"symbol":"SOLUSDT","market_id":3,"market_type":"perp","status":"open"}]}`)
	}))
	defer ts.Close()

	got, err := discoverLighterSymbols(ts.URL)
	if err != nil {
		t.Fatalf("discoverLighterSymbols err=%v", err)
	}
	want := []string{"BTCUSDT", "SOLUSDT"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected symbols: got=%v want=%v", got, want)
	}
}

func TestDedupeSymbols(t *testing.T) {
	got := dedupeSymbols([]string{"btcusdt", "BTCUSDT", "ETHUSDT", ""})
	want := []string{"BTCUSDT", "ETHUSDT"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected symbols: got=%v want=%v", got, want)
	}
}

func TestLoadPaperUniverseConfigNoSourceVenue(t *testing.T) {
	t.Setenv("BOT_SYMBOL_SOURCE_MODE", "dynamic")
	t.Setenv("BOT_SYMBOLS_MAX", "25")
	cfg := loadPaperUniverseConfig("BTCUSDT", "BTCUSDT")
	if cfg.Mode != "dynamic" {
		t.Fatalf("unexpected mode: %s", cfg.Mode)
	}
	if cfg.MaxSymbols != 25 {
		t.Fatalf("unexpected max symbols: %d", cfg.MaxSymbols)
	}
}
