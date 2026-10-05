---
id: profile/nudge
title: Profile nudge and limits
version: 1
milestone: M9
requires: [auth/sign-in]
actors: [A]
flows: [23, 23a]
xcuitest: [apps/mobile/MonacoUITests/Journeys/ProfileNudgeJourney.swift, apps/mobile/MonacoUITests/Journeys/ProfileNudgeJourneyUITests.swift]
---

# Profile nudge and limits

A signed-in member hits the photo rate limit, sees the onboarding nudge for the step they skipped, sees no nudge once onboarding is complete, and opens the handle editor from the Profile tab's header. These scenarios were S5 to S8 of [profile/edit](edit.md) until that journey outgrew the runner's 300 s budget. The design is [Flow 23](../../architecture/auth.md#handle), and the banner copy comes from [`auth_state`](../../architecture/auth.md#auth_state).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Everything [auth/sign-in](../auth/sign-in.md) needs |
| P2 | Actor A has a handle and a display name |
| P3 | Before each scenario, `scripts/qa/journey.py` runs `apps/mobile/qa/journeys/profile/nudge.setup.sh` with the scenario id. It sets A's `auth_state` to `ONBOARDING_COMPLETED` for S3 and to `AWAITING_SOCIALS` for every other scenario, so no one edits the database by hand. `AWAITING_PHONE` cannot be reached here: signing in by text verifies the phone |

## Scenarios

### S1 Photo changes are rate-limited

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S1.1 | tap, then tap, up to 8 times | the Profile tab, `profile-photo-picker`, then `face-option-fox` | | Each pick ends in a `monaco-toast-banner` within 30 s. One of them reads "Too many requests. Try again in a moment." |

### S2 The nudge banner

Starts signed in (auth/sign-in), with A at `AWAITING_SOCIALS` (P3).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S2.1 | tap | the Profile tab | | `onboarding-nudge` shows within 15 s and reads "Connect X to find people you follow" |
| S2.2 | tap | the Home tab | | `onboarding-nudge` shows within 10 s |
| S2.3 | tap, then tap | `onboarding-nudge-open`, then "Not now" | | `onboarding-socials-step` shows within 5 s. "Not now" closes it within 5 s |
| S2.4 | tap | `onboarding-nudge-close` | | `onboarding-nudge` is gone within 5 s |
| S2.5 | tap | the Profile tab | | `profile-header` shows and `onboarding-nudge` does not |

### S3 No banner once onboarding is complete

Starts signed in (auth/sign-in), with A at `ONBOARDING_COMPLETED` (P3).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S3.1 | tap | the Profile tab | | `profile-header` shows within 15 s and `onboarding-nudge` does not |
| S3.2 | tap | the Home tab | | `onboarding-nudge` does not show within 5 s |

### S4 Open the handle editor

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S4.1 | tap | the Profile tab, then `profile-handle` | | `handle-step-field` shows within 5 s, holding the handle `profile-handle` showed without the "@" |

## Not covered

- The "Add your number to find friends" banner. A text-message sign-in leaves A past `AWAITING_PHONE` (P3). `OnboardingNudgeTests` covers that copy.
- The nudge vanishing while the app is open. It needs `auth_state` changed mid-scenario, so S3 covers the state after the change instead.
- Linking X from the sheet in S2.3. [onboarding/first-run](../onboarding/first-run.md) covers it.
- Saving a new handle. A handle is unique and can change only once per period, so a run would use up actor A's next change.
