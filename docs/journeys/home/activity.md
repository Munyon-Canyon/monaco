---
id: home/activity
title: Open Activity from the Home bell
version: 1
milestone: UI v3
requires: [auth/sign-in]
actors: [A]
flows: []
xcuitest: [apps/mobile/MonacoUITests/Journeys/HomeActivityJourney.swift, apps/mobile/MonacoUITests/Journeys/HomeActivityJourneyUITests.swift]
---

# Open Activity from the Home bell

A signed-in member taps the bell on Home and lands on Activity, then goes back. The backend serves no notifications yet, so Activity shows its "opens soon" caption and nothing else: no fixture people, no amounts, no Mark all read. The journey asserts that caption. When `GET /v1/me/notifications` ships, this doc gains the list and Mark all read steps. The spec is [Catch up](../../screens.md#catch-up-activityfeedroute) and the bell in [Home](../../screens.md#home).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Everything [auth/sign-in](../auth/sign-in.md) needs |

## Scenarios

### S1 The bell opens Activity

Starts signed in (auth/sign-in).

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap | the Home tab, then `home-notifications-button` | | Within 15 s, the Activity screen shows under the inline title "Activity". screens.md: the bell opens `ActivityFeedRoute` |
| S1.2 | A | wait | `activity-coming-soon` | | Within 15 s, `activity-coming-soon` reads "Your activity shows up here soon." screens.md: live app until the API ships, the nav title and one centred caption, "Your activity shows up here soon." |
| S1.3 | A | read | the Activity screen | | There is no `activity-mark-all-read`, no `activity-empty` and no `activity-item-…` row, and no toast shows. screens.md: no fixture people or amounts |
| S1.4 | A | tap | the navigation bar's back button | | Within 10 s, the Home tab shows `home-notifications-button` again, and the tab bar is still there |

## Ground truth

Opening Activity writes nothing. `apps/mobile/qa/journeys/home/activity.truth.sh` reads A's `users` row and checks that it still has its handle and `auth_state`.

## Known failures on staging

None known.

## Not covered

- The list, the group labels, "Mark all read" and the unread dot on the bell. They need `GET /v1/me/notifications` (paged, newest first), `POST /v1/me/notifications/read` and an unread flag on a Home read, which the backend does not have. `ActivityFeedModelTests` and the `-activityFeedHarness` sample cover them against fixtures.
- The Feed tab's bell. It shows its own "on its way" screen until Activity is wired there.
