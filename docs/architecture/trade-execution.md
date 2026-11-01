# Trade execution

**Status:** Decided 2026-09-26. Reconciled 2026-09-27 with [backend-platform.md](backend-platform.md), which wins where the two conflict. Built in [Rollout](backend-platform.md#rollout) step 4 (flows 9 to 13, with crash-point tests). The old backend has part of it; see [Gap between this and the code](#gap-between-this-and-the-code).

## Decision

A trade moves through three stages, each owned by a different module, joined by an event:

1. **Governance** (`governance` module). A member proposes a trade. Voters in the cabal vote. The tally marks the proposal `passed`, `failed`, or `expired`.
2. **Trade engine** (`trading` module). A passed proposal emits a `proposal.passed` event. The trade engine consumes it, runs pre-trade checks (treasury has the funds, price is inside slippage tolerance, asset is tradable), and if every check passes, hands the trade to the swap layer.
3. **Swap layer** (`trading` module). The swap layer inserts a row in trading's `swaps` table, gets a signed transaction from Jupiter, calls Jupiter `/execute`, and drives the row to exactly one terminal state through a single guarded update. Every path that can finish a swap (the `/execute` response, a Pending re-poll, a crash sweeper, an optional Helius webhook) goes through that same update, so a swap has one current state and a full history no matter which path finished it.

Modules never call each other's packages ([dependency rules](backend-platform.md#dependency-rules-enforced-by-depguard)). Stages hand off through NATS JetStream. Each state change appends an `events` row inside the same `uow.Do` transaction ([Unit of Work](backend-platform.md#patterns-and-where-each-earns-its-place)), and the relay publishes it. Side effects of a finished trade (the ledger, notifications, feed items, rankings, closing the proposal) are consumers of `trade.confirmed` / `trade.failed`. The swap path does not know which consumers exist.

## Why

- **Voting is the product; execution is plumbing.** Splitting them keeps governance rules (voter sets, majority vs unanimous, expiry) free of Solana and Jupiter concerns.
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

A durable JetStream pull consumer in the `trading` module, filtered on `proposal.passed`, run through `bus.Dispatch`. One proposal maps to at most one live execution attempt. The handler calls `msg.InProgress()` while a swap is in flight so the ack deadline does not expire during a 2-minute `/execute`. Its idempotency comes from its own claim, not from the single `event_deliveries` transaction; see [event-bus.md](event-bus.md#consumers-and-handlers).

**Claim.** In trading's own tables only. If `event_deliveries` already holds this event for `trading`, or a `swaps` row for the proposal exists in `created`, `submitted`, or `confirmed`, ack and stop (idempotent redelivery). A `failed` row does not block a retry. The partial unique index on `swaps` is what makes the claim safe under concurrent deliveries.

**Pre-trade checks**, in order. Each refusal is an [`errs` code](backend-platform.md#errors) of kind `Blocked`:

| Check | Buy | Sell | Code |
| --- | --- | --- | --- |
| Asset tradable | Mint resolves in the market catalog; mint not paused (pre-IPO tokens can pause) | Same | `AssetUntradable` |
| Treasury funds | Treasury on-chain USDC ≥ `usdc_micros` | Treasury holds ≥ `token_amount` of the mint | `InsufficientFunds` |
| Within the trade cap | A cabal with a max-per-trade rule refuses a buy over that share of its ledger cash after cash-out reservations, read again now; a retry that reuses stranded cash skips it | Not checked | `OverTradeCap` |
| Route exists | Jupiter returns a route for the exact-in amount. When it doesn't, one probe quote (1 USDC on a buy, at most one whole token on a sell) tells `NoRoute` (the probe routes, so a smaller amount may work) from `AssetPaused` (the probe doesn't route either, or the amount is already at or below it) | Same | `NoRoute`, `AssetPaused` |
| Cabal not paused | Cabal not banned (cabal query port), and no active pause in funding's pause record, which is an ops pause (funding query port); an external deposit does not pause | Same | `CabalPaused` |

The price is not a pre-trade check. A cabal votes on an amount (USDC to spend, or tokens to sell), not a price: at execution the engine takes a fresh Jupiter quote for that amount, and it becomes the swap's `quote_out_amount`. The swap layer holds the live order to the cabal's slippage around that execution quote, so a move since the vote never blocks a trade; only a move between the execution quote and the order ends as a retryable `price_moved` swap. Slippage tolerance is a cabal rule with a platform default (proposal: 100 bps default, cabal can tighten, platform caps at 300 bps). The engine reads it through the cabal module's query port. Treasury USDC is read from chain, not from the ledger, matching the platform-balance rule elsewhere. Every amount and bps value is an integer.

The pause is owned by `funding` and read at check time, so there is no window between detection and pause and no pause state in trading ([deposits-withdrawals.md](deposits-withdrawals.md#pause)). A buy needs only the USDC it spends: a transfer fee on the bought mint is taken from the tokens received, so the fill is the chain delta net of it (below), not extra USDC. The relayer pays SOL fees. Pre-IPO transfer-fee handling is deferred (decided 2026-09-27).

A refused check appends `trade.blocked` with the code and the numbers it compared, records the delivery, acks, and stops. Governance consumes `trade.blocked`, sets the proposal to `execution_blocked` and emits `proposal.execution_blocked`. The engine logs the decision once, with `have` and `need` ([Logs as evidence](backend-platform.md#logs-as-evidence)). It never retries a refusal, `InsufficientFunds` included, even when a fund-to-cabal sweep is in flight. Any voter can re-submit it as a new proposal.

An upstream failure while running the checks is not a refusal. Jupiter, Privy or RPC down returns an `Unavailable` code (`JupiterUnavailable` and the like), which is retryable, so `bus.Dispatch` naks with backoff and the checks run again on redelivery ([Surfacing](backend-platform.md#surfacing)). After `MaxDeliver` the message goes to `DEADLETTER`.

**Hand-off.** If all checks pass, the engine calls the swap layer with the proposal id as the idempotency key. The claim transaction takes the proposal's row lock (`FOR UPDATE`, through governance's query port) and reads whether it is still `passed`, then inserts the `created` row in that same transaction. If the proposal is no longer `passed`, the row fails with `source_cancelled` before anything is signed. A void takes the same lock before it looks for a live swap, so one of the two waits and the loser sees the final state: a void that lands first stops the trade, and a void that waits on a claim is refused because a live swap exists. If that read fails, the row fails with `never_submitted` and the bus naks, so the redelivery makes a fresh claim.

### Stage 3: swap layer

```
created ──► submitted ──► confirmed
   │            │
   │            └───────► failed
   └────────────────────► failed   (sweeper: never_submitted)
```

The status is a Go type with a `transitions` table and a pure `Next(from, event)`. Adapters apply it as a guarded update, and `exhaustive` fails a missed case ([State machine](backend-platform.md#patterns-and-where-each-earns-its-place)).

1. **Insert `created`.** New `swaps` row: `proposal_id`, `cabal_id`, `action`, mints, `amount`, `status = created`. Order from Jupiter with `taker` set to the treasury and `payer` set to the relayer, because treasuries hold no SOL and the relayer pays every swap's fee. Before signing, the swap layer holds the order to the Review floor `F = MinOut(quote_out_amount)` at the cabal's slippage. Jupiter returns `otherAmountThreshold`, the minimum output the transaction enforces on chain at the requested `slippageBps`. An order whose `outAmount` is below `F` fails with `price_moved`, because the market moved past the cabal's limit. An order whose `otherAmountThreshold` is at least `F` is signed. Otherwise the layer requests one more order at `floor((outAmount − F) × 10000 / outAmount)` bps, the widest slippage that keeps the threshold at `F`, and applies the same two checks to it. A re-order whose threshold is still below `F` fails with `price_moved`. No signed transaction can fill below the Review floor. Sign with the treasury's Privy wallet (app-owned, `PRIVY_AUTHORIZATION_*`), then co-sign as fee payer with the relayer key. The signer refuses the transaction unless the relayer's signature 0 and the treasury's signature are both present, and the swap stores signature 0, the relayer's, as `tx_signature`. Then store Jupiter's `requestId` and the signed transaction bytes and flip to `submitted` in one guarded write that commits before any send. The signed tx is persisted **before** the network call, so a crash can never lose track of a transaction that may have landed, and a `created` row was never signed or sent.
2. **Call `/execute`.** Jupiter lands the transaction and polls the chain itself. The response is normally terminal:
   - `Success` → guarded update to `confirmed`, set `tx_signature`, `confirmed_at`, fill amounts, cost basis.
     Fills come from chain deltas. A sell's fill is the treasury's USDC token-account delta in the confirmed transaction (`getTransaction` pre and post token balances), never Ultra's `outputAmountResult`. Jupiter takes its fee from a sell's USDC output in a second transfer out of the treasury's account, so `outputAmountResult` is the pre-fee figure, 10 bps above what the treasury kept (#5279). A buy's fill is likewise the delta of the bought token in the treasury's account, never `outputAmountResult`, which is missing or overstated when the mint charges a Token-2022 transfer fee. If the delta can't be read, or is zero, the swap stays `submitted`, `trading.swap.fill_unread` alerts, and the sweeper confirms it from the same delta on a later tick. The sweeper reads the delta for buys and sells alike. Sells confirmed before this rule recorded the pre-fee figure. `monacoctl swaps overstated-sells` reads each confirmed Solana sell's delta and lists the sells recorded above it, with the overstatement per cabal, and the sells whose delta it couldn't read. It changes nothing; correcting the ledger is an operator action.
   - `Failed` with a program error or a code from -1 to -5 (Jupiter refused the request before it sent anything) → guarded update to `failed` with Jupiter's error code.
   - `Failed` with a code of -1000 or lower (failed to land, expired, timed out, unknown, invalid transaction) → the row stays `submitted`. Jupiter does not document these as final, so a late landing is possible. The sweeper fails the row with `blockhash_expired` only once the blockhash is invalid and the signature is absent.
   - `Pending` → re-send the same `/execute` with the stored `requestId` every 2 s. Safe for up to 2 minutes; Jupiter dedupes on `requestId`, so this never double-executes.
3. **One `uow.Do` per terminal transition.** It does, atomically:
   - `UPDATE … SET status = $new, … WHERE id = $id AND status = 'submitted'`. If rows affected = 0, another path already finished it: roll back and return.
   - Append `trade.confirmed` or `trade.failed`. The payload carries the fill amounts as a snapshot for consumers.
   - Nothing else. Consumers do the rest ([flow 11](backend-platform.md#flows)): `treasury` writes the `cabal_txns` header and ledger entries in the same transaction as its own event, `governance` sets the proposal to `executed` and emits `proposal.executed`, and `ranking`, `feed` and `notify` react.
4. **Sweeper.** A poller in the worker (every 30 s, under the poller advisory lock from [Deploy rule 1](backend-platform.md#deploy-and-observability)). It moves `created` rows older than 2 minutes to `failed` with `never_submitted`, with no on-chain lookup. That is safe because the signed bytes and the `submitted` status commit together before any send, so a `created` row was never sent. Failing it frees the claim for a retry. It also selects rows in `submitted` older than 2 minutes. It calls `getSignatureStatuses` on the stored signature and resolves through the same guarded update. Finalized → `confirmed`. For a not-found signature, it decodes the stored signed transaction and asks `isBlockhashValid`; only an invalid blockhash followed by a fresh not-found status becomes `failed` with `blockhash_expired`. Still processing, including an unfinalized error, stays pending for the next tick. The recovery migration adds `out_decimals` and `source_batch_size` to the swap row. Historical rows whose out mint is neither configured USDC nor a catalog asset retain `out_decimals = -1`; the sweeper refuses to guess their amount precision and leaves them retriable. Rows that fail local recovery validation rotate by `updated_at`, so they do not starve newer stale rows. This is the crash-recovery path and the only path that can finish a row the original process abandoned. There is no proposal poller behind it: a `proposal.passed` that no handler finished lands in `DEADLETTER`, and `monacoctl deadletter retry` is the one recovery path ([flow 27](backend-platform.md#flows)).
5. **Optional Helius webhook.** An address webhook on treasury wallets posts confirmed transactions. The handler looks up the row by `tx_signature` and runs the same guarded update. It sometimes wins the race with `/execute`, which is fine: the loser sees zero rows affected and stops. Its purpose is faster UI, not correctness. Not required for launch.

**Invariants**

- `tx_signature` is unique. `execute_request_id` is unique.
- At most one `swaps` row per proposal in a non-`failed` state, whatever its mint or chain: a partial unique index on the source id `WHERE source_kind = 'proposal' AND status <> 'failed'`. Other sources, a cash-out sale with one leg per mint, stay unique per `(source_kind, source_id, in_mint)`.
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
  - `monacoctl swaps force-resolve` reads the chain before it books anything. It asks for the signature's finalized status first. `--to failed` is refused when the transaction landed, is still processing, or was not found while its blockhash is still valid. `--to confirmed` is refused when the transaction failed or has not finalized, and when `--out-amount` differs from the treasury's token balance delta in that transaction. A refusal names the contradiction and changes nothing. `--override` books the operator's value anyway, and an RPC failure also needs it. A routed swap settles on an EVM chain, so the command reads its swap-step receipt instead (`eth_getTransactionReceipt`, then `eth_getUserOperationReceipt`) with the treasury's token transfer in its logs, and applies the same verdicts: a successful receipt contradicts `--to failed`, a reverted one contradicts `--to confirmed`, a received amount other than `--out-amount` contradicts `confirmed`, and no receipt (`not_found`) contradicts nothing. The command records the actor as `operator:<user>` and writes a `swap_operator_actions` row (`force_resolve`) with the target, the chain state, the chain's out amount and whether the override was used.

### Routing across chains

**Status:** Decided 2026-10-11 in M24 ([#4917](https://github.com/Munyon-Canyon/monaco/issues/4917), "Plan addition: best-price routing across chains"). Built by #4969 (listings, quoters and router), #4970 (execution, bridging and the step journal) and #4971 (valuation). [Flow 30](backend-platform.md#flows) runs the whole path against fakes.

A stock can trade on several chains, from several issuers. Every trade buys or sells at the best all-in price across them, and the cabal never sees a chain: one price, one holding per stock.

**Rules that don't change.** Every user balance and every cabal's idle cash stay on Solana. Only USDC crosses chains; a stock stays on the chain it trades on. A cabal still votes on an amount, the vote-time quote is display only, and slippage is measured only against the execution quote.

#### Listings

`market` owns `asset_listings`: one row per issuer per stock per chain, with the token address (and, for xStocks on EVM, its `w<symbol>` wrapper), decimals, `shares_per_token` (how many underlying shares one token is worth, refreshed every catalog run, since Ondo's grows with reinvested dividends) and the venue (`jupiter` on Solana, `uniswap` on EVM). A listing starts disabled and is enabled only after it is verified on chain: an EVM token's `symbol()` and `decimals()` match the issuer, a Solana mint exists under SPL Token or Token-2022. `set_tradable_override` applies per listing. The seed and the issuers' APIs are in `docs/how-to/stock-listings.md` (#4969).

Which chains list which issuers, and whether those listings are on. Arbitrum and Ethereum are deposit and withdrawal networks only, not trading venues (#5491). The router quotes only `chain.TradingNetworks()` (Solana, Base, Robinhood Chain): their rows below stay in `asset_listings` but never reach a comparison, and `BestOn` pinned to either returns `invalid_network`. "Issuer's flag" means a listing is enabled once it verifies on chain and its issuer reports it tradable; "off by override" means `tradable_override = false` keeps it disabled whatever the issuer says.

| Issuer | Chain | On |
| --- | --- | --- |
| xStocks | Solana | Issuer's flag |
| xStocks | Arbitrum | Issuer's flag (#5361) |
| xStocks | Ethereum | Off by override (#5186) |
| Ondo | Solana | Issuer's flag |
| Ondo | Ethereum | Issuer's flag |
| Coinbase (B20) | Base | Issuer's flag |
| Robinhood | Robinhood Chain (cash asset USDG, not USDC) | Issuer's flag (#5203) |

**At launch** (real quotes from #4969, 2026-10-11):

- **xStocks on Ethereum stay disabled.** The Uniswap Trading API found 31–99% price impact on Ethereum at cabal trade sizes. They stay in `asset_listings` with `enabled = false`, and the router never quotes them.
- **xStocks on Arbitrum follow the issuer's flag (#5361).** Uniswap found no Arbitrum route at launch, so they started off by override. Migration `20261012090003_market_arbitrum_xstocks_follow_issuer.sql` clears that override, and the sync inserts new Arbitrum rows with none. Since #5491 the router no longer quotes them, because Arbitrum is a deposit and withdrawal network only.
- **Coinbase B20 on Base routes through Uniswap.** Its pools are Uniswap v4 hooked pools, which the Trading API quotes and routes, so no aggregator such as Kyber is needed.

#### Quote, compare, pick

`Router.Best(stock, side, amount)` (`market`):

1. Quotes every enabled listing of the stock in parallel, each with a timeout: Jupiter for Solana listings, the Uniswap Trading API for EVM ones, both behind the `multichain.Quoter` interface.
   Each listing gets `MONACO_TIMEOUT_UNISWAP_QUOTE` (default 4 s) for its bridge estimate, quote and simulation. Execution keeps its own Uniswap client timeout, `MONACO_TIMEOUT_UNISWAP` (default 5 s). Listings on the same EVM network share one bridge estimate per comparison. The comparison router reuses a Privy bridge estimate for 60 s per network and amount bucket (the amount rounded up to a power-of-two number of whole dollars, at least $1). It keeps the arrived/sent ratio and applies it to the new amount, rounded down to the micro, so a reused estimate never overstates what arrives. A failed estimate is never reused, and execution asks Privy for its own quote. An EVM buy quotes the default and hookless Uniswap routes side by side, reads the simulation account's code (`eth_getCode`) at the same time, and keeps the better route that finished within the budget. A route whose quote is already below the other route's simulated output gets no swap build or simulation, since a simulation only lowers a quote. The hookless route asks for V2 and V3 pools only. Uniswap's `V4_NO_HOOKS` option took 1.8–5 s and returned the same v3 route on Base that a V2/V3 request returns in 0.15–0.8 s. On Robinhood Chain it found a no-hook v4 pool, which the default route finds too. Execution still builds its hookless candidate with `V4_NO_HOOKS`. Every Uniswap call a process makes (router quotes, sponsored routes, swap builds and execution) waits on one shared 5 calls/s limiter (`module.Deps.Uniswap`, the API key allows 6), so a burst queues instead of drawing a 429. A route that Uniswap answers with `NoRouteFoundError` is remembered for 3 minutes per chain, token pair, side and route options, and is not asked again in that time. A timeout, a 429 or any other error is never remembered. Each step's duration, including the rate-limit wait, is logged at debug level as `market.router.uniswap_steps`. A listing that runs out of time is listed as `stale` and logged with `upstream_unavailable`, its error and its `duration_ms` in `market.router.venue_dropped`. The comparison returns once every listing has a quote or a drop, so a slow EVM venue never holds back the Solana quote past the budget. On 2026-10-11 a $1 buy on Base measured 0.3–0.8 s for the bridge estimate, a median of 0.75 s for the default quote and 0.3 s for the V2/V3 hookless quote (both spiked to 3.1–3.3 s about once in 30 runs), up to 1 s waiting for the rate limit, and 20–90 ms each for `eth_getCode` and the `eth_call` simulation. The `V4_NO_HOOKS` quote it replaced took 1.5–3.5 s and timed out under load.
2. Converts each quote to **net underlying shares** (a sell: net USDC), after every cost as a line item: swap price and price impact, pool or route fees, the Privy bridge fee for an EVM listing (7.5 bps each way on 2026-10-10), and destination gas.
3. Drops quotes that are expired, older than 15 s, or above the price-impact ceiling.
4. Ranks the rest. Solana stays the winner unless an EVM route beats it by at least `ROUTER_MIN_EDGE_BPS` (default 10), which pays for the bridge's latency and extra failure surface.

`propose_trade` and `preview_trade` call it, store the winner's quote as the display quote, and return the ranked list as `price_comparison` (venue, network, net shares or USDC, each cost, and the winner). When the proposal passes, the comparison is written to `trade_routes` (`comparison` jsonb, `winner_listing_id`) through the pass event, and the engine executes against that winner. `trade.routed` records the decision on the swap.

A buy's route check is the router. `preview_trade`, `propose_trade` and the live comparison of an open proposal ask `Router.Best`, or `Router.BestOn` for a buy pinned to one chain, and a sell is gated by where the cabal holds the asset (next paragraph). When the router answers `no_route` for a buy, the gate falls back to `CheckRoute` on the asset's own Solana mint, as execution does (a buy the router can't route runs as a plain Solana swap), so a Solana stock with no `asset_listings` row still trades. A buy gets `no_route` only when every listing dropped with no route or over the impact ceiling and the Solana mint has no route either. When no listing quoted and at least one dropped for another reason (a timeout, an upstream error, a stale or expired quote, a failed simulation), the answer is `quote_unavailable`, which the app shows as "try again" instead of "try another stock". A Solana listing that Jupiter has no route for lets an EVM listing win; a Solana quote that failed for any other reason still fails the router closed with `quote_unavailable`.

A sell proposal carries the location it sells from. The `network` field pins it: absent, a sell uses the cabal's Solana holding and, only when the cabal holds none there, its single EVM holding of the stock (the proposal then records that network). A pin to Solana or to a trading chain sells the holding on that chain and nothing else. `token_amount` is in the base units of the listing held on that chain, the figure `holdings[].listings[].token_amount` reports. Admission reads the ledger's `EVMPositions`, resolves each held `network:token` key to its enabled listing through the market catalog, and checks the amount against that listing's units. A pin the cabal holds nothing on, a network that does not trade, two EVM holdings of one stock with no pin, and a gated listing are refused (`invalid_network`, `asset_untradable`); a holding too small is `cabal_shares_short` even when another location could cover it, and a sale with no route is `no_route`. An EVM sale is quoted with the listing venue's exact-output quote scaled to the sold units (the same estimate cash-out uses), and the proposal stores a one-quote sell comparison whose winner is the held listing, so execution runs the routed sell and bridge back with no further routing. Listings gated with `trading_halted` stay disabled, so they refuse a sale.

Jupiter's `/order` sends some stocks to its market-maker router, JupiterZ, which answers HTTP 400 `Quote not available from market maker` when no maker quotes. The Jupiter client then asks once more with `excludeRouters=jupiterz`, so the aggregator can route the order. On 2026-10-11 that refusal made NFLXx look unroutable at $1 while the aggregator quoted it.

A member can pin a buy to one chain with `network` on the proposal. The stored comparison stays the router's, and the display quote is the pinned chain's best listing. At execution a pinned buy re-quotes only its pinned network's best listing (an EVM pin asks `Router.BestOn`, which skips the Solana quote) and never changes chain. A retry reads the pin from `trade_routes.pinned_network`. When the pinned chain has no route, the proposal ends `execution_blocked` with `pinned_chain_unavailable`; when trading on that EVM chain is off, with `chain_trading_off`. A Solana pin keeps the Solana-only fallback when the router fails.

The app picks the chain before the member types an amount, so Review does not wait for a full comparison:

1. **Stock page prefetch.** Opening a stock calls `GET /v1/assets/{symbol}/best-chain`. It runs `Router.Best` for a $1 buy and returns the winner's network, venue, listing and net shares per $1, every chain's figures as `price_comparison`, and `quoted_at`. The answer is cached per symbol in the api process and shared by every member. An answer under 20 s old is served as is. One under 60 s old is served while one refresh per symbol runs in the background. An older one is refreshed before the answer, for at most a second: if the refresh is slower or fails, the last good answer under an hour old is returned with `stale` true (#5797). Each refresh logs `market.best_chain.refreshed` with its `duration_ms`. A background refresh that fails logs `market.best_chain.refresh_failed` and keeps the cached answer, and the next read tries again. The figure is display only.
2. **Pinned preview.** The app previews the member's amount with `network` set to that winner. A pinned buy preview asks `Router.BestOn`, which quotes only the pinned chain's listings, so it costs one venue's quote instead of every chain's.
3. **Re-route at execution.** Execution quotes again, as above, so a member never trades on the prefetched figure.

#### Simulation account

EVM quotes are simulated from one account stored in `evm_simulation_account`, one row per database (#5336). The target is an app-owned Privy EVM wallet with the treasuries' ownership: app-owned, with the `PRIVY_AUTHORIZATION_KEY_ID` quorum as owner. Each environment gets its own wallet under its own Privy app. The wallet is never funded: the simulation sets the cash balance and the Permit2 allowance with `eth_call` state overrides.

Until that wallet exists, the row is a seeded legacy row: `0xf15262a7…`, the Base treasury of the 2026-10-11 QA cabal, marked `legacy_network = 'base'` because Base is the only chain it is delegated on. It has no Privy wallet id. Quotes on Base keep simulating from it, so nothing changes when the seed lands.

The simulation runs through the account's ERC-7579 `execute`, so the account needs an EIP-7702 delegation on each chain. `monacoctl market simulation-account` sets it up and is safe to rerun:

1. It reuses a stored dedicated wallet. Over a legacy row, or with no row, it creates one (Privy idempotency key `market:simulation-account`).
2. For each EVM chain with an RPC URL, it reads `eth_getCode`. A chain that has no delegation gets one zero-value self-transfer through Privy `eth_sendTransaction` with `sponsor: true`, so Privy's paymaster pays the gas and attaches the delegation.
3. It prints each chain's code and the delegate Privy set.
4. A new wallet replaces the row only once it is delegated on the legacy row's chain. Otherwise the command prints `kept the legacy account` and the legacy row stays. A dedicated row is never overwritten.
5. It exits 1 if any chain stays undelegated after `--timeout` (default 2 m).

`Router.Best` checks the account once per comparison: one `eth_getCode` per EVM chain among the listings, shared by every listing on that chain. The check fails for a chain whose code is `0x`, a chain whose delegate isn't the wrapper's `DefaultDelegate`, a chain the legacy row doesn't cover, or every chain when no row is stored. Each venue on a failing chain is listed as `simulation_failed` with `reason_detail` "not set up on this chain yet", and `market.router.venue_dropped` logs it as `simulation_account_not_delegated`. The app shows "Unavailable: not set up on this chain yet". Such a venue counts as no route: when no listing quotes, the router answers `no_route` and the gate falls back to the Solana mint. A failed `eth_getCode` lists the venue as plain `simulation_failed`.

#### Execute on an EVM chain

A Solana winner takes the path above, unchanged. An EVM winner runs as a journal of steps, one `trade_steps` row per step, unique on `(trade_id, step)`. Each step stores its Privy action id before it polls, so a crash resumes the step instead of repeating it, and each change appends `trade.step_changed`.

```
buy:   reserve cash ─► bridge_out ─► requote ─► swap ─► record fill (cabal_positions.listing_id)
sell:  swap ─► bridge_back ─► settle as today
```

1. **Reserve** the trade amount of treasury USDC (the existing reservation).
2. **`bridge_out`.** `WalletActions.Transfer` of exactly that amount from the cabal's Solana treasury to its EVM treasury on the winning network. On Robinhood Chain the transfer delivers USDG. While it runs, a `treasury_in_transit` row holds the amount, so the pot total doesn't dip during the bridge. The cabal's EVM treasury is created on its first EVM trade (`EnsureTreasuryEVMWallet`, `treasury_evm_wallets`): one app-owned address serves every EVM network.
   Privy's bridge action names no destination transaction, so the step reads the arrival from the chain. When Privy reports the action succeeded, `EVMArrivals.Inbound` finds the first block at or after the step's `created_at` minus 2 minutes, then scans `Transfer` logs of the cash token into the EVM treasury forward from there, up to 2 blocks behind the head. Each `eth_getLogs` call covers at most `EVM_LOGS_MAX_BLOCKS` blocks (default 10, the Alchemy free tier's cap), and each tick reads at most 30 such chunks. A per-step cursor in the worker carries the next block between ticks, so a tick never rescans; a restart loses the cursor and starts again from the step's time. When the RPC refuses a range with a message that names a range or limit, the chunk is halved and retried once in the same tick, logged as `trading.evm_arrivals.chunk`, and later ticks keep the narrower chunk. Calls in one scan are spaced 250 ms apart, because the free tier also caps compute units per second and other pollers share the key. A 429 ends the tick with the cursor kept at the chunk that failed, and the network's scans then wait 15 s, doubling on each further 429 up to 60 s, before the next call; a tick that finishes without an error clears the wait. The scan stops at the first matching transfer. On 2026-10-13 the Alchemy free tier answered an 11-block request with HTTP 400 and served 40 sequential 10-block requests in 3.8 s without a 429, so a 30-chunk tick covers 10 minutes of Base blocks (2 s each) in about 3 s. It takes the earliest transfer of at most the sent amount whose hash no other step recorded. The step records it as `arrived_micros` and `arrival_tx` in the same guarded update that marks it `succeeded`, and its fee is the sent amount minus the arrival. `(network, arrival_tx)` is unique across steps that are not reused, so two bridge outs never claim one transfer. Until the transfer shows, the step stays `submitted` with `upstream_unavailable`, and the resume looks again. A succeeded `bridge_out` without an arrival, from before this rule or reused from one, is observed once when the swap loads, and the treasury posts one `bridge:bridge_out:adjust` txn that moves the difference between the estimated and the real fee from the EVM cash to fees.
3. **Requote** the winning listing after the bridge lands, for the amount that arrived. That quote is the execution quote.
4. **`swap`** of exactly the amount that arrived, on the EVM chain from the EVM treasury, signed through Privy, held to the cabal's slippage around the execution quote. A swap that fails with an error that won't retry logs `trading.swap.refused` with the code and the error's detail. A Privy 4xx carries Privy's `error` (first 200 bytes) and `code` as `privy_error` and `privy_code`.
5. **Record** the fill with the position's `listing_id`, through `trade.confirmed` as before.

A sale of an EVM position swaps to USDC on that chain, then `bridge_back` returns the proceeds to the Solana treasury (in transit until it lands), then settles as today. When a cabal holds the same stock on several chains, a sale (and a cash out's sale) takes lots by best net proceeds after the bridge back.

#### Partial failure

| What failed | Where the money is | What happens |
| --- | --- | --- |
| `bridge_out` rejected, or failed with no on-chain step | Solana treasury | The reservation is released and the swap fails as usual. Retry with `RetryTrade`. |
| `bridge_out` or `bridge_back` failed after Privy broadcast (an action `failed` with a step that has a transaction hash, is confirmed or pending, or is a bridge leg with no hash) | Unknown: the money may be gone from the source and not yet on the destination | The step stays `submitted` and nothing is booked back, because a return would show money that the chain says is gone. `trading.bridge.outcome_unknown` alerts on every look. A `bridge_out` also parks its swap (`parked_code` `bridge_outcome_unknown`, `trading.routed_swap.parked`), so the poller skips it; `fail-parked` refuses while the step is `submitted`. An operator checks the Solana and EVM wallets, then `unpark` re-drives the swap (the action is read again, never sent again) or a force-resolve books the outcome. A retry that finds an earlier attempt's action in this state sends nothing. |
| Bridge landed, `swap` failed | USDC on the EVM treasury | It stays there as cabal cash and still counts in the pot total. The swap step retries under the `RetryTrade` and force-resolve rules. A terminal failure returns the cash. The EVM trade poller holds a failed buy for 24 hours while its proposal can still retry, then adds a `bridge_back` step for the landed amount and sends it under `trade:<swap>:bridge_back:<attempt>`. A proposal that can no longer retry returns at once. A buy that reused another buy's cash returns it once, from the buy that holds it. A `RetryTrade` after the return bridges out fresh from Solana. `monacoctl swaps return-evm-cash --swap <id> --reason <text> [--yes]` returns cash that is already stranded without waiting the 24 hours. It follows a `reused:<swap>` bridge_out to the buy that holds the cash and prints which one it acts on. It refuses when that buy already has a `bridge_back` step, when a later swap spent the cash, when the buy is parked (use `unpark` or `fail-parked`) or not failed, and when the treasury wallet holds none. It reads the cash token's on-chain balance, queues a `pending` `bridge_back` step for the smaller of the landed amount and that balance, and writes a `swap_operator_actions` row. The EVM trade poller sends it. |
| Sale's `swap` landed, `bridge_back` failed | USDC on the EVM treasury | Counted in the pot as cash. The EVM trade poller alerts with `trading.bridge_back.failed` once the failure is a sweep age old, and sets the step back to `pending` under the next attempt, up to 5 attempts (`retry_bridge_back` with the actor `trading.bridge_back_sweep`). After the 5th it stops retrying and alerts on every tick. `monacoctl swaps failed-bridge-backs` lists each failed `bridge_back`. `monacoctl swaps retry-bridge-back --swap <id> --reason <text>` sets the step back to `pending` under the next attempt, adds the failed attempt's Privy action id, if it has one, to `prior_privy_action_ids`, and writes a `swap_operator_actions` row. The EVM trade poller then sends it again under `trade:<swap>:bridge_back:<attempt>`. Before it sends, it reads every earlier attempt's Privy action: if one landed late, the step adopts it and nothing new is sent, and if one is still pending the poller asks again on the next tick. An attempt Privy refused before sending (no action id) is sent again without that check. Attempt 2 and later post under their own ledger ids, and a cash out's sale, already settled on attempt 1's failure, is not settled again. |
| A routed swap parked: its resume failed with an error that won't retry (`swaps.parked_at`, `parked_code`) | Wherever its last step left it: the Solana treasury before `bridge_out` landed, USDC on the EVM treasury after. The cash hold stays. | `trading.routed_swap.parked` alerts once and the EVM trade poller skips the swap. `monacoctl swaps parked` lists parked swaps with each step's status. `monacoctl swaps unpark --swap <id> --reason <text>` clears the park, and the poller re-drives the swap under the same step keys. It refuses once the stranded cash has left (a `bridge_back` step, or another trade reused it). `monacoctl swaps fail-parked --swap <id> --reason <text>` ends the swap `failed` with `never_submitted` and releases the hold, only when no step was broadcast or confirmed: the swap is `created`, its `swap` step is absent or failed, and `bridge_out` is not `pending` or `submitted`. Cash a succeeded `bridge_out` left on the EVM treasury goes back through the stranded-cash rule once the proposal can't retry. Each action writes a `swap_operator_actions` row with the operator and the reason. |
| A routed swap `submitted` with no receipt for 10 minutes (`swap_receipt_missing`) | USDC spent from the EVM treasury; the swap may still land | The swap parks and `trading.routed_swap.parked` alerts, so the poller stops resuming it. An operator reads the receipt or the treasury's token balance, then `monacoctl swaps force-resolve` books it `confirmed` with the received amount or `failed`, or `fail-parked` ends it `failed` (`force_resolved`). Both read the swap's EVM receipt first and refuse a swap whose receipt shows it landed (`force-resolve --override` books it anyway; `fail-parked` has no override and also refuses when the receipt cannot be read), and both record the `chain_state` in the `swap_operator_actions` row. Both close the `swap` step. If the receipt shows up first, `unpark` re-drives the swap and it settles. The venue reads an ERC-4337 `user_operation_hash` through `eth_getUserOperationReceipt` when the node has no transaction receipt for the hash. |

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
