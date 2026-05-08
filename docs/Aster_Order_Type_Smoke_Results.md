# Aster Order-Type Smoke Results

Date: 2026-05-08  
Scope: `POST /fapi/v3/order` via live API credentials, small notional test (`~$5`) on `BTCUSDT`.

## Command Used

```bash
go run ./cmd/live --test-order-types --venue=aster --symbol=BTCUSDT --notional-usd=5
```

## Tested Order Types (UI -> API behavior)

1. `Limit` (`type=LIMIT`, `timeInForce=GTC`): **Accepted**
2. `Stop Limit` (`type=STOP`, with `price` + `stopPrice`): **Rejected** during this run due to margin check
3. `Stop Market` (`type=STOP_MARKET`, `stopPrice`): **Accepted**
4. `Trailing Stop` (`type=TRAILING_STOP_MARKET`, `callbackRate`, `activationPrice`): **Accepted**
5. `Post Only` (`type=LIMIT`, `timeInForce=GTX`): **Accepted**
6. `TWAP` (`type=TWAP` probe): **Rejected** with invalid order type
7. `Scaled Order` (`type=SCALED` probe): **Rejected** with invalid order type

## Exact API Errors Observed

- Stop Limit failure:
  - `{"code":-2019,"msg":"Margin is insufficient."}`
- TWAP probe failure:
  - `{"code":-1116,"msg":"Invalid orderType."}`
- Scaled probe failure:
  - `{"code":-1116,"msg":"Invalid orderType."}`

## Cleanup/Close Verification

- Every accepted `NEW` order created in the test was canceled successfully.
- For each canceled order, `GET /fapi/v3/order` returned `status=CANCELED`.
- Final position check (`/fapi/v3/positionRisk`) showed:
  - `symbol=BTCUSDT`
  - `positionAmt=0.000`
  - `notional=0`

## Implementation Notes Added to Adapter

- BTC price precision handling in smoke test now uses 1 decimal.
- Trailing stop probe now uses valid activation-side rule to avoid immediate-trigger rejection:
  - BUY requires `activationPrice < latest`
  - SELL requires `activationPrice > latest`
- Runner now performs automatic cancel cleanup for all non-filled active test orders.
