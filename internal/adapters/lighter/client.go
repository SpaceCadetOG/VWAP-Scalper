package lighter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
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

func (c *Client) doPOSTJSON(path string, payload any) ([]byte, error) {
	u := c.baseURL + path
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(b))
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
		return nil, &APIError{StatusCode: resp.StatusCode, Method: http.MethodPost, Path: path, Body: string(out)}
	}
	return out, nil
}

func (c *Client) doPOSTForm(path string, vals url.Values) ([]byte, error) {
	u := c.baseURL + path
	req, err := http.NewRequest(http.MethodPost, u, strings.NewReader(vals.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return nil, &APIError{StatusCode: resp.StatusCode, Method: http.MethodPost, Path: path, Body: string(out)}
	}
	return out, nil
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

type APIKey struct {
	AccountIndex int64  `json:"account_index"`
	APIKeyIndex  uint8  `json:"api_key_index"`
	Nonce        int64  `json:"nonce"`
	PublicKey    string `json:"public_key"`
}

type APIKeysResponse struct {
	Code    int      `json:"code"`
	Message string   `json:"message"`
	APIKeys []APIKey `json:"api_keys"`
}

type OrderBookMeta struct {
	Symbol                 string `json:"symbol"`
	MarketID               int16  `json:"market_id"`
	MarketType             string `json:"market_type"`
	Status                 string `json:"status"`
	MinBaseAmount          string `json:"min_base_amount"`
	MinQuoteAmount         string `json:"min_quote_amount"`
	SupportedSizeDecimals  int    `json:"supported_size_decimals"`
	SupportedPriceDecimals int    `json:"supported_price_decimals"`
	SupportedQuoteDecimals int    `json:"supported_quote_decimals"`
}

type OrderBooksResponse struct {
	Code       int             `json:"code"`
	Message    string          `json:"message"`
	OrderBooks []OrderBookMeta `json:"order_books"`
}

type BookOrder struct {
	OrderIndex          int64  `json:"order_index"`
	OrderID             string `json:"order_id"`
	OwnerAccountIndex   int64  `json:"owner_account_index"`
	InitialBaseAmount   string `json:"initial_base_amount"`
	RemainingBaseAmount string `json:"remaining_base_amount"`
	Price               string `json:"price"`
	OrderExpiry         int64  `json:"order_expiry"`
	TransactionTime     int64  `json:"transaction_time"`
}

type OrderBookOrdersResponse struct {
	Code      int         `json:"code"`
	Message   string      `json:"message"`
	TotalAsks int64       `json:"total_asks"`
	Asks      []BookOrder `json:"asks"`
	TotalBids int64       `json:"total_bids"`
	Bids      []BookOrder `json:"bids"`
}

type SendTxResponse struct {
	Code                     int    `json:"code"`
	Message                  string `json:"message"`
	TxHash                   string `json:"tx_hash"`
	PredictedExecutionTimeMS int64  `json:"predicted_execution_time_ms"`
	VolumeQuotaRemaining     int64  `json:"volume_quota_remaining"`
}

type SendTxBatchResponse struct {
	Code                     int      `json:"code"`
	Message                  string   `json:"message"`
	TxHash                   []string `json:"tx_hash"`
	PredictedExecutionTimeMS int64    `json:"predicted_execution_time_ms"`
	VolumeQuotaRemaining     int64    `json:"volume_quota_remaining"`
}

type EnrichedTx struct {
	Code          int    `json:"code"`
	Message       string `json:"message"`
	Hash          string `json:"hash"`
	Type          int    `json:"type"`
	Info          string `json:"info"`
	EventInfo     string `json:"event_info"`
	Status        int64  `json:"status"`
	SequenceIndex int64  `json:"sequence_index"`
	ExecutedAt    int64  `json:"executed_at"`
}

type Order struct {
	OrderIndex          int64  `json:"order_index"`
	ClientOrderIndex    int64  `json:"client_order_index"`
	OrderID             string `json:"order_id"`
	MarketIndex         int16  `json:"market_index"`
	InitialBaseAmount   string `json:"initial_base_amount"`
	RemainingBaseAmount string `json:"remaining_base_amount"`
	Price               string `json:"price"`
	Type                string `json:"type"`
	TimeInForce         string `json:"time_in_force"`
	Status              string `json:"status"`
}

type OrdersResponse struct {
	Code       int     `json:"code"`
	Message    string  `json:"message"`
	NextCursor string  `json:"next_cursor"`
	Orders     []Order `json:"orders"`
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

func (c *Client) APIKeys(accountIndex int64, apiKeyIndex uint8) (*APIKeysResponse, error) {
	q := map[string]string{
		"account_index": fmt.Sprintf("%d", accountIndex),
		"api_key_index": fmt.Sprintf("%d", apiKeyIndex),
	}
	b, err := c.doGET("/api/v1/apikeys", q)
	if err != nil {
		return nil, err
	}
	var out APIKeysResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) OrderBooks(filter string) (*OrderBooksResponse, error) {
	q := map[string]string{}
	if strings.TrimSpace(filter) != "" {
		q["filter"] = strings.TrimSpace(filter)
	}
	b, err := c.doGET("/api/v1/orderBooks", q)
	if err != nil {
		return nil, err
	}
	var out OrderBooksResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) OrderBookOrders(marketID int16, limit int) (*OrderBookOrdersResponse, error) {
	q := map[string]string{
		"market_id": fmt.Sprintf("%d", marketID),
		"limit":     fmt.Sprintf("%d", limit),
	}
	b, err := c.doGET("/api/v1/orderBookOrders", q)
	if err != nil {
		return nil, err
	}
	var out OrderBookOrdersResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) SendTx(txType uint8, txInfo string, priceProtection bool) (*SendTxResponse, error) {
	vals := url.Values{}
	vals.Set("tx_type", strconv.FormatUint(uint64(txType), 10))
	vals.Set("tx_info", txInfo)
	if priceProtection {
		vals.Set("price_protection", "true")
	} else {
		vals.Set("price_protection", "false")
	}
	b, err := c.doPOSTForm("/api/v1/sendTx", vals)
	if err != nil {
		return nil, err
	}
	var out SendTxResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) SendTxBatch(txTypes []uint8, txInfos []string) (*SendTxBatchResponse, error) {
	typeVals := make([]int, 0, len(txTypes))
	for _, t := range txTypes {
		typeVals = append(typeVals, int(t))
	}
	rawTypes, err := json.Marshal(typeVals)
	if err != nil {
		return nil, err
	}
	rawInfos, err := json.Marshal(txInfos)
	if err != nil {
		return nil, err
	}
	vals := url.Values{}
	vals.Set("tx_types", string(rawTypes))
	vals.Set("tx_infos", string(rawInfos))
	b, err := c.doPOSTForm("/api/v1/sendTxBatch", vals)
	if err != nil {
		return nil, err
	}
	var out SendTxBatchResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) TxByHash(txHash string) (*EnrichedTx, error) {
	q := map[string]string{
		"by":    "hash",
		"value": strings.TrimSpace(txHash),
	}
	b, err := c.doGET("/api/v1/tx", q)
	if err != nil {
		return nil, err
	}
	var out EnrichedTx
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) AccountActiveOrders(accountIndex int64, marketID int16, auth string) (*OrdersResponse, error) {
	q := map[string]string{
		"account_index": fmt.Sprintf("%d", accountIndex),
		"market_id":     fmt.Sprintf("%d", marketID),
	}
	if strings.TrimSpace(auth) != "" {
		q["auth"] = strings.TrimSpace(auth)
	}
	b, err := c.doGET("/api/v1/accountActiveOrders", q)
	if err != nil {
		return nil, err
	}
	var out OrdersResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) AccountInactiveOrders(accountIndex int64, marketID int16, limit int, auth string) (*OrdersResponse, error) {
	q := map[string]string{
		"account_index": fmt.Sprintf("%d", accountIndex),
		"market_id":     fmt.Sprintf("%d", marketID),
		"limit":         fmt.Sprintf("%d", limit),
	}
	if strings.TrimSpace(auth) != "" {
		q["auth"] = strings.TrimSpace(auth)
	}
	b, err := c.doGET("/api/v1/accountInactiveOrders", q)
	if err != nil {
		return nil, err
	}
	var out OrdersResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
