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

- The `proposal.passed` payload carries everything the engine needs, so the engine never reads governance tables: `proposal_id`, `cabal_id`, `kind` (`buy` or `sell`), `symbol`, `mint`, `usdc_micros` (`money.Micros`, buy) or `token_amount` (a `uint64` of the mint's base units, encoded as a JSON string, sell), `quote_out_amount`, `proposer_id`. Amounts are integers ([Money and types](backend-platform.md#money-and-types)).
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
| Route exists | Jupiter returns a route for the exact-in amount. When it doesn't, one probe quote (1 USDC on a buy, at most one whole token on a sell) tells `NoRoute` (the probe routes, so a smaller amount may work) from `AssetPaused` (the probe doesn't route either, or the amount is already at or below it) | Same | `NoRoute`, `AssetPaused` |
| Cabal not paused | Cabal not banned (cabal query port), and no active pause in funding's pause record, whether for an unresolved external deposit or for ops (funding query port) | Same | `CabalPaused` |
| Agent budget (intents only) | Intent fits the agent's remaining budget, re-read through the agents module's query port | Same | `AgentBudgetExceeded` |

The price is not a pre-trade check. A cabal votes on an amount (USDC to spend, or tokens to sell), not a price: at execution the engine takes a fresh Jupiter quote for that amount, and it becomes the swap's `quote_out_amount`. The swap layer holds the live order to the cabal's slippage around that execution quote, so a move since the vote never blocks a trade; only a move between the execution quote and the order ends as a retryable `price_moved` swap. Slippage tolerance is a cabal rule with a platform default (proposal: 100 bps default, cabal can tighten, platform caps at 300 bps). The engine reads it through the cabal module's query port. Treasury USDC is read from chain, not from the ledger, matching the platform-balance rule elsewhere. Every amount and bps value is an integer.

