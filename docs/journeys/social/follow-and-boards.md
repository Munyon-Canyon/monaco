---
id: social/follow-and-boards
title: Follow and the boards
version: 4
milestone: F launch (phase 1)
requires: [auth/sign-in]
actors: [A, B]
flows: [20, 19]
xcuitest: [apps/mobile/MonacoUITests/Journeys/FollowAndBoardsJourney.swift, apps/mobile/MonacoUITests/Journeys/FollowAndBoardsJourneyUITests.swift]
---

# Follow and the boards

A member finds another member on the Leaderboard's People tab (opened from Home's leaderboard row), opens their profile and follows them, and sees them under Following on Find friends. The followed member opens the Profile tab, reads the new follower and finds them in the follower list. The first member then unfollows, and Following goes empty again. The design is Leaderboard C (`13-leaderboard-C-people`), `10-profile-other`, the user profile (#620), `ProfileFollowCountsSlot` on the [Profile tab](../../screens.md#profile-tab) and the Following list on Find friends.

The format of this doc is in [App journeys](../README.md). It covers the path across screens that [ranking/leaderboards](../ranking/leaderboards.md), [profile/user-profile](../profile/user-profile.md) and [profile/follow-lists](../profile/follow-lists.md) each cover one part of.

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and B have each signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv`. `apps/mobile/qa/journeys/social/follow-and-boards.setup.sh` resolves both user ids from it. It marks A and B as done with onboarding, sets their display names ("Alfred" and "Bartholomez") and runs `qa_flow_seed 19a ok` (`F19aResolveCallsOK` in `testkit/flows/f19a.go`, which seeds eight voters with 5, 8, 7, 6, 5, 5, 5 and 2 resolved votes this month under fresh users and does not use A or B). The setup moves the votes of the third voter (7 resolved) to A and of the seventh (5 resolved) to B, so both rank with at least five resolved votes and neither sits in "Top three" (a podium place has no `leaderboard-row-<id>` identifier). The setup runs before every run; each run adds its own seeded voters |
| P2 | A follows nobody, B follows nobody and B has no followers. Before S1, the setup script ends every live follow by A or B and every live follow of B through `DELETE /v1/users/{id}/follow`, and hands the truth check `followRowsBefore`, the count of A-to-B rows in `follows` |
| P3 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |

## Scenarios

### S1 Both rank on the Leaderboard

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap, tap, then tap | the Home tab, `home-leaderboard-row`, then `leaderboard-tab-people` | | `leaderboard-page` shows People, and `leaderboard-row-<A id>` and `leaderboard-row-<B id>` each read a rate with "percent" and no "not ranked" within 30 s (`13-leaderboard-C-people`: "Around you" rows) |

### S2 Following with no follows

Starts where S1 ended.

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap, tap, then tap | Settings from the Profile tab, `profile-find-friends`, then `friends-tab-following` | | `friends-following-empty` reads "You're not following anyone yet" within 10 s (screens.md Find friends: "'You're not following anyone yet'") |

### S3 Follow from the board

Starts where S2 ended.

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S3.1 | A | tap, tap, tap, then tap | the Home tab, `home-leaderboard-row`, `leaderboard-tab-people`, then `leaderboard-row-<B id>` | | `user-profile-card` shows within 15 s with `user-profile-name` reading "Bartholomez", `user-profile-handle` starting with "@", `user-profile-followers` reading "0 followers", `user-profile-following` reading "0 following" and `user-profile-follow` reading "Follow" |
| S3.2 | A | tap | `user-profile-follow` | | `user-profile-follow` reads "Following" and `user-profile-followers` reads "1 follower" within 5 s, and `user-profile-following` still reads "0 following" |
| S3.3 | A | tap, tap, then tap | Settings from the Profile tab, `profile-find-friends`, then `friends-tab-following` | | `friends-following-row-<B id>` shows within 15 s reading "Bartholomez", and `friends-following-empty` is gone (screens.md Find friends: "Following lists the people the viewer follows") |

### S4 B sees the follower

Starts where S3 ended.

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S4.1 | B | tap | the Profile tab | | `profile-followers` reads "1 follower" and `profile-following` reads "0 following" within 15 s |
| S4.2 | B | tap, then tap | `profile-followers`, then `follow-list-open-<A id>` | | The "Followers" screen shows within 10 s with `follow-list-row-<A id>` and its `follow-list-follow-<A id>` button reading "Follow" within 15 s. Tapping A's row shows `user-profile-name` reading "Alfred" within 15 s |

### S5 Unfollow

Starts where S4 ended.

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S5.1 | A | tap, tap, tap, tap, then tap | the Home tab, `home-leaderboard-row`, `leaderboard-tab-people`, `leaderboard-row-<B id>`, then `user-profile-follow` | | B's profile shows `user-profile-follow` reading "Following" within 15 s. After the tap it reads "Follow" and `user-profile-followers` reads "0 followers" within 5 s, with `user-profile-following` still reading "0 following" |
| S5.2 | A | tap, tap, then tap | Settings from the Profile tab, `profile-find-friends`, then `friends-tab-following` | | `friends-following-empty` reads "You're not following anyone yet" within 10 s |

## Ground truth

`apps/mobile/qa/journeys/social/follow-and-boards.truth.sh` reads `follows` through `apps/mobile/qa/journeys/psql.sh`. After S3, `follows` has one live row from A to B. After S5, it has none. The setup script reads the first count when the test reaches S4 and hands it over as `liveAfterFollow`, so the check can still see the state S3 left. At the end, the check requires `liveAfterFollow` to be 1, no live row from A to B, and more A-to-B rows than `followRowsBefore`, which shows the follow and the unfollow both reached the backend.

## Known failures on staging

None known. B's profile (`user-profile-card`, #6530) is read here, not changed.

## Not covered

- The ranked order and the crown. F draws the crown on the podium only and the journey keeps A and B out of the top three, so it reads their rows, not their places.
- A Friends tab with the followed member. That needs the Friends board (#6499), which `ranking/leaderboards` reads empty. S2.1, S3.3 and S5.2 read the Following list on Find friends.
- The avatar on B's profile. `MonacoAvatar` is hidden from accessibility, so no identifier can reach it. S3.1 reads the name, the handle and the counts around it.
- The exact "@handle". It is `qa_<name>` plus a suffix when another user holds that handle, so it differs across databases. S3.1 requires an "@" and a name.
- A trade changing the ranks. A trade needs a funded treasury; `ranking/leaderboards` covers the boards.
- Following back from the follower list. S4.2 reads the "Follow" button and does not tap it, so B ends the journey following nobody.
