---
id: cabals/browse
title: Browse and search cabals
version: 5
milestone: M10
requires: [auth/sign-in]
actors: [A]
flows: [03]
xcuitest: [apps/mobile/MonacoUITests/Journeys/CabalsBrowseJourney.swift, apps/mobile/MonacoUITests/Journeys/CabalsBrowseJourneyUITests.swift]
---

# Browse and search cabals

A member opens the Cabals tab, sees their cabals as cards, opens one, then filters them by name, then searches public cabals on Discover cabals and asks to join one from the search rows. The tab's Cabal leaderboard is checked in its own scenario, since it waits on backend routes. The rules are [Cabals](../../architecture/cabals.md#join-and-access-requests).

Old app (`c838bd24`): the Cabals tab (`Groups/CabalsTabView.swift`): "Your cabals" cards (`CabalsStripSection.swift`) -> cabal; "Find a cabal by name" on the tab -> results (`CabalsSearchResultsSection.swift`) -> Join or Request; "Top cabals" (`CabalsLeaderboardSection.swift`); the return chart (`CabalsPnLChartSection.swift`). Spec: the Cabals tab, `CabalsListSlot`, `CabalsJoinSlot`, `CabalsBoardSlot` and Discover cabals in [screens.md](../../screens.md#cabals-tab).

The format of this doc is in [App journeys](../README.md).

Discover cabals is where `cabals-search-field` lives. A step that says "Discover cabals" reaches it from the Cabals tab by typing a name the actor has no cabal called, such as `zzqq`, in `cabals-filter-field` ("Find your cabals"), then tapping `cabals-search-public` ("Search public cabals"), which pushes Discover cabals with that text in `cabals-search-field`; an actor with no cabals taps "Explore cabals" instead. The step then replaces the text in `cabals-search-field`.

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actor A has signed in once (`auth/sign-in`), and its `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | `apps/mobile/qa/journeys/cabals/browse.setup.sh` ran right before the scenario. It marks A as done with onboarding. A creates `QA mine {QA.run}`. A new dev user "QA host" creates the cabal `QA ask {QA.run}` |

## Scenarios

### S1 Cards, search, join and request

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap | the Cabals tab | | `cabals-filter-field` reads "Find your cabals", `cabals-list` has the header "Your cabals" with a `cabals-list-card-<id>` naming `QA mine {QA.run}` within 15 s |
| S1.2 | A | tap | the `cabals-list-card-<id>` for `QA mine {QA.run}` | | `cabal-header-name` reads `QA mine {QA.run}` within 15 s |
| S1.3 | A | wait for the toast to close, go back, open Discover cabals, clear the field, then type | `cabals-search-field` | `QA ask {QA.run}` | One `cabals-search-result-<id>` names `QA ask {QA.run}` with "1 member · Approval required", and its `cabals-search-enter-<id>` reads "Request" and is labelled "Request to join QA ask {QA.run}" within 10 s |
| S1.4 | A | tap | the `cabals-search-enter-<id>` | | The toast "Request sent. You'll be in once the creator says yes." shows within 10 s, and the row shows `cabals-search-requested` "Request pending", with the notice "Your request to join QA ask {QA.run} is pending." above the list |

### S3 Cabal leaderboard

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S3.1 | A | tap, then scroll to | the Cabals tab, then `cabals-board` | | `cabals-board` reads "Cabal leaderboard" / "Public" and lists up to three ranked rows (`cabals-board-row-<id>`), or the empty state "No public rankings for this period." (`cabals-board-empty`), within 15 s |

## Ground truth

`apps/mobile/qa/journeys/cabals/browse.truth.sh` reads the run's cabals. After S1, A has a pending request on `QA ask {QA.run}`.

## Known failures on staging

None known.

## Not covered

- The request and unread counts on a card. The card no longer draws them and reads them out in its VoiceOver value. `cabals/approve-request` covers the requests themselves on the cabal screen, and chat waits on #623.
- The vote-needed badge, the leaderboard filter sheet (its selects are disabled until the API has filters) and "Load more rankings".
- The "+" (`cabals-new-button`) and the "Explore cabals" empty state. `cabals/create-cabal` covers the "+" into Start a cabal, and the QA account always has a cabal, so the empty state is out of reach.
