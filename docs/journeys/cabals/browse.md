---
id: cabals/browse
title: Browse and search cabals
version: 9
milestone: M10
requires: [auth/sign-in]
actors: [A]
flows: [03]
xcuitest: [apps/mobile/MonacoUITests/Journeys/CabalsBrowseJourney.swift, apps/mobile/MonacoUITests/Journeys/CabalsBrowseJourneyUITests.swift]
---

# Browse and search cabals

A member opens the Cabals tab, sees their cabals as app-icon tiles beside a "New cabal" tile, opens one, then searches public cabals through "Find a cabal" behind the "+" menu and asks to join one from the search rows. A second scenario presses and holds a tile and reads the actions menu. The rules are [Cabals](../../architecture/cabals.md#join-and-access-requests). The frames are `03-cabals-tab` and `03-cabals-tab-menu` in `docs/design/f/screens/`.

The Cabals tab in F has no search field and no cabal leaderboard. Public search sits behind the "+" round (`cabals-new-button`): its menu holds "Start a cabal", "Join with a link" and "Find a cabal" (`cabals-menu-find`, founder decision Q7, #6522). Leaderboard C's Cabals tab ([ranking/leaderboards](../ranking/leaderboards.md), #6577) covers the ranking that used to sit here. A step that says "Find a cabal" taps `cabals-new-button`, then `cabals-menu-find`, and then types into the search field.

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actor A has signed in once (`auth/sign-in`), and its `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | `apps/mobile/qa/journeys/cabals/browse.setup.sh` ran right before the scenario. It marks A as done with onboarding. A creates `QA mine {QA.run}`. A new dev user "QA host" creates the cabal `QA ask {QA.run}` |

## Scenarios

### S1 Tiles, search and request

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap | the Cabals tab | | `cabals-grid` has the "Your cabals" header with a count, a tile `cabals-tile-<id>` labelled `QA mine {QA.run}`, and the `cabals-new-tile` "New cabal" tile within 15 s |
| S1.2 | A | tap | the `cabals-tile-<id>` for `QA mine {QA.run}` | | `cabal-header-name` reads `QA mine {QA.run}` within 15 s |
| S1.3 | A | wait for the toast to close, go back, open Find a cabal, then type | the search field | `QA ask {QA.run}` | One `cabals-search-result-<id>` names `QA ask {QA.run}` with "1 member", and its `cabals-search-enter-<id>` reads "Request" and is labelled "Request to join QA ask {QA.run}" within 10 s |
| S1.4 | A | tap | the `cabals-search-enter-<id>` | | The toast "Request sent. You'll be in once the creator says yes." shows within 10 s, and the row shows `cabals-search-requested` "Request pending", with the notice "Your request to join QA ask {QA.run} is pending." above the list |

### S2 Long press a tile

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | go back from Find a cabal, tap, then press and hold | `BackButton`, the Cabals tab, then the `cabals-tile-<id>` for `QA mine {QA.run}` | | The menu shows "Fund" (`cabals-menu-fund`), "Propose buy", "Invite" and "Share" within 10 s, and no "Vote on …" item, since the cabal has no open vote |
| S2.2 | A | tap, then tap | "Fund", then `amount-keypad-close` | | `amount-keypad-screen` shows within 10 s; after closing it the Cabals tab shows again with `cabals-grid` |

## Ground truth

`apps/mobile/qa/journeys/cabals/browse.truth.sh` reads the run's cabals. After S1, A has a pending request on `QA ask {QA.run}`.

## Known failures on staging

None known.

## Not covered

- The request and unread counts on a card. The card no longer draws them and reads them out in its VoiceOver value. `cabals/approve-request` covers the requests themselves on the cabal screen, and chat waits on #623.
- The vote-needed badge and the "Vote on …" menu item, which need an open vote. `vote/*` journeys cover the vote.
- The "Invite" and "Share" menu items beyond their presence: [cabals/invite](invite.md) covers the invite sheet.
- Dragging a tile to reorder it.
- The "Invited" tile: [cabals/invite](invite.md) covers it.
- The Cabal leaderboard: gone from the tab, covered by [ranking/leaderboards](../ranking/leaderboards.md) (#6577).
- "Start a cabal": `cabals/create-cabal` covers the "+" and the "New cabal" tile.
