package tests

import (
	"sync"
	"testing"

	"github.com/SpaceCadetOG/VWAP-Scalper/internal/adapters/signing"
)

func TestNonceManagerMonotonicSingleKey(t *testing.T) {
	nm := signing.NewNonceManager(func() int64 { return 1000 })
	a := nm.Next("k")
	b := nm.Next("k")
	c := nm.Next("k")
	if !(a < b && b < c) {
		t.Fatalf("expected monotonic increase, got %d %d %d", a, b, c)
	}
}

func TestNonceManagerConcurrentMonotonic(t *testing.T) {
	nm := signing.NewNonceManager(func() int64 { return 2000 })
	const n = 200
	vals := make([]int64, n)
	wg := sync.WaitGroup{}
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			vals[i] = nm.Next("k")
		}()
	}
	wg.Wait()
	seen := map[int64]struct{}{}
	for _, v := range vals {
		if _, ok := seen[v]; ok {
			t.Fatalf("duplicate nonce %d", v)
		}
		seen[v] = struct{}{}
	}
}
