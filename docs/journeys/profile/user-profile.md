---
id: profile/user-profile
title: Another member's profile
version: 5
milestone: M17
requires: [auth/sign-in]
actors: [A]
flows: [20]
xcuitest: [apps/mobile/MonacoUITests/Journeys/ProfileUserProfileJourney.swift, apps/mobile/MonacoUITests/Journeys/ProfileUserProfileJourneyUITests.swift]
---

# Another member's profile

A member opens a cabal, opens the cabal's Members tab, taps another member's row, reads their profile, follows and unfollows them, reads the cabals they have in common, and opens the "…" menu. On their own row the profile hides Follow and the menu. The design is the [User profile](../../screens.md#profile-tab) (`UserProfileRoute`). The old app opened its profile from a leaderboard or member row; the new app's is `UserProfileRoute`, which renders `UserProfileScreen`.

The format of this doc is in [App journeys](../README.md). The Old app column names the old app's tap or element for each step, or says the step is new in the spec.

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and B have each signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | Before each scenario, `apps/mobile/qa/journeys/profile/user-profile.setup.sh` marks A and B as done with onboarding and sets B's display name to "Bartholomez" |
| P3 | The setup script reads the cabals A and B share (`GET /v1/users/{id}/shared-cabals`, which leaves out a cabal the ranking run has not valued yet) and picks the first. When they share none, it has A create the cabal `QA profile {QA.run}` and B join it through the API, and S3.2 fails because the list leaves that cabal out. It hands the test `cabalID`, `cabalName`, `meID` (A's user id) and `memberID` (B's user id) |
| P4 | Before S2, the setup script ends any follow of B by A through `DELETE /v1/users/{id}/follow`, and hands the truth check `followRowsBefore`, the count of A-to-B rows in `follows` |

## Scenarios

### S1 Open a member's profile

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S1.1 | open | `openCabal(named: cabalName)` | | The cabal screen shows within 15 s, with `cabal-header-name` reading `cabalName` | A leaderboard or Groups row opens the cabal |
| S1.2 | tap | the member's row on the Members tab (B's name, "Bartholomez") | | `user-profile-header` shows within 15 s | A member row opens the profile (`UserProfileRoute`) |
| S1.3 | read | `user-profile-header` | | `user-profile-name` reads "Bartholomez", and the counts row below the identity card shows a count and "follower" or "followers" (`user-profile-followers`) and a count and "following" (`user-profile-following`). `user-profile-right-calls` reads "Right calls · 1M" with a figure | The profile header: avatar, name, "@handle", counts |
| S1.4 | read | `user-profile-more` | | The "…" menu shows in the toolbar, labelled "More" | The report and block menu |

### S2 Follow, then unfollow

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S2.1 | open, open, then tap | `openCabal(named: cabalName)`, `openCabalTab("Members")`, then the member's row (B's name, "Bartholomez") | | `user-profile-follow` reads "Follow" within 15 s | The "Follow" button |
| S2.2 | tap | `user-profile-follow` | | Within 10 s it reads "Following" | "Follow" turns into "Following" |
| S2.3 | tap | `user-profile-follow` | | Within 10 s it reads "Follow" again | "Following" (Unfollow) turns back into "Follow" |

### S3 Cabals in common

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S3.1 | open, open, then tap | `openCabal(named: cabalName)`, `openCabalTab("Members")`, then the member's row (B's name, "Bartholomez") | | `user-profile-header` shows within 15 s | A member row opens the profile (`UserProfileRoute`) |
| S3.2 | tap, then scroll to | the Cabals tab (`user-profile-tab-cabals`), then `user-profile-cabal-row-{cabalID}` | | The tab lists a row reading `cabalName` within 15 s, with B's share unless B turned off sharing their performance (Q42) | The profile lists the shared groups (`UserProfileSharedCabalsSlot`) |
| S3.3 | tap | `user-profile-cabal-row-{cabalID}` | | The cabal screen shows within 15 s, with `cabal-details-button` | A shared group row opens the group |

### S4 Report and block

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S4.1 | open, open, then tap | `openCabal(named: cabalName)`, `openCabalTab("Members")`, then the member's row (B's name, "Bartholomez") | | `user-profile-more` shows within 15 s | The profile toolbar |
| S4.2 | tap | `user-profile-more` | | The menu shows "Report" and "Block" within 5 s, both enabled, as screens.md's "a "…" menu with "Report" and "Block"" | The report and block menu |

### S5 Your own row

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S5.1 | open, open, then tap | `openCabal(named: cabalName)`, `openCabalTab("Members")`, then A's own row ("Alfred") | | `user-profile-header` shows within 15 s, and neither `user-profile-follow` nor `user-profile-more` shows, as screens.md's "Hidden on your own profile: the Follow button and the menu" | None, new in spec |

## Ground truth

S2 follows B and then unfollows B through `POST` and `DELETE /v1/users/{id}/follow`. `apps/mobile/qa/journeys/profile/user-profile.truth.sh` checks through `apps/mobile/qa/journeys/psql.sh` that `follows` holds more A-to-B rows than `followRowsBefore`, so the follow reached the backend, and that none of them is live, so the unfollow did too.

## Known failures on staging

None known.

## Not covered

- The "@handle" line. `UserProfileHeaderSlot.swift` shows `user-profile-handle` only when it differs from the name, and B's handle is not fixed across databases.
- The follower and following numbers. S1.3 reads the words only. `profile/follow-lists` covers the counts and lists.
- The empty "No cabals in common" / "You and Maya aren't in a cabal together yet." P3 picks a cabal A and B share, so the shared list is never empty.
- "Profile not found" / "This profile isn't available." for a banned or deleted user (`user-profile-unavailable`). No journey actor can be banned or deleted without breaking every other journey.
