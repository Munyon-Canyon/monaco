---
id: home/activity
title: Catch up from the Home bell
version: 2
milestone: F launch
requires: [auth/sign-in]
actors: [A]
flows: []
xcuitest: [apps/mobile/MonacoUITests/Journeys/HomeActivityJourney.swift, apps/mobile/MonacoUITests/Journeys/HomeActivityJourneyUITests.swift]
---

# Catch up from the Home bell

A signed-in member taps the bell on Home and lands on Catch up, which replaces Activity. The bell's badge is the Catch up count. Catch up lists what waits on the member ("Waiting on you"), "Replies and mentions", "Invites" and "Results", swipes a row to Clear it, and shows "You're caught up" when nothing is left. The spec is [Catch up](../../screens.md#catch-up-activityfeedroute) and the bell in [Home](../../screens.md#home); the frames are `16-catch-up` and `16-catch-up-empty`.

The route is still named `ActivityFeedRoute` and the journey keeps its id `home/activity`.

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Everything [auth/sign-in](../auth/sign-in.md) needs |
| P2 | `apps/mobile/qa/journeys/home/activity.setup.sh` ran right before the scenario. It closes every open proposal A can vote on. A new dev user, "QA catchup host", creates the cabal `QA catch up {QA.run}` and A joins it. The host proposes two $5.00 buys. A votes Yes on the second and the host's Yes passes it, which gives A a Results item. A comments on the first and the host replies, which gives A a Replies and mentions item, and the first stays open, which gives A a Waiting on you item. The script waits until `GET /v1/me/catch-up` lists all three, and hands the test `waitingKey`, `replyKey` and `resultKey` |

A may have other Catch up items from earlier runs, such as invites, so the journey reads the count n from the screen and asserts n - 1 after the clear, never a fixed number.

## Scenarios

### S1 The bell opens Catch up

Starts signed in (auth/sign-in).

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap | the Home tab, then `home-notifications-button` | | The bell's label reads "Catch up, n new" with n at least 3 and its badge `home-bell-badge` shows within 15 s. Tapping it opens the "Catch up" screen with `catch-up-page` within 15 s, and `catch-up-subline` reads "n to catch up" |
| S1.2 | A | read | `catch-up-section-waiting`, `catch-up-section-replies`, `catch-up-section-results` | | Within 15 s "Waiting on you" lists `catch-up-row-<waitingKey>` with its tally and `catch-up-vote-<waitingKey>` "Vote", "Replies and mentions" lists `catch-up-row-<replyKey>`, and "Results" lists `catch-up-row-<resultKey>` |
| S1.3 | A | swipe, then tap | `catch-up-row-<resultKey>` left to right, then "Clear" | | "Clear" shows (or the full swipe clears at once). Within 10 s `catch-up-subline` reads "n - 1 to catch up", `catch-up-section-cleared` shows "Cleared today" and no toast shows |
| S1.4 | A | tap, then tap | `catch-up-vote-<waitingKey>`, then `vote-hold-close` | | The hold sheet `vote-hold-sheet` shows with `vote-hold-row` within 10 s, and closing it returns to `catch-up-page` still reading "n - 1 to catch up" |
| S1.5 | A | tap | the navigation bar's back button | | Within 15 s the Home tab shows `home-notifications-button` reading "Catch up, n - 1 new", and the tab bar is still there |

## Ground truth

`apps/mobile/qa/journeys/home/activity.truth.sh` reads the hand-off. A's `catch_up_state` row for `resultKey` has `cleared_at` set, and neither `waitingKey` nor `replyKey` is cleared.

## Known failures on staging

None known.

## Not covered

- The one-by-one card view (`catch-up-one-by-one`, #6571) and the Invites row. The invite journey covers accepting.
- The empty state `16-catch-up-empty`. A's items from other journeys cannot be cleared away for certain, so the empty state is covered by `CatchUpModelTests` and the sample harness.
- The Feed tab's bell.
- The screen has no `catch-up-root` identifier. The journey uses `catch-up-page`, the list that holds the sections, and the "Catch up" navigation title.
