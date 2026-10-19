---
id: stocks/watchlist
title: Manage the watchlist
version: 1
milestone: UI v3
requires: [auth/sign-in]
actors: [A]
flows: []
xcuitest: [apps/mobile/MonacoUITests/Journeys/StocksWatchlistJourney.swift, apps/mobile/MonacoUITests/Journeys/StocksWatchlistJourneyUITests.swift]
---

# Manage the watchlist

A signed-in member opens Manage under Watchlist on the Markets tab, stars a stock, goes back and sees it in their watchlist, then takes it off from the stock screen's star. The list lives on the device only, so the journey asserts what the screens show and nothing on the server. The spec is [Markets tab](../../screens.md#markets-tab), [Watchlist](../../screens.md#watchlist) and the star in [Stock screen](../../screens.md#stock-screen).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Everything [auth/sign-in](../auth/sign-in.md) needs. The app is freshly installed for the run, so A's watchlist starts empty |
| P2 | Before each scenario, `scripts/qa/journey.py` runs `apps/mobile/qa/journeys/stocks/watchlist.setup.sh`, which seeds the same catalogue as [stocks/browse](browse.md) P2 and P3. `JRNYAx` is an `equity` named "Journey Alpha" with the ticker "JRNYA" |

## Scenarios

### S1 Star a stock from Manage

Starts signed in (auth/sign-in).

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap | the Markets tab, `tab-assets` | | Within 15 s, `assets-root` shows and `markets-watchlist-add` shows in the empty Watchlist card "Your market, at a glance". screens.md: with no stars the card "Your market, at a glance" / "Add stocks to your watchlist." with "Add" |
| S1.2 | A | tap | `markets-watchlist-manage` | | Within 10 s, `watchlist-root` shows under the title "Your watchlist", and `watchlist-count` reads "No stocks selected". screens.md: "Manage" opens `WatchlistRoute`; the count line is "No stocks selected" |
| S1.3 | A | type | `monaco-search-field` | `JRNYA` | Within 15 s, `watchlist-row-JRNYAx` shows reading "Journey Alpha" and "JRNYA", with `watchlist-star-JRNYAx` labelled "Add Journey Alpha". screens.md: the search sends `q`; an unstarred row has an outline star labelled "Add <name>" |
| S1.4 | A | tap | `watchlist-star-JRNYAx` | | Within 5 s, `watchlist-star-JRNYAx` is labelled "Remove Journey Alpha" and `watchlist-count` reads "1 stock selected". screens.md: the star toggles the stock at once; a starred stock is labelled "Remove <name>" |
| S1.5 | A | tap | the navigation bar's back button | | Within 10 s, `markets-watchlist-row-JRNYAx` shows reading "JRNYA", and `markets-watchlist-add` is gone. screens.md: the first three watched stocks show as rows |

### S2 Take it off from the stock screen

Starts where S1 left off: on the Markets tab with `JRNYAx` in the Watchlist.

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap | `markets-watchlist-row-JRNYAx` | | Within 15 s, `asset-detail-root` shows, `asset-detail-name` reads "Journey Alpha" and `asset-watch-star` is labelled "Remove from watchlist". screens.md: the star reads "Add to watchlist" or "Remove from watchlist" |
| S2.2 | A | tap | `asset-watch-star` | | Within 5 s, `asset-watch-star` is labelled "Add to watchlist" and no toast shows. screens.md: the star adds or removes at once, with no toast |
| S2.3 | A | tap | the navigation bar's back button | | Within 10 s, `markets-watchlist-add` shows again and `markets-watchlist-row-JRNYAx` is gone |

## Ground truth

The watchlist is on the device and nothing is sent to the server. `apps/mobile/qa/journeys/stocks/watchlist.truth.sh` runs the shared [stocks/browse](browse.md) check: the seeded rows still hold their values, no proposal names one, and the seeded rows are then removed.

## Known failures on staging

None known.

## Not covered

- The 50-star cap and its toast "Your watchlist holds up to 50 stocks.". It needs 51 catalogue rows. `WatchlistModelTests` in `packages/mobile-core` covers the cap.
- The watchlist staying after the app is closed. A journey scenario is one app session. `WatchlistModelTests` covers the store.
- A second account on the device not seeing the first one's list. It needs a second sign-in, and `WatchlistModelTests` covers the per-user key.
- The empty-watchlist and no-match states of the Manage screen. `StocksTabSampleUITests` covers them against the sample harness.
