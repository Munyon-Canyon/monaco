---
id: profile/follow-lists
title: Followers and following
version: 1
milestone: M17
requires: [auth/sign-in]
actors: [A]
flows: [20]
xcuitest: [apps/mobile/MonacoUITests/Journeys/ProfileFollowListsJourney.swift, apps/mobile/MonacoUITests/Journeys/ProfileFollowListsJourneyUITests.swift]
---

# Followers and following

A member opens the Profile tab, reads their follower and following counts, and opens each list to find the person who follows them and the person they follow. The design is `ProfileFollowCountsSlot` on the [Profile tab](../../screens.md#profile-tab): "12 Followers · 8 Following", each opening the follow list. The old app's version is the counts under the Profile header at `c838bd24`.

The format of this doc is in [App journeys](../README.md). The Old app column names the old app's tap or element for each step, or says the step is new in the spec.

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and B have each signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | Before each scenario, `apps/mobile/qa/journeys/profile/follow-lists.setup.sh` marks A and B as done with onboarding and sets B's display name to "Bartholomez" |
| P3 | The setup script makes B follow A and A follow B through `POST /v1/users/{id}/follow`. Following again changes nothing |

## Scenarios

### S1 Followers

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S1.1 | tap | the Profile tab | | Within 15 s, `profile-followers` and `profile-following` show, reading "Followers" and "Following" | The counts under the Profile header |
| S1.2 | tap | `profile-followers` | | The "Followers" screen shows within 10 s | The followers count opens the followers list |
| S1.3 | read | the list | | A row reading "Bartholomez" shows within 15 s, and `follow-list-coming` ("Followers show up here soon.") does not show | The followers list rows |

### S2 Following

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S2.1 | tap | the Profile tab | | `profile-following` shows within 15 s | The counts under the Profile header |
| S2.2 | tap | `profile-following` | | The "Following" screen shows within 10 s | The following count opens the following list |
| S2.3 | read | the list | | A row reading "Bartholomez" shows within 15 s, and `follow-list-coming` ("People you follow show up here soon.") does not show | The following list rows |

### S3 The counts

Starts signed in (auth/sign-in). Fails on staging until the route lands (Known failures).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S3.1 | tap | the Profile tab | | Within 15 s, `profile-followers` reads a whole number of 1 or more and "Followers", and `profile-following` a whole number of 1 or more and "Following", as screens.md's "12 Followers · 8 Following" | The counts under the Profile header |

## Ground truth

The journey only reads. `apps/mobile/qa/journeys/profile/follow-lists.truth.sh` checks through `apps/mobile/qa/journeys/psql.sh` that the follows P3 seeded are still live after the run: B follows A, and A follows B.

## Known failures on staging

| Step | What fails | Blocked by |
| --- | --- | --- |
| S1.3 | The list shows "Followers show up here soon.", because no route lists a user's followers | #620 |
| S2.3 | The list shows "People you follow show up here soon.", because no route lists who a user follows | #620 |
| S3.1 | The links read "Followers" and "Following" with no numbers, because no route serves the counts | #620 |

## Not covered

- An empty list. P3 seeds both directions, and removing them would change the starting state of [profile/user-profile](user-profile.md).
- Tapping a row in a list to open that member's profile. The list has no rows on staging, so `FollowListView.swift` has no row identifier yet. #620 should give each row `follow-list-row-{userID}`.
- The counts and lists on another member's profile (`user-profile-followers`, `user-profile-following`). They open the same `FollowListRoute`.
