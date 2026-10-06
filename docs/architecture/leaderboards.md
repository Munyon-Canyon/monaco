# Leaderboards

**Status:** Decided 2026-09-26. Direction set; cadences and thresholds are starting values to tune. Reconciled with [backend-platform.md](backend-platform.md) 2026-09-27: owned by the `ranking` module, [flow 19](backend-platform.md#flows), built in [Rollout](backend-platform.md#rollout) step 6. Prices come from the `market` module ([price-history.md](price-history.md)).

## Decision

**No network call and no per-cabal loop on the request path.** A leaderboard request reads precomputed rows from Postgres (or a process cache of them) and returns. All the expensive work, valuing every cabal and member, happens in a background job in the worker, **in batches**, on a fixed cadence and on money events.

The `ranking` module owns valuation snapshots and leaderboards ([Repository layout](backend-platform.md#repository-layout)). It owns no prices and no positions. It reads both through other modules' read-only query ports ([Dependency rules](backend-platform.md#dependency-rules-enforced-by-depguard)).

Four pieces:

1. **Prices from `market`.** The `market` module samples every catalog asset on its own cadence and stores the history ([price-history.md](price-history.md)). Everything that needs a spot price reads the latest stored price through `market`'s query port, never the network.
2. **Positions from the ledger, not the chain.** Cabal holdings and USDC come from the `treasury` module's `cabal_positions` projection, through its query port, not an RPC per treasury per request. A background reconcile checks chain against ledger.
3. **Batch valuation job → `leaderboard_entries`.** Every 2 minutes, and on `trade.confirmed`, `cabal.funded` and `cashout.completed` (flow 19), one job loads every input for every cabal and member in a fixed number of port calls, runs the valuation math in memory, and writes every board at once. It appends `ranking.snapshot_written` in the same transaction.
4. **Push.** When a board changes, `ranking.snapshot_written` becomes a hint on the app's one SSE stream, `/v1/stream` ([SSE hub](backend-platform.md#sse-hub)). That, not polling, is what makes it feel live.

Target: board read p95 under 50 ms. Money-event-to-board freshness is the 1 s debounce plus one run. Price-to-board freshness is up to 4 minutes: a move waits up to 2 minutes for the next sample and up to 2 more for the next valuation run. Valuation runs every 2 minutes to match the price poller (decided 2026-09-27). That is live enough for a ranking that moves on returns, not ticks. Network calls to price vendors independent of user count.

## Today

From the code, as of 2026-09-26. The RFC rewrite ignores this code and deletes it at rollout step 7; it stays here as the problem statement.

| Board | Route | Ranks | Work per request |
| --- | --- | --- | --- |
| Cabals | `GET /v1/groups/leaderboard` (`app/groups_tab.go`) | Cabals by % return | Every cabal valued live: ~8 queries, one query per holding for cost basis, one treasury USDC RPC, marks per holding. 30 s in-memory cache, 6 workers. |
| Home | `GET /v1/home` (`app/home.go`) | Cabals + people (lifetime) | Same, **sequential** loop over every cabal. |
| Dashboard | `GET /v1/home/dashboard?leaderboardRange=` (`app/home_dashboard.go`) | People, 1H/1D/1W/1M/ALL | Sequential per cabal; ranged boards use a `nav_snapshots` row as the start. |
| Cabal members | `GET /v1/groups/{id}/view` (`app/group_view.go`) | Members of one cabal | One cabal valued live. |

Problems:

- **Network on the request path.** `pricechain.MarkedPot` marks holdings one at a time: a Pyth call per symbol, then Jupiter with a single mint, although the Jupiter client accepts many. Treasury USDC is a Privy/Solana RPC per cabal per request with no cache.
- **N+1 queries.** Cost basis is one query per holding; `GetTreasuryByGroupID` and `ListPositionsByGroup` run twice per cabal.
- **No price history.** No asset price table exists. `nav_snapshots` is written only on deposits, fills and payouts, so between trades a cabal has no point-in-time value.
- **Ranged boards are wrong when members deposit mid-range.** Start equity is *current* share units × the start NAV per share (comment in `home_dashboard.go`: no historical share ledger). A member who doubled their stake yesterday shows a fake gain on the 1W board.
- **Inconsistent prices within one board.** Each cabal is valued with whatever mark was cached when its worker got to it, so two cabals holding the same stock can be priced seconds apart.

## 1. Prices: from `market`

The earlier draft of this doc designed its own `asset_prices` tables and a 5 s Pyth-plus-Jupiter poller. That is superseded (platform default 2026-09-27). There is one `market` poller and one table, `price_points`, with `price_micros` as a `bigint`. Pyth is dropped. The poller ticks every 120 s (decided 2026-09-27) and publishes one batched `price.tick` per poll (flow 18, [NATS hosting and budget](backend-platform.md#nats-hosting-and-budget)). Charts and sparklines in [price-history.md](price-history.md) read the same table, so boards and charts can never disagree on a price.

What `ranking` needs from `market`'s query port:

- **One latest price per asset**, with the time it was observed, in one call for every asset. The job calls it once per run.
- **A price as of a time**, for ranged boards and snapshots.
- **Validation before a price becomes latest.** A move of more than 20 % from the previous sample is held for one more sample before it is accepted, so a single bad print or a thin-liquidity spike on a tokenized stock cannot reshuffle the board. At a 120 s tick, a held move reaches boards about 2 minutes later. This rule runs in `market`, so every reader gets the same price.

Staleness is the board's rule, below: a price older than 5 minutes excludes the cabal from that run. At a 120 s tick that tolerates one missed sample and flags the second.

**Off-hours** (default 2026-09-27). `market` samples every asset always, so the table and charts keep the 24/7 token price. Boards do not use it while the US market is shut. For an asset whose underlying trades on a US exchange, the valuation job values it at the close price, the last sample at or before the session close, until the market reopens. Weekend thin-market moves on the token cannot reshuffle the boards. The staleness rule measures that asset against the close sample, not against now. Pre-IPO tokens (PreStocks, Tessera) have no underlying exchange and no close, so boards value them at the latest sample at all times (default 2026-09-27).

## 2. Positions: from the ledger

Valuation needs, per cabal: units of each asset held, USDC held, total share units; per member: share units and net contributed. All of it lives in the `treasury` module's ledgers and projections ([data-model.md](data-model.md)):

- `cabal_positions (cabal_id, asset_id, units, cost_basis_micros)`, the projection of `cabal_txns` entries, updated in the same transaction as each confirmed swap, deposit and cash out.
- `user_positions (user_id, cabal_id, share_units, net_contributed_micros)`.

`ranking` reads them through `treasury`'s query port. It never joins `treasury` or `market` tables in its own SQL.

**Pot value is `treasury`'s.** `treasury` exports `PotValue(ctx, cabalID) (money.Micros, error)` on its query port: the cabal's USDC plus every holding at the latest `market` price. `ranking`, `governance` (the propose-time pot check, [proposals.md](proposals.md#crud-surface)) and `cabal` read pot value there and never compute it themselves. The batch job below applies the same definition in memory from one price read, so every row on a board shares `prices_as_of`.

The cabal screen reads the pot through `GET /v1/cabals/{id}/pot` (`treasury`'s `CabalPot`). One request makes one price read, and only when the pot holds a stock, so the pot, its holdings and the caller's slice always agree. It returns `pot_value_micros` (equal to `PotValue`), `cash_micros`, each holding with its units, price, value, weight and gain over cost basis, the cabal's all-time `pnl_micros` and `return_bps` over net contributed, `prices_as_of`, and `me`, the caller's share units, value, slice and gain. `me` is null for a non-member. Weights and slices are floored basis points that sum to 10000, with the remainder on the largest row. A holding without a price fresher than 5 minutes returns `price_unavailable`; the pot is never valued at cost basis. Any signed-in user may read it, and a banned cabal stays readable.

The app refreshes the pot on two hints:

- `cabal.<id>.activity_changed` (`events.CabalActivityChangedHint`), which `treasury` publishes after commit when a trade is submitted, confirmed or fails, when a fund settles, and when a cash out completes or fails.
- `global.prices_updated`, which the `market` poller publishes when prices move.

Treasury USDC comes from the ledger, **not an RPC**. A reconcile job (per cabal, every few minutes, staggered) compares on-chain balances to the ledger. A mismatch it cannot explain is either an external deposit (bounced, see [deposits-withdrawals.md](deposits-withdrawals.md#direct-transfers-to-a-cabal-treasury-are-not-allowed)) or a bug; either way the cabal is flagged and excluded from boards until resolved. The flag reaches `ranking` through the same port.

Cost basis is stored on `cabal_positions`, not recomputed with a query per holding.

## 3. Batch valuation job

Runs in the worker every 2 minutes, and on `trade.confirmed`, `cabal.funded` or `cashout.completed` for any cabal, debounced to 1 s (flow 19). The money events arrive on `ranking`'s durable consumer through `bus.Dispatch` ([event-bus.md](event-bus.md#consumers-and-handlers)). The timer is a poller: it takes a Postgres advisory lock so one worker replica runs each tick ([Deploy and observability](backend-platform.md#deploy-and-observability)).

**Inputs, loaded in a fixed number of port calls, whatever the number of cabals:**

1. Every cabal's holdings and USDC, total shares, net contributed and status (`treasury`).
2. Every member position: user id, cabal id, share units, net contributed (`treasury`).
3. The latest price of every held asset, read once (`market`).
4. For ranged boards: each cabal's value and each member's share units at the range start (`ranking`'s own snapshots, and `treasury`'s user ledger).

**Compute** in memory with pure functions in `ranking/domain`, ported from the legacy Go domain package deleted in M7 (last present at `242c2609`; `ComputePotNAV`, `MemberEquity`, `ComputeMemberPnL`, `BuildGroupBoard`, `BuildPeopleBoard`). Amounts are `money.Micros` and returns are integer basis points; multiply-then-divide goes through `math/big` and rounds down ([Money and types](backend-platform.md#money-and-types)). Doing the math in Go rather than SQL keeps one tested implementation of money math.

Per-cabal valuation runs as the RFC's pipeline (load holdings, value, write snapshot), with buffered stages of 64 and 4 workers per stage from `platform/concurrency` ([Concurrency rules](backend-platform.md#concurrency-rules)). Ranking runs once over all valued cabals after the pipeline drains.

**One price snapshot per run.** The job reads prices once and records the time they were observed (`prices_as_of`). Every row on every board in that run is priced from the same read, so rankings compare like with like.

**Output**, written in one `uow.Do`, replacing the previous run's rows and appending `ranking.snapshot_written { run_id }`:

```
leaderboard_entries   board (cabals | people | cabal_members:<cabal_id>), range (1H | 1D | 1W | 1M | ALL),
                      rank, subject_id, value_micros, pnl_micros, return_bps,
                      prices_as_of, computed_at, flags (unpriced_assets, stale_prices)
                      PK (board, range, rank)
leaderboard_runs      run_id, prices_as_of, started_at, finished_at, rows_written, cabals_excluded
```

Plus a per-run (every 2 minutes) **`cabal_value_snapshots (cabal_id, value_micros, nav_per_share_micros, total_shares, at)`** row per cabal, from the same run. This fills the gap `nav_snapshots` leaves between trades and is the start point for ranged boards and value charts.

`pnl_micros` and `return_bps` can be negative. They use the signed integer type `money.SignedMicros` (int64) in `platform/money` (platform default 2026-09-27, [Money and types](backend-platform.md#money-and-types)), and gain and net flows in the ranged-board math below do too. `value_micros` stays `money.Micros`.

### Ranged boards, done correctly

For a member over a range `[t0, now]`:

```
start_equity  = share_units_at(t0) × nav_per_share_at(t0)
end_equity    = share_units_now × nav_per_share_now
net_flows     = contributions − cash outs during (t0, now]
gain          = end_equity − start_equity − net_flows
return        = gain / (start_equity + time-weighted net_flows)      -- modified Dietz
```

`share_units_at(t0)` comes from the user ledger (sum of share-unit entries up to `t0`, through `treasury`'s port), and `nav_per_share_at(t0)` from `cabal_value_snapshots`. This removes today's fake gain for members who deposit mid-range. Modified Dietz is a pure function in `ranking/domain`, covered by property tests like the rest of the money math ([Testing](backend-platform.md#testing)).

## 4. Request path and "real time"

- Board routes read `leaderboard_entries` for `(board, range)`, keyset-paginated by rank, plus the caller's own row. This is a CQRS-lite query straight from sqlc ([Patterns](backend-platform.md#patterns-and-where-each-earns-its-place)). A small in-process cache keyed by `run_id` avoids even the DB read for hot boards. Routes move from `/v1/groups/...` and `/v1/home/...` to the generated contract at the iOS cutover ([Decided](backend-platform.md#decided)).
- Each response carries `computed_at` and `prices_as_of`, so the app can show "live" or "updated 2 min ago" honestly. Every value, rank and display name is computed on the server; the app only formats ([Thin client](backend-platform.md#thin-client)).
- `ranking.snapshot_written` becomes one hint, `hint.global.leaderboards_updated` with `{run_id, computed_at}`, on `/v1/stream` under the SSE hub's `global` key, which every connection joins (platform default 2026-09-27; one hint per run, because every run rewrites every board and range, where a `{board, range, run_id}` hint per board would send about 15 per run). Boards are global, so every open app may learn a run finished. The app refetches the board it shows and animates rank changes. Ranks and values change only on a run, and a profile change that renames rows publishes the same hint with the current run. The hint is droppable: a client that misses it reads the new run on its next fetch.
- On app open, the first read is immediate from precomputed rows. No cold valuation.
- **Portfolio and P&L routes are `ranking`'s**, read from the same precomputed rows and `cabal_value_snapshots`: `GET /v1/me/portfolio` (the caller's stakes across cabals), `GET /v1/me/pnl?range=` (the caller's P&L over a range) and `GET /v1/cabals/{id}/value?range=` (a cabal's value series over a range). `range` takes the board ranges `1H`, `1D`, `1W`, `1M`, `ALL`.
- **Boards:** cabals, people (lifetime and ranged), members within a cabal, and friends-only (default 2026-09-27). Friends-only is the people board filtered at read time to the people the viewer follows, which the route gets from `social`'s query port ([followers.md](followers.md)). It needs no extra valuation.

## Validity rules

A leaderboard that is fast and wrong is worse than slow. Rules the job enforces:

1. **Every value on a board comes from one price read.** No mixing.
2. **Unpriced means excluded, not guessed.** Today a holding with no live mark is valued at cost basis silently. On the board, a cabal with any holding missing a price fresher than 5 minutes is dropped from that run and flagged. The last good row is kept with a `stale_prices` flag rather than inventing a value.
3. **Ledger and chain must agree.** Cabals flagged by the reconcile job, paused for an external deposit, banned, or faker/seed cabals are excluded from public boards.
4. **Minimum size to rank.** Existing `IncludeOnBoard` rule stays: a cabal or person needs a minimum net contribution to appear, so a $1 pot up 400 % does not top the board. Threshold is a product knob.
5. **Conservation check each run.** For every cabal, the sum of member equity equals pot value (within rounding). A run that fails this for a cabal excludes that cabal and reports a `KindInternal` error, which alerts ([Errors](backend-platform.md#errors)). This catches share-unit or ledger bugs before users see them.
6. **Deterministic ties.** Ties break by earlier `created_at`, then id, so ranks never flicker between identical runs.
7. **Audit.** `leaderboard_runs` plus `prices_as_of` let anyone reconstruct why a row had its value: which prices, which positions. `monacoctl replay` does not rebuild `leaderboard_entries`: only the valuation job writes them, and replay runs just the `ranking_membership` and `ranking_names` durables, which rebuild `ranking_triggers` and rename subjects on rows the job wrote ([Replay and seeded states](backend-platform.md#replay-and-seeded-states)). Rebuilding a past board from the event log is not a command yet (#659).
8. **Rank by % return** (default 2026-09-27). `return_bps` orders every board, with rule 4's minimum size so small pots cannot dominate. A $ gain tab comes later; `pnl_micros` is already stored for it.

## Cost and latency budget

| | Today | After |
| --- | --- | --- |
| Price vendor calls | Per holding per request (10 s cache) | None from `ranking`; `market` samples on its own cadence |
| Solana RPC on board read | 1 per cabal | 0 |
| DB queries per board read | ~8 per cabal + 1 per holding | 1 to 2 |
| Board read latency | Grows with cabal count | Constant, target p95 < 50 ms |
| Freshness | Up to 30 s cache + compute | Money event: 1 s debounce + job + push. Price change: up to 4 min (2 min sample + 2 min valuation) + job + push |

## Rollout

`ranking` lands in RFC [Rollout](backend-platform.md#rollout) step 6, together with `market`, on top of the `treasury` projections from step 3. The old backend gets no incremental fixes. Inside step 6, in order:

1. `market` query port for latest and as-of prices ([price-history.md](price-history.md)).
2. `treasury` query port for positions, member shares and reconcile flags.
3. Valuation job, `leaderboard_entries`, `leaderboard_runs`, `cabal_value_snapshots`; board routes read precomputed rows.
4. Modified Dietz ranged boards on the user ledger.
5. `ranking.snapshot_written` hint on `/v1/stream`.

## Alternatives considered

| Alternative | Why not |
| --- | --- |
| Keep valuing on request with longer caches | Latency still grows with cabal count, and a cold cache after deploy stalls every open. |
| Read "latest price" straight from the history table | Works with the right index, but it belongs to `market`. `ranking` asks the port; how `market` answers is its own choice. |
| One SQL join across `cabal_positions` and the price table | Fast, but reads two other modules' tables. The module walls forbid it; ports keep each module's schema private. |
| Compute P&L in SQL | Fast, but duplicates money math that lives, tested, in Go. Two implementations drift. |
| Materialized views refreshed by cron | `REFRESH MATERIALIZED VIEW` rebuilds everything and cannot carry price times, flags or exclusions cleanly. An explicit job with an output table is easier to reason about. |
| Redis for boards | Not needed at current scale; Postgres plus an in-process cache meets the target. Revisit with multiple API instances and high read volume. |
| Own 5 s price poller in this doc | Earlier draft. Superseded by `market` and [price-history.md](price-history.md); Pyth is out by product decision. |

## Open questions

None at the moment.

Log: [log/leaderboards.md](log/leaderboards.md).
