package hyperliquid

import (
	"bytes"
	"encoding/json"
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
		base = "https://api.hyperliquid.xyz"
	}
	base = strings.TrimRight(base, "/")
	return &Client{baseURL: base, http: &http.Client{Timeout: 12 * time.Second}}
}

type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string { return fmt.Sprintf("http %d: %s", e.StatusCode, e.Body) }

func (c *Client) info(payload any) ([]byte, error) {
	b, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/info", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(out)}
	}
	return out, nil
}

func (c *Client) ClearinghouseState(user string) (map[string]any, error) {
	b, err := c.info(map[string]any{"type": "clearinghouseState", "user": user})
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) SpotClearinghouseState(user string) (map[string]any, error) {
	b, err := c.info(map[string]any{"type": "spotClearinghouseState", "user": user})
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}
