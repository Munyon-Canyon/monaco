---
id: social/follow-and-boards
title: Follow and the boards
version: 1
milestone: M17
requires: [auth/sign-in]
actors: [A, B]
flows: [20, 19]
xcuitest: [apps/mobile/MonacoUITests/Journeys/FollowAndBoardsJourney.swift, apps/mobile/MonacoUITests/Journeys/FollowAndBoardsJourneyUITests.swift]
---

# Follow and the boards

A member finds another member on Home's "Top investors", opens their profile, follows them, and sees them on the "Friends" board. The followed member opens the Profile tab, reads the new follower and finds them in the follower list. The first member then unfollows, and the Friends board goes empty again. The design is `HomePeopleBoardSlot` on [Home](../../screens.md#home) (#699, #658), the user profile (#620) and `ProfileFollowCountsSlot` on the [Profile tab](../../screens.md#profile-tab).

The format of this doc is in [App journeys](../README.md). It covers the path across screens that [ranking/leaderboards](../ranking/leaderboards.md), [profile/user-profile](../profile/user-profile.md) and [profile/follow-lists](../profile/follow-lists.md) each cover one part of.

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and B have each signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv`. `apps/mobile/qa/journeys/social/follow-and-boards.setup.sh` resolves both user ids from it, the way `auth/sign-in.truth.sh` does. It marks A and B as done with onboarding, sets their display names ("Alfred" and "Bartholomez") and runs `monacoctl seed two-cabals-ranked --users <A id>,<B id>` under `scripts/with-dotenv-local.sh`, so both rank on "Top investors" with no money moved. The command never creates a user and is idempotent, so the setup runs it before every run |
| P2 | A follows nobody, B follows nobody and B has no followers. Before S1, the setup script ends every live follow by A or B and every live follow of B through `DELETE /v1/users/{id}/follow`, and hands the truth check `followRowsBefore`, the count of A-to-B rows in `follows` |
| P3 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |

## Scenarios

### S1 Both rank on the board

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap, then scroll to | the Home tab, then "Top investors" | | "Top investors" shows with `home-leaderboard-range-ALL` selected, the first ranked row is labelled "First" (the crown), and `home-leaderboard-row-<A id>` and `home-leaderboard-row-<B id>` each show a return, within 30 s (screens.md `HomePeopleBoardSlot`: "ranked rows (crown for first, then numbers, avatar, name, return over gain or loss)") |

### S2 Friends with no follows

Starts where S1 ended.

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap | "Friends" in `home-leaderboard-filter` | | `home-leaderboard-friends-empty` reads "Follow people to see how they do." with `home-leaderboard-find-friends` ("Find friends") within 10 s |
| S2.2 | A | tap | "Everyone" in `home-leaderboard-filter` | | `home-leaderboard-row-<A id>` and `home-leaderboard-row-<B id>` show again within 10 s |

### S3 Follow from the board

Starts where S2 ended.

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S3.1 | A | tap | `home-leaderboard-row-<B id>` | | `user-profile-header` shows within 15 s with `user-profile-name` reading "Bartholomez", `user-profile-handle` starting with "@", `user-profile-followers` reading "0 Followers", `user-profile-following` reading "0 Following" and `user-profile-follow` reading "Follow" |
| S3.2 | A | tap | `user-profile-follow` | | `user-profile-follow` reads "Following" and `user-profile-followers` reads "1 Followers" within 5 s, and `user-profile-following` still reads "0 Following" |
| S3.3 | A | go back, then tap | "Friends" in `home-leaderboard-filter` | | `home-leaderboard-row-<B id>` shows, and A's own row, labelled "You", shows (`home-leaderboard-row-<A id>`, or `home-leaderboard-me` when pinned), within 10 s, and `home-leaderboard-friends-empty` is gone (screens.md `HomePeopleBoardSlot`: "the viewer's row pinned at the bottom when off the page") |

### S4 B sees the follower

Starts where S3 ended.

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S4.1 | B | tap | the Profile tab | | `profile-followers` reads "1 Followers" and `profile-following` reads "0 Following" within 15 s |
| S4.2 | B | tap, then tap | `profile-followers`, then `follow-list-open-<A id>` | | The "Followers" screen shows within 10 s with `follow-list-row-<A id>` and its `follow-list-follow-<A id>` button reading "Follow" within 15 s. Tapping A's row shows `user-profile-name` reading "Alfred" within 15 s |

### S5 Unfollow

Starts where S4 ended.

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S5.1 | A | tap, tap, then tap | the Home tab, `home-leaderboard-row-<B id>`, then `user-profile-follow` | | B's profile shows `user-profile-follow` reading "Following" within 15 s. After the tap it reads "Follow" and `user-profile-followers` reads "0 Followers" within 5 s, with `user-profile-following` still reading "0 Following" |
| S5.2 | A | go back, then tap | "Friends" in `home-leaderboard-filter` | | `home-leaderboard-friends-empty` reads "Follow people to see how they do." with `home-leaderboard-find-friends` within 10 s |

## Ground truth

`apps/mobile/qa/journeys/social/follow-and-boards.truth.sh` reads `follows` through `apps/mobile/qa/journeys/psql.sh`. After S3, `follows` has one live row from A to B. After S5, it has none. The setup script reads the first count when the test reaches S4 and hands it over as `liveAfterFollow`, so the check can still see the state S3 left. At the end, the check requires `liveAfterFollow` to be 1, no live row from A to B, and more A-to-B rows than `followRowsBefore`, which shows the follow and the unfollow both reached the backend.

## Known failures on staging

None.

## Not covered

- The avatar on B's profile. `MonacoAvatar` is hidden from accessibility, so no identifier can reach it. S3.1 reads the name, the handle and the counts around it.
- The exact "@handle". It is `qa_<name>` plus a suffix when another user holds that handle, so it differs across databases. S3.1 requires an "@" and a name.
- The range chips other than "All" and the ranked order after a trade. A trade needs a funded treasury; `ranking/leaderboards` covers the other boards.
- "Find friends" opening Friends on Monaco. S2.1 and S5.2 read the button, and the contact search belongs to `profile/find-friends`.
- Following back from the follower list. S4.2 reads the "Follow" button and does not tap it, so B ends the journey following nobody.
