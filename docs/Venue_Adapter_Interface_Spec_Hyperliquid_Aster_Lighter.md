# Venue Adapter Interface Spec
## Targets
- Hyperliquid
- Aster
- Lighter
- Coinbase (spot-only)

## 1) Design Goals
- One strategy core for Chapters 1-26.
- Venue-specific differences isolated in adapters.
- Deterministic order lifecycle handling.
- Strict risk controls shared across all venues.

## 2) Canonical Interfaces

### 2.1 Market Data Adapter
Required methods:
- `connectMarketData(ctx)`
- `subscribeCandles(symbol, interval)`
- `subscribeBBO(symbol)`
- `subscribeTrades(symbol)`
- `subscribeDepth(symbol, levels)`
- `unsubscribe(channelKey)`
- `snapshot(symbol)`
- `health()`

Canonical events:
- `CandleEvent { venue, symbol, interval, openTime, closeTime, o,h,l,c,v, tradeCount }`
- `BBOEvent { venue, symbol, ts, bidPx, bidSz, askPx, askSz }`
- `TradeEvent { venue, symbol, ts, px, sz, side, id }`
- `DepthEvent { venue, symbol, ts, bids[], asks[], seq }`

### 2.2 Trading Adapter
Required methods:
- `placeOrder(req)`
- `cancelOrder(req)`
- `cancelAll(symbol)`
- `replaceOrder(req)`
- `getOpenOrders(symbol)`
- `getOrder(orderRef)`
- `getPositions()`
- `getBalances()`
- `getFills(start,end,symbol)`
- `getFunding(symbol,start,end)`
- `ping()`

Canonical order request:
- `OrderRequest {`
- `  venue, accountId, symbol, side(BUY|SELL),`
- `  type(MARKET|LIMIT|STOP|TAKE_PROFIT),`
- `  tif(GTC|IOC|FOK|ALO_POST_ONLY),`
- `  qty, px?, stopPx?, reduceOnly?, postOnly?,`
- `  clientOrderId, tags{}, expireAt?,`
- `}`

Canonical order response:
- `OrderAck { venueOrderId, clientOrderId, accepted, reason?, ts }`

Canonical lifecycle states:
- `NEW -> ACKED -> WORKING -> PARTIALLY_FILLED -> FILLED`
- `NEW -> ACKED -> REJECTED`
- `WORKING -> CANCELED|EXPIRED`

### 2.3 Account Stream Adapter
Required methods:
- `connectUserStream(ctx)`
- `subscribeOrders()`
- `subscribeFills()`
- `subscribePositions()`
- `keepalive()`
- `health()`

Canonical events:
- `OrderUpdateEvent`
- `FillEvent`
- `PositionEvent`
- `BalanceEvent`

## 3) Cross-Venue Normalization Rules
- Prices and sizes normalized to decimal strings internally (no float).
- All timestamps converted to epoch milliseconds.
- Symbol map resolved at startup (`venueSymbol <-> canonicalSymbol`).
- Tick/lot validation performed before send.
- Convert venue-specific rejects into canonical error codes.

Canonical error groups:
- `ERR_RATE_LIMIT`
- `ERR_INVALID_NONCE`
- `ERR_INVALID_SIGNATURE`
- `ERR_INSUFFICIENT_MARGIN`
- `ERR_MIN_NOTIONAL`
- `ERR_POST_ONLY_WOULD_TAKE`
- `ERR_ORDER_NOT_FOUND`
- `ERR_TEMPORARY`

## 4) Venue-Specific Adapter Contracts

### 4.1 Hyperliquid Adapter
Notes:
- Use API wallet/agent signing model.
- Nonces are tracked per signer; keep atomic nonce service per signer key.
- Support REST and WS (`wss://api.hyperliquid.xyz/ws`, testnet equivalent).
- Batch actions supported; keep batch-size aware weight estimation.

Must implement:
- `placeBatchOrders([]OrderRequest)`
- `cancelBatch([]OrderRef)`
- `nonceProvider.next(signer)` with monotonic guarantee

Special handling:
- Open-order caps and reduce-only/trigger constraints.
- Distinguish transport ACK vs matching/rejection outcome.

### 4.2 Aster Adapter
Notes:
- Base futures endpoint family under `/fapi/v3`.
- Signed endpoints require `user`, `signer`, `nonce`, `signature` payload model.
- User stream via `listenKey`; maintain periodic keepalive.
- WebSocket market stream supports raw and combined modes.

Must implement:
- `startListenKey()`
- `keepaliveListenKey()`
- `recreateListenKeyOnExpire()`

Special handling:
- 503 means execution status can be unknown; reconcile via query.
- Handle 429/418 backoff and circuit breaker.

### 4.3 Lighter Adapter
Notes:
- Use account index + API key signing model.
- Nonce tracked per API key; support explicit nextNonce fetch.
- Trade tx via `sendTx`/`sendTxBatch` (REST or WS JSON types).
- Weighted rate limits plus transaction-type caps.

