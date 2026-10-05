---
id: profile/edit
title: Edit your profile
version: 3
milestone: M9
requires: [auth/sign-in]
actors: [A]
flows: [23, 23a]
xcuitest: [apps/mobile/MonacoUITests/Journeys/ProfileEditJourney.swift, apps/mobile/MonacoUITests/Journeys/ProfileEditJourneyUITests.swift]
---

# Edit your profile

A signed-in member renames themselves on the Profile tab's header, changes their face, and finds both again after a relaunch. The photo rate limit, the onboarding nudge and the handle editor are in [profile/nudge](nudge.md), which keeps each journey inside the runner's 300 s budget. The design is [Flow 23](../../architecture/auth.md#handle).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Everything [auth/sign-in](../auth/sign-in.md) needs |
| P2 | Actor A has a handle and a display name |
| P3 | Before each scenario, `scripts/qa/journey.py` runs `apps/mobile/qa/journeys/profile/edit.setup.sh` with the scenario id. It sets A's `auth_state` to `AWAITING_SOCIALS`, so no one edits the database by hand |
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

## Ground truth

After a run, A's `users` row has `display_name = 'Alfred {QA.run}'` and a `photo_url`. `apps/mobile/qa/journeys/profile/edit.truth.sh` checks both, through `psql` on the host or in the Compose `monaco-postgres` container.

## Not covered

- Telling a library photo from an animal on the avatar. An animal is stored as a photo too, so both read "Change your face". The toast in S3.2 says which one was picked.
- A name the server rejects after the field accepts it. `SessionAPITests` covers the `display_name_invalid` mapping.
