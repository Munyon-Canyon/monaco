---
id: profile/find-friends
title: Find friends
version: 1
milestone: M22
requires: [auth/sign-in]
actors: [A]
flows: [20]
xcuitest: [apps/mobile/MonacoUITests/Journeys/ProfileFindFriendsJourney.swift, apps/mobile/MonacoUITests/Journeys/ProfileFindFriendsJourneyUITests.swift]
---

# Find friends

A member opens "Find friends" from the Profile tab, reads the contacts explainer, backs out with "Not now", then searches for another member by handle and by name and follows them. The design is `ProfileFindFriendsSlot` on the [Profile tab](../../screens.md#profile-tab): Row "Find friends" (contacts, plus search by name or handle from #2142), opening "Friends on Monaco". The old app's version is Profile -> Find friends at `c838bd24`.

The format of this doc is in [App journeys](../README.md). The Old app column names the old app's tap or element for each step, or says the step is new in the spec.

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and B have each signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | Before each scenario, `apps/mobile/qa/journeys/profile/find-friends.setup.sh` marks A and B as done with onboarding and sets B's display name to "Bartholomez" |
| P3 | The setup script ends any follow of B by A through `DELETE /v1/users/{id}/follow`. It hands the test `memberID` (B's user id) and `memberHandle` (B's handle), and hands the truth check `seededAt`, the database time after the unfollow |

## Scenarios

### S1 Open Find friends and back out

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S1.1 | tap, then scroll to | the Profile tab, then `profile-find-friends` | | The row reads "Find friends" | The "Find friends" row on Profile |
| S1.2 | tap | `profile-find-friends` | | The "Friends on Monaco" screen shows within 10 s, with "See which of your contacts are already on Monaco. Only scrambled numbers leave your phone, never your address book." | The contacts explainer |
| S1.3 | tap | `friends-not-now` | | `profile-header` shows again within 10 s | "Not now" |

### S2 Search by handle and follow

Starts signed in (auth/sign-in). Fails on staging until the search lands (Known failures).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S2.1 | tap, scroll to, then tap | the Profile tab, `profile-find-friends`, then `friends-search-field` | | The search field is enabled within 10 s | The search field on Find friends |
| S2.2 | type | `friends-search-field` | `{memberHandle}` | `friends-result-{memberID}` shows within 15 s, reading "Bartholomez" and "@" with the handle | Search results by handle |
| S2.3 | tap | `friends-result-follow-{memberID}` | | Within 10 s it reads "Following" | "Follow" on a result row |

### S3 Search by name

Starts signed in (auth/sign-in). Fails on staging until the search lands (Known failures).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S3.1 | tap, scroll to, then tap | the Profile tab, `profile-find-friends`, then `friends-search-field` | | The search field is enabled within 10 s | The search field on Find friends |
| S3.2 | type | `friends-search-field` | `Bartholomez` | `friends-result-{memberID}` shows within 15 s, reading "Bartholomez" | Search results by name |

## Ground truth

S2 follows B from a search result. `apps/mobile/qa/journeys/profile/find-friends.truth.sh` checks through `apps/mobile/qa/journeys/psql.sh` that any live A-to-B row in `follows` was created after `seededAt`, so the run wrote it. Until #2142 lands, S2 stops at S2.1 and A does not follow B, which the check reports and passes.

## Known failures on staging

| Step | What fails | Blocked by |
| --- | --- | --- |
| S2.1 | "Friends on Monaco" has no search field, only the contacts explainer and a disabled "Find friends" under "Finding friends opens soon.". `GET /v1/users` is live: this is a UI gap | #2142 |
| S2.2 | No search results, for the same reason | #2142 |
| S2.3 | No result row to follow, for the same reason | #2142 |
| S3.1 | No search field | #2142 |
| S3.2 | No search results | #2142 |

## Not covered

- The contacts match itself: the enabled "Find friends" button on the explainer, the contacts permission and the matched list. Contacts is a later M22 ticket, and the simulator's address book is not a fixed state.
- `friends-search-field`, `friends-result-{userID}` and `friends-result-follow-{userID}` do not exist in `ContactsExplainerView.swift` yet. They are the identifiers #2142 should add. If it picks other names, this doc and its steps bump to version 2.
- Searching for yourself, and a search with no match. #2142 sets their copy.
