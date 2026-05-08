package aster

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	gethmath "github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

type APIError struct {
	StatusCode int
	Method     string
	Path       string
	Body       string
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("http %d %s %s: %s", e.StatusCode, e.Method, e.Path, e.Body)
}

type Config struct {
	BaseURL    string
	User       string
	Signer     string
	PrivateKey string
	ChainID    int64
	APIKey     string
	APISecret  string
}

type Client struct {
	baseURL    string
	user       string
	signer     string
	privateKey string
	chainID    int64
	apiKey     string
	apiSecret  string
	client     *http.Client

	nonceMu   sync.Mutex
	lastNonce int64
}

func NewClient(cfg Config) (*Client, error) {
	base := strings.TrimSpace(cfg.BaseURL)
	if base == "" {
		base = "https://fapi.asterdex.com"
	}
	base = strings.TrimRight(base, "/")

	c := &Client{
		baseURL:    base,
		user:       strings.TrimSpace(cfg.User),
		signer:     strings.TrimSpace(cfg.Signer),
		privateKey: strings.TrimSpace(cfg.PrivateKey),
		chainID:    cfg.ChainID,
		apiKey:     strings.TrimSpace(cfg.APIKey),
		apiSecret:  strings.TrimSpace(cfg.APISecret),
		client:     &http.Client{Timeout: 12 * time.Second},
	}

	if c.chainID <= 0 {
		c.chainID = 1666
	}
	if c.user == "" || c.signer == "" || c.privateKey == "" {
		return nil, fmt.Errorf("aster agent auth requires user/signer/private_key")
	}
	if err := c.validateSignerMatchesPrivateKey(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Client) validateSignerMatchesPrivateKey() error {
	keyHex := strings.TrimPrefix(strings.TrimPrefix(c.privateKey, "0x"), "0X")
	priv, err := crypto.HexToECDSA(keyHex)
	if err != nil {
		return fmt.Errorf("invalid aster private key: %w", err)
	}
	derived := crypto.PubkeyToAddress(priv.PublicKey).Hex()
	if !strings.EqualFold(derived, c.signer) {
		return fmt.Errorf("signer_private_key_mismatch: derived=%s configured=%s", derived, c.signer)
	}
	return nil
}

func (c *Client) nextNonceUS() int64 {
	n := time.Now().UnixMicro()
	c.nonceMu.Lock()
	defer c.nonceMu.Unlock()
	if n <= c.lastNonce {
		n = c.lastNonce + 1
	}
	c.lastNonce = n
	return n
}

func (c *Client) signedGET(path string, vals url.Values) ([]byte, error) {
	if vals == nil {
		vals = url.Values{}
	}

	payload, err := c.signAndEncodeAgent(cloneValues(vals), true)
	if err != nil {
		return nil, err
	}

	u := c.baseURL + path + "?" + payload
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	b, err := c.do(req, http.MethodGet, path)
	if err == nil {
		return b, nil
	}
	apiErr, ok := err.(*APIError)
	if !ok || !strings.Contains(apiErr.Body, "Signature check failed") {
		return nil, err
	}

	// Signature fallback: alternate V-format (0/1 <-> 27/28).
	payload, err = c.signAndEncodeAgent(cloneValues(vals), false)
	if err != nil {
		return nil, err
	}
	u = c.baseURL + path + "?" + payload
	req, err = http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	return c.do(req, http.MethodGet, path)
}

func (c *Client) signAndEncodeAgent(vals url.Values, legacyV bool) (string, error) {
	nonce := strconv.FormatInt(c.nextNonceUS(), 10)
	norm := normalizeSignedValues(vals)
	norm.Set("nonce", nonce)
	norm.Set("user", c.user)
	norm.Set("signer", c.signer)
	msg := encodeCanonicalQuery(norm)

	sig, recovered, err := c.signAgent(msg, legacyV)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(recovered, c.signer) {
		return "", fmt.Errorf("signer_private_key_mismatch: recovered=%s configured=%s", recovered, c.signer)
	}

	if msg == "" {
		return "signature=" + url.QueryEscape(sig), nil
	}
	return msg + "&signature=" + url.QueryEscape(sig), nil
}

func normalizeSignedValues(vals url.Values) url.Values {
	out := url.Values{}
	for k, vv := range vals {
		key := strings.TrimSpace(k)
		if key == "" || key == "signature" {
			continue
		}
		clean := make([]string, 0, len(vv))
		for _, v := range vv {
			v = strings.TrimSpace(v)
			if v != "" {
				clean = append(clean, v)
			}
		}
		if len(clean) == 0 {
			continue
		}
		sort.Strings(clean)
		out[key] = clean
	}
	return out
}

func encodeCanonicalQuery(vals url.Values) string {
	if len(vals) == 0 {
		return ""
	}
	keys := make([]string, 0, len(vals))
	for k := range vals {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		ek := url.QueryEscape(k)
		for _, v := range vals[k] {
			parts = append(parts, ek+"="+url.QueryEscape(v))
		}
	}
	return strings.Join(parts, "&")
}

func (c *Client) signAgent(msg string, legacyV bool) (string, string, error) {
	keyHex := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(c.privateKey, "0x"), "0X"))
	if keyHex == "" {
		return "", "", fmt.Errorf("empty private key")
	}
	priv, err := crypto.HexToECDSA(keyHex)
	if err != nil {
		return "", "", fmt.Errorf("invalid private key: %w", err)
	}
	hash, err := asterAgentTypedDataHash(msg, c.chainID)
	if err != nil {
		return "", "", err
	}
	sig, err := crypto.Sign(hash, priv)
	if err != nil {
		return "", "", fmt.Errorf("sign agent payload: %w", err)
	}
	recovery := append([]byte(nil), sig...)
	pub, err := crypto.SigToPub(hash, recovery)
	if err != nil {
		return "", "", fmt.Errorf("recover signer: %w", err)
	}
	recovered := crypto.PubkeyToAddress(*pub).Hex()
	if legacyV {
		sig[64] += 27
	}
	return "0x" + hex.EncodeToString(sig), recovered, nil
}

