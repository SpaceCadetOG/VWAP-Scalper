package lighter

import (
	"fmt"
	"strings"

	"github.com/SpaceCadetOG/VWAP-Scalper/internal/adapters/signing"
)

// SignerService provides Step-4 nonce/signing ownership for Lighter api key indexed flows.
type SignerService struct {
	apiKeyIndex string
	apiKey      string
	apiSecret   string
	nonces      *signing.NonceManager
}

func NewSignerService(apiKeyIndex, apiKey, apiSecret string) (*SignerService, error) {
	idx := strings.TrimSpace(apiKeyIndex)
	key := strings.TrimSpace(apiKey)
	sec := strings.TrimSpace(apiSecret)
	if idx == "" {
		return nil, fmt.Errorf("lighter api key index required")
	}
	if key == "" || sec == "" {
		return nil, fmt.Errorf("lighter api key and api secret required")
	}
	return &SignerService{
		apiKeyIndex: idx,
		apiKey:      key,
		apiSecret:   sec,
		nonces:      signing.NewMicrosNonceManager(),
	}, nil
}

func (s *SignerService) NextNonce() int64 {
	return s.nonces.Next(s.apiKeyIndex)
}

func (s *SignerService) APIKeyIndex() string { return s.apiKeyIndex }
