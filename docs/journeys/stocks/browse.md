---
id: stocks/browse
title: Browse stocks
version: 2
milestone: M11
requires: [auth/sign-in]
actors: [A]
flows: []
xcuitest: [apps/mobile/MonacoUITests/Journeys/StocksBrowseJourney.swift, apps/mobile/MonacoUITests/Journeys/StocksBrowseJourneyUITests.swift]
---

# Browse stocks

A signed-in member opens the Stocks tab, reads the All list and the Popular and Pre-IPO chips, searches by name, finds nothing for a nonsense query, and pulls to refresh. The spec is [Stocks tab](../../screens.md#stocks-tab). The old app's tab is `apps/mobile/Monaco/Features/Assets/AssetsTabView.swift` at `c838bd24`, which every step cites as "old app".

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Everything [auth/sign-in](../auth/sign-in.md) needs |
| P2 | Before each scenario, `scripts/qa/journey.py` runs `apps/mobile/qa/journeys/stocks/browse.setup.sh` with the scenario id. It runs `apps/mobile/qa/journeys/stocks/browse.catalogue.sql`, which upserts four catalogue rows into `assets`, marked chain-checked and tradable by override, and two `price_points` samples for each, stamped twelve hours ago and now. No step taps to create them |
| P3 | `JRNYAx` is an `equity` named "Journey Alpha" with `popular_rank` 1, priced $123.45. `JRNYPx` is a `pre_ipo` named "Journey Private", priced $50.00. `JRNYZx` is an `equity` named "Journey Zulu" with no `popular_rank`, priced $10.00. The fourth row, `JRNYQx`, is a second listing of "Journey Private" that [stocks/asset-detail](asset-detail.md) reads |
| P4 | The market data is the seeded rows. The run never calls the live Jupiter API: `journey.py` starts the local backend, and the steps read only what `GET /v1/assets` serves from the database |

## Scenarios

### S1 Browse with the chips

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S1.1 | tap | the Stocks tab, `tab-assets` | | Within 15 s, `assets-root` shows under the large title "Stocks", with `monaco-search-field` holding the placeholder "Search Apple, Tesla, NVDA…". Old app: the Stocks tab opened `assets-root` with the same placeholder. screens.md: Large title "Stocks". Search "Search Apple, Tesla, NVDA…" |
| S1.2 | wait | `assets-grid` | | Within 15 s, the chips `stocks-chip-All`, `stocks-chip-Popular` and `stocks-chip-Pre-IPO` show under the search field with "All" selected, and `assets-row-JRNYAx` shows reading "JRNYA" and "Journey Alpha" with "$123.45". screens.md: chips "All" (`filter=all`, default), "Popular", "Pre-IPO"; rows show logo, ticker over name, price, day change chip |
| S1.3 | tap | `stocks-chip-Popular` | | Within 10 s, `assets-row-JRNYAx` shows and `assets-row-JRNYPx` and `assets-row-JRNYZx` do not. screens.md: "Popular" (`filter=popular`) |
| S1.4 | tap | `stocks-chip-Pre-IPO` | | Within 10 s, `assets-row-JRNYPx` shows reading "Journey Private", and `assets-row-JRNYAx` does not. screens.md: "Pre-IPO" (`filter=pre_ipo`) |
| S1.5 | tap, then scroll to | `stocks-chip-All`, then `assets-row-JRNYZx` | | Within 30 s, `assets-row-JRNYZx` reading "Journey Zulu" shows. Scrolling loads more pages until the row is in. screens.md: "All" (`filter=all`, paged) |

### S2 Search

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S2.1 | tap, then type | the Stocks tab, then `monaco-search-field` | `Journey Alpha` | Within 10 s, `assets-grid-search` replaces the list, holds `assets-row-JRNYAx`, and holds no `assets-row-JRNYZx`. Old app: search results in `assets-grid-search`. screens.md: results replace the list |
| S2.2 | clear, then type | `monaco-search-field` | `zzqj{QA.run}` | Within 10 s, `assets-search-empty` reads "No stocks match “zzqj{QA.run}”". Old app: "No matches for that search". screens.md: no match shows "No stocks match “<q>”" |
| S2.3 | clear | `monaco-search-field` | | Within 10 s, `assets-grid` is back with the selected chip and `assets-row-JRNYAx`. Old app: an empty query showed the browse sections again |

### S3 Pull to refresh

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S3.1 | tap | the Stocks tab, `tab-assets` | | Within 15 s, `assets-row-JRNYAx` shows in `assets-grid` |
| S3.2 | pull down | `assets-grid` | | Within 15 s, `assets-row-JRNYAx` still shows with "$123.45", and neither `assets-failed` ("Couldn't load stocks.") nor a toast shows. Old app: `.refreshable` on `assets-grid-popular`. screens.md: Stocks tab, pull to refresh |

## Ground truth

Browsing writes nothing. After a run, `apps/mobile/qa/journeys/stocks/browse.truth.sh` checks that the three seeded rows still hold the values P3 gives, so a run that read anything else read another catalogue, and that no proposal names a seeded asset. It then deletes the seeded rows and their samples, so the dev Stocks tab shows only the real catalogue between runs.

## Known failures on staging

| Step | Why | Blocking ticket |
| --- | --- | --- |
| S1.1, S2.1, S3.1 | The Stocks tab is blank on staging. `StocksTabView` hangs its `.task` on a `Group` that is empty until the task has run, so the task never runs, the model is never made, and `assets-root` has no content. Every later step of the scenario fails behind it | #577 |
| S1.2 | `assets-root` and the "Stocks" title show, but no `assets-row-JRNYAx` appears within 15 s: the seeded `JRNY…x` stocks are not in the list. #2739 ("Load the Stocks tab when it first appears") did not bring them in | #577 |

## Not covered

- The moon glyph labelled "Market closed" on a row. It shows only while the home market is closed, so a step for it passes or fails by wall-clock time. `MarketSessionCopyTests` in `packages/mobile-core` covers the session copy.
- The day change chip's value. The seeded samples sit one day apart, and the change window moves with the clock. The step reads the price, which the seed fixes.
- The old app's "In your cabals", "Up for vote" and "Top movers" sections. `docs/screens.md` does not list them for the Stocks tab, so they are not part of the spec this journey checks.
- The failure states "Couldn't load stocks." with "Try again". They need the backend to fail mid-run, which belongs to the flow layer.
- Logos. The seeded rows have no `logo_url`, so each row draws its monogram.
