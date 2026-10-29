---
id: ranking/leaderboards
title: Leaderboard C
version: 8
milestone: F launch (phase 1)
requires: [auth/sign-in]
actors: [A]
flows: []
xcuitest: [apps/mobile/MonacoUITests/Journeys/RankingLeaderboardsJourney.swift, apps/mobile/MonacoUITests/Journeys/RankingLeaderboardsJourneyUITests.swift]
---

# Leaderboard C

Monaco ranks right calls, never dollars. Home has one leaderboard row ("Christelle leads · 92%", "This month · you are #8 of 24") that opens the Leaderboard page: the periods Week, Month and All time, the tabs People, Cabals and Friends, the You card, "Top three", "Around you", and the members still under the five-vote floor. A member needs at least 5 resolved votes in the period to rank, else the row reads "n more to rank". The design frames are `13-leaderboard-C-people`, `-C-cabals`, `-C` (Friends), `-C-unranked` and `02-home-dark`.

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actor A has signed in once (`auth/sign-in`), and its `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv`. Only A drives the app |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | `apps/mobile/qa/journeys/ranking/leaderboards.setup.sh` ran right before S1. It marks A as done with onboarding, then runs `qa_flow_seed 19a ok` (`F19aResolveCallsOK`, which seeds eight voters with 5, 8, 7, 6, 5, 5, 5 and 2 resolved votes this month under fresh users). The seed does not use A, so the setup moves the votes of the third voter (7 resolved, 4 right, 57%, below the top three) to A and hands the test A's id and the id of the voter with two resolved votes. S2 and S3 run with no setup of their own. The S1 setup also ends every live follow by A (`DELETE /v1/users/{id}/follow`), so A follows nobody and the Friends tab holds A alone |

## Scenarios

### S1 Home's row opens the Leaderboard

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap, then tap | the Home tab, then `home-leaderboard-row` | | The row reads "… leads · …%" and "This month · you are #n of m" within 15 s, and tapping it opens `leaderboard-page` titled "Leaderboard" within 15 s |
| S1.2 | A | read | `leaderboard-period-week`, `-month`, `-all`, `leaderboard-tab-people`, `-cabals`, `-friends` | | "Month" is the selected period and People is the selected tab (`13-leaderboard-C-people`) |
| S1.3 | A | read | `leaderboard-you-card`, "Top three", "Around you" | | The You card reads "#n of m people". "Top three" shows three `leaderboard-podium-<rank>` places. "Around you" lists `leaderboard-row-<A id>` reading a rate with "percent" and "n of m right" |
| S1.4 | A | read | `leaderboard-section-unranked`, `leaderboard-row-<unranked id>` | | A is ranked, so People shows no `leaderboard-section-unranked` and no row for the voter with two resolved votes, who is under the five-vote floor. The list shows only to an unranked viewer (#6970) |

### S2 The Cabals tab

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap, tap, then tap | the Home tab, `home-leaderboard-row`, then `leaderboard-tab-cabals` | | The tab is selected and at least one cabal row (`leaderboard-row-<id>` or `leaderboard-podium-<rank>`) reads a rate with "percent" and "n of m right" within 15 s (`13-leaderboard-C-cabals`) |

### S3 The Friends tab with no follows holds only A

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S3.1 | A | tap, tap, then tap | the Home tab, `home-leaderboard-row`, then `leaderboard-tab-friends` | | Within 15 s the tab shows a ranked place (`leaderboard-podium-<rank>` or `leaderboard-row-<id>`), every place reads "you" in its spoken label (so the only one is A), and `leaderboard-empty` never shows. The Friends board always includes the viewer: `loadFriends` in `apps/backend/internal/modules/ranking/app/read_call_board.go` appends the viewer to the followed ids (`13-leaderboard-C`) |

## Ground truth

`apps/mobile/qa/journeys/ranking/leaderboards.truth.sh` reads `call_voters`. After the run, A still has seven vote rows and the unranked voter two, so reading the boards wrote and lost nothing.

## Known failures on staging

None known. Targets this journey does not read because the app has no identifier for them: the ⓘ round (`leaderboard-info` exists but no step opens it) and the climb bar and the crown on #1 (hidden from accessibility; S1.3 counts the three podium places instead).

## Not covered

- The Friends tab with follows. That needs a seeded follow and belongs to `social/follow-and-boards`.
- The "Not ranked yet" list and card. They show only to an unranked viewer (a seed where A is under the floor), then the viewer and people they follow.
- The Week and All time periods' contents. The seed resolves its votes across the month, so only Month is stable.
- The leaderboard moment, the ⓘ sheet and the docked row. Unit tests cover them.
- Ranks that change after a trade. A trade needs a funded cabal, which belongs to the trade journeys.
