---
id: stocks/asset-detail
title: Look at a stock
version: 2
milestone: M11
requires: [auth/sign-in, stocks/browse]
actors: [A]
flows: [18]
xcuitest: [apps/mobile/MonacoUITests/Journeys/StocksAssetDetailJourney.swift, apps/mobile/MonacoUITests/Journeys/StocksAssetDetailJourneyUITests.swift]
---

# Look at a stock

A signed-in member opens a stock from the Stocks tab and reads its screen: the hero with price and session, the chart with its range chips, the Stats card with the 52-week bar, and the Propose buy button, which opens Pick a cabal and then Amount. On a Pre-IPO token they also read the private-market reference, About and "Also available from". When one of their cabals holds the stock, they see "Your cabals' position" and Propose sell. The spec is [Stocks: Asset screen](../../screens.md#stocks-tab). The old app's screen is `apps/mobile/Monaco/Features/Assets/AssetDetailView.swift` at `c838bd24`, which every step cites as "old app".

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Everything [auth/sign-in](../auth/sign-in.md) needs, and the Stocks tab [stocks/browse](browse.md) opens |
| P2 | Before each scenario, `scripts/qa/journey.py` runs `apps/mobile/qa/journeys/stocks/asset-detail.setup.sh` with the scenario id. It runs `apps/mobile/qa/journeys/stocks/browse.setup.sh`, which upserts the catalogue rows of [stocks/browse](browse.md) (P3 there), among them `JRNYQx`, a second `pre_ipo` listing of "Journey Private" from issuer `tessera`, with the same `company_key` as `JRNYPx`. Each row gets two `price_points` samples, stamped twelve hours ago and now. No step taps to create them |
| P3 | `JRNYAx` is `issuer_tradable`, so its Propose buy is enabled |
| P4 | S4 needs a cabal of A's that holds `JRNYAx`. A treasury position comes only from a confirmed trade, and the trade path is #2136's, so the setup script seeds no holding and S4 runs against a member with none |
| P5 | S1 needs A in at least two cabals so Propose buy opens the cabal picker. The S1 setup counts the cabals A votes in and has A create open cabals `QA stocks {QA.run} <n>` through the API for the shortfall (cabal creation is rate limited, and earlier runs leave cabals), and marks A done with onboarding |
| P6 | The run never calls the live Jupiter API. The steps read only what `GET /v1/assets/{symbol}` and its chart serve from the seeded rows |

## Scenarios

### S1 The hero, the chart and Propose buy

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S1.1 | tap, then tap | the Stocks tab `tab-assets`, then `assets-row-JRNYAx` | | Within 15 s, `asset-detail-root` shows under the inline title "JRNYA". Old app: a Stocks row pushed `AssetDetailView` (`asset-detail-root`). screens.md: Title is the ticker |
| S1.2 | wait | `asset-detail-name`, `asset-detail-price` | | `asset-detail-name` reads "Journey Alpha", `asset-detail-price` reads "$123.45", and "Trading 24/7 on Solana" shows. Old app: the hero's name, ticker and price. screens.md: Name and ticker, price in `quoteHero`, "Trading 24/7 on Solana" |
| S1.3 | wait | `asset-detail-range-change`, `asset-chart-ranges` | | Within 15 s, `asset-detail-range-change` reads "Past day · JRNYA", and `asset-chart-ranges` holds "1D", "1W", "1M", "3M", "1Y" and "ALL" with "1D" selected. Old app: the range chips under the chart. screens.md: the change chip for the selected range ("Past day · GOOGL"), chips 1D 1W 1M 3M 1Y ALL |
| S1.4 | wait | `asset-detail-chart` | | Within 15 s, `asset-detail-chart` shows the two seeded samples. Old app: the scrub chart. screens.md: a scrubbable chart |
| S1.5 | tap | "1W" in `asset-chart-ranges` | | Within 10 s, "1W" is selected and `asset-detail-range-change` reads "Past week · JRNYA". Old app: a range chip reloaded the series. screens.md: chips 1D 1W 1M 3M 1Y ALL |
| S1.6 | tap | "1Y" in `asset-chart-ranges` | | Within 10 s, "1Y" is selected and `asset-detail-range-change` reads "Past year · JRNYA". Old app: a range chip reloaded the series. screens.md: chips 1D 1W 1M 3M 1Y ALL |
| S1.7 | wait | `asset-detail-propose-buy` | | `asset-detail-propose-buy` reads "Propose buy", is enabled, and "Your cabal votes before anything is bought" shows under it. Old app: the pinned Propose buy CTA. screens.md: Pinned CTA "Propose buy" with the caption "Your cabal votes before anything is bought" |
| S1.8 | tap | `asset-detail-propose-buy` | | Within 10 s, the cabal picker shows with the title "Which cabal should buy JRNYAx?" and at least two cabal rows. Old app: Propose buy opened the cabal picker. screens.md: Propose from a stock (#613) |
| S1.9 | tap | the first cabal row | | Within 10 s, `propose-amount-screen` shows titled "Amount" with the stock row for JRNYAx. A backs out without proposing. Old app: the picker opened the amount screen. screens.md: Propose from a stock (#613) |

### S2 Stats and the 52-week bar

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S2.1 | tap, then tap | the Stocks tab `tab-assets`, then `assets-row-JRNYAx` | | Within 15 s, `asset-detail-root` shows |
| S2.2 | scroll to | "Stats" | | Within 10 s, a "Stats" card shows open, day high and low, previous close, 52-week high and low and buy premium. Old app: the Stats card. screens.md: "Stats" (open, day high and low, previous close, 52-week high and low, buy premium) |
| S2.3 | wait | "52-week range" | | The 52-week range bar shows under Stats. Old app: the 52-week bar. screens.md: the 52-week range bar |

### S3 A Pre-IPO token

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S3.1 | tap, scroll to, then tap | the Stocks tab `tab-assets`, then `assets-row-JRNYPx` | | Within 15 s, `asset-detail-root` shows and `asset-detail-name` reads "Journey Private". Old app: a Pre-IPO row pushed `AssetDetailView`. screens.md: Stocks: Asset screen |
| S3.2 | scroll to | `asset-other-listings` | | Within 10 s, "Also available from" shows with `asset-other-listing-JRNYQx`. Old app: "Also available from" issuers. screens.md: "Also available from" issuers |
| S3.3 | tap | `asset-other-listing-JRNYQx` | | Within 10 s, `asset-detail-root` shows for that listing, titled "JRNYQx". Old app: an issuer row opened that listing's asset screen. screens.md: "Also available from" issuers |

### S4 Your cabals' position and Propose sell

Starts signed in (auth/sign-in), with a cabal of A's holding `JRNYAx` (P4).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S4.1 | tap, then tap | the Stocks tab `tab-assets`, then `assets-row-JRNYAx` | | Within 15 s, `asset-detail-root` shows |
| S4.2 | scroll to | "Your cabals' position" | | Within 10 s, "Your cabals' position" shows one `asset-position-holding-…` row per cabal that holds it. Old app: the position card. screens.md: When one of the viewer's cabals holds it: "Your cabals' position" with one row per cabal |
| S4.3 | tap | `asset-detail-sell` | | Within 10 s, the propose screen opens with "Propose sell". Old app: Propose sell from the position card. screens.md: plus "Propose sell" when a cabal holds it |

## Ground truth

Looking at a stock writes nothing, and S1.9 backs out of Amount. After a run, `apps/mobile/qa/journeys/stocks/asset-detail.truth.sh` queries `price_points` and checks that the latest row for the JRNYAx mint is the 123450000 micros sample from the last 30 minutes, so the chart and price read the sampled series (flow 18, Prices). It then runs the [stocks/browse](browse.md) check that the seeded rows still hold the values P2 gives and that no proposal names a seeded asset, and deletes the seeded rows. The samples are seeded, so this proves the screen reads `price_points`, not that the live sampler ran.

## Known failures on staging

| Step | Why | Blocking ticket |
| --- | --- | --- |
| S2.2, S2.3 | The asset screen dropped the Stats card and the 52-week bar | #577 |
| S4.2, S4.3 | The asset screen dropped "Your cabals' position" and Propose sell, and no path seeds a holding | #577, #2136 |

## Not covered

- The pot on the picker rows and the Amount helper "The pot has $0.00". Staging's picker shows no pot, and the copy moves with #2706.
- Review and Send. [governance/propose-from-asset](../governance/propose-from-asset.md) covers them.
- Searching GOOGL and the "No stocks match" copy. [stocks/browse](browse.md) S2 covers search on the seeded catalogue.

- The market session chip, `asset-detail-session` ("Market closed"). It shows by wall-clock time. `MarketSessionCopyTests` covers the copy.
- Scrubbing the chart. A drag reads out a price at a point that moves with the seed's timestamps. `AssetChartSeriesTests` covers the nearest-point math.
- "Price history builds up over time." and "The curve draws as the token trades." for a short history. The seed gives every row two samples, which draws a curve. `AssetDetailClientModelTests` covers the short-history rule.
- "Can't buy right now" on an untradable asset. The sample harness `-MonacoAssetDetailSample untradable` shows it.
- The live price tick flash. It needs a new sample mid-scenario.
- "Private-market reference" and "About <token>". The asset screen does not render them (`GET /v1/assets/{symbol}` does not serve them), so S3 no longer has steps for them.
- Targets with no accessibility identifier: "Stats" and "52-week range" (S2) and "Your cabals' position" (S4) have no view in `apps/mobile/Monaco/Features/Assets/AssetDetailClientView.swift` on staging. The steps target their copy until #577 adds the views and their identifiers.
