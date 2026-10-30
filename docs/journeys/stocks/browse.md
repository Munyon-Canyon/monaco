---
id: stocks/browse
title: Browse stocks
version: 7
milestone: M11
requires: [auth/sign-in]
actors: [A]
flows: []
xcuitest: [apps/mobile/MonacoUITests/Journeys/StocksBrowseJourney.swift, apps/mobile/MonacoUITests/Journeys/StocksBrowseJourneyUITests.swift]
---

# Browse stocks

A signed-in member opens the Markets tab, reads the search "Stocks and Pre-IPO" and the chips All, Watchlist, Pre-IPO and Held, searches by name, opens the Pre-IPO and Held chips, finds nothing for a nonsense query, and pulls to refresh. The spec is [Markets tab](../../screens.md#markets-tab) and the F frames `06-markets-A`, `06-markets-search` and `06-markets-held`. F has no Popular section and no Sort or Filter selects.

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Everything [auth/sign-in](../auth/sign-in.md) needs |
| P2 | Before each scenario, `scripts/qa/journey.py` runs `apps/mobile/qa/journeys/stocks/browse.setup.sh` with the scenario id. It runs `apps/mobile/qa/journeys/stocks/browse.catalogue.sql`, which upserts four catalogue rows into `assets`, marked chain-checked and tradable by override, and two `price_points` samples for each, stamped six days ago and now. No step taps to create them |
| P3 | `JRNYAx` is an `equity` named "Journey Alpha" with `popular_rank` 1, priced $123.45. `JRNYPx` is a `pre_ipo` named "Journey Private", priced $50.00. `JRNYZx` is an `equity` named "Journey Zulu" with no `popular_rank`, priced $10.00. The fourth row, `JRNYQx`, is a second listing of "Journey Private" that [stocks/asset-detail](asset-detail.md) reads |
| P4 | The market data is the seeded rows. The run never calls the live Jupiter API: `journey.py` starts the local backend, and the steps read only what `GET /v1/assets` serves from the database |
| P5 | Before S1 the setup takes A out of every cabal that holds a stock (journeys like governance/propose-sell seed one through `qa_seed_scenario cabal-with-confirmed-trade A`), so S1.5's Held empty state does not depend on journey order. A later journey that seeds holdings for A must not rely on browse |

## Scenarios

### S1 Browse with the chips

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S1.1 | tap | the Markets tab, `tab-assets` | | Within 15 s, `assets-root` shows under the large title "Markets", with `monaco-search-field` holding the placeholder "Stocks and Pre-IPO" and the chips `markets-chip-All` (selected), `markets-chip-Watchlist`, `markets-chip-Pre-IPO` and `markets-chip-Held`. The chip identifiers carry the chip's title, so they are not lowercase |
| S1.2 | type | `monaco-search-field` | `Journey` | Within 15 s, `assets-row-JRNYAx` shows reading "JRNYA", "Journey Alpha" and "$123.45", and there is no `stocks-sort` and no `stocks-filter` |
| S1.3 | tap Cancel, then tap | `markets-search-cancel`, then `markets-chip-Pre-IPO` | | Within 10 s, `markets-chip-Pre-IPO` is selected and `markets-preipo-row-JRNYPx` reads "Journey Private" with "Eligible" or "Research only", and `markets-preipo-row-JRNYAx` does not show |
| S1.4 | tap, type, then scroll to | `markets-chip-All`, `monaco-search-field`, then `assets-row-JRNYZx` | `Journey` | Within 30 s, `assets-row-JRNYZx` reading "Journey Zulu" shows. The All chip draws only Watchlist, Pre-IPO and Moving today, so the paged catalogue is reached through search |
| S1.5 | tap Cancel, then tap | `markets-search-cancel`, then `markets-chip-Held` | | Within 10 s, `markets-held-empty` shows "Your cabals don't hold any stocks yet." for a member whose cabals hold nothing |

### S2 Search

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S2.1 | tap, tap, then type | the Markets tab, `markets-chip-All` (S1 left Held selected, and Cancel keeps the chip), then `monaco-search-field` | `Journey Alpha` | Within 10 s, `assets-row-JRNYAx` shows and `assets-row-JRNYZx` does not |
| S2.2 | clear, then type | `monaco-search-field` | `zzqj{QA.run}` | Within 10 s, `assets-empty` reads "No results for" and the query |
| S2.3 | tap Cancel | `markets-search-cancel` | | Within 10 s, `assets-empty` is gone and the chips are back with `markets-chip-All` selected |

### S3 Pull to refresh

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S3.1 | tap, then tap | the Markets tab, then `markets-chip-Pre-IPO` | | Within 15 s, `markets-preipo-row-JRNYPx` shows |
| S3.2 | pull down | `assets-root` | | Within 15 s, `markets-preipo-row-JRNYPx` still shows reading "Journey Private", and neither `assets-failed` ("Couldn't load this view") nor a toast shows |

## Ground truth

Browsing writes nothing. After a run, `apps/mobile/qa/journeys/stocks/browse.truth.sh` checks that the three seeded rows still hold the values P3 gives, so a run that read anything else read another catalogue, and that no proposal names a seeded asset. It then deletes the seeded rows and their samples, so the dev Markets tab shows only the real catalogue between runs.

## Known failures on staging

None known. The ticket words S2.2 as "No matching stocks", but Markets search says "No results for “<query>”" (`assets-empty`), and the step follows the app.

## Not covered

- The moon glyph labelled "Market closed" on a row. It shows only while the home market is closed, so a step for it passes or fails by wall-clock time. `MarketSessionCopyTests` in `packages/mobile-core` covers the session copy.
- The day change chip's value. The seeded samples sit one day apart, and the change window moves with the clock. The step reads the price, which the seed fixes.
- Top movers and Moving today. The seeded rows carry no day change a mover ranking could read, so `StocksTabSampleUITests` covers them against the sample harness (`-MonacoStocksTabSample ready`). [stocks/watchlist](watchlist.md) covers the Watchlist section, Manage and the stars.
- The "Prices delayed" notice and the full-screen "Couldn't load this view" with "Try again". They need the backend to fail mid-run, which belongs to the flow layer. `StocksTabSampleUITests` covers them (`stale` and `failed`).
- "Secondary estimate · <date>" on a Pre-IPO row. The seeded rows carry no estimate, so the row reads its ticker where the estimate line goes.
- Logos. The seeded rows have no `logo_url`, so each row draws its monogram.
