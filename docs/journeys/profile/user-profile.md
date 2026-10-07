---
id: profile/user-profile
title: Another member's profile
version: 1
milestone: M17
requires: [auth/sign-in]
actors: [A]
flows: [20]
xcuitest: [apps/mobile/MonacoUITests/Journeys/ProfileUserProfileJourney.swift, apps/mobile/MonacoUITests/Journeys/ProfileUserProfileJourneyUITests.swift]
---

# Another member's profile

A member opens a cabal, taps another member's row on the member board, reads their profile, follows and unfollows them, reads the cabals they share, and opens the "…" menu. On their own row the profile hides Follow and the menu. The design is the [User profile](../../screens.md#profile-tab) (`UserProfileRoute`). The old app opened its profile from a leaderboard or member row; the new app's is `UserProfileRoute`, which renders `UserProfileScreen`.

The format of this doc is in [App journeys](../README.md). The Old app column names the old app's tap or element for each step, or says the step is new in the spec.

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and B have each signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | Before each scenario, `apps/mobile/qa/journeys/profile/user-profile.setup.sh` marks A and B as done with onboarding and sets B's display name to "Bartholomez" |
| P3 | The setup script has A create the open cabal `QA profile {QA.run}` and B join it through the API, unless both already are members. It hands the test `cabalID`, `cabalName`, `meID` (A's user id) and `memberID` (B's user id) |
| P4 | Before S2, the setup script ends any follow of B by A through `DELETE /v1/users/{id}/follow`, and hands the truth check `followRowsBefore`, the count of A-to-B rows in `follows` |

## Scenarios

### S1 Open a member's profile

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S1.1 | tap, then tap | the Profile tab, then `cabal-row-{cabalID}` | | The cabal screen shows within 15 s, with `cabal-details-button` | A leaderboard or Groups row opens the cabal |
| S1.2 | scroll to, then tap | `cabal-member-{memberID}` | | `user-profile-header` shows within 15 s | A member row opens the profile (`UserProfileRoute`) |
| S1.3 | read | `user-profile-header` | | `user-profile-name` reads "Bartholomez", and the header shows "Followers" (`user-profile-followers`) and "Following" (`user-profile-following`), as screens.md's header "avatar, name, "@handle", follower counts" | The profile header: avatar, name, "@handle", counts |
| S1.4 | read | `user-profile-more` | | The "…" menu shows in the toolbar, labelled "More" | The report and block menu |

### S2 Follow, then unfollow

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S2.1 | tap, tap, then tap | the Profile tab, `cabal-row-{cabalID}`, then `cabal-member-{memberID}` | | `user-profile-follow` reads "Follow" within 15 s | The "Follow" button |
| S2.2 | tap | `user-profile-follow` | | Within 10 s it reads "Following" | "Follow" turns into "Following" |
| S2.3 | tap | `user-profile-follow` | | Within 10 s it reads "Follow" again | "Following" (Unfollow) turns back into "Follow" |

### S3 Cabals you share

Starts signed in (auth/sign-in). Fails on staging until the route lands (Known failures).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S3.1 | tap, tap, then tap | the Profile tab, `cabal-row-{cabalID}`, then `cabal-member-{memberID}` | | `user-profile-header` shows within 15 s | A member row opens the profile (`UserProfileRoute`) |
| S3.2 | scroll to | "Cabals you share" | | "Cabals you share" lists a row for `cabalName` within 15 s, and `user-profile-shared-coming` ("Shared cabals show up here soon.") does not show | The profile lists the shared groups (`UserProfileSharedCabalsSlot`) |
| S3.3 | tap | the row for `cabalName` | | The cabal screen shows within 15 s, with `cabal-details-button` | A shared group row opens the group |

### S4 Report and block

Starts signed in (auth/sign-in). Fails on staging until the routes land (Known failures).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S4.1 | tap, tap, then tap | the Profile tab, `cabal-row-{cabalID}`, then `cabal-member-{memberID}` | | `user-profile-more` shows within 15 s | The profile toolbar |
| S4.2 | tap | `user-profile-more` | | The menu shows "Report" and "Block" within 5 s, both enabled, as screens.md's "a "…" menu with "Report" and "Block"" | The report and block menu |

### S5 Your own row

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S5.1 | tap, tap, then tap | the Profile tab, `cabal-row-{cabalID}`, then `cabal-member-{meID}` | | `user-profile-header` shows within 15 s, and neither `user-profile-follow` nor `user-profile-more` shows, as screens.md's "Hidden on your own profile: the Follow button and the menu" | None, new in spec |

## Ground truth

S2 follows B and then unfollows B through `POST` and `DELETE /v1/users/{id}/follow`. `apps/mobile/qa/journeys/profile/user-profile.truth.sh` checks through `apps/mobile/qa/journeys/psql.sh` that `follows` holds more A-to-B rows than `followRowsBefore`, so the follow reached the backend, and that none of them is live, so the unfollow did too.

## Known failures on staging

| Step | What fails | Blocked by |
| --- | --- | --- |
| S3.2 | "Cabals you share" shows "Shared cabals show up here soon.", because no route serves the cabals two members share | #2145 |
| S3.3 | No shared cabal row to tap, for the same reason | #2145 |
| S4.2 | "Report" and "Block" are disabled under "Report and block open soon.", because no route reports or blocks a user | #2144, #2145 |

## Not covered

- The "@handle" line. `UserProfileHeaderSlot.swift` shows `user-profile-handle` only when it differs from the name, and B's handle is not fixed across databases.
- The follower and following counts. The header shows the words "Followers" and "Following" with no numbers until #620. `profile/follow-lists` covers the counts and lists.
- The empty "No cabals in common" / "You and Maya aren't in a cabal together yet." P3 puts A and B in one cabal, so the shared list is never empty.
- "This account isn't available." for a banned or deleted user (`user-profile-unavailable`). No journey actor can be banned or deleted without breaking every other journey.
- The rows on a shared cabal have no accessibility identifier yet. S3.3 taps the row by `cabalName`. `UserProfileSharedCabalsSlot.swift` should give each row `user-profile-shared-{cabalID}` when #2145 builds it.
