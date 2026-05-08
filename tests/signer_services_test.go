package tests

import (
	"testing"

	hyperliquid "github.com/SpaceCadetOG/VWAP-Scalper/internal/adapters/hyperliquid"
	lighter "github.com/SpaceCadetOG/VWAP-Scalper/internal/adapters/lighter"
)

func TestHyperliquidSignerServiceValidation(t *testing.T) {
	if _, err := hyperliquid.NewSignerService("", "x"); err == nil {
		t.Fatalf("expected signer address validation error")
	}
	if _, err := hyperliquid.NewSignerService("0xabc", ""); err == nil {
		t.Fatalf("expected private key validation error")
	}
	s, err := hyperliquid.NewSignerService("0xAbC", "0xpriv")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	n1 := s.NextNonce()
	n2 := s.NextNonce()
	if n2 <= n1 {
		t.Fatalf("expected monotonic nonce")
	}
}

func TestLighterSignerServiceValidation(t *testing.T) {
	if _, err := lighter.NewSignerService("", "k", "s"); err == nil {
		t.Fatalf("expected api key index validation error")
	}
	if _, err := lighter.NewSignerService("1", "", "s"); err == nil {
		t.Fatalf("expected api key validation error")
	}
	s, err := lighter.NewSignerService("3", "k", "s")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	n1 := s.NextNonce()
	n2 := s.NextNonce()
	if n2 <= n1 {
		t.Fatalf("expected monotonic nonce")
	}
}
