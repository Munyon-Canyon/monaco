---
id: stocks/browse
title: Browse stocks
version: 4
milestone: M11
requires: [auth/sign-in]
actors: [A]
flows: []
xcuitest: [apps/mobile/MonacoUITests/Journeys/StocksBrowseJourney.swift, apps/mobile/MonacoUITests/Journeys/StocksBrowseJourneyUITests.swift]
---

# Browse stocks

A signed-in member opens the Markets tab, reads the All stocks list and the Filter select's Popular and Pre-IPO options, searches by name, finds nothing for a nonsense query, and pulls to refresh. The spec is [Markets tab](../../screens.md#markets-tab). The old app's tab is `apps/mobile/Monaco/Features/Assets/AssetsTabView.swift` at `c838bd24`, which every step cites as "old app".

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Everything [auth/sign-in](../auth/sign-in.md) needs |
| P2 | Before each scenario, `scripts/qa/journey.py` runs `apps/mobile/qa/journeys/stocks/browse.setup.sh` with the scenario id. It runs `apps/mobile/qa/journeys/stocks/browse.catalogue.sql`, which upserts four catalogue rows into `assets`, marked chain-checked and tradable by override, and two `price_points` samples for each, stamped twelve hours ago and now. No step taps to create them |
| P3 | `JRNYAx` is an `equity` named "Journey Alpha" with `popular_rank` 1, priced $123.45. `JRNYPx` is a `pre_ipo` named "Journey Private", priced $50.00. `JRNYZx` is an `equity` named "Journey Zulu" with no `popular_rank`, priced $10.00. The fourth row, `JRNYQx`, is a second listing of "Journey Private" that [stocks/asset-detail](asset-detail.md) reads |
| P4 | The market data is the seeded rows. The run never calls the live Jupiter API: `journey.py` starts the local backend, and the steps read only what `GET /v1/assets` serves from the database |

## Scenarios

### S1 Browse with the Filter select

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S1.1 | tap | the Markets tab, `tab-assets` | | Within 15 s, `assets-root` shows under the large title "Markets", with `monaco-search-field` holding the placeholder "Search stocks or tickers". Old app: the Markets tab opened `assets-root` with a search placeholder. screens.md: Large title "Markets". Search "Search stocks or tickers" |
| S1.2 | scroll to | `stocks-filter`, then `assets-row-JRNYAx` | | Within 15 s, the Sort select `stocks-sort` and the Filter select `stocks-filter` show under "All stocks", with the Filter select reading "All", and `assets-row-JRNYAx` shows reading "JRNYA" and "Journey Alpha" with "$123.45". screens.md: "All stocks" with the Sort and Filter selects, "All" (`filter=all`, the default); rows show logo, name over ticker, price, day change |
| S1.3 | tap | `stocks-filter`, then "Popular" | | Within 10 s, the Filter select reads "Popular", `assets-row-JRNYAx` shows and `assets-row-JRNYPx` and `assets-row-JRNYZx` do not. screens.md: "Popular" (`filter=popular`) |
| S1.4 | tap | `stocks-filter`, then "Pre-IPO" | | Within 10 s, the Filter select reads "Pre-IPO", `assets-row-JRNYPx` shows reading "Journey Private", and `assets-row-JRNYAx` does not. screens.md: "Pre-IPO" (`filter=pre_ipo`) |
| S1.5 | tap, then scroll to | `stocks-filter`, then "All", then `assets-row-JRNYZx` | | Within 30 s, `assets-row-JRNYZx` reading "Journey Zulu" shows. Scrolling loads more pages until the row is in. screens.md: "All" (`filter=all`, paged) |

### S2 Search

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S2.1 | tap, then type | the Markets tab, then `monaco-search-field` | `Journey Alpha` | Within 10 s, `assets-grid-search` replaces the list, holds `assets-row-JRNYAx`, and holds no `assets-row-JRNYZx`. Old app: search results in `assets-grid-search`. screens.md: results replace the list |
| S2.2 | clear, then type | `monaco-search-field` | `zzqj{QA.run}` | Within 10 s, `assets-search-empty` reads "No matching stocks". Old app: "No matches for that search". screens.md: no match shows "No matching stocks" with "Try a company name, ticker or another filter." |
| S2.3 | clear | `monaco-search-field` | | Within 10 s, `assets-grid` is back with the selected filter and `assets-row-JRNYAx`. Old app: an empty query showed the browse sections again |

### S3 Pull to refresh

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S3.1 | tap, then scroll to | the Markets tab, `tab-assets`, then `assets-row-JRNYAx` | | Within 15 s, `assets-row-JRNYAx` shows in `assets-grid` |
| S3.2 | pull down | `assets-grid` | | Within 15 s, `assets-row-JRNYAx` still shows with "$123.45", and neither `assets-failed` ("Couldn't load this view") nor a toast shows. Old app: `.refreshable` on `assets-grid-popular`. screens.md: Markets tab, pull to refresh |

## Ground truth

Browsing writes nothing. After a run, `apps/mobile/qa/journeys/stocks/browse.truth.sh` checks that the three seeded rows still hold the values P3 gives, so a run that read anything else read another catalogue, and that no proposal names a seeded asset. It then deletes the seeded rows and their samples, so the dev Markets tab shows only the real catalogue between runs.

## Known failures on staging

| Step | Why | Blocking ticket |
| --- | --- | --- |
| S1.1, S2.1, S3.1 | The Stocks tab is blank on staging. `StocksTabView` hangs its `.task` on a `Group` that is empty until the task has run, so the task never runs, the model is never made, and `assets-root` has no content. Every later step of the scenario fails behind it | #577 |
| S1.2 | `assets-root` and the "Markets" title show, but no `assets-row-JRNYAx` appears within 15 s: the seeded `JRNY…x` stocks are not in the list. #2739 ("Load the Stocks tab when it first appears") did not bring them in | #577 |

## Not covered

- The moon glyph labelled "Market closed" on a row. It shows only while the home market is closed, so a step for it passes or fails by wall-clock time. `MarketSessionCopyTests` in `packages/mobile-core` covers the session copy.
- The day change chip's value. The seeded samples sit one day apart, and the change window moves with the clock. The step reads the price, which the seed fixes.
- Top movers, Watchlist and Popular now. The seeded rows carry no day change a mover ranking could read and no stars, so `StocksTabSampleUITests` covers them against the sample harness (`-MonacoStocksTabSample ready` and `watchlistEmpty`).
- The "Prices delayed" notice and the full-screen "Couldn't load this view" with "Try again". They need the backend to fail mid-run, which belongs to the flow layer. `StocksTabSampleUITests` covers them (`stale` and `failed`).
- Logos. The seeded rows have no `logo_url`, so each row draws its monogram.
