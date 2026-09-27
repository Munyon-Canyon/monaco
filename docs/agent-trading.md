# Agent trading operator guide

An agent trades for one cabal with nothing but its key. It never holds the cabal's money. It sends buy and sell intents to Monaco. Monaco checks each one against the agent's budget and executes it from the cabal treasury, through the same swap provider (Jupiter by default, Definitive Flash when `SWAP_PROVIDER=flash`) and Privy treasury path as a member vote. Fills stay in the **cabal treasury**.

The stocks are xStocks on Solana, named by symbol, such as `AAPLx`.

The backend is being rewritten ([backend-platform.md](architecture/backend-platform.md)). This page describes the target contract. Until the cutover, the current backend still answers with the older shapes in [legacy/api.md](legacy/api.md).

## Setup

1. A member proposes **add agent** with a name and a USDC budget.
2. The cabal votes. When it passes, Monaco mints a key: `monaco_ak_` followed by 32 random characters (about 158 bits from `crypto/rand`).
3. Any member opens **Group → Agent** and taps **Copy connect instructions**. The text holds the key, the API base URL and a link to `/v1/agent/skill.md`. Paste it into the agent. **Copy key** copies the key alone.

The key is readable by cabal members until the agent is removed. Revoking the agent wipes it. **Never log the key.**

Five-character keys minted before this format keep working. They can be guessed, so rotate them. Vote the agent out and back in to get a long key.

## API base URL

The API base is `PUBLIC_API_BASE_URL`. Local dev uses `http://127.0.0.1:8080`. A deployed API sets it to its public https URL, and the connect instructions carry the same value.

## Auth

Send the key on every call:

```http
X-Monaco-Agent-Key: monaco_ak_k7m2p9x4…
```

The key alone names the agent and its cabal. No group id and no member login are needed. A missing, unknown or revoked key gets **401**.

Only wrong keys are counted, 10 per cabal and 10 per caller address, refilling one a minute:

- An **address** that has used its 10 gets **429** with `Retry-After` on every call, whatever key it sends. An agent only lands here by sending wrong keys itself.
- Wrong keys aimed at your **cabal** from elsewhere never lock out an agent on a long key. A call with a `monaco_ak_…` key is always checked, and passes if the key is right.
- An agent still on a five-character key is refused with **429** while its cabal's allowance is spent. Rotate to a long key to be rid of it.

The caller address is the TCP peer, or the left-most `X-Forwarded-For` entry when the API runs with `TRUST_PROXY_HEADERS=true`.

## Routes

| Route | Key | Purpose |
|-------|-----|---------|
| `GET /v1/agent` | yes | Cabal, agent, status, budget, cash and the agent's own holdings |
| `GET /v1/agent/assets` | yes | The stocks the agent may trade, with a current price for each |
| `POST /v1/agent/intents` | yes | Buy or sell |
| `GET /v1/agent/intents/{intentId}` | yes | Where one intent ended up |
| `GET /v1/agent/skill.md` | no | Instructions written for an LLM agent |

Money comes back two ways. Decimal strings (`"10.50"`, `"0.25000000"`) are for people and LLMs. Integer atomic fields (`usdcMicros`, `sharesAtomic`) are for code. USDC has 6 decimals. xStocks have 8.

## Read the agent

```bash
curl -sS -H "X-Monaco-Agent-Key: $MONACO_AGENT_KEY" "$MONACO_API/v1/agent"
```

```json
{
  "cabalName": "Tech Bros",
  "agentName": "Momentum",
  "status": "active",
  "budget": {
    "allocationUsd": "100.00", "allocationUsdcMicros": 100000000,
    "availableUsd": "42.10", "availableUsdcMicros": 42100000
  },
  "cashAvailableUsd": "42.10", "cashAvailableUsdcMicros": 42100000,
  "holdings": [
    {"symbol": "AAPLx", "name": "Apple xStock", "shares": "0.25000000", "sharesAtomic": 25000000,
     "markUsd": "230.12", "markUsdcMicros": 230120000, "valueUsd": "57.53", "valueUsdcMicros": 57530000}
  ],
  "limits": {"intentsPerHour": 30, "reasonMaxChars": 280},
  "docsUrl": "http://127.0.0.1:8080/v1/agent/skill.md"
}
```

- `status` is `active` or `paused`.
- `cashAvailableUsd` is what the agent can spend right now. It is the smaller of its available budget and the treasury's USDC.
- `holdings` lists only what the agent itself bought and still holds. The mark fields are null when Monaco has no price for that stock right now.

## List assets

```bash
curl -sS -H "X-Monaco-Agent-Key: $MONACO_AGENT_KEY" "$MONACO_API/v1/agent/assets?limit=25"
```

