---
id: cabals/browse
title: Browse and search cabals
version: 3
milestone: M10
requires: [auth/sign-in]
actors: [A]
flows: [03]
xcuitest: [apps/mobile/MonacoUITests/Journeys/CabalsBrowseJourney.swift, apps/mobile/MonacoUITests/Journeys/CabalsBrowseJourneyUITests.swift]
---

# Browse and search cabals

A member opens the Cabals tab, sees their cabals as cards, opens one, then searches by name and asks to join a cabal from the search rows. The tab's return chart and Top cabals board are checked in their own scenarios, since both wait on backend routes. The rules are [Cabals](../../architecture/cabals.md#join-and-access-requests).

Old app (`c838bd24`): the Cabals tab (`Groups/CabalsTabView.swift`): "Your cabals" cards (`CabalsStripSection.swift`) -> cabal; "Find a cabal by name" -> results (`CabalsSearchResultsSection.swift`) -> Join or Request; "Top cabals" (`CabalsLeaderboardSection.swift`); the return chart (`CabalsPnLChartSection.swift`). Spec: the Cabals tab, `CabalsListSlot`, `CabalsJoinSlot`, `CabalsValueChartSlot` and `CabalsBoardSlot` in [screens.md](../../screens.md#cabals-tab).

The format of this doc is in [App journeys](../README.md).

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
| S1.1 | A | tap | the Cabals tab | | `cabals-search-field` reads "Find a cabal by name", `cabals-list` has the header "Your cabals" with a `cabals-list-card-<id>` naming `QA mine {QA.run}` within 15 s |
| S1.2 | A | tap | the `cabals-list-card-<id>` for `QA mine {QA.run}` | | `cabal-header-name` reads `QA mine {QA.run}` within 15 s |
| S1.3 | A | wait for the toast to close, clear the field, then type | `cabals-search-field` | `QA ask {QA.run}` | One `cabals-search-result-<id>` names `QA ask {QA.run}` with "1 member · By request", and its `cabals-search-enter-<id>` is labelled "Ask to join QA ask {QA.run}" within 10 s |
| S1.4 | A | tap | the `cabals-search-enter-<id>` | | The toast "Request sent. You'll be in once the creator says yes." shows within 10 s, and the row shows `cabals-search-requested` "Request sent" |

### S2 Your cabals' return

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap | the Cabals tab | | `cabals-value-chart` has the header "Your cabals' return" and draws a line for `QA mine {QA.run}`, with no `cabals-value-chart-coming`, within 15 s |

### S3 Top cabals

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S3.1 | A | tap, then scroll to | the Cabals tab, then `cabals-board` | | `cabals-board` reads "Top cabals" / "Ranked by return across everyone on Monaco" and lists ranked rows, or the empty state "No cabal has put money in yet", with no `cabals-board-coming`, within 15 s |

## Ground truth

`apps/mobile/qa/journeys/cabals/browse.truth.sh` reads the run's cabals. After S1, A has a pending request on `QA ask {QA.run}`.

## Known failures on staging

- S2.1: the return chart reads "Your cabals' return shows up here soon." (`cabals-value-chart-coming`). Blocked by #660.
- S3.1: Top cabals reads "Rankings show up here soon." (`cabals-board-coming`). Blocked by #699 and #617.

## Not covered

- The request badge (`cabals-list-card-requests`) and chat's unread badge on a card. `cabals/approve-request` covers the requests themselves on the cabal screen, and chat waits on #623.
- The "+ New cabal" card (`cabals-list-new`). It is the last card of a lazy strip, and the QA account collects a cabal every run, so the card is out of reach (`apps/mobile/Monaco/Features/Groups/CabalsListSlot.swift`). `cabals/create-cabal` covers the New cabal sheet.