func asterAgentTypedDataHash(msg string, chainID int64) ([]byte, error) {
	if strings.TrimSpace(msg) == "" {
		return nil, fmt.Errorf("agent signing message cannot be empty")
	}
	if chainID <= 0 {
		return nil, fmt.Errorf("agent signing chain id must be positive")
	}
	typedData := apitypes.TypedData{
		Types: apitypes.Types{
			"EIP712Domain": []apitypes.Type{
				{Name: "name", Type: "string"},
				{Name: "version", Type: "string"},
				{Name: "chainId", Type: "uint256"},
				{Name: "verifyingContract", Type: "address"},
			},
			"Message": []apitypes.Type{{Name: "msg", Type: "string"}},
		},
		PrimaryType: "Message",
		Domain: apitypes.TypedDataDomain{
			Name:              "AsterSignTransaction",
			Version:           "1",
			ChainId:           gethmath.NewHexOrDecimal256(chainID),
			VerifyingContract: "0x0000000000000000000000000000000000000000",
		},
		Message: apitypes.TypedDataMessage{"msg": msg},
	}
	hash, _, err := apitypes.TypedDataAndHash(typedData)
	if err != nil {
		return nil, fmt.Errorf("hash agent typed data: %w", err)
	}
	return hash, nil
}

func (c *Client) do(req *http.Request, method, path string) ([]byte, error) {
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return nil, &APIError{StatusCode: resp.StatusCode, Method: method, Path: path, Body: string(b)}
	}
	return b, nil
}

func cloneValues(v url.Values) url.Values {
	if v == nil {
		return url.Values{}
	}
	out := make(url.Values, len(v))
	for k, vv := range v {
		cp := make([]string, len(vv))
		copy(cp, vv)
		out[k] = cp
	}
	return out
}

func decodeJSONNumbers[T any](b []byte, out *T) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	return dec.Decode(out)
}

func (c *Client) Ping() error {
	for _, p := range []string{"/fapi/v3/ping", "/fapi/v1/ping"} {
		u := c.baseURL + p
		req, _ := http.NewRequest(http.MethodGet, u, nil)
		if _, err := c.do(req, http.MethodGet, p); err == nil {
			return nil
		}
	}
	return fmt.Errorf("aster ping failed")
}

func (c *Client) ServerTime() (int64, error) {
	for _, p := range []string{"/fapi/v3/time", "/fapi/v1/time"} {
		u := c.baseURL + p
		req, _ := http.NewRequest(http.MethodGet, u, nil)
		b, err := c.do(req, http.MethodGet, p)
		if err != nil {
			continue
		}
		var out struct{ ServerTime int64 `json:"serverTime"` }
		if err := decodeJSONNumbers(b, &out); err == nil && out.ServerTime > 0 {
			return out.ServerTime, nil
		}
	}
	return 0, fmt.Errorf("unable to fetch server time")
}

func (c *Client) GetBalanceRaw() ([]map[string]any, error) {
	b, err := c.signedGET("/fapi/v3/balance", url.Values{})
	if err != nil {
		return nil, err
	}
	var rows []map[string]any
	if err := decodeJSONNumbers(b, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (c *Client) GetAccountSummaryRaw() (map[string]any, error) {
	b, err := c.signedGET("/fapi/v3/account", url.Values{})
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := decodeJSONNumbers(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) signedGETHMAC(path string, vals url.Values) ([]byte, error) {
	if vals == nil {
		vals = url.Values{}
	}
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	vals.Set("timestamp", ts)
	if vals.Get("recvWindow") == "" {
		vals.Set("recvWindow", "5000")
	}
	vals.Del("signature")
	qs := vals.Encode()
	mac := hmac.New(sha256.New, []byte(c.apiSecret))
	_, _ = mac.Write([]byte(qs))
	sig := hex.EncodeToString(mac.Sum(nil))
	payload := qs + "&signature=" + sig
	u := c.baseURL + path + "?" + payload
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-MBX-APIKEY", c.apiKey)
	return c.do(req, http.MethodGet, path)
}
