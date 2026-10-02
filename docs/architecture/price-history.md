# Price history and charts

**Status:** Decided 2026-09-26. The `price_points` table and the live sampler (build steps 1 and 2 below) are built; the rest is not. See [Gap between this and the code](#gap-between-this-and-the-code). The backend shape (modules, pollers, NATS, rollout) follows [backend-platform.md](backend-platform.md), which wins where the two differ. [leaderboards.md](leaderboards.md#1-prices-from-market) reads the same table from the same poller (default 2026-09-27).

## Decision

Postgres is the price source. One table, `price_points`, holds one USD price per token per timestamp as integer `price_micros`, and every chart, sparkline, leaderboard valuation and P&L curve in the app is drawn from it (default 2026-09-27). External vendors only fill that table; no request from a user ever reaches a vendor.

Two writers feed the table:

1. **Jupiter Price API v3** samples the live price of every catalog token on every tick of the one `market` price poller. It is already integrated, takes 50 mints per call, and is free. This is the intraday history going forward.
2. **CoinGecko** backfills the past when a token joins the catalog (one time, three calls per token) and reconciles gaps once a night. On the free Demo key this costs about 70 calls a day for the whole catalog, against a cap of 10,000 a month.

Charts are bucketed from `price_points` at read time and cached in memory per symbol and range, with a short TTL that matches the sampler cadence. Freshness is two minutes, which is what the product needs: the chart is not a trading terminal.

Pyth is out, for charts and for leaderboards (default 2026-09-27). Jupiter's undocumented `datapi.jup.ag` candle endpoint is out. Yahoo is out: the rewrite ports no Yahoo adapter, and the CoinGecko backfill runs before iOS cuts over, so the table already has history on day one (default 2026-09-27).

In the rewrite the `market` module owns all of this: the catalog, `price_points`, the sampler, backfill, reconcile and the chart reads ([Repository layout](backend-platform.md#repository-layout)). The sampler is the price poller of flow 18 in [`flows.tsv`](backend-platform.md#flows). Each tick publishes one batched core-NATS message carrying every asset, never one message per asset, and nothing is stored as an event ([NATS hosting and budget](backend-platform.md#nats-hosting-and-budget)). `ranking` and the [SSE hub](backend-platform.md#sse-hub) consume it. The subject is `price.tick` everywhere (default 2026-09-27). The tick runs every 120 s (decided 2026-09-27).

## Why

- **Vendor traffic must not scale with users.** Today every chart request that misses a one-minute in-memory cache goes to a vendor, and every process restart empties that cache. With a table, vendor calls are a function of catalog size and time, never of user count.
- **The free tier is enough if we barely use it.** CoinGecko's keyless API 429s after a handful of calls a minute and forbids scheduled polling. The Demo key is free but capped at 10,000 credits a month. That cap is unreachable for a backfill-plus-nightly-reconcile job and hopeless for a live feed. Designing around "backfill, never poll" is what keeps it free.
- **We already pay for the live price.** Jupiter Price v3 is called for catalog display prices today. Writing each answer down turns a throwaway spot price into history at no extra vendor cost, and at a resolution (2 minutes) no free history vendor offers.
- **Price the instrument, not the proxy.** Jupiter's price is the token on Solana at the venue the cabal's swap routes through. Yahoo charts the underlying equity on its home exchange, which is a different number outside market hours and for every pre-IPO token has no equivalent at all.
- **History survives vendor churn.** Jupiter's candle endpoint returned empty for every mint, including SOL, on 2026-09-26. Pyth Benchmarks started requiring a key on 2026-08-26 and refuses equity feeds without an entitlement. Both broke charts in production without any code change on our side. A table we own does not do that.

## How it works

### Sources surveyed (2026-09-26)

Every source was probed live against AAPLx (`XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp`), tSpaceX from Tessera (`TSPXcLV76s6V2zDiZQ18kBfcbnjaE2ZzNT3ga2Pd99v`) and ANDURL from PreStocks (`PresTj4Yc2bAR197Er7wz4UUKSfqt6FryBEdAriBoQB`).

| Source | History? | Lookup | Cost | Verdict |
| --- | --- | --- | --- | --- |
| **Helius** | None. DAS `getAsset` gives a spot price cached 10 min for the top 10k tokens. Wallet API `history` and `balance-at` give balance changes with no USD at transaction time. Enhanced Transactions is in maintenance mode. | | Free plan 1M credits | Not a price source. Keep for RPC and the optional swap webhook only. |
| **CoinGecko** `coins/solana/contract/{mint}/market_chart` | Yes. Price points, not OHLC. 5-min for `days=1`, hourly for 2–90, daily above. 365 days on Demo, 2 years on Basic. | By mint. All three test mints listed. | Keyless: ~10–30/min, 429 after 3 burst calls, "not suitable for production workloads, scheduled polling". Demo key: $0, 10k credits/mo, attribution required. Basic: $35/mo, 100k/mo, commercial license. | **Backfill and reconcile source.** |
| **Jupiter Price API v3** `price/v3?ids=` | No. Spot only. 50 mints per call. | By mint. | Free, keyed key raises rate limit. | **Live sampler source.** |
| **Jupiter charts** `datapi.jup.ag/v2/charts/{mint}` | Was. Returned `{"candles":[]}` for every mint including SOL on 2026-09-26. Undocumented, not under the API licence, ToS-gray. | By mint. | None. | Dead. Delete `internal/jupitercharts`. |
| **Pyth Hermes / Benchmarks** | Yes, for the underlying equity. Key required since 2026-08-26. Equity feeds need an entitlement the key does not carry (403 "Not entitled"). | By feed id. | Free key, entitlements unclear. | Out by product decision. |
| **GeckoTerminal** `pools/{pool}/ohlcv/{tf}` | Yes, OHLC, about 6 months. | By pool. xStocks trade across 3+ pools; the main AAPLx pool was 16 days old. | Free, ~10/min. | Pool-level history is fragmented. No. |
| **Birdeye** `defi/ohlcv?address={mint}` | Yes, OHLC, full token life. | By mint. | Free 30k CU/mo, Lite $39/mo. | Only if 1-min candles older than a day are ever needed. No for now. |
| **Moralis** | Yes, by pair. | By pair. | ~266 calls/day free. | No. |
| **Bitquery** | Yes, GraphQL aggregation. | By mint. | 7-day trial then $39/mo, history is an add-on. | No. |
| **DexScreener** | No. | | | No. |
| **xStocks, PreStocks, Tessera public APIs** | No. Spot mark only. | | Free. | Catalog and mark source only. |

### The table

```sql
CREATE TABLE price_points (
    mint       text        NOT NULL,
    ts         timestamptz NOT NULL,
    price_micros bigint    NOT NULL,
    source     text        NOT NULL,   -- 'jupiter' | 'coingecko'
    PRIMARY KEY (mint, ts)
);
```

The migration is an atlas versioned SQL file; queries are sqlc ([Decided](backend-platform.md#decided)). The price is `bigint` micros of USD per whole token, so it maps straight onto `platform/money` with no float anywhere ([Money and types](backend-platform.md#money-and-types)) (default 2026-09-27). Resolution is 1e-6 USD.

Writes are `INSERT ... ON CONFLICT (mint, ts) DO NOTHING`. The first writer for a timestamp wins, so a reconcile never overwrites a live sample and a re-run backfill is a no-op. Timestamps are truncated to the writer's resolution (the 2-minute bucket for the sampler, the vendor's bucket for backfill) before insert.

### Decimals and the UI multiplier

`price_micros` prices one whole token. Valuing a holding of base units needs the mint's decimals and, for a Token-2022 mint, its scaled UI multiplier. The issuer APIs carry neither, so the xStocks adapter writes 8 decimals and a 1/1 multiplier. The catalog poller asks the chain (`chain/solana.MintConfig`) about every mint it has not checked, at most 200 a tick and 8 at a time. It stores the chain's decimals, the multiplier in force as an exact fraction and `chain_checked_at`. When the issuer's decimals disagree, the chain's value wins and the poller logs `market.catalog.decimals_corrected`. An RPC failure leaves that mint unchecked for the next tick and counts in `poller_errors_total`. Until an asset is checked, `ListTradable` leaves it out and `Tradable()` is false whatever its override says, while `AssetByMint` still returns it.

xStocks use the multiplier for dividends, so it is above 1 for most of them. AAPLx read 1.0032690125398187 on 2026-09-30. A mint is checked once, and rechecking it after its next dividend step is #1121.

### Writer 1: live sampler

The `market` price poller in `cmd/worker` is the only one (flow 18). On each 120 s tick it asks the catalog for every routable mint (about 60 xStocks, 8 PreStocks, 3 Tessera today), batches them 50 per Jupiter Price v3 call, publishes one `price.tick`, and inserts one row per mint with `ts` truncated to its 2-minute bucket. `ON CONFLICT DO NOTHING` keeps the first sample in each bucket, so the table holds at most one row per mint per 2 minutes, even when a restart runs an extra tick. Vendor calls depend on catalog size and cadence, never on how many people are looking.

Prices are sampled always, including when the equity market is shut (default 2026-09-27). Charts draw the live token price and mark after-hours at read. Leaderboards value holdings at the last regular-session close while the market is shut, so weekend moves in a thin market do not reshuffle boards ([leaderboards.md](leaderboards.md)).

The same tick appends `asset.price_moved` when an asset's change since its previous close crosses a feed threshold ([feed.md](feed.md#writing-feed-items)). There is no second poller.

**Market calendar.** Whether the US market is open, and when the last session closed, comes from a static NYSE calendar in `market`'s code: regular hours plus a table of NYSE holidays and early closes, updated once a year. No calendar vendor is called (default 2026-09-29; see #535). The table covers 2026 and 2027. A time it cannot answer returns `calendar_expired`, which alerts, so add the next year's block to `market/domain/holidays.go` before the year ends (default 2026-09-30; see #546).

Poller rules from the RFC apply. It takes a Postgres advisory lock per tick, so only one worker samples ([Deploy rule 1](backend-platform.md#deploy-and-observability)). A failed tick is logged, counted in `poller_errors_total{poller,code}`, and never stops the loop ([Surfacing](backend-platform.md#surfacing)). A tick that wrote nothing still logs that it ran ([Logs as evidence](backend-platform.md#rules)). The Jupiter call goes through the `market` adapter with its configured deadline, circuit breaker and retry.

Once the table is the source, sparklines are a DB read and nothing needs pre-warming against a vendor. Today's `SparkWarmer` has no counterpart in the rewrite.

Thin pre-IPO tokens can print a noisy spot. The sampler stores the raw sample; smoothing, if it turns out to be needed, is a read-time concern so the record stays honest.

### Writer 2: CoinGecko backfill and reconcile

**Backfill** runs once per mint, triggered when a mint first appears in the catalog and by a `monacoctl` backfill subcommand for the initial load (`monacoctl` is the RFC's ops CLI for backfills; [Repository layout](backend-platform.md#repository-layout)). Three calls per mint:

| Call | Granularity | Fills |
| --- | --- | --- |
| `days=365` | daily | 1Y, ALL |
| `days=90` | hourly | 1W, 1M, 3M |
| `days=1` | 5-min | 1D, until the sampler has run for a day |

About 210 credits for the whole catalog today.

**Reconcile** runs nightly. One `days=2` hourly call per mint fills any hole the sampler left (deploys, outages). About 70 credits a day, roughly 2,100 a month, under a quarter of the Demo cap.

CoinGecko is called with the Demo key (`COINGECKO_API_KEY` in encrypted `.env.production`, read once by `platform/config`; header `x-cg-demo-api-key`, host `api.coingecko.com`). The client is a `market` adapter: an anti-corruption layer with functional options, a circuit breaker and a client-side limiter that holds every call to well under the plan rate ([Patterns](backend-platform.md#patterns-and-where-each-earns-its-place)). A 429 is a retryable `KindUnavailable` code and backs off for the `retry-after` the response names. Missing key means backfill is skipped and logged, never that the keyless API is polled.

### Reads

`GET /v1/assets/{symbol}/chart?range=` and every sparkline resolve symbol to mint, read `price_points` for the range's window, and bucket:

| Range | Bucket |
| --- | --- |
| 1D | 5 min |
| 1W, 1M, 3M | 1 hour |
| 1Y, ALL | 1 day |

Each bucket's open, high, low and close come from the samples inside it, on the server, so the app draws what it receives ([Thin client](backend-platform.md#thin-client)). The route and its response schema live in `api/openapi.yaml`, and the generated Swift client replaces today's hand-written `AssetChartSeries` decoding when iOS cuts over in Rollout step 7. The read is a CQRS-lite query through sqlc. An in-memory cache sits in front of it, with the TTL set to the sampler cadence for 1D (120 s) and 15 min for the rest, and a single-flight that collapses concurrent misses. Empty answers are cached too.

Range `ALL` is capped at whatever the table holds. On the Demo key that is 365 days from backfill day, which covers every xStock (launched June 2025) and every pre-IPO token in the catalog.

A cabal's P&L curve reads `cabal_value_snapshots`, written every 2 minutes by the `ranking` valuation job, not this table times holdings (default 2026-09-27; [leaderboards.md](leaderboards.md)). `nav_snapshots` is superseded.

### Retention

A monthly `market` poller, under the same advisory-lock rule, thins old rows: 2-minute samples kept 7 days, 5-minute for 90 days, hourly forever. Seventy mints at hourly resolution is about 600k rows a year.

### Budget

| Vendor | Calls | Per month |
| --- | --- | --- |
| Jupiter Price v3 | 2 per 120 s tick, 1 a minute | ~22k. The free tier allows 60 requests a minute in a 60-second sliding window, and Price, Swap and Token calls share one bucket ([Jupiter rate limits](https://developers.jup.ag/docs/portal/rate-limits), read 2026-09-27; the [pricing page](https://developers.jup.ag/pricing) states 1 request a second). The poller uses 1 of the 60 and leaves 59 a minute for quotes and token lookups. No monthly cap is published for the free tier. |
| CoinGecko Demo | 3 per new mint once, 1 per mint nightly | ~2.1k of 10k |

User traffic contributes zero to either row.

## Alternatives considered

- **CoinGecko as the live source with a shorter cache.** Nineteen pinned symbols refreshed every 5 min is 5.5k calls a day; Demo lasts two days. Basic at $35 gives 3.3k a day, which is one refresh of the whole catalog every 30 minutes with no headroom. Rejected: the table plus the free sampler is fresher and cheaper.
- **Birdeye by mint.** Real OHLC, full history, by mint. Rejected for now because the free tier is 30k compute units a month with per-call costs undocumented, and the design above needs no candle vendor at all. Revisit if sub-minute candles older than a day become a product need.
- **GeckoTerminal by pool.** Free and keyless, but history is per pool and xStocks liquidity is spread across several pools that come and go. Rejected.
- **Keep Yahoo for the underlying equity.** Draws the wrong instrument (equity, not token) and nothing for pre-IPO tokens. Not ported to the rewrite.
- **Pyth as the primary mark for leaderboards.** Equity feeds need an entitlement the key lacks, and Pyth prices the underlying stock, not the token the cabal holds. Dropped.
- **A second table (`asset_prices`) for valuation.** Two writers of the same fact drift. One table read by charts and boards.
- **CoinGecko Basic ($35/mo) for a 2-year backfill.** 365 days on the Demo key covers every token in the catalog. Not bought (default 2026-09-27).
- **Redis instead of Postgres.** Nothing else in the backend uses Redis, and the data is append-mostly time series that the DB handles fine at this scale. Rejected.
- **Derive prices from swap events on-chain (Helius Parsed Events).** Only prices the moments someone traded, which for a thin pre-IPO token is hours apart. Rejected; the sampler is uniform.

## Gap between this and the code

What exists today (2026-09-26), from `apps/backend`:

- `CHART_SOURCE=yahoo` (default) serves the underlying equity's curve from `internal/yahoocharts`; `CHART_SOURCE=jupiter` uses `internal/jupitercharts`, which currently returns empty.
- `internal/pricechain` holds a one-minute in-memory chart cache and single-flight; `internal/pyth` holds the chart range, window and bar types and a separate series cache.
- `internal/worker/spark_warmer.go` re-warms the 19 pinned symbols' 1D range every 5 min.
- Jupiter Price v3 is called for catalog display prices (`internal/jupiter`); the answer is not stored.
- No persistent price store.

The rewrite ignores that code ([backend-platform.md](backend-platform.md)). This decision lands in the `market` module in [Rollout](backend-platform.md#rollout) step 6, in build order:

1. `price_points` atlas migration and sqlc queries.
2. Price poller writing Jupiter Price v3 answers and publishing `price.tick`.
3. CoinGecko adapter with the limiter and 429 backoff; `monacoctl` backfill subcommand, run for the whole catalog before iOS cuts over in step 7; catalog-add trigger; nightly reconcile poller. `COINGECKO_API_KEY` in encrypted `.env.production` and `.env.local`.
4. Chart read path bucketing from the table, behind the in-memory cache with the TTLs above.
5. Retention poller.
6. "Data provided by CoinGecko" attribution on the asset screen while on the Demo plan.

Every step is a `flows.tsv` row with its outcomes tested before the step closes. `internal/jupitercharts`, `internal/yahoocharts`, `SparkWarmer` and `CHART_SOURCE` go when the old backend is deleted in step 7.

## Open questions

None.

Log: [log/price-history.md](log/price-history.md).
