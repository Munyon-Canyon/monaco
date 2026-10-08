---
id: ranking/leaderboards
title: The leaderboards
version: 2
milestone: M16
requires: [auth/sign-in]
actors: [A]
flows: []
xcuitest: [apps/mobile/MonacoUITests/Journeys/RankingLeaderboardsJourney.swift, apps/mobile/MonacoUITests/Journeys/RankingLeaderboardsJourneyUITests.swift]
---

# The leaderboards

Monaco ranks people and cabals by return. Home has "Top investors" across everyone, each cabal has a "Leaderboard" of its members, and the Cabals tab has "Top cabals". A row opens that person's profile. The copy is in [screens.md](../../screens.md#home) (Home slot 6), `CabalMemberBoardSlot` on the [cabal screen](../../screens.md#cabal-screen-cabalroute) and `CabalsBoardSlot` on the [Cabals tab](../../screens.md#cabals-tab).

Old app (`c838bd24`): `Home/HomeLeaderboardSection.swift` (Top investors, "Everyone" / "Friends", range chips), a row opening the profile with the cabals you share (now `UserProfileRoute`), the member board on `GroupDetailView` opening a profile, and Top cabals on the Cabals tab.

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A, B and C have signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv`. Only A drives the app |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | `apps/mobile/qa/journeys/ranking/leaderboards.setup.sh` ran right before the scenario. It marks A, B and C as done with onboarding and sets their display names. A creates the cabal `QA ranks {QA.run}` and B and C join it through the API, the same shape as testkit `cabal-with-members` (one creator, two open joiners). The setup hands the cabal id, B's user id and B's display name to the test |

## Scenarios

### S1 Top investors on Home

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap, then scroll | the Home tab, then "Top investors" | | The header "Top investors" shows within 15 s (screens.md `HomePeopleBoardSlot`: ""Top investors""; old app: `HomeLeaderboardSection` header) |
| S1.2 | A | read | the range chips | | The chips "1H", "1D", "1W", "1M" and "All" show within 10 s (screens.md `HomePeopleBoardSlot`: "chips 1H 1D 1W 1M All (default All)"; old app: `HomeLeaderboardSection` range chips) |
| S1.3 | A | read | the board segment | | "Everyone" and "Friends" show within 10 s (screens.md `HomePeopleBoardSlot`: "the "Everyone" / "Friends" segment (Friends after #658)"; old app: `HomeLeaderboardSection` Everyone / Friends) |
| S1.4 | A | tap | `home-leaderboard-row-<B id>` | | A ranked row for B shows; tapping it opens B's profile with `user-profile-group-<cabal id>` for `QA ranks {QA.run}` within 15 s (screens.md `HomePeopleBoardSlot`: "ranked rows (crown for first, then numbers, avatar, name, return over gain or loss)"; old app: `HomeLeaderboardSection` row opens the profile, now `UserProfileRoute`) |

### S2 A cabal's member board

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `QA ranks {QA.run}` | `cabal-header-name` reads `QA ranks {QA.run}` within 15 s |
| S2.2 | A | scroll | "Leaderboard" | | The header "Leaderboard" and three `cabal-member-<user id>` rows show within 15 s, A's labelled "You" (screens.md `CabalMemberBoardSlot`: ""Leaderboard": ranked members with the viewer's row washed and labelled "You""; old app: `GroupDetailView` member board) |
| S2.3 | A | tap | `cabal-member-<B id>` | | `user-profile-name` reads B's display name within 15 s (screens.md `CabalMemberBoardSlot`: "Rows open `UserProfileRoute`"; old app: member row opens the profile) |
| S2.4 | A | go back, then read | the member board | | Each member row shows a return, and `cabal-member-board-coming` "Rankings show up here soon." is gone within 10 s (screens.md `CabalMemberBoardSlot`: "ranked members"; old app: `GroupDetailView` member board ranked by P&L) |

### S3 Top cabals on the Cabals tab

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S3.1 | A | tap | the Cabals tab | | `cabals-board` shows "Top cabals" and "Ranked by return across everyone on Monaco" within 15 s (screens.md `CabalsBoardSlot`: ""Top cabals" / "Ranked by return across everyone on Monaco""; old app: Cabals tab Top cabals) |
| S3.2 | A | read | `cabals-board` | | Ranked rows or the empty copy "No cabal has put money in yet" replace `cabals-board-coming` "Rankings show up here soon." within 10 s (screens.md `CabalsBoardSlot`: "Rows: rank, tile, name, return over pot value. Empty: "No cabal has put money in yet" / "The first one to fund takes the top spot.""; old app: Cabals tab Top cabals rows) |

## Ground truth

`apps/mobile/qa/journeys/ranking/leaderboards.truth.sh` reads `cabal_members`. `QA ranks {QA.run}` has three members, A as creator and B and C as members, so reading the boards and opening a profile wrote nothing.

## Known failures on staging

- S1.2: `HomePeopleBoardSlot` shows "Rankings show up here soon." with no range chips. Blocked by #617 and #619 (ranked boards with P&L) and #699 (the slot).
- S1.3: no "Everyone" / "Friends" segment. Blocked by #658.
- S1.4: no ranked rows, so no row to tap. Blocked by #617 and #619. Opening the cabals you share from the row is blocked by #699.
- S2.4: member rows carry no return and the board still shows "Rankings show up here soon.". Blocked by #617 and #619.
- S3.2: `CabalsBoardSlot` shows "Rankings show up here soon." with no rows and no empty copy. Blocked by #617 and #619.

## Not covered

- The crown for first place, numbered ranks and the viewer's row pinned at the bottom of Top investors. They need ranked rows (#617, #619) and identifiers on rank and pin that `apps/mobile/Monaco/Features/Home/HomePeopleBoardSlot.swift` does not have yet; this ticket does not touch app code.
- The range chips on the live board have no identifiers. S1.2 matches them by label. #699 should add `home-leaderboard-range-<range>` as the old `HomeLeaderboardSection` had.
- Ranks that change after a trade. That needs a funded cabal and a confirmed trade (`funds:`), which belongs to the trade journeys once #617 lands.
