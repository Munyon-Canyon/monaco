---
id: stocks/asset-detail
title: Look at a stock
version: 1
milestone: M11
requires: [auth/sign-in, stocks/browse]
actors: [A]
flows: []
xcuitest: [apps/mobile/MonacoUITests/Journeys/StocksAssetDetailJourney.swift, apps/mobile/MonacoUITests/Journeys/StocksAssetDetailJourneyUITests.swift]
---

# Look at a stock

A signed-in member opens a stock from the Stocks tab and reads its screen: the hero with price and session, the chart with its range chips, the Stats card with the 52-week bar, and the Propose buy button. On a Pre-IPO token they also read the private-market reference, About and "Also available from". When one of their cabals holds the stock, they see "Your cabals' position" and Propose sell. The spec is [Stocks: Asset screen](../../screens.md#stocks-tab). The old app's screen is `apps/mobile/Monaco/Features/Assets/AssetDetailView.swift` at `c838bd24`, which every step cites as "old app".

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Everything [auth/sign-in](../auth/sign-in.md) needs, and the Stocks tab [stocks/browse](browse.md) opens |
| P2 | Before each scenario, `scripts/qa/journey.py` runs `apps/mobile/qa/journeys/stocks/asset-detail.setup.sh` with the scenario id. It upserts the catalogue rows of [stocks/browse](browse.md) (P3 there) and adds `JRNYQx`, a second `pre_ipo` listing of "Journey Private" from issuer `tessera`, with the same `company_key` as `JRNYPx`. Each row gets two `price_points` samples, stamped one day ago and now. No step taps to create them |
| P3 | `JRNYAx` is `issuer_tradable`, so its Propose buy is enabled |
| P4 | S4 needs a cabal of A's that holds `JRNYAx`. A treasury position comes only from a confirmed trade, and the trade path is #2136's, so the setup script seeds no holding and S4 runs against a member with none |
| P5 | The run never calls the live Jupiter API. The steps read only what `GET /v1/assets/{symbol}` and its chart serve from the seeded rows |

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
| S1.6 | wait | `asset-detail-propose-buy` | | `asset-detail-propose-buy` reads "Propose buy", is enabled, and "Your cabal votes before anything is bought" shows under it. Old app: the pinned Propose buy CTA. screens.md: Pinned CTA "Propose buy" with the caption "Your cabal votes before anything is bought" |
| S1.7 | tap | `asset-detail-propose-buy` | | Within 10 s, the propose screen for JRNYA shows, with "Propose buy" in its navigation bar. Old app: Propose buy opened the cabal picker, then the buy form. screens.md: Propose (#613) |

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
| S3.2 | scroll to | "Private-market reference" | | Within 10 s, "Private-market reference" shows with the premium or discount. Old app: the Pre-IPO block. screens.md: Pre-IPO adds "Private-market reference" with the premium or discount |
| S3.3 | scroll to | "About Journey Private" | | Within 10 s, "About Journey Private" shows. Old app: the Pre-IPO About block. screens.md: "About <token>" |
| S3.4 | scroll to | `asset-other-listings` | | Within 10 s, "Also available from" shows with `asset-other-listing-JRNYQx`. Old app: "Also available from" issuers. screens.md: "Also available from" issuers |

### S4 Your cabals' position and Propose sell

Starts signed in (auth/sign-in), with a cabal of A's holding `JRNYAx` (P4).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S4.1 | tap, then tap | the Stocks tab `tab-assets`, then `assets-row-JRNYAx` | | Within 15 s, `asset-detail-root` shows |
| S4.2 | scroll to | "Your cabals' position" | | Within 10 s, "Your cabals' position" shows one `asset-position-holding-…` row per cabal that holds it. Old app: the position card. screens.md: When one of the viewer's cabals holds it: "Your cabals' position" with one row per cabal |
| S4.3 | tap | `asset-detail-sell` | | Within 10 s, the propose screen opens with "Propose sell". Old app: Propose sell from the position card. screens.md: plus "Propose sell" when a cabal holds it |

## Ground truth

Looking at a stock writes nothing, and S1.7 only opens the propose screen. After a run, `apps/mobile/qa/journeys/stocks/asset-detail.truth.sh` checks that the seeded rows still hold the values P2 gives, and that no proposal names a seeded asset.

## Known failures on staging

| Step | Why | Blocking ticket |
| --- | --- | --- |
| S1.1, S2.1, S3.1, S4.1 | Every scenario reaches the asset screen through the Stocks tab, which is blank on staging (see [stocks/browse](browse.md)). Every later step of the scenario fails behind it | #577 |
| S1.7 | Propose buy opens `NotMigratedView`, not the propose screen | #613 |
| S2.2, S2.3 | The asset screen dropped the Stats card and the 52-week bar | #577 |
| S3.2, S3.3, S3.4 | The asset screen dropped the Pre-IPO block, About and "Also available from" | #577 |
| S4.2, S4.3 | The asset screen dropped "Your cabals' position" and Propose sell, and no path seeds a holding | #577, #2136 |

## Not covered

- The market session chip, `asset-detail-session` ("Market closed"). It shows by wall-clock time. `MarketSessionCopyTests` covers the copy.
- Scrubbing the chart. A drag reads out a price at a point that moves with the seed's timestamps. `AssetChartSeriesTests` covers the nearest-point math.
- "Price history builds up over time." and "The curve draws as the token trades." for a short history. The seed gives every row two samples, which draws a curve. `AssetDetailClientModelTests` covers the short-history rule.
- "Can't buy right now" on an untradable asset. The sample harness `-MonacoAssetDetailSample untradable` shows it.
- The live price tick flash. It needs a new sample mid-scenario.
- Targets with no accessibility identifier: "Stats" and "52-week range" (S2), "Private-market reference" and "About <token>" (S3) and "Your cabals' position" (S4) have no view in `apps/mobile/Monaco/Features/Assets/AssetDetailClientView.swift` on staging. The steps target their copy until #577 adds the views and their identifiers.
