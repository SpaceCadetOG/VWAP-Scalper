# Perps Signing Requirements (Hyperliquid, Aster, Lighter)

This document locks mandatory signing and execution-lifecycle rules for perpetual futures trading across the three perps venue adapters.

Note: Coinbase is integrated as spot-only and is out of scope for this perps signing document.

## 1) Hyperliquid Perps Requirements

### Signing/Auth
- Use API wallet / agent wallet model for authenticated exchange actions.
- Sign requests with the designated signer key for the account/subaccount context.
- Enforce signer-scoped nonce monotonicity.

### Nonce Rules
- Nonce source is maintained per signer.
- Nonce allocation must be atomic under concurrent workers.
- Any nonce mismatch/replay rejection must trigger immediate nonce resync and retry policy.

### Order + Lifecycle Requirements
- Support perps order actions via exchange endpoint model.
- Support batch action handling where available.
- Distinguish transport-level acceptance from final order state.
- Reconcile final state through user stream + periodic REST sync.

### Failure/Retry/Rate-Limit
- Normalize and classify rate limit errors.
- Backoff with jitter on transient failures.
- Never blind-retry non-idempotent actions without reconciliation.

## 2) Aster Futures v3 Perps Requirements

### Signing/Auth
- Signed requests must include `user`, `signer`, `nonce`, and `signature` payload requirements.
- Nonce freshness and validity windows must be respected for every signed request.

### Nonce Rules
- Nonce source is generated per signer context (microsecond-level expectation where required by endpoint behavior).
- Reject duplicate or stale nonce locally before send when detectable.
- On signature/nonce rejection, perform signer+clock sanity checks and controlled resend.

### User Stream Lifecycle
- Use `listenKey` flow for private streams.
- Implement create, keepalive refresh, and recreation on expiry/disconnect.

### Order + Lifecycle Requirements
- Perps trading endpoints are `/fapi/v3/...`.
- Treat unknown execution cases (e.g., timeout-style uncertain states) as `PENDING_RECONCILIATION`.
- Reconcile via order query endpoints and account stream events.

### Failure/Retry/Rate-Limit
- Handle 429/418 with cooldown and circuit-breaker behavior.
- Avoid duplicate order risk by client order id + reconciliation before retry.

## 3) Lighter Perps Requirements

### Signing/Auth
- Use account index + API key index signing model.
- Signed order flow goes through `sendTx` / `sendTxBatch` semantics.

### Nonce Rules
- Nonce source is per API key index.
- Nonce allocation must be monotonic and concurrency-safe.
- Implement nonce resync from venue when divergence is detected.

### Order + Lifecycle Requirements
- `sendTx`/`sendTxBatch` acknowledgement is not equivalent to final sequencer execution success.
- Final status must be reconciled from private stream/account updates plus query fallback.
- Batch submission must preserve deterministic client references for idempotent reconciliation.

### Failure/Retry/Rate-Limit
- Respect weighted transaction buckets and endpoint limits.
- Classify temporary vs terminal failures before retry.
- Use bounded retries with reconciliation checkpoints.

## 4) Perps Constraints Matrix (Mandatory)

| Constraint | Hyperliquid | Aster Futures v3 | Lighter |
|---|---|---|---|
| Order types | Must map canonical MARKET/LIMIT/trigger variants to venue-supported perps types | Must map canonical types to `/fapi/v3` supported futures types | Must map canonical types to tx schema-supported perps order types |
| Reduce-only | Enforce venue reduce-only flags and reject unsupported combinations | Enforce reduce-only behavior per endpoint semantics | Enforce reduce-only in tx payload where supported |
| Trigger stops / TP | Map stop/trigger fields to venue-specific trigger model | Map stop params to v3 futures order fields | Map trigger logic through tx order format |
| Tick/Lot/Min Notional | Validate pre-send against venue symbol metadata | Validate pre-send from exchange info metadata | Validate pre-send from market metadata |
| Leverage/Margin mode | Respect venue leverage/margin constraints and account mode | Respect futures account settings and margin constraints | Respect account-level leverage/margin constraints |
| Funding/Position updates | Consume user/account updates and funding data for PnL/risk | Use account/user stream + endpoint reconciliation | Use account stream + query reconciliation |

## 5) Adapter Contract Checklist (Must Pass Before Strategy Coding)

Each venue adapter must define and pass:
- Nonce source definition
- Signing payload definition
- Ack vs final-execution reconciliation rule
- Failure/retry/rate-limit handling notes

No strategycore implementation should begin until this checklist is complete across Hyperliquid, Aster, and Lighter.

## 6) Realism Policy
- Testnet is API-contract validation only.
- Live-feed paper trading is the realism gate for execution quality.
- Live trading is only enabled after paper metrics + risk controls pass acceptance criteria.
