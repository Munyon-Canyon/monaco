---
id: profile/edit
title: Edit your profile
version: 2
milestone: M9
requires: [auth/sign-in]
actors: [A]
flows: [23, 23a]
xcuitest: [apps/mobile/MonacoUITests/Journeys/ProfileEditJourney.swift, apps/mobile/MonacoUITests/Journeys/ProfileEditJourneyUITests.swift]
---

# Edit your profile

A signed-in member renames themselves on the Profile tab's header, changes their face, and finds both again after a relaunch. They also see the onboarding nudge for the step they skipped. The design is [Flow 23](../../architecture/auth.md#handle), and the banner copy comes from [`auth_state`](../../architecture/auth.md#auth_state).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Everything [auth/sign-in](../auth/sign-in.md) needs |
| P2 | Actor A has a handle and a display name |
| P3 | Before each scenario, `scripts/qa/journey.py` runs `apps/mobile/qa/journeys/profile/edit.setup.sh` with the scenario id. It sets A's `auth_state` to `ONBOARDING_COMPLETED` for S7 and to `AWAITING_SOCIALS` for every other scenario, so no one edits the database by hand. `AWAITING_PHONE` cannot be reached here: signing in by text verifies the phone |
| P4 | The simulator's photo library has at least one photo. A new simulator ships with sample photos |
| P5 | S4 starts from the name S1 saved and the photo S3 uploaded. Run on its own, S4 first renames A to `Alfred {QA.run}`, and uploads a photo if A has none |

## Scenarios

### S1 Change the name

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S1.1 | tap | the Profile tab | | Within 15 s, `profile-header` shows the 96 pt `profile-photo-picker`, `profile-display-name`, `profile-handle` reading "@" and the handle, and `profile-member-since` reading "Member since" with a month and year |
| S1.2 | tap | `profile-edit-button` | | "Edit profile" shows within 5 s, with the name from S1.1 in `profile-name-field`. "Done" closes it |
| S1.3 | tap, type, then tap | `profile-edit-button`, `profile-name-field`, then `profile-name-save` | `Alfred {QA.run}` | `monaco-toast-banner` reads "Name updated." within 10 s, and `profile-display-name` reads "Alfred {QA.run}" when it shows |

### S2 An empty name is refused

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S2.1 | tap, then tap | the Profile tab, then `profile-edit-button` | | "Edit profile" shows within 5 s |
| S2.2 | clear | `profile-name-field` | | `profile-name-error` reads "Display name is required." within 2 s, and `profile-name-save` is disabled |
| S2.3 | tap | "Done" | | "Edit profile" closes within 5 s, and `profile-display-name` reads what it read in S2.1: "Alfred {QA.run}" after S1 |

### S3 Change the photo

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S3.1 | tap, then tap | the Profile tab, then `profile-photo-picker` | | `face-picker-sheet` shows within 5 s with "Your face", eight `face-option-…` animals and `face-choose-photo` reading "Choose a photo" |
| S3.2 | tap, then tap | `face-choose-photo`, then the first image labelled "Photo, …" in the library | | `monaco-toast-banner` reads "Profile photo updated." within 20 s, and `profile-photo-picker` is labelled "Change your face", the label it has once a photo is stored instead of the seed animal |

### S4 Both survive a relaunch

Starts signed in (auth/sign-in), from S1 and S3 (P5).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S4.1 | relaunch, then tap | the app, then the Profile tab | | Within 30 s, `profile-display-name` reads "Alfred {QA.run}" and `profile-photo-picker` is labelled "Change your face" |

### S5 Photo changes are rate-limited

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S5.1 | tap, then tap, up to 8 times | the Profile tab, `profile-photo-picker`, then `face-option-fox` | | Each pick ends in a `monaco-toast-banner` within 30 s. One of them reads "Too many requests. Try again in a moment." |

### S6 The nudge banner

Starts signed in (auth/sign-in), with A at `AWAITING_SOCIALS` (P3).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S6.1 | tap | the Profile tab | | `onboarding-nudge` shows within 15 s and reads "Connect X to find people you follow" |
| S6.2 | tap | the Home tab | | `onboarding-nudge` shows within 10 s |
| S6.3 | tap | `onboarding-nudge-open` | | A sheet titled "Connect X" shows within 5 s. "Done" closes it |
| S6.4 | tap | `onboarding-nudge-close` | | `onboarding-nudge` is gone within 5 s |
| S6.5 | tap | the Profile tab | | `profile-header` shows and `onboarding-nudge` does not |

### S7 No banner once onboarding is complete

Starts signed in (auth/sign-in), with A at `ONBOARDING_COMPLETED` (P3).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S7.1 | tap | the Profile tab | | `profile-header` shows within 15 s and `onboarding-nudge` does not |
| S7.2 | tap | the Home tab | | `onboarding-nudge` does not show within 5 s |

### S8 Open the handle editor

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S8.1 | tap | the Profile tab, then `profile-handle` | | `handle-step-field` shows within 5 s, holding the handle `profile-handle` showed without the "@" |

## Ground truth

After a run, A's `users` row has `display_name = 'Alfred {QA.run}'` and a `photo_url`. `apps/mobile/qa/journeys/profile/edit.truth.sh` checks both, through `psql` on the host or in the Compose `monaco-postgres` container.

## Not covered

- Telling a library photo from an animal on the avatar. An animal is stored as a photo too, so both read "Change your face". The toast in S3.2 says which one was picked.
- The "Add your number to find friends" banner. A text-message sign-in leaves A past `AWAITING_PHONE` (P3). `OnboardingNudgeTests` covers that copy.
- The nudge vanishing while the app is open. It needs `auth_state` changed mid-scenario, so S7 covers the state after the change instead.
- The phone and X link screens. #694 replaces the placeholder sheet.
- Saving a new handle. A handle is unique and can change only once per period, so a run would use up actor A's next change.
- A name the server rejects after the field accepts it. `SessionAPITests` covers the `display_name_invalid` mapping.