The pause is owned by `funding` and read at check time, so there is no window between detection and pause and no pause state in trading ([deposits-withdrawals.md](deposits-withdrawals.md#pause)). Buy fee headroom is computed from the mint's transfer-fee config, not a fixed buffer or a guessed percentage. The relayer pays SOL fees, so headroom only covers token transfer fees. Pre-IPO transfer-fee handling is deferred (decided 2026-09-27). `SubmitAgentIntent` checks the agent budget at submit ([flow 17](backend-platform.md#flows)); the engine checks it again here, because checks at execution time are the guarantee.

A refused check appends `trade.blocked` with the code and the numbers it compared, records the delivery, acks, and stops. Governance consumes `trade.blocked`, sets the proposal to `execution_blocked` and emits `proposal.execution_blocked`. The engine logs the decision once, with `have` and `need` ([Logs as evidence](backend-platform.md#logs-as-evidence)). It never retries a refusal, `InsufficientFunds` included, even when a fund-to-cabal sweep is in flight. Any voter can re-submit it as a new proposal.

An upstream failure while running the checks is not a refusal. Jupiter, Privy or RPC down returns an `Unavailable` code (`JupiterUnavailable` and the like), which is retryable, so `bus.Dispatch` naks with backoff and the checks run again on redelivery ([Surfacing](backend-platform.md#surfacing)). After `MaxDeliver` the message goes to `DEADLETTER`.

**Hand-off.** If all checks pass, the engine calls the swap layer with the proposal id (or intent id) as the idempotency key. Once the `created` row commits, the engine reads the proposal's status through governance's query port. If the proposal is no longer `passed`, the row fails with `source_cancelled` before anything is signed. A void that lands after the row exists is refused because a live swap exists, so the two cannot race. If that read fails, the row fails with `never_submitted` and the bus naks, so the redelivery makes a fresh claim.

### Stage 3: swap layer

```
created ──► submitted ──► confirmed
   │            │
   │            └───────► failed
   └────────────────────► failed   (sweeper: never_submitted)
```

The status is a Go type with a `transitions` table and a pure `Next(from, event)`. Adapters apply it as a guarded update, and `exhaustive` fails a missed case ([State machine](backend-platform.md#patterns-and-where-each-earns-its-place)).

1. **Insert `created`.** New `swaps` row: `proposal_id` or `intent_id`, `cabal_id`, `action`, mints, `amount`, `status = created`. Order from Jupiter with `taker` set to the treasury and `payer` set to the relayer, because treasuries hold no SOL and the relayer pays every swap's fee. Before signing, the swap layer holds the order to the Review floor `F = MinOut(quote_out_amount)` at the cabal's slippage. Jupiter returns `otherAmountThreshold`, the minimum output the transaction enforces on chain at the requested `slippageBps`. An order whose `outAmount` is below `F` fails with `price_moved`, because the market moved past the cabal's limit. An order whose `otherAmountThreshold` is at least `F` is signed. Otherwise the layer requests one more order at `floor((outAmount − F) × 10000 / outAmount)` bps, the widest slippage that keeps the threshold at `F`, and applies the same two checks to it. A re-order whose threshold is still below `F` fails with `price_moved`. No signed transaction can fill below the Review floor. Sign with the treasury's Privy wallet (app-owned, `PRIVY_AUTHORIZATION_*`), then co-sign as fee payer with the relayer key. The signer refuses the transaction unless the relayer's signature 0 and the treasury's signature are both present, and the swap stores signature 0, the relayer's, as `tx_signature`. Then store Jupiter's `requestId` and the signed transaction bytes and flip to `submitted` in one guarded write that commits before any send. The signed tx is persisted **before** the network call, so a crash can never lose track of a transaction that may have landed, and a `created` row was never signed or sent.
2. **Call `/execute`.** Jupiter lands the transaction and polls the chain itself. The response is normally terminal:
   - `Success` → guarded update to `confirmed`, set `tx_signature`, `confirmed_at`, fill amounts, cost basis.
     Fills come from chain deltas. A sell's fill is the treasury's USDC token-account delta in the confirmed transaction (`getTransaction` pre and post token balances), never Ultra's `outputAmountResult`. Jupiter takes its fee from a sell's USDC output in a second transfer out of the treasury's account, so `outputAmountResult` is the pre-fee figure, 10 bps above what the treasury kept (#5279). A buy keeps `outputAmountResult`, which equals the delta of the bought token. If the delta can't be read, or is zero, the swap stays `submitted`, `trading.swap.fill_unread` alerts, and the sweeper confirms it from the same delta on a later tick. The sweeper reads the delta for buys and sells alike. Sells confirmed before this rule recorded the pre-fee figure. `monacoctl swaps overstated-sells` reads each confirmed Solana sell's delta and lists the sells recorded above it, with the overstatement per cabal, and the sells whose delta it couldn't read. It changes nothing; correcting the ledger is an operator action.
   - `Failed` → guarded update to `failed` with Jupiter's error code.
   - `Pending` → re-send the same `/execute` with the stored `requestId` every 2 s. Safe for up to 2 minutes; Jupiter dedupes on `requestId`, so this never double-executes.
3. **One `uow.Do` per terminal transition.** It does, atomically:
   - `UPDATE … SET status = $new, … WHERE id = $id AND status = 'submitted'`. If rows affected = 0, another path already finished it: roll back and return.
   - Append `trade.confirmed` or `trade.failed`. The payload carries the fill amounts as a snapshot for consumers.
   - Nothing else. Consumers do the rest ([flow 11](backend-platform.md#flows)): `treasury` writes the `cabal_txns` header and ledger entries in the same transaction as its own event, `governance` sets the proposal to `executed` and emits `proposal.executed`, and `ranking`, `feed` and `notify` react.
4. **Sweeper.** A poller in the worker (every 30 s, under the poller advisory lock from [Deploy rule 1](backend-platform.md#deploy-and-observability)). It moves `created` rows older than 2 minutes to `failed` with `never_submitted`, with no on-chain lookup. That is safe because the signed bytes and the `submitted` status commit together before any send, so a `created` row was never sent. Failing it frees the claim for a retry. It also selects rows in `submitted` older than 2 minutes. It calls `getSignatureStatuses` on the stored signature and resolves through the same guarded update. Finalized → `confirmed`. For a not-found signature, it decodes the stored signed transaction and asks `isBlockhashValid`; only an invalid blockhash followed by a fresh not-found status becomes `failed` with `blockhash_expired`. Still processing, including an unfinalized error, stays pending for the next tick. The recovery migration adds `out_decimals` and `source_batch_size` to the swap row. Historical rows whose out mint is neither configured USDC nor a catalog asset retain `out_decimals = -1`; the sweeper refuses to guess their amount precision and leaves them retriable. Rows that fail local recovery validation rotate by `updated_at`, so they do not starve newer stale rows. This is the crash-recovery path and the only path that can finish a row the original process abandoned. There is no proposal poller behind it: a `proposal.passed` that no handler finished lands in `DEADLETTER`, and `monacoctl deadletter retry` is the one recovery path ([flow 27](backend-platform.md#flows)).
5. **Optional Helius webhook.** An address webhook on treasury wallets posts confirmed transactions. The handler looks up the row by `tx_signature` and runs the same guarded update. It sometimes wins the race with `/execute`, which is fine: the loser sees zero rows affected and stops. Its purpose is faster UI, not correctness. Not required for launch.

**Invariants**

- `tx_signature` is unique. `execute_request_id` is unique.
- At most one `swaps` row per proposal (or intent) in a non-`failed` state: a partial unique index on the source id `WHERE status <> 'failed'`.
- Status only moves forward. No path writes `submitted` back to `created`, or `confirmed` to anything. Only the sweeper (`never_submitted`) and the engine's hand-off (`source_cancelled`, or `never_submitted` when the status read fails) move `created` to `failed`.
- The `events` table is append-only and is the audit log. The swap status is a projection of the latest event.
- `events` rows are written only inside the same transaction as the state change they announce. Nothing publishes to NATS except the relay.

**Crash points.** Flows 11 and 12 list these in their [flow files](backend-platform.md#outcomes-as-a-map), each with a crash-point test that restarts and asserts convergence:

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

- `RetryTrade` ([flow 12](backend-platform.md#flows)) re-runs the engine for the proposal: re-check, then a new swap row. Allowed only when the latest `swaps` row is `failed`. It takes an `Idempotency-Key` like every mutating call ([Thin client](backend-platform.md#thin-client)). The trade parameters come from the failed `swaps` row, not from governance tables. The route is `POST /v1/swaps/{id}/retry` (flow 12 in the [flows table](backend-platform.md#flows)). Every retry takes a fresh execution quote, like the first run. The body may carry `at_current_price: true` for a `price_moved` swap; it is accepted for older clients and changes nothing. The client never sends a price.
- An ops command to force-resolve a stuck `submitted` row by signature, going through the same guarded update. It runs from `monacoctl` or the admin module ([flow 26](backend-platform.md#flows)). Useful when Jupiter and RPC disagree.

### Routing across chains

**Status:** Decided 2026-10-11 in M24 ([#4917](https://github.com/Munyon-Canyon/monaco/issues/4917), "Plan addition: best-price routing across chains"). Built by #4969 (listings, quoters and router), #4970 (execution, bridging and the step journal) and #4971 (valuation). [Flow 30](backend-platform.md#flows) runs the whole path against fakes.

A stock can trade on several chains, from several issuers. Every trade buys or sells at the best all-in price across them, and the cabal never sees a chain: one price, one holding per stock.

**Rules that don't change.** Every user balance and every cabal's idle cash stay on Solana. Only USDC crosses chains; a stock stays on the chain it trades on. A cabal still votes on an amount, the vote-time quote is display only, and slippage is measured only against the execution quote.

#### Listings

`market` owns `asset_listings`: one row per issuer per stock per chain, with the token address (and, for xStocks on EVM, its `w<symbol>` wrapper), decimals, `shares_per_token` (how many underlying shares one token is worth, refreshed every catalog run, since Ondo's grows with reinvested dividends) and the venue (`jupiter` on Solana, `uniswap` on EVM). A listing starts disabled and is enabled only after it is verified on chain: an EVM token's `symbol()` and `decimals()` match the issuer, a Solana mint exists under SPL Token or Token-2022. `set_tradable_override` applies per listing. The seed and the issuers' APIs are in `docs/how-to/stock-listings.md` (#4969).

| Issuer | Networks |
| --- | --- |
| xStocks | Solana, Ethereum, Arbitrum |
| Ondo | Solana, Ethereum |
| Coinbase (B20) | Base |
| Robinhood | Robinhood Chain (cash asset USDG, not USDC) |

**At launch** (real quotes from #4969, 2026-10-11):

- **xStocks on Ethereum and Arbitrum stay disabled.** The Uniswap Trading API found no route for them on Arbitrum, and 31–99% price impact on Ethereum at cabal trade sizes. They stay in `asset_listings` with `enabled = false`, and the router never quotes them.
- **Coinbase B20 on Base routes through Uniswap.** Its pools are Uniswap v4 hooked pools, which the Trading API quotes and routes, so no aggregator such as Kyber is needed.

#### Quote, compare, pick

`Router.Best(stock, side, amount)` (`market`):

1. Quotes every enabled listing of the stock in parallel, each with a timeout: Jupiter for Solana listings, the Uniswap Trading API for EVM ones, both behind the `multichain.Quoter` interface.
   Each listing gets `MONACO_TIMEOUT_UNISWAP_QUOTE` (default 4 s) for its bridge estimate, quote and simulation. Execution keeps its own Uniswap client timeout, `MONACO_TIMEOUT_UNISWAP` (default 5 s). Listings on the same EVM network share one bridge estimate per comparison. An EVM buy quotes the default and hookless Uniswap routes side by side and keeps the better route that finished within the budget. A listing that runs out of time is listed as `stale` and logged with `upstream_unavailable`, its error and its `duration_ms` in `market.router.venue_dropped`. The comparison returns once every listing has a quote or a drop, so a slow EVM venue never holds back the Solana quote past the budget. On 2026-10-11 a $1 buy measured 0.4–0.75 s for the bridge estimate, 0.15–1.5 s for the default quote, 1.5–4.4 s for the hookless quote (2.2–2.5 s alone) and under 0.1 s for the swap build and simulation.
2. Converts each quote to **net underlying shares** (a sell: net USDC), after every cost as a line item: swap price and price impact, pool or route fees, the Privy bridge fee for an EVM listing (7.5 bps each way on 2026-10-10), and destination gas.
3. Drops quotes that are expired, older than 15 s, or above the price-impact ceiling.
4. Ranks the rest. Solana stays the winner unless an EVM route beats it by at least `ROUTER_MIN_EDGE_BPS` (default 10), which pays for the bridge's latency and extra failure surface.

`propose_trade` and `preview_trade` call it, store the winner's quote as the display quote, and return the ranked list as `price_comparison` (venue, network, net shares or USDC, each cost, and the winner). When the proposal passes, the comparison is written to `trade_routes` (`comparison` jsonb, `winner_listing_id`) through the pass event, and the engine executes against that winner. `trade.routed` records the decision on the swap.

A buy's route check is the router. `preview_trade`, `propose_trade` and the live comparison of an open proposal ask `Router.Best`, or `Router.BestOn` for a buy pinned to one chain, and the Solana-only `CheckRoute` gates sells. When the router answers `no_route` for a buy, the gate falls back to `CheckRoute` on the asset's own Solana mint, as execution does (a buy the router can't route runs as a plain Solana swap), so a Solana stock with no `asset_listings` row still trades. A buy gets `no_route` only when every listing dropped with no route or over the impact ceiling and the Solana mint has no route either. When no listing quoted and at least one dropped for another reason (a timeout, an upstream error, a stale or expired quote, a failed simulation), the answer is `quote_unavailable`, which the app shows as "try again" instead of "try another stock". A Solana listing that Jupiter has no route for lets an EVM listing win; a Solana quote that failed for any other reason still fails the router closed with `quote_unavailable`.

Jupiter's `/order` sends some stocks to its market-maker router, JupiterZ, which answers HTTP 400 `Quote not available from market maker` when no maker quotes. The Jupiter client then asks once more with `excludeRouters=jupiterz`, so the aggregator can route the order. On 2026-10-11 that refusal made NFLXx look unroutable at $1 while the aggregator quoted it.

A member can pin a buy to one chain with `network` on the proposal. The stored comparison stays the router's, and the display quote is the pinned chain's best listing. At execution a pinned buy re-quotes only its pinned network's best listing (an EVM pin asks `Router.BestOn`, which skips the Solana quote) and never changes chain. A retry reads the pin from `trade_routes.pinned_network`. When the pinned chain has no route, the proposal ends `execution_blocked` with `pinned_chain_unavailable`; when trading on that EVM chain is off, with `chain_trading_off`. A Solana pin keeps the Solana-only fallback when the router fails.

#### Simulation account

An EVM quote is simulated from one app-owned Privy EVM wallet, never from a cabal treasury (#5336). The wallet has the treasuries' ownership: app-owned, with the `PRIVY_AUTHORIZATION_KEY_ID` quorum as owner. `market` stores its Privy id and address in `evm_simulation_account`, one row per database, so each environment has its own wallet under its own Privy app. The wallet is never funded: the simulation sets the cash balance and the Permit2 allowance with `eth_call` state overrides.

The simulation runs through the account's ERC-7579 `execute`, so the account needs an EIP-7702 delegation on each chain. `monacoctl market simulation-account` sets it up and is safe to rerun:

1. It reads the stored wallet, or creates one (Privy idempotency key `market:simulation-account`) and stores it.
2. For each EVM chain with an RPC URL, it reads `eth_getCode`. A chain that has no delegation gets one zero-value self-transfer through Privy `eth_sendTransaction` with `sponsor: true`, so Privy's paymaster pays the gas and attaches the delegation.
3. It prints each chain's code and the delegate Privy set, and exits 1 if any chain stays undelegated after `--timeout` (default 2 m).

#### Execute on an EVM chain

A Solana winner takes the path above, unchanged. An EVM winner runs as a journal of steps, one `trade_steps` row per step, unique on `(trade_id, step)`. Each step stores its Privy action id before it polls, so a crash resumes the step instead of repeating it, and each change appends `trade.step_changed`.

```
buy:   reserve cash ─► bridge_out ─► requote ─► swap ─► record fill (cabal_positions.listing_id)
sell:  swap ─► bridge_back ─► settle as today
```

1. **Reserve** the trade amount of treasury USDC (the existing reservation).
2. **`bridge_out`.** `WalletActions.Transfer` of exactly that amount from the cabal's Solana treasury to its EVM treasury on the winning network. On Robinhood Chain the transfer delivers USDG. While it runs, a `treasury_in_transit` row holds the amount, so the pot total doesn't dip during the bridge. The cabal's EVM treasury is created on its first EVM trade (`EnsureTreasuryEVMWallet`, `treasury_evm_wallets`): one app-owned address serves every EVM network.
3. **Requote** the winning listing after the bridge lands. That quote is the execution quote.
4. **`swap`** on the EVM chain from the EVM treasury, signed through Privy, held to the cabal's slippage around the execution quote.
5. **Record** the fill with the position's `listing_id`, through `trade.confirmed` as before.

A sale of an EVM position swaps to USDC on that chain, then `bridge_back` returns the proceeds to the Solana treasury (in transit until it lands), then settles as today. When a cabal holds the same stock on several chains, a sale (and a cash out's sale) takes lots by best net proceeds after the bridge back.

#### Partial failure

| What failed | Where the money is | What happens |
| --- | --- | --- |
| `bridge_out` rejected, or failed with no on-chain step | Solana treasury | The reservation is released and the swap fails as usual. Retry with `RetryTrade`. |
| Bridge landed, `swap` failed | USDC on the EVM treasury | It stays there as cabal cash and still counts in the pot total. The swap step retries under the `RetryTrade` and force-resolve rules, or `bridge_back` returns the cash to Solana. |
| Sale's `swap` landed, `bridge_back` failed | USDC on the EVM treasury | Counted in the pot as cash. Nothing retries it on its own. `monacoctl swaps failed-bridge-backs` lists each failed `bridge_back`. `monacoctl swaps retry-bridge-back --swap <id> --reason <text>` sets the step back to `pending` under the next attempt, adds the failed attempt's Privy action id, if it has one, to `prior_privy_action_ids`, and writes a `swap_operator_actions` row. The EVM trade poller then sends it again under `trade:<swap>:bridge_back:<attempt>`. Before it sends, it reads every earlier attempt's Privy action: if one landed late, the step adopts it and nothing new is sent, and if one is still pending the poller asks again on the next tick. An attempt Privy refused before sending (no action id) is sent again without that check. Attempt 2 and later post under their own ledger ids, and a cash out's sale, already settled on attempt 1's failure, is not settled again. |
| A routed swap parked: its resume failed with an error that won't retry (`swaps.parked_at`, `parked_code`) | Wherever its last step left it: the Solana treasury before `bridge_out` landed, USDC on the EVM treasury after. The cash hold stays. | `trading.routed_swap.parked` alerts once and the EVM trade poller skips the swap. `monacoctl swaps parked` lists parked swaps with each step's status. `monacoctl swaps unpark --swap <id> --reason <text>` clears the park, and the poller re-drives the swap under the same step keys. It refuses once the stranded cash has left (a `bridge_back` step, or another trade reused it). `monacoctl swaps fail-parked --swap <id> --reason <text>` ends the swap `failed` with `never_submitted` and releases the hold, only when no step was broadcast or confirmed: the swap is `created`, its `swap` step is absent or failed, and `bridge_out` is not `pending` or `submitted`. Cash a succeeded `bridge_out` left on the EVM treasury goes back through the stranded-cash rule once the proposal can't retry. Each action writes a `swap_operator_actions` row with the operator and the reason. |

#### Valuation

The pot total is Solana USDC, plus USDC on every treasury EVM wallet, plus every position at its listing's price per underlying share, plus `treasury_in_transit`. An EVM listing's price is its Uniswap mid price converted with `shares_per_token`, checked against the Solana listing of the same stock (a gap over 2% is flagged). Leaderboards, P&L and share issuance read that one total. Holdings are grouped by stock on `/pot`, with an optional per-listing breakdown; the app never splits a stock by chain.

## Alternatives considered

| Alternative | Why not |
| --- | --- |
| **Execute inline in the vote handler** when the tally passes | Ties a user's HTTP request to a 2-minute swap. Crash mid-request loses the trade. |
| **A proposal poller as the trigger or as a safety net** (the old backend's 15 s poller) | A scan, not an event: 15 s latency on every trade, and no recorded reason for why execution did not happen. As a safety net it would be a second recovery path next to `DEADLETTER` and `monacoctl deadletter retry`. Dropped (default 2026-09-27). |
| **Direct calls for notifications and feed** from the swap code | Loses events when a downstream service is down, or fires them for trades that then roll back. Also crosses module walls, which `depguard` forbids. |
| **Trading writes the ledger in the swap transaction** | `cabal_txns` belongs to the `treasury` module ([repository layout](backend-platform.md#repository-layout)). Trading owns `swaps`; treasury writes `cabal_txns` as a consumer of `trade.confirmed`. |
| **Pause state inside trading, fed by an event** | Leaves a window between detection and the consumer running, and treasury would need its own copy. Funding owns the pause and both read it at check time. |
| **Slippage measured against the quote from the vote** | Rejected on 2026-10-09 (#4588). A cabal buys an amount, not a share count: holding the order to the vote-time quote failed every buy whose stock moved more than the slippage since the vote. The reference is the quote taken at execution; the vote-time `quote_out_amount` is display only. |
| **Self-run bridging** (our own EVM RPC client, scanner, nonces, gas top-ups and burn messages) | The first M24 plan. Privy's `/transfer` runs detection, gas and bridging for USDC, so the plan was replaced on 2026-10-10 ([#4917](https://github.com/Munyon-Canyon/monaco/issues/4917)). |
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

Log: [log/trade-execution.md](log/trade-execution.md).
