---
id: home/dashboard
title: The Home dashboard
version: 1
milestone: M16
requires: [auth/sign-in]
actors: [A, C]
flows: []
xcuitest: [apps/mobile/MonacoUITests/Journeys/HomeDashboardJourney.swift, apps/mobile/MonacoUITests/Journeys/HomeDashboardJourneyUITests.swift]
---

# The Home dashboard

Home is the first tab a member sees. It shows what they have in cabals, their account balance, the votes waiting on them and their cabals. A member with no cabal gets a button that takes them to the Cabals tab. The slot order and copy are in [Home](../../screens.md#home).

Old app (`c838bd24`): `Home/HomeView.swift`. The net-worth hero with its gain chip, the P&L chart with range chips, the balance row, "Your cabals" with a row per cabal that opens the cabal, the empty "Browse cabals" state, pending votes, pull to refresh, and the avatar that opens Profile.

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and C have signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | `apps/mobile/qa/journeys/home/dashboard.setup.sh` ran right before the scenario. It marks the actor as done with onboarding and sets their display name. For S1, A creates the open cabal `QA home {QA.run}` through the API, and the setup hands its id to the test. For S2, C leaves every cabal C belongs to through the API, so C has none |

## Scenarios

### S1 A member with a cabal reads Home

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap | the Home tab | | The hero reads "Your money in cabals" within 15 s (screens.md `HomePortfolioSlot`: "The ink hero: "Your money in cabals""; old app: Home tab, `HomeView` net-worth hero) |
| S1.2 | A | read | the balance row | | "Account balance" shows with its amount `platform-balance-value`, and `home-add-money-link` "Add money" and `home-withdraw-link` "Withdraw" sit under it (screens.md `HomeBalanceSlot`: "A row with the coin glyph, "Account balance" and the amount", "two text buttons, "Add money" … and "Withdraw""; old app: `HomeView` balance row) |
| S1.3 | A | scroll | "Your cabals" | | The header "Your cabals" and `cabal-row-<id>` for `QA home {QA.run}` show within 15 s (screens.md `HomeCabalsSlot`: ""Your cabals". One row per cabal"; old app: `HomeView` Your cabals) |
| S1.4 | A | pull down | the Home screen | | After the refresh, "Your cabals" and the row for `QA home {QA.run}` are still there within 15 s (screens.md Home: "Pull to refresh."; old app: `HomeView` `.refreshable`) |
| S1.5 | A | tap | `cabal-row-<id>` | | `cabal-header-name` reads `QA home {QA.run}` within 15 s (screens.md `HomeCabalsSlot`: "Row opens `CabalRoute`"; old app: `HomeView` cabal row opens `GroupDetailView`) |
| S1.6 | A | tap | the Home tab, then `home-profile-button` | | The Profile tab is selected and `profile-header` shows within 15 s (screens.md Home: "the viewer's avatar at the top right opens the Profile tab"; old app: `HomeView` avatar opens Profile) |
| S1.7 | A | tap, then read | the Home tab, then the hero | | The hero shows a total in dollars and the chip's "all time" within 10 s (screens.md `HomePortfolioSlot`: "the total in `moneyHero`", "a chip "▲ $0.14 · 0.1%" and "all time""; old app: `HomeView` hero total and gain chip) |
| S1.8 | A | read | the hero chart | | `home-pnl-chart` and the range chip "1D" show within 10 s (screens.md `HomePortfolioSlot`: "an area chart of `GET /v1/me/pnl-history` with range chips 1H to All (default 1D)"; old app: `HomeView` P&L chart and range chips) |

### S2 A member with no cabal is sent to browse

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | C | tap | the Home tab | | `home-cabals-empty` shows "No cabals yet" and "Start one with friends or join an open one." within 15 s (screens.md `HomeCabalsSlot`: "Empty: "No cabals yet" / "Start one with friends or join an open one.""; old app: `HomeView` empty state) |
| S2.2 | C | tap | "Browse cabals" | | The Cabals tab is selected and `cabals-root` shows within 10 s (screens.md `HomeCabalsSlot`: "an outline button "Browse cabals" that selects the Cabals tab"; old app: `HomeView` "Browse cabals") |

## Ground truth

`apps/mobile/qa/journeys/home/dashboard.truth.sh` reads `cabals` and `cabal_members`. `QA home {QA.run}` exists with A as its only member, so reading Home and pulling to refresh wrote nothing. C belongs to no cabal.

## Known failures on staging

- S1.7: the hero shows "Your total shows up here soon." with no total and no chip. `HomePortfolioSlot` waits on `GET /v1/me/portfolio` and `GET /v1/me/pnl-history`. Blocked by #660.
- S1.8: no P&L chart and no range chips in the hero. Blocked by #660.

## Not covered

- The hero, chip, chart and range chips have no accessibility identifiers on the live `HomePortfolioSlot` (`apps/mobile/Monaco/Features/Home/HomePortfolioSlot.swift`). S1.7 and S1.8 match by label and by `home-pnl-chart` from the old `HomePnLChartSection`. #660 should add `home-portfolio-total`, `home-portfolio-chip` and `home-pnl-range-<range>`; this ticket does not touch app code.
- "Needs your vote" (`HomePendingVotesSlot`). A pending vote needs an open proposal, and a proposal needs a funded treasury. The proposal journeys cover it with a `funds:` block.
- The pot value and slice on a Home cabal row ("Pot $950.69"). Blocked by #660; the cabal pot journey (`cabals/pot`) covers the same numbers on the cabal screen.
- The onboarding nudge (`HomeNudgeSlot`). `onboarding/first-run` covers it.
- "$50.00 funding a cabal" under the balance. That needs money moving; the fund journey covers it.
