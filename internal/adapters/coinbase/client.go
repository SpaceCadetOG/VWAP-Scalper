package coinbase

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string) *Client {
	base := strings.TrimSpace(baseURL)
	if base == "" {
		base = "https://api.exchange.coinbase.com"
	}
	base = strings.TrimRight(base, "/")
	return &Client{baseURL: base, http: &http.Client{Timeout: 12 * time.Second}}
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
