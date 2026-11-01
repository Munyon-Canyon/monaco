---
id: home/activity
title: Catch up from the Home bell
version: 3
milestone: F launch
requires: [auth/sign-in]
actors: [A]
flows: []
xcuitest: [apps/mobile/MonacoUITests/Journeys/HomeActivityJourney.swift, apps/mobile/MonacoUITests/Journeys/HomeActivityJourneyUITests.swift]
---

# Catch up from the Home bell

A signed-in member sees the Catch up count on Home's bell, opens Catch up, finds a vote waiting, a reply and a result, clears the result, opens the vote and comes back to a lower count. The spec is [Catch up](../../screens.md#catch-up-activityfeedroute) and the bell in [Home](../../screens.md#home); the design is `16-catch-up`.

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Everything [auth/sign-in](../auth/sign-in.md) needs |
| P2 | `activity.setup.sh` has given A one open proposal to vote on, a reply from B to A's comment and an expired proposal A voted on |

## Scenarios

### S1 The bell opens Catch up

Starts signed in (auth/sign-in) as A, with the setup above.

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap | the Home tab, then `home-notifications-button` | | Within 15 s the bell's label reads "Catch up, 3 to catch up" (the badge is hidden from accessibility, so the label carries its count). After the tap, within 15 s the navigation bar reads "Catch up" and `catch-up-subline` begins "3 to catch up". screens.md: the bell opens Catch up and its badge is the count |
| S1.2 | A | read | `catch-up-section-waiting`, `catch-up-section-replies`, `catch-up-section-results` | | The sections read "Waiting on you", "Replies and mentions" and "Results". `catch-up-row-proposal:<proposalID>` and `catch-up-vote-proposal:<proposalID>` exist (the Vote button), the reply row exists under Replies and mentions, and `catch-up-row-proposal:<resultProposalID>` exists under Results. screens.md: each section lists its items |
| S1.3 | A | swipe | `catch-up-row-proposal:<resultProposalID>`, left to right, then tap Clear | | "Clear" shows, and after the tap within 10 s `catch-up-section-results` is gone, `catch-up-section-cleared` reads "Cleared today" with `catch-up-row-proposal:<resultProposalID>` under it, and `catch-up-subline` begins "2 to catch up". screens.md: swipe to Clear moves the row to Cleared today and the count drops by one |
| S1.4 | A | tap | `catch-up-vote-proposal:<proposalID>` | | Within 15 s `vote-hold-sheet` shows. After closing it with `vote-hold-close`, Catch up shows again with `catch-up-subline`. screens.md: Vote opens the hold sheet; back returns to Catch up |
| S1.5 | A | tap | the navigation bar's back button | | Within 10 s `home-notifications-button` shows on Home and its label reads "Catch up, 2 to catch up". screens.md: the bell's badge reads the new count |

## Ground truth

Clearing writes one row. `apps/mobile/qa/journeys/home/activity.truth.sh` reads A's `catch_up_state` rows with `cleared_at` set and checks that the only one is the result proposal's `proposal:<id>` item; nothing else is cleared.

## Known failures on staging

- S1.1 names the screen by its navigation title and `catch-up-subline`. The app sets no `catch-up-root` identifier, so a step cannot target one.

## Not covered

- The one-by-one card view (`catch-up-one-by-one`), which #6571 covers.
- The Invites row. The invite journey covers accepting.
- The Feed tab's bell. It shows its own "on its way" screen until Catch up is wired there.
- The seeded result is an expired proposal, not a passed one. A passed one needs a confirmed trade, which `TRADE_ENGINE=stub` does not produce here.
