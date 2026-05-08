package tests

import (
	"testing"

	"github.com/SpaceCadetOG/VWAP-Scalper/internal/models"
)

func TestSymbolRegistryUpsertGet(t *testing.T) {
	r := models.NewSymbolRegistry()
	spec := models.SymbolSpec{
		Venue:       models.VenueHyperliquid,
		Symbol:      "BTC",
		TickSize:    "0.1",
		LotSize:     "0.001",
		MinNotional: "10",
		MinQty:      "0.001",
	}
	r.Upsert("BTC-USD-PERP", spec)

	got, ok := r.Get("btc-usd-perp", models.VenueHyperliquid)
	if !ok {
		t.Fatalf("expected spec to exist")
	}
	if got.Symbol != "BTC" {
		t.Fatalf("unexpected symbol: %s", got.Symbol)
	}
}

func TestValidateOrderInputOK(t *testing.T) {
	spec := models.SymbolSpec{
		TickSize:    "0.1",
		LotSize:     "0.001",
		MinNotional: "10",
		MinQty:      "0.001",
		MaxQty:      "100",
	}
	if err := models.ValidateOrderInput(spec, "0.01", "1000.0"); err != nil {
		t.Fatalf("expected valid order, got error: %v", err)
	}
}

func TestValidateOrderInputRejectsLotStep(t *testing.T) {
	spec := models.SymbolSpec{TickSize: "0.1", LotSize: "0.001", MinNotional: "10", MinQty: "0.001"}
	if err := models.ValidateOrderInput(spec, "0.0015", "1000.0"); err == nil {
		t.Fatalf("expected lot size validation error")
	}
}

func TestValidateOrderInputRejectsTickStep(t *testing.T) {
	spec := models.SymbolSpec{TickSize: "0.1", LotSize: "0.001", MinNotional: "10", MinQty: "0.001"}
	if err := models.ValidateOrderInput(spec, "0.01", "1000.05"); err == nil {
		t.Fatalf("expected tick size validation error")
	}
}

func TestValidateOrderInputRejectsMinNotional(t *testing.T) {
	spec := models.SymbolSpec{TickSize: "0.1", LotSize: "0.001", MinNotional: "10", MinQty: "0.001"}
	if err := models.ValidateOrderInput(spec, "0.001", "1000.0"); err == nil {
		t.Fatalf("expected min notional validation error")
	}
}
