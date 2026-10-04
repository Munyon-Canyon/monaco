---
id: profile/edit
title: Edit your profile
version: 1
milestone: M9
requires: [auth/sign-in]
actors: [A]
flows: [23]
xcuitest: [apps/mobile/MonacoUITests/Journeys/ProfileEditJourney.swift, apps/mobile/MonacoUITests/Journeys/ProfileEditJourneyUITests.swift]
---

# Edit your profile

A signed-in member sees the onboarding nudge for the step they skipped, renames themselves, and changes their face. The design is [Flow 23](../../architecture/auth.md#handle) and the banner copy comes from [`auth_state`](../../architecture/auth.md#auth_state).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Everything [auth/sign-in](../auth/sign-in.md) needs |
| P2 | Actor A has a handle and a display name. A test that finds the name "QA Name" renames A to "Alfred", its `accounts.tsv` name, first |
| P3 | For S1 to S4, A's `auth_state` is `AWAITING_SOCIALS`. For S5, it is `ONBOARDING_COMPLETED`. The person running the journey sets it with `psql` and runs S5 on its own with `--scenario S5`. `AWAITING_PHONE` cannot be reached here: signing in by text verifies the phone, and opening the session moves A on to `AWAITING_SOCIALS` |
| P4 | The simulator's photo library has at least one photo. A new simulator ships with sample photos |

## Scenarios

### S1 The nudge banner

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S1.1 | tap | the Profile tab | | `onboarding-nudge` shows within 15 s and reads "Connect X to find people you follow". `profile-handle` reads "@" and the handle |
| S1.2 | tap | the Home tab | | `onboarding-nudge` shows within 10 s |
| S1.3 | tap | `onboarding-nudge-open` | | A sheet titled "Connect X" shows within 5 s. "Done" closes it |
| S1.4 | tap | `onboarding-nudge-close` | | `onboarding-nudge` is gone within 5 s |
| S1.5 | tap | the Profile tab | | `profile-header` shows and `onboarding-nudge` does not |

### S2 Rename

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S2.1 | tap | the Profile tab | | `profile-display-name` and `profile-handle` show within 15 s |
| S2.2 | tap | `profile-edit-button` | | `profile-name-field` shows within 5 s |
| S2.3 | type, then tap | `profile-name-field`, then `profile-name-save` | `  QA   Name ` | `monaco-toast-banner` reads "Name updated." within 15 s, and `profile-display-name` reads "QA Name" |
| S2.4 | tap, type, then tap | `profile-edit-button`, `profile-name-field`, `profile-name-save` | `Alfred` | `profile-display-name` reads "Alfred" within 15 s, so the next run starts from A's `accounts.tsv` name |

### S3 Open the handle editor

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S3.1 | tap | the Profile tab, then `profile-handle` | | "Edit handle isn't on the new backend yet." shows within 5 s |

### S4 Change the face

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S4.1 | tap | the Profile tab, then `profile-photo-picker` | | `face-picker-sheet` shows within 5 s |
| S4.2 | tap, then tap | `face-choose-photo`, then the first image labelled "Photo, …" in the library | | `monaco-toast-banner` reads "Profile photo updated." within 30 s |
| S4.3 | tap, then tap, up to 4 times | `profile-photo-picker`, then `face-option-fox` | | Each pick ends in a `monaco-toast-banner` within 30 s. One of them reads "Too many requests. Try again in a moment." |

### S5 No banner once onboarding is complete

Starts signed in (auth/sign-in), with A at `ONBOARDING_COMPLETED` (P3).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S5.1 | tap | the Profile tab | | `profile-header` shows within 15 s and `onboarding-nudge` does not |
| S5.2 | tap | the Home tab | | `onboarding-nudge` does not show within 5 s |

## Ground truth

After S4, A's `users.photo_url` is set. `apps/mobile/qa/journeys/profile/edit.truth.sh` checks it.

## Not covered

- The "Add your number to find friends" banner. A text-message sign-in leaves A past `AWAITING_PHONE` (P3). `OnboardingNudgeTests` covers that copy.
- The nudge vanishing while the app is open. It needs `auth_state` changed in the database mid-scenario, so S5 covers the state after the change instead.
- The phone and X link screens. #694 replaces the placeholder sheet.
- Editing the handle. #693 fills `HandleEditRoute`.
- A rejected display name. `SessionAPITests` covers the `display_name_invalid` mapping, and the field's own rules stop most bad names before Save.
