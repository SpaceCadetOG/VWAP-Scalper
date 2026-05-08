package signing

import (
	"sync"
	"time"
)

// Clock returns current time units used by a nonce source.
type Clock func() int64

// NonceManager provides monotonic nonce allocation per key (e.g., signer/apiKeyIndex).
type NonceManager struct {
	mu    sync.Mutex
	last  map[string]int64
	clock Clock
}

func NewNonceManager(clock Clock) *NonceManager {
	if clock == nil {
		clock = func() int64 { return time.Now().UnixMilli() }
	}
	return &NonceManager{last: make(map[string]int64), clock: clock}
}

func NewMillisNonceManager() *NonceManager {
	return NewNonceManager(func() int64 { return time.Now().UnixMilli() })
}

func NewMicrosNonceManager() *NonceManager {
	return NewNonceManager(func() int64 { return time.Now().UnixMicro() })
}

func (n *NonceManager) Next(key string) int64 {
	n.mu.Lock()
	defer n.mu.Unlock()
	v := n.clock()
	last := n.last[key]
	if v <= last {
		v = last + 1
	}
	n.last[key] = v
	return v
}

func (n *NonceManager) BumpAtLeast(key string, min int64) int64 {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.last[key] < min {
		n.last[key] = min
	}
	return n.last[key]
}
