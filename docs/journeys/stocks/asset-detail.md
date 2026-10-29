---
id: stocks/asset-detail
title: Look at a stock
version: 12
milestone: M11
requires: [auth/sign-in, stocks/browse]
actors: [A]
flows: [18]
xcuitest: [apps/mobile/MonacoUITests/Journeys/StocksAssetDetailJourney.swift, apps/mobile/MonacoUITests/Journeys/StocksAssetDetailJourneyUITests.swift]
---

# Look at a stock

A signed-in member opens a stock from the Markets tab and reads its screen (F frames `07-stock-dark` and `07-stock-preipo`): the header with the watch star, the price with "+$… · …% today", the line chart with the ranges 1H, 1D, 1W, 1M, 3M, 6M and All, the Market stats grid, and Propose buy, which opens Choose a cabal and then Amount. On a Pre-IPO token they read Round stats. When one of their cabals holds the stock, they see "In your cabals" and Propose sell. The page draws the line only at launch (founder decision Q34).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Everything [auth/sign-in](../auth/sign-in.md) needs, and the Markets tab [stocks/browse](browse.md) opens |
| P2 | Before each scenario, `scripts/qa/journey.py` runs `apps/mobile/qa/journeys/stocks/asset-detail.setup.sh` with the scenario id. It runs `apps/mobile/qa/journeys/stocks/browse.setup.sh`, which upserts the catalogue rows of [stocks/browse](browse.md) (P3 there), among them `JRNYQx`, a second `pre_ipo` listing of "Journey Private" from issuer `tessera`, with the same `company_key` as `JRNYPx`. Each row gets two `price_points` samples, stamped twelve hours ago and now, and `JRNYAx` gets 120 more at five-minute spacing ending at now, so its chart has bars to draw, pinch and pan. The setup then lowers the older `JRNYAx` sample (twelve hours ago) to $120.00, so the year range has a width and the 52-week bar draws (it needs a high above the low). No step taps to create them |
| P3 | `JRNYAx` is `issuer_tradable`, so its Propose a buy is enabled |
| P4 | S5 needs a cabal of A's that holds `JRNYAx`. A treasury position comes only from a confirmed trade, and the trade path is #2136's, so the setup script seeds no holding and S5 runs against a member with none |
| P5 | S1 needs A in at least two cabals so Propose a buy opens the cabal picker. The S1 setup counts the cabals A votes in and has A create cabals `QA stocks {QA.run} <n>` through the API for the shortfall (cabal creation is rate limited, and earlier runs leave cabals), and marks A done with onboarding |
| P6 | The run never calls the live Jupiter API. The steps read only what `GET /v1/assets/{symbol}` and its chart serve from the seeded rows |

## Scenarios

