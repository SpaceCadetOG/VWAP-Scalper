package lighter

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
		base = "https://mainnet.zklighter.elliot.ai"
	}
	base = strings.TrimRight(base, "/")
	return &Client{
		baseURL: base,
		http:    &http.Client{Timeout: 12 * time.Second},
	}
}

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

func (c *Client) doGET(path string, query map[string]string) ([]byte, error) {
	u, err := url.Parse(c.baseURL + path)
	if err != nil {
		return nil, err
	}
	if query != nil {
		q := u.Query()
		for k, v := range query {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return nil, &APIError{StatusCode: resp.StatusCode, Method: http.MethodGet, Path: path, Body: string(b)}
	}
	return b, nil
}

func (c *Client) Health() error {
	_, err := c.doGET("/api/v1/publicPoolsMetadata", map[string]string{"index": "0", "limit": "1"})
	return err
}

type AccountPosition struct {
	Symbol        string `json:"symbol"`
	Position      string `json:"position"`
	UnrealizedPnl string `json:"unrealized_pnl"`
}

type Account struct {
	L1Address        string            `json:"l1_address"`
	AccountIndex     int64             `json:"account_index"`
	AvailableBalance string            `json:"available_balance"`
	Collateral       string            `json:"collateral"`
	TotalAssetValue  string            `json:"total_asset_value"`
	Positions        []AccountPosition `json:"positions"`
}

type AccountByL1Response struct {
	Code     int       `json:"code"`
	Total    int       `json:"total"`
	Accounts []Account `json:"accounts"`
}

func (c *Client) AccountByL1(addr string) (*AccountByL1Response, error) {
	b, err := c.doGET("/api/v1/account", map[string]string{"by": "l1_address", "value": addr})
	if err != nil {
		return nil, err
	}
	var out AccountByL1Response
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) AccountByIndex(index string) (*AccountByL1Response, error) {
	b, err := c.doGET("/api/v1/account", map[string]string{"by": "index", "value": index})
	if err != nil {
		return nil, err
	}
	var out AccountByL1Response
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
