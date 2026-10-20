---
id: ranking/leaderboards
title: The leaderboards
version: 6
milestone: M16
requires: [auth/sign-in]
actors: [A]
flows: []
xcuitest: [apps/mobile/MonacoUITests/Journeys/RankingLeaderboardsJourney.swift, apps/mobile/MonacoUITests/Journeys/RankingLeaderboardsJourneyUITests.swift]
---

# The leaderboards

Monaco ranks people and cabals by return. Home has a "Leaderboard" of cabals or people, and the Cabals tab has the "Cabal leaderboard". A row opens that person's profile. The copy is in [screens.md](../../screens.md#home) (Home slot 6) and the [Leaderboard](../../screens.md#leaderboard-leaderboardroute) page, `CabalMemberBoardSlot` on the [cabal screen](../../screens.md#cabal-screen-cabalroute) and `CabalsBoardSlot` on the [Cabals tab](../../screens.md#cabals-tab).

Old app (`c838bd24`): `Home/HomeLeaderboardSection.swift` (Top investors, "Everyone" / "Friends", range chips), a row opening the profile with the cabals you share (now `UserProfileRoute`), the member board on `GroupDetailView` opening a profile, and Top cabals on the Cabals tab.

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A, B and C have signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv`. Only A drives the app |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | `apps/mobile/qa/journeys/ranking/leaderboards.setup.sh` ran right before the scenario. It marks A, B and C as done with onboarding and sets their display names. A creates the cabal `QA ranks {QA.run}` and B and C join it through the API, the same shape as testkit `cabal-with-members` (one creator, two open joiners). The setup hands the cabal id, B's user id and B's display name to the test |

## Scenarios

### S1 The Home leaderboard

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap, then scroll | the Home tab, then "Leaderboard" | | The header "Leaderboard" with "View all" and `home-people-board` show within 15 s (screens.md Home slot 6: "Leaderboard" and "View all") |
| S1.2 | A | read | `home-leaderboard-category`, `home-leaderboard-period` | | The category select reads "Cabals", the period select reads "1M" and the line "Cabal P&L · 1M" shows within 10 s (screens.md: a Cabals / People menu select and a 1W / 1M / All menu select, default Cabals, 1M) |
| S1.3 | A | tap | `home-leaderboard-category`, then "People" | | The select reads "People" and the line "Personal P&L · 1M" shows within 10 s (screens.md: the line "Cabal P&L · 1M" or "Personal P&L · 1M") |
| S1.4 | A | read | `home-people-board` | | Ranked `home-leaderboard-row-<id>` rows show, or the empty copy `home-leaderboard-empty` "No investors yet" does, within 15 s, and `home-leaderboard-loading` is gone (screens.md: the top three `BoardRow`s; empty: "No investors yet" / "Fund a cabal to get on the board.") |

### S3 Cabal leaderboard on the Cabals tab

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S3.1 | A | tap | the Cabals tab | | `cabals-board` shows "Cabal leaderboard" and the context line "Cabal P&L · 1M" within 15 s (screens.md `CabalsBoardSlot`; old app: Cabals tab Top cabals) |
| S3.2 | A | read | `cabals-board` | | Ranked rows or the empty copy "No public rankings for this period." replace `cabals-board-coming` "Rankings show up here soon." within 10 s (screens.md `CabalsBoardSlot`: rows with rank, tile, name and P&L; old app: Cabals tab Top cabals rows) |

### S4 View all opens the Leaderboard

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S4.1 | A | tap | the Home tab, then "View all" under "Leaderboard" | | `leaderboard-page` shows within 15 s, with the inline title "Leaderboard" (screens.md Leaderboard: the standalone page; Home's "View all" opens it) |
| S4.2 | A | read | `leaderboard-category`, `leaderboard-period` | | The category menu and the period menu read what Home's Leaderboard card read when "View all" was tapped (`home-leaderboard-category`, `home-leaderboard-period`) within 10 s, and ranked `leaderboard-row-<id>` rows show, or `leaderboard-empty` "No eligible rankings" or `leaderboard-stale` "Rankings paused while valuations update." does (screens.md Leaderboard: tools, rows and the empty state) |

## Ground truth

`apps/mobile/qa/journeys/ranking/leaderboards.truth.sh` reads `cabal_members`. `QA ranks {QA.run}` has three members, A as creator and B and C as members, so reading the boards and opening a profile wrote nothing.

## Known failures on staging

None.

## Not covered

- A cabal's "Member standings" (the former S2). The board is hidden while the cabal holds no position, and the journey's cabal is unfunded, so a run cannot show it without moving money.
- The rank-change glyph and "Rank change" on the boards. The backend serves no rank movement.
- The leaderboard filter sheet (sort, membership and minimum members) is covered by unit tests, not by this journey. The 3M period is missing because the API has no 3M range yet.
- Ranks that change after a trade. That needs a funded cabal and a confirmed trade (`funds:`), which belongs to the trade journeys.
- A ranked People row for B. Personal ranking needs funded positions, so S1.4 accepts the empty copy "No investors yet".
