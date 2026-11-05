---
id: home/dashboard
title: The Home dashboard
version: 13
rpc: fake-trade
milestone: M16
requires: [auth/sign-in]
actors: [A, C]
flows: []
xcuitest: [apps/mobile/MonacoUITests/Journeys/HomeDashboardJourney.swift, apps/mobile/MonacoUITests/Journeys/HomeDashboardJourneyUITests.swift]
---

# The Home dashboard

Home is the first tab a member sees. It shows the wordmark and the bell, their return in the 32 pt figure, one cash row, their cabals as rows and the Leaderboard. A member with no cabal gets a card that lets them start a cabal or join with a link. The slot order and copy are in [Home](../../screens.md#home).

Old app (`c838bd24`): `Home/HomeView.swift`. The net-worth hero with its gain chip, the P&L chart with range chips, the balance row, "Your cabals" with a row per cabal that opens the cabal, the empty state, pending votes, pull to refresh, and the avatar that opens Profile.

Frames: `docs/design/v5/screens/02-home-dark.png` and `docs/design/v5/screens/02-home-empty.png`.

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and C have signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | `apps/mobile/qa/journeys/home/dashboard.setup.sh` ran right before the scenario. It marks the actor as done with onboarding and sets their display name. For S1, the setup reads A's portfolio (`GET /v1/me/portfolio`) and hands the test A's largest funded cabal's id and name. When A holds none, the setup seeds the journey catalogue and gets a holding through the product path on the fakes with `qa_fake_holding A JRNYAx 25 "QA home {QA.run}"` (`rpc: fake-trade`; it never writes `cabal_positions`), and hands that cabal over. For S2, C leaves every cabal C belongs to through the API, so C has none |

## Scenarios

### S1 A member with a cabal reads Home

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap | the Home tab | | `home-wordmark` and `home-notifications-button` (the bell) show, `home-welcome` reads "Welcome, Alfred" (A's first name from `accounts.tsv`, which the setup writes as A's display name) above `home-portfolio-total`, which shows a dollar total, and A's return line `home-portfolio-chip` shows and its spoken label ends "past month" within 15 s (F `02-home-dark`, #6496, #7871) |
| S1.2 | A | read | the money strip | | `home-cash-row` reads "Cash" and "USDC" with no APY text and no `home-apy-info` button, and a dollar amount (`platform-balance-value`), and `home-add-money-link` reads "Deposit", within 10 s (F `02-home-dark`, founder decision Q19) |
| S1.3 | A | scroll to | "Your cabals" | | The header "Your cabals" and `cabal-row-<id>` for `cabalName` show within 15 s, whether or not A has funded it, each row reading the share in dollars or "Not funded", with no sideways rail (F `02-home-dark`) |
| S1.4 | A | pull down | the Home screen | | After the refresh, "Your cabals" and the card for `cabalName` are still there within 15 s (screens.md Home: “Pull to refresh.”; old app: `HomeView` `.refreshable`) |
| S1.5 | A | tap | `cabal-row-<id>` | | The cabal opens on Chat: `cabal-inline-title` reads `cabalName` and `chat-view` shows within 15 s (screens.md `HomeCabalsSlot`: A card opens `CabalRoute`; old app: `HomeView` cabal row opens `GroupDetailView`) |
| S1.6 | A | tap | the Home tab, then `home-profile-button` | | The Profile tab is selected and `profile-card` shows within 15 s (screens.md Home: the viewer's avatar, which opens the Profile tab; old app: `HomeView` avatar opens Profile) |
| S1.7 | A | tap, then read | the Home tab, then the chart | | The plain range labels `home-portfolio-range-<range>` (1H 1D 1W 1M 3M 6M All) show under the chart with 1M selected within 10 s (F `02-home-dark`) |

### S2 A member with no cabal is offered a new one

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | C | tap | the Home tab | | `home-cabals-empty` shows "There is no place like Monaco." and "Trade with your friends.", with `home-first-use-start` "Start a cabal" and `home-first-use-join` "Join with a link" side by side, and the feed preview headed "Public cabals", within 15 s (F `02-home-empty`, #7872; screens.md `HomeFirstUseSlot`; old app: `HomeView` empty state) |
| S2.2 | C | tap | "Start a cabal" | | The create cabal screen shows `create-group-name` within 10 s (screens.md `HomeFirstUseSlot`; old app: `HomeView` “Browse cabals”) |

## Ground truth

`apps/mobile/qa/journeys/home/dashboard.truth.sh` reads `cabals` and `cabal_members`. `QA home {QA.run}` either does not exist (A held a cabal) or has A as its only member, so reading Home and pulling to refresh wrote nothing. C belongs to no cabal.

## Not covered

- The hero chart and its period chips (`home-pnl-chart`, `home-portfolio-range-<range>`). They show only for an account with money in a cabal; S1's cabal is unfunded, and A's other cabals are not a fixed state. Without money the hero shows "$0.00" and "No positions yet" with a quiet chart. The funded-cabal journeys move real USDC.
- The bell (`home-notifications-button`) is [home/activity](activity.md). "Withdraw" is no longer on Home: it is a row on Profile and a tile on Your money.
- The Leaderboard slot with its Cabals / People and 1W / 1M / All menus and "View all". [ranking/leaderboards](../ranking/leaderboards.md) covers it, and [social/follow-and-boards](../social/follow-and-boards.md) uses the People board.
- "Proposals" (`HomePendingVotesSlot`). A pending vote needs an open proposal, and a proposal needs a funded treasury. The proposal journeys cover it with a `funds:` block.
- The pot value and share on a Home card. The cabal pot journey (`cabals/pot`) covers the same numbers on the cabal screen.
- The onboarding nudge (`OnboardingNudgeBanner`). `profile/nudge` and `onboarding/first-run` cover it.
- "$50.00 pending" under the money strip. That needs money moving; the fund journey covers it.