Must implement:
- `nextNonce(apiKeyIndex)`
- `sendTx(orderTx)`
- `sendTxBatch([]tx)`

Special handling:
- API `code=200` does not guarantee sequencer execution.
- Subscribe account channels and reconcile final execution status.

## 5) Router Interface (3-Venue Execution)
Required method:
- `route(orderIntent) -> venueDecision`

Inputs:
- Best bid/ask and depth by venue
- Recent fill latency by venue
- Recent rejection rate by venue
- Fees/funding impact
- Risk and exposure constraints

Decision policies:
- Breakout/impulse setups: prefer low-latency fill path.
- Reversion/fade setups: prefer tighter spread + deeper top book.
- If confidence ties, split small IOC probes then promote winner.

Failover:
- If chosen venue rejects/timeouts, retry once on second-best venue if strategy TTL not expired.

## 6) Risk Engine Contract (Global)
Hard gates before any send:
- Daily loss limit
- Max concurrent risk
- Max symbol exposure (global and per venue)
- Max strikes / cooldown state
- Spread/liquidity sanity filters

Required methods:
- `preTradeCheck(intent) -> allow/deny`
- `onOrderAck(update)`
- `onFill(update)`
- `onMarkToMarket(prices)`
- `shouldForceExit(position)`

## 7) Time/TTL Rules (Scalping Critical)
Per-order TTL fields:
- `signalTs`
- `maxEntryDelayMs`
- `maxHoldMs`
- `noFollowThroughMs`

If TTL breached:
- Cancel working orders
- Exit open position by policy (market or protective limit)

## 8) Data Model for Chapter-State Engine
State output enum:
- `STATE_COMPRESSION`
- `STATE_OPEN_DRIVE`
- `STATE_NEWS_REACTION`
- `STATE_DOUBLE_TAP`
- `STATE_MULTI_SESSION_EQ`
- `STATE_EXPANSION`
- `STATE_EXHAUSTION`
- `STATE_CHOP`

Every state must include:
- `confidenceScore (0-100)`
- `invalidators[]`
- `expiryMs`

## 9) Observability + Replay Requirements
Logs (structured):
- Every outbound request payload hash + nonce + clientOrderId
- Every inbound ACK/update/fill with latency stamps
- Every risk deny with exact rule id

Replay files:
- `market_events.ndjson`
- `order_events.ndjson`
- `position_events.ndjson`
- `state_events.ndjson`

## 10) Minimum Acceptance Tests
1. Place/cancel lifecycle passes on each venue testnet.
2. Nonce collision test under concurrent order sends.
3. Partial fill + replace + final fill reconciliation.
4. WS disconnect recovery within 5s and no orphan state.
5. 429 backoff compliance and recovery.
6. Unknown execution reconciliation (Aster 503 path).
7. Global risk gate blocks duplicate cross-venue exposure.

## 11) Implementation Order
1. Symbol metadata loaders + tick/lot validators.
2. Nonce/signing services per venue.
3. Market data adapters.
4. Trading adapters.
5. Account stream adapters.
6. Router.
7. Global risk.
8. Replay and acceptance tests.

## 12) Runtime Config Skeleton
- `VENUES_ENABLED=hyperliquid,aster,lighter`
- `ROUTER_MODE=latency_spread_hybrid`
- `GLOBAL_MAX_DAILY_LOSS_PCT=1.5`
- `GLOBAL_MAX_OPEN_RISK_R=3.0`
- `ORDER_DEFAULT_MAX_ENTRY_DELAY_MS=1500`
- `ORDER_DEFAULT_NO_FOLLOW_THROUGH_MS=120000`
- `ORDER_DEFAULT_MAX_HOLD_MS=180000`
- `ADAPTER_RECONNECT_BACKOFF_MS=250,500,1000,2000,5000`

---
This spec is the canonical adapter contract for implementing multi-venue VWAP execution across Hyperliquid, Aster, Lighter, and Coinbase (spot-only).

## 13) Build Checklist / Roadmap Status

Project: `VWAP_Scalper` (3-venue perps execution foundation)

- [x] Step 1: Go module + baseline runtime layout initialized
- [x] Step 2: Canonical models
- [x] Step 3: Symbol metadata + tick/lot validators
- [x] Step 4: Nonce/signing services
- [ ] Step 5: Market data adapters
- [ ] Step 6: Trading adapters
- [ ] Step 7: Account stream adapters
- [ ] Step 8: Router
- [ ] Step 9: Global risk engine
- [ ] Step 10: Replay/paper engine
- [ ] Step 11: Testnet acceptance tests

Current Step 1 deliverables:
- `go.mod` initialized (`github.com/SpaceCadetOG/VWAP-Scalper`)
- baseline directories created (`cmd`, `internal`, `configs`, `tests`)
- bootstrap entrypoint added at `cmd/live/main.go`
- runtime config example added at `configs/live.env.example`
