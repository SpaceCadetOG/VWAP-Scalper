package models

// Common enums used across adapters, strategy, routing, and risk.

type Side string

const (
	SideBuy  Side = "BUY"
	SideSell Side = "SELL"
)

type OrderType string

const (
	OrderTypeMarket     OrderType = "MARKET"
	OrderTypeLimit      OrderType = "LIMIT"
	OrderTypeStop       OrderType = "STOP"
	OrderTypeTakeProfit OrderType = "TAKE_PROFIT"
)

type TimeInForce string

const (
	TIFGTC         TimeInForce = "GTC"
	TIFIOC         TimeInForce = "IOC"
	TIFFOK         TimeInForce = "FOK"
	TIFALOPostOnly TimeInForce = "ALO_POST_ONLY"
)

type OrderStatus string

const (
	OrderStatusNew             OrderStatus = "NEW"
	OrderStatusAcked           OrderStatus = "ACKED"
	OrderStatusWorking         OrderStatus = "WORKING"
	OrderStatusPartiallyFilled OrderStatus = "PARTIALLY_FILLED"
	OrderStatusFilled          OrderStatus = "FILLED"
	OrderStatusRejected        OrderStatus = "REJECTED"
	OrderStatusCanceled        OrderStatus = "CANCELED"
	OrderStatusExpired         OrderStatus = "EXPIRED"
)

type ErrorCode string

const (
	ErrRateLimit         ErrorCode = "ERR_RATE_LIMIT"
	ErrInvalidNonce      ErrorCode = "ERR_INVALID_NONCE"
	ErrInvalidSignature  ErrorCode = "ERR_INVALID_SIGNATURE"
	ErrInsufficientMargin ErrorCode = "ERR_INSUFFICIENT_MARGIN"
	ErrMinNotional       ErrorCode = "ERR_MIN_NOTIONAL"
	ErrPostOnlyWouldTake ErrorCode = "ERR_POST_ONLY_WOULD_TAKE"
	ErrOrderNotFound     ErrorCode = "ERR_ORDER_NOT_FOUND"
	ErrTemporary         ErrorCode = "ERR_TEMPORARY"
)

type MarketState string

const (
	StateCompression    MarketState = "STATE_COMPRESSION"
	StateOpenDrive      MarketState = "STATE_OPEN_DRIVE"
	StateNewsReaction   MarketState = "STATE_NEWS_REACTION"
	StateDoubleTap      MarketState = "STATE_DOUBLE_TAP"
	StateMultiSessionEQ MarketState = "STATE_MULTI_SESSION_EQ"
	StateExpansion      MarketState = "STATE_EXPANSION"
	StateExhaustion     MarketState = "STATE_EXHAUSTION"
	StateChop           MarketState = "STATE_CHOP"
)