```json
{"assets": [{"symbol": "AAPLx", "name": "Apple xStock", "solanaMint": "Xs…", "routable": true,
             "markUsd": "230.12", "markUsdcMicros": 230120000}], "hasMore": false}
```

Optional query parameters are `query`, `limit` (max 100) and `offset`. Only routable stocks are listed. `markUsd` is null for a stock whose price is unavailable, and the list still returns 200.

Use these symbols and prices. Do not call Jupiter, xStocks or Solana from the agent.

## Post an intent

A buy spends USD. Send exactly one of `usd` (a decimal string, up to 6 decimals) or `usdcMicros` (an integer, USD × 10⁶).

```bash
curl -sS -X POST \
  -H "X-Monaco-Agent-Key: $MONACO_AGENT_KEY" \
  -H "Idempotency-Key: 3f1c…" \
  -H "Content-Type: application/json" \
  "$MONACO_API/v1/agent/intents" \
  -d '{"side":"buy","symbol":"AAPLx","usd":"10.50","reason":"breakout above 20d high"}'
```

A sell names shares. Send exactly one of `shares` (a decimal string, up to 8 decimals) or `tokenAmount` (an integer, shares × 10⁸).

```bash
curl -sS -X POST \
  -H "X-Monaco-Agent-Key: $MONACO_AGENT_KEY" \
  -H "Idempotency-Key: 9a2e…" \
  -H "Content-Type: application/json" \
  "$MONACO_API/v1/agent/intents" \
  -d '{"side":"sell","symbol":"AAPLx","shares":"0.25","reason":"lost momentum"}'
```

Both forms at once, neither form, or the other side's field is a **422**.

`reason` is optional, up to 280 characters. Monaco stores it with the intent and shows it to the cabal. Say briefly why the agent made the trade.

Success returns the `intentId` with `"status": "accepted"`, before the swap settles. The swap then shows as pending and then confirmed in cabal activity.

`scripts/demo/agent-intent.sh buy AAPLx 1` sends one intent from the terminal. See its `--help`.

## Read an intent

```bash
curl -sS -H "X-Monaco-Agent-Key: $MONACO_AGENT_KEY" "$MONACO_API/v1/agent/intents/$INTENT_ID"
```

```json
{"intentId": "…", "side": "buy", "symbol": "AAPLx", "status": "executed",
 "rejectReason": null, "reason": "breakout",
 "usdcMicros": 10500000, "tokenAmount": null,
 "transactionId": "…", "txSignature": "5x…", "filledTokenAmount": 4560000, "filledUsdcMicros": 10500000,
 "createdAt": "2026-09-24T12:00:00Z"}
```

`status` is `accepted` while the swap is in flight, then `executed`, `rejected` or `failed`. Another agent's intent and an unknown id are both **404**.

## Idempotency and retries

Send an `Idempotency-Key` header on every intent, unique per trade decision. The server stores the first response and replays it for a resend with the same key, so a retry never trades twice. An intent goes through the async trade engine, and the response returns before the swap settles. Read `GET /v1/agent/intents/{intentId}` for the outcome. See [Thin client](architecture/backend-platform.md#thin-client) and [Flows](architecture/backend-platform.md#flows).

## Rate limits

Each key may send 30 intents an hour. Reads (`GET /v1/agent*`) allow a burst of 120 and refill one every 30 seconds. Over either limit, Monaco answers **429** with `Retry-After` in seconds. Wait that long, then continue. A resend under the same `Idempotency-Key` still counts as a request.

## Errors

Every error body is RFC 9457 `application/problem+json` with a stable `code`, a user-facing `message`, a `trace_id`, and `retryable`. Branch on `code`, not on the `message` text. Quote `trace_id` when reporting a problem. See [Errors](architecture/backend-platform.md#errors).

## Budget

The add-agent vote sets the agent's USDC budget. Available budget is:

> allocation − executed buys − in-flight buys + proceeds of the agent's confirmed sells

It never goes below zero. **Sells refill the budget.** An agent with a $100 budget that buys $100 of stock and later sells it for $110 has $110 to spend again. The treasury's USDC is still the hard ceiling.

One agent's intents are decided one at a time, and a buy reserves its amount before the swap is sent. Parallel intents cannot overshoot the budget.

## Sell only what you bought

An agent may sell only what its own confirmed buys returned, less what it has already sold or is selling. Stocks the cabal bought by vote are never the agent's to sell, even in the same treasury. Selling those takes a sell proposal. If a member-voted sell took part of what the agent bought, the agent is limited to what the treasury still holds. `holdings` in `GET /v1/agent` shows exactly what the agent may sell.

## Pause, resume, revoke

Each is a cabal vote. A **paused** agent keeps its key, and its intents get **403**. A **revoked** agent's key gets **401**. Resume restores the same key.

## Security

Keep the key in a secret store. To rotate it, revoke the agent and vote in a new one.
