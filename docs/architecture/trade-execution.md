# Trade execution

**Status:** Decided 2026-09-26. Reconciled 2026-09-27 with [backend-platform.md](backend-platform.md), which wins where the two conflict. Built in [Rollout](backend-platform.md#rollout) step 4 (flows 9 to 13, with crash-point tests). The old backend has part of it; see [Gap between this and the code](#gap-between-this-and-the-code).

## Decision

A trade moves through three stages, each owned by a different module, joined by an event:

1. **Governance** (`governance` module). A member proposes a trade. Voters in the cabal vote. The tally marks the proposal `passed`, `failed`, or `expired`.
2. **Trade engine** (`trading` module). A passed proposal emits a `proposal.passed` event. The trade engine consumes it, runs pre-trade checks (treasury has the funds, price is inside slippage tolerance, asset is tradable), and if every check passes, hands the trade to the swap layer.
3. **Swap layer** (`trading` module). The swap layer inserts a row in trading's `swaps` table, gets a signed transaction from Jupiter, calls Jupiter `/execute`, and drives the row to exactly one terminal state through a single guarded update. Every path that can finish a swap (the `/execute` response, a Pending re-poll, a crash sweeper, an optional Helius webhook) goes through that same update, so a swap has one current state and a full history no matter which path finished it.

Modules never call each other's packages ([dependency rules](backend-platform.md#dependency-rules-enforced-by-depguard)). Stages hand off through NATS JetStream. Each state change appends an `events` row inside the same `uow.Do` transaction ([Unit of Work](backend-platform.md#patterns-and-where-each-earns-its-place)), and the relay publishes it. Side effects of a finished trade (the ledger, notifications, feed items, rankings, closing the proposal) are consumers of `trade.confirmed` / `trade.failed`. The swap path does not know which consumers exist.

## Why

- **Voting is the product; execution is plumbing.** Splitting them keeps governance rules (voter sets, majority vs unanimous, expiry) free of Solana and Jupiter concerns. It also lets the trade engine serve agent intents through the same checks ([flow 17](backend-platform.md#flows)).
- **Money code must be idempotent.** Jupiter `/execute` blocks and normally returns a terminal result. But the process can die mid-call, the network can drop the response, and the same proposal can be delivered twice. A guarded update (`WHERE status = 'submitted'`) plus a unique `tx_signature` makes double-finishing impossible, regardless of which path wins.
- **Pre-trade checks belong at execution time, not proposal time.** The pot value, treasury USDC, and price all move between proposal and pass. Checks at proposal time are a UX courtesy. Checks at execution time are the guarantee.
- **Events over direct calls.** A notification for a trade that later rolled back is wrong. A trade that confirms but never notifies because the push service was down is also wrong. Writing the event in the same transaction and relaying it gives at-least-once delivery with no lost or phantom events.

## How it works

### Stage 1: propose and vote

Full design in [proposals.md](proposals.md). Relevant to execution:

- The `proposal.passed` payload carries everything the engine needs, so the engine never reads governance tables: `proposal_id`, `cabal_id`, `kind` (`buy` or `sell`), `symbol`, `usdc_micros` (`money.Micros`, buy) or `token_amount` (`money.BaseUnits`, sell), `quote_out_amount`, `proposer_id`. Amounts are integers ([Money and types](backend-platform.md#money-and-types)).
- Proposal-time checks are advisory: a buy is refused up front if Jupiter cannot route it or if it exceeds the whole pot. The market module answers both through its query port ([flow 9](backend-platform.md#flows)). The engine repeats them with fresher data.
- The tally runs in one `uow.Do` after every vote and on expiry. When status flips to `passed`, that same transaction appends `proposal.passed`. The tally does not call the engine.

### Stage 2: trade engine

A durable JetStream pull consumer in the `trading` module, filtered on `proposal.passed` and `agent.intent_created`, run through `bus.Dispatch`. One proposal or intent maps to at most one live execution attempt. The handler calls `msg.InProgress()` while a swap is in flight so the ack deadline does not expire during a 2-minute `/execute`. Its idempotency comes from its own claim, not from the single `event_deliveries` transaction; see [event-bus.md](event-bus.md#consumers-and-handlers).

**Claim.** In trading's own tables only. If `event_deliveries` already holds this event for `trading`, or a `swaps` row for the proposal exists in `created`, `submitted`, or `confirmed`, ack and stop (idempotent redelivery). A `failed` row does not block a retry. The partial unique index on `swaps` is what makes the claim safe under concurrent deliveries.

**Pre-trade checks**, in order. Each refusal is an [`errs` code](backend-platform.md#errors) of kind `Blocked`:

| Check | Buy | Sell | Code |
| --- | --- | --- | --- |
| Asset tradable | Mint resolves in the market catalog; mint not paused (pre-IPO tokens can pause) | Same | `AssetUntradable` |
| Treasury funds | Treasury on-chain USDC ≥ `usdc_micros` plus fee headroom | Treasury holds ≥ `token_amount` of the mint | `InsufficientFunds` |
| Price within tolerance | Fresh quote; output amount ≥ `quote_out_amount` × (1 − slippage bps) | Same, on USDC out | `SlippageExceeded` |
| Route exists | Jupiter returns a route for the exact-in amount | Same | `Unroutable` |
| Cabal not paused | Cabal not banned (cabal query port), and no active pause in funding's pause record, whether for an unresolved external deposit or for ops (funding query port) | Same | `CabalPaused` |
| Agent budget (intents only) | Intent fits the agent's remaining budget, re-read through the agents module's query port | Same | `AgentBudgetExceeded` |

Slippage tolerance is a cabal rule with a platform default (proposal: 100 bps default, cabal can tighten, platform caps at 300 bps). The engine reads it through the cabal module's query port. Treasury USDC is read from chain, not from the ledger, matching the platform-balance rule elsewhere. Every amount and bps value is an integer.

The pause is owned by `funding` and read at check time, so there is no window between detection and pause and no pause state in trading ([deposits-withdrawals.md](deposits-withdrawals.md#pause)). Buy fee headroom is computed from the mint's transfer-fee config, not a fixed buffer or a guessed percentage. The relayer pays SOL fees, so headroom only covers token transfer fees. Pre-IPO transfer-fee handling is deferred (decided 2026-09-27). `SubmitAgentIntent` checks the agent budget at submit ([flow 17](backend-platform.md#flows)); the engine checks it again here, because checks at execution time are the guarantee.

A refused check appends `trade.blocked` with the code and the numbers it compared, records the delivery, acks, and stops. Governance consumes `trade.blocked`, sets the proposal to `execution_blocked` and emits `proposal.execution_blocked`. The engine logs the decision once, with `have` and `need` ([Logs as evidence](backend-platform.md#logs-as-evidence)). It never retries a refusal, `InsufficientFunds` and `SlippageExceeded` included, even when a fund-to-cabal sweep is in flight. Any voter can re-submit it as a new proposal.

An upstream failure while running the checks is not a refusal. Jupiter, Privy or RPC down returns an `Unavailable` code (`JupiterUnavailable` and the like), which is retryable, so `bus.Dispatch` naks with backoff and the checks run again on redelivery ([Surfacing](backend-platform.md#surfacing)). After `MaxDeliver` the message goes to `DEADLETTER`.

**Hand-off.** If all checks pass, the engine calls the swap layer with the proposal id (or intent id) as the idempotency key.

### Stage 3: swap layer

```
created ──► submitted ──► confirmed
   │            │
   │            └───────► failed
   └────────────────────► failed   (sweeper: never_submitted)
```

The status is a Go type with a `transitions` table and a pure `Next(from, event)`. Adapters apply it as a guarded update, and `exhaustive` fails a missed case ([State machine](backend-platform.md#patterns-and-where-each-earns-its-place)).

1. **Insert `created`.** New `swaps` row: `proposal_id` or `intent_id`, `cabal_id`, `action`, mints, `amount`, `status = created`. Quote Jupiter, sign with the treasury's Privy wallet (app-owned, `PRIVY_AUTHORIZATION_*`), and then store Jupiter's `requestId` and the signed transaction bytes and flip to `submitted` in one guarded write that commits before any send. The signed tx is persisted **before** the network call, so a crash can never lose track of a transaction that may have landed, and a `created` row was never signed or sent.
2. **Call `/execute`.** Jupiter lands the transaction and polls the chain itself. The response is normally terminal:
   - `Success` → guarded update to `confirmed`, set `tx_signature`, `confirmed_at`, fill amounts, cost basis.
   - `Failed` → guarded update to `failed` with Jupiter's error code.
   - `Pending` → re-send the same `/execute` with the stored `requestId` every 2 s. Safe for up to 2 minutes; Jupiter dedupes on `requestId`, so this never double-executes.
3. **One `uow.Do` per terminal transition.** It does, atomically:
   - `UPDATE … SET status = $new, … WHERE id = $id AND status = 'submitted'`. If rows affected = 0, another path already finished it: roll back and return.
   - Append `trade.confirmed` or `trade.failed`. The payload carries the fill amounts as a snapshot for consumers.
   - Nothing else. Consumers do the rest ([flow 11](backend-platform.md#flows)): `treasury` writes the `cabal_txns` header and ledger entries in the same transaction as its own event, `governance` sets the proposal to `executed` and emits `proposal.executed`, and `ranking`, `feed` and `notify` react.
4. **Sweeper.** A poller in the worker (every 30 s, under the poller advisory lock from [Deploy rule 1](backend-platform.md#deploy-and-observability)). It moves `created` rows older than 2 minutes to `failed` with `never_submitted`, with no on-chain lookup. That is safe because the signed bytes and the `submitted` status commit together before any send, so a `created` row was never sent. Failing it frees the claim for a retry. It also selects rows in `submitted` older than 2 minutes. It calls `getSignatureStatuses` on the signature derived from the stored bytes and resolves through the same guarded update. Finalized → `confirmed`. Not found after blockhash expiry → `failed` with `blockhash_expired`. Still processing → leave for the next tick. This is the crash-recovery path and the only path that can finish a row the original process abandoned. There is no proposal poller behind it: a `proposal.passed` that no handler finished lands in `DEADLETTER`, and `monacoctl deadletter retry` is the one recovery path ([flow 27](backend-platform.md#flows)).
5. **Optional Helius webhook.** An address webhook on treasury wallets posts confirmed transactions. The handler looks up the row by `tx_signature` and runs the same guarded update. It sometimes wins the race with `/execute`, which is fine: the loser sees zero rows affected and stops. Its purpose is faster UI, not correctness. Not required for launch.

**Invariants**

- `tx_signature` is unique. `execute_request_id` is unique.
- At most one `swaps` row per proposal (or intent) in a non-`failed` state: a partial unique index on the source id `WHERE status <> 'failed'`.
- Status only moves forward. No path writes `submitted` back to `created`, or `confirmed` to anything. Only the sweeper moves `created` to `failed`.
- The `events` table is append-only and is the audit log. The swap status is a projection of the latest event.
- `events` rows are written only inside the same transaction as the state change they announce. Nothing publishes to NATS except the relay.

**Crash points.** Flows 11 and 12 list these in [`flows.tsv`](backend-platform.md#outcomes-as-a-map), each with a crash-point test that restarts and asserts convergence:

| Crash point | What survives | What converges it |
| --- | --- | --- |
| `after-insert` | `created` row, nothing signed or sent | Sweeper: older than 2 minutes → `failed` with `never_submitted`; retry allowed |
| `after-sign` | `submitted` row with signed bytes, never sent | Sweeper: not found after blockhash expiry → `failed` |
| `after-execute` | `submitted` row; the swap may have landed | Sweeper: finalized → `confirmed` |
| `before-commit` | Terminal transition rolled back; row still `submitted` | Sweeper, same as above |
| `after-publish` | Event published, `published_at` not set | Relay republishes; JetStream drops it on `Nats-Msg-Id`, and `event_deliveries` drops any second delivery |

### Delivery

Relay, streams (`EVENTS`, `DEADLETTER`), consumer settings, retries and dedupe on `event_deliveries` are in [event-bus.md](event-bus.md) and, where they differ, [NATS hosting and budget](backend-platform.md#nats-hosting-and-budget). Trade execution is the first domain on the bus.

### Retry and manual paths

- `RetryTrade` ([flow 12](backend-platform.md#flows)) re-runs the engine for the proposal: re-check, then a new swap row. Allowed only when the latest `swaps` row is `failed`. It takes an `Idempotency-Key` like every mutating call ([Thin client](backend-platform.md#thin-client)). The trade parameters come from the failed `swaps` row, not from governance tables. The route is `POST /v1/swaps/{id}/retry` (flow 12 in the [flows table](backend-platform.md#flows)).
- An ops command to force-resolve a stuck `submitted` row by signature, going through the same guarded update. It runs from `monacoctl` or the admin module ([flow 26](backend-platform.md#flows)). Useful when Jupiter and RPC disagree.

## Alternatives considered

| Alternative | Why not |
| --- | --- |
| **Execute inline in the vote handler** when the tally passes | Ties a user's HTTP request to a 2-minute swap. Crash mid-request loses the trade. |
| **A proposal poller as the trigger or as a safety net** (the old backend's 15 s poller) | A scan, not an event: 15 s latency on every trade, and no recorded reason for why execution did not happen. As a safety net it would be a second recovery path next to `DEADLETTER` and `monacoctl deadletter retry`. Dropped (default 2026-09-27). |
| **Direct calls for notifications and feed** from the swap code | Loses events when a downstream service is down, or fires them for trades that then roll back. Also crosses module walls, which `depguard` forbids. |
| **Trading writes the ledger in the swap transaction** | `cabal_txns` belongs to the `treasury` module ([repository layout](backend-platform.md#repository-layout)). Trading owns `swaps`; treasury writes `cabal_txns` as a consumer of `trade.confirmed`. |
| **Pause state inside trading, fed by an event** | Leaves a window between detection and the consumer running, and treasury would need its own copy. Funding owns the pause and both read it at check time. |
| **Slippage measured against the quote at pass time** | Fresher, but not what voters saw. The reference is `quote_out_amount`, the quote at proposal creation, shown in the proposal UI. |
| **Status polling on chain instead of Jupiter `/execute`** | Jupiter already does this and handles priority fees and rebroadcast. We use the chain read only as the crash fallback. |
| **Pre-trade checks at proposal time only** | Pot and price move during voting. Checking again at execute is the only guarantee. |
| **Auto-retry blocked proposals** (e.g. slippage) | Rejected for now. A cabal voted on a specific price expectation; silently retrying later trades on a price they did not approve. Voters re-propose instead. Upstream outages are retried by the bus, because they are `Unavailable`, not `Blocked`. |
| **Custom on-chain program for governance and execution** | No custody or trust benefit at our scale. Privy plus Postgres already gives one signer and one ledger. Large cost, more surface, slower iteration. |

## Gap between this and the code

The rewrite replaces the old backend rather than refactoring it ([backend-platform.md](backend-platform.md)). This list records what the old backend does today (checked 2026-09-27 in `apps/backend`), so behavior is known during cutover:

- Propose, vote, tally, `passed` status: `internal/app/governance.go`.
- Proposal execute poller every 15 s; `ExecuteOnPassService` runs buy or sell via `SwapService`: `internal/app/start_buy.go`, `internal/worker/proposal_execute_poller.go`.
- `SwapService` quotes, signs, submits, and awaits fill through the `swapprovider.Provider` interface (Jupiter or Flash): `internal/app/swap.go`, `internal/swapprovider/provider.go`.
- `transactions` statuses: `pending`, `confirmed`, `failed`. Idempotent on `tx_signature` and `execute_request_id`.
- Proposal-time checks (route exists, amount ≤ pot).
- `POST /v1/transactions/{id}/retry`.

What the old backend lacks and the rewrite builds in Rollout step 4: `created` and `submitted` statuses with signed bytes and `requestId` stored before the network call; the guarded update as the only terminal-transition primitive; the trade engine with the check table and `trade.blocked`; the `swaps` table; the sweeper; slippage tolerance as a cabal rule. The Helius webhook comes later. Events, relay and `bus.Dispatch` land first, in step 2.

## Open questions

None.

**Deferred.** Pre-IPO transfer-fee handling (decided 2026-09-27).

## Log

- 2026-09-29: `RetryTrade` route is `POST /v1/swaps/{id}/retry` (default; see #535).
- 2026-09-27: Decided 2026-09-27: pre-IPO transfer-fee handling is deferred. The headroom rule stays as written, because it reads any mint's transfer-fee config. No checks remain.
- 2026-09-27: Default 2026-09-27 (reversible): A17 restated. The sweeper fails `created` rows older than 2 minutes with `never_submitted`, with no on-chain lookup. It is safe because the signed bytes, `requestId` and the `submitted` status commit in one guarded write before any send, so a `created` row was never signed or sent. `submitted` rows keep the `getSignatureStatuses` path. A chain-check variant was drafted and withdrawn the same day.
- 2026-09-27: Applied the 2026-09-27 decisions. Default 2026-09-27 (reversible): trading owns a `swaps` table and treasury writes `cabal_txns` as a consumer of `trade.confirmed`, in the same transaction as its own event; the pause is owned by `funding` and read through its query port at check time, ops pauses included; governance emits `proposal.executed` and `proposal.execution_blocked`; the sweeper fails `created` rows older than 2 minutes (new `after-insert` crash point); the 15 s safety-net poller is dropped in favor of `DEADLETTER` plus `monacoctl deadletter retry`; the engine re-checks agent budget; `InsufficientFunds` never auto-retries; slippage is measured against the proposal-time quote; fee headroom comes from the mint's transfer-fee config. Open: verify pre-IPO mints expose that config.
- 2026-09-27: Reconciled with [backend-platform.md](backend-platform.md). Stages named by module (`governance`, `trading`); the claim reads only trading's tables and the engine reads trade parameters from the event payload. Pre-trade failure codes become `errs` codes of kind `Blocked` (`group_frozen` renamed `CabalPaused`); upstream failures are retryable `Unavailable` codes the bus naks. `treasury` writes the ledger as a consumer of `trade.confirmed`, not in the swap transaction. Agent intents use the same engine (flow 17). Crash-point table for flows 11 and 12, `uow.Do`, state-machine pattern, sweeper under the poller lock, `RetryTrade` command, `cabal_id` naming, Rollout step 4. New open questions: stuck `created` rows, pause lag, safety-net poller, agent budget re-check, swap table name.
- 2026-09-26: Outbox worker replaced by the NATS event bus ([event-bus.md](event-bus.md)). Swap transitions write only an `events` row; side effects are consumers.
- 2026-09-26: Pre-trade check also blocks cabals paused for an external deposit ([deposits-withdrawals.md](deposits-withdrawals.md#direct-transfers-to-a-cabal-treasury-are-not-allowed)).
- 2026-09-26: Stage 1 detail moved to proposals.md.
- 2026-09-26: Initial decision recorded from design discussion. Swap-layer idempotency design (guarded update, requestId re-poll, sweeper, optional Helius) carried over from prior chat; governance → engine → swap staging and pre-trade check table added.
