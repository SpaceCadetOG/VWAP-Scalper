package coinbase

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	http    *http.Client
	apiKey  string
	secret  string
	pass    string
}

func NewClient(baseURL string) *Client {
	return NewClientWithAuth(baseURL, "", "", "")
}

func NewClientWithAuth(baseURL, apiKey, apiSecret, passphrase string) *Client {
	base := strings.TrimSpace(baseURL)
	if base == "" {
		base = "https://api.exchange.coinbase.com"
	}
	base = strings.TrimRight(base, "/")
	return &Client{
		baseURL: base,
		http:    &http.Client{Timeout: 12 * time.Second},
		apiKey:  strings.TrimSpace(apiKey),
		secret:  strings.TrimSpace(apiSecret),
		pass:    strings.TrimSpace(passphrase),
	}
}

func (c *Client) Time() (string, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/time", nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("http %d: %s", resp.StatusCode, string(b))
	}
	return string(b), nil
}

type Account struct {
	ID        string `json:"id"`
	Currency  string `json:"currency"`
	Balance   string `json:"balance"`
	Available string `json:"available"`
	Hold      string `json:"hold"`
	ProfileID string `json:"profile_id"`
}

func (c *Client) ListAccounts() ([]Account, error) {
	b, err := c.authGET("/accounts")
	if err != nil {
		return nil, err
	}
	var out []Account
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) ProductTicker(productID string) (float64, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/products/"+productID+"/ticker", nil)
	if err != nil {
		return 0, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return 0, fmt.Errorf("http %d: %s", resp.StatusCode, string(b))
	}
	var out struct {
		Price string `json:"price"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return 0, err
	}
	if out.Price == "" {
		return 0, fmt.Errorf("empty price for %s", productID)
	}
	var p float64
	_, err = fmt.Sscanf(out.Price, "%f", &p)
	if err != nil || p <= 0 || math.IsNaN(p) || math.IsInf(p, 0) {
		return 0, fmt.Errorf("invalid price for %s: %s", productID, out.Price)
	}
	return p, nil
}

func (c *Client) authGET(path string) ([]byte, error) {
	if c.apiKey == "" || c.secret == "" || c.pass == "" {
		return nil, fmt.Errorf("missing coinbase auth credentials")
	}
	ts := fmt.Sprintf("%d", time.Now().Unix())
	prehash := ts + http.MethodGet + path

	key, err := base64.StdEncoding.DecodeString(c.secret)
	if err != nil {
		return nil, fmt.Errorf("invalid coinbase secret base64: %w", err)
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(prehash))
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("CB-ACCESS-KEY", c.apiKey)
	req.Header.Set("CB-ACCESS-SIGN", sig)
	req.Header.Set("CB-ACCESS-TIMESTAMP", ts)
	req.Header.Set("CB-ACCESS-PASSPHRASE", c.pass)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, string(b))
	}
	return b, nil
}
