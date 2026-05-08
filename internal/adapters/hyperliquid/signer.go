package hyperliquid

import (
	"fmt"
	"strings"

	"github.com/SpaceCadetOG/VWAP-Scalper/internal/adapters/signing"
)

// SignerService provides Step-4 nonce/signing ownership for Hyperliquid API wallet usage.
type SignerService struct {
	signerAddress string
	privateKey    string
	nonces        *signing.NonceManager
}

func NewSignerService(signerAddress, privateKey string) (*SignerService, error) {
	sa := strings.TrimSpace(signerAddress)
	pk := strings.TrimSpace(privateKey)
	if sa == "" {
		return nil, fmt.Errorf("hyperliquid signer address required")
	}
	if pk == "" {
		return nil, fmt.Errorf("hyperliquid api wallet private key required")
	}
	return &SignerService{
		signerAddress: strings.ToLower(sa),
		privateKey:    pk,
		nonces:        signing.NewMillisNonceManager(),
	}, nil
}

func (s *SignerService) NextNonce() int64 {
	return s.nonces.Next(s.signerAddress)
}

func (s *SignerService) SignerAddress() string { return s.signerAddress }