### S1 The header, the chart and Propose a buy

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S1.1 | tap, type, then tap | the Markets tab `tab-assets`, then `monaco-search-field`, then `assets-row-JRNYAx` | `JRNYA` | Within 15 s of typing, `assets-row-JRNYAx` shows, because the Markets tab lists the whole catalogue a page at a time and renders only the rows in view. Within 15 s of the tap, `asset-detail-root` shows under the inline title "Journey Alpha". Old app: a Stocks row pushed `AssetDetailView` (`asset-detail-root`). screens.md: the title is the stock's name |
| S1.2 | wait | `asset-detail-name`, `asset-detail-price` | | `asset-detail-name` reads "Journey Alpha", and `asset-detail-price` reads "$123.45". Old app: the hero's name, ticker and price. screens.md: Name and ticker, price in `moneyFont(.hero)` |
| S1.3 | wait | `asset-detail-range-change`, `asset-chart-ranges` | | Within 15 s, `asset-detail-range-change` reads a change line ending "today" ("+$… · …% today"), and `asset-chart-ranges` holds "1H", "1D", "1W", "1M", "3M", "6M" and "All" with "1D" selected |
| S1.4 | wait | `asset-detail-chart` | | Within 15 s, `asset-detail-chart` shows the seeded line |
| S1.5 | tap | "1W" in `asset-chart-ranges` | | Within 10 s, "1W" is selected |
| S1.6 | tap | "All" in `asset-chart-ranges` | | Within 10 s, "All" is selected |
| S1.7 | scroll to | `asset-detail-propose-buy` | | `asset-detail-propose-buy` reads "Propose buy" and is enabled |
| S1.8 | tap | `asset-detail-propose-buy` | | Within 10 s, the cabal picker shows as a half sheet titled "Choose a cabal", `propose-pick-cabal-question` asks "Which cabal should buy JRNYA?", and at least two cabal rows follow |
| S1.9 | tap | the first cabal row | | Within 10 s, `propose-amount-screen` shows titled "Buy JRNYA" with the header `propose-amount-operation` reading "Buy Journey Alpha". A backs out without proposing. Old app: the picker opened the amount screen. screens.md: Propose from a stock (#613) |

### S2 Market stats and the 52-week bar

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S2.1 | tap, type, then tap | the Markets tab `tab-assets`, then `monaco-search-field`, then `assets-row-JRNYAx` | `JRNYA` | Within 15 s, `asset-detail-root` shows |
| S2.2 | scroll to | `asset-stats` | | Within 10 s, `asset-stats` shows the Market stats grid: open, previous close, day high and low, and 52-week high and low, leaving out a cell the candles cannot give, then a "24h volume" cell when the seeded asset has a volume reading from the last 36 hours. Old app: the Stats card. screens.md: "Market stats" (open, previous close, day high and low, 52-week high and low, 24h volume). Market cap and supply open soon, so the grid has no cell for them |

### S3 A Pre-IPO token

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S3.1 | tap, type, then tap | the Markets tab `tab-assets`, then `monaco-search-field`, then `assets-row-JRNYPx` | `JRNYPx` | Within 15 s, `asset-detail-root` shows and `asset-detail-name` reads "Journey Private" |
| S3.2 | scroll to | "Round stats" | | Within 10 s, "Round stats" shows (`07-stock-preipo`) |

### S4 Scrub the line

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S4.1 | tap, type, then tap | the Markets tab `tab-assets`, then `monaco-search-field`, then `assets-row-JRNYAx` | `JRNYA` | Within 15 s, `asset-detail-root` and `asset-detail-chart` show, with no interval picker `asset-chart-interval` and no candle toggle `asset-chart-style` (the page is line only at launch, Q34) |
| S4.2 | press | `asset-detail-chart` | hold 1.5 s | The chart is still on screen while pressed. The crosshair (`asset-chart-crosshair`) clears on release, so the step asserts the chart only |

### S5 Cabals holding the stock and Propose a sell

Starts signed in (auth/sign-in), with a cabal of A's holding `JRNYAx` (P4).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S5.1 | tap, type, then tap | the Markets tab `tab-assets`, then `monaco-search-field`, then `assets-row-JRNYAx` | `JRNYA` | Within 15 s, `asset-detail-root` shows |
| S5.2 | scroll to | `asset-in-your-cabals` | | Within 10 s, `asset-in-your-cabals` ("In your cabals") shows one `asset-held-<cabal id>` row per cabal that holds it. Old app: the position card. screens.md: "Cabals holding <TICKER>" with one row per cabal of the viewer's that holds it |
| S5.3 | scroll to, then tap | `asset-detail-propose-sell` | | Within 10 s, the propose screen opens with "Propose sell". Old app: Propose sell from the position card. screens.md: plus "Propose a sell" when a cabal holds it |

## Ground truth

Looking at a stock writes nothing, and S1.9 backs out of Amount. After a run, `apps/mobile/qa/journeys/stocks/asset-detail.truth.sh` queries `price_points` and checks that the latest row for the JRNYAx mint is the 123450000 micros sample from the last 30 minutes, so the chart and price read the sampled series (flow 18, Prices). It then runs the [stocks/browse](browse.md) check that the seeded rows still hold the values P2 gives and that no proposal names a seeded asset, and deletes the seeded rows. The samples are seeded, so this proves the screen reads `price_points`, not that the live sampler ran.

## Known failures on staging

| Step | Why | Blocking ticket |
| --- | --- | --- |
| S5.2, S5.3 | No path seeds a holding of `JRNYAx` for A (P4), so "In your cabals" is hidden and Propose a sell does not show | #2136 |

## Not covered

- "Also available from" and its listings (`asset-other-listing-*`). The F stock page (`07-stock-preipo`, #6544) does not draw it.
- The 52-week bar `asset-year-range`. The F stock page has no identifier for it, so no step asserts it. It is a follow-up against the stock page ticket.
- Zoom, pan and the jump-to-latest button, which need candles (Q34). "Secondary estimate" on the Pre-IPO page: the seeded rows carry no estimate.

- The cash on the picker rows ("<amount> cash") and the Amount helper "The cabal can spend $0.00". [governance/propose-from-asset](../governance/propose-from-asset.md) reads the Amount screen of an empty pot.
- Review and Send. [governance/propose-from-asset](../governance/propose-from-asset.md) covers them.
- Searching GOOGL and the "No matching stocks" copy. [stocks/browse](browse.md) S2 covers search on the seeded catalogue.

- The market session chip, `asset-detail-session` ("Market closed"). It shows by wall-clock time. `MarketSessionCopyTests` covers the copy.
- Loading older bars at the left edge. The seed has one page, so there is no older page to fetch. `AssetDetailClientModelTests` covers `loadOlder`.
- "Price history builds up over time." and "The curve draws as the token trades." for a short history. The seed gives every row two samples, which draws a curve. `AssetDetailClientModelTests` covers the short-history rule.
- The "Unavailable" button and the warn notice for an untradable asset, the stale price notice and the "opens soon" lines. `AssetDetailSampleUITests` covers them against the sample harness (`untradable`, `stale`, `open`). [stocks/watchlist](watchlist.md) covers the watch star.
- The "24h volume" cell. No setup seeds an `asset_volumes` row for the journey assets, so S2.2 draws no cell for it. `MarketMappingTests` covers the mapping and `UsdAmountFormatter.compact(micros:)`.
- The live price tick flash. It needs a new sample mid-scenario.
- "Private-market reference" and "About <token>". The asset screen does not render them (`GET /v1/assets/{symbol}` does not serve them), so S3 no longer has steps for them.
