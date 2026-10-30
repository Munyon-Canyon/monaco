---
id: profile/edit
title: Edit your profile
version: 6
milestone: M9
requires: [auth/sign-in]
actors: [A]
flows: [23, 23a]
simslim: photos
xcuitest: [apps/mobile/MonacoUITests/Journeys/ProfileEditJourney.swift, apps/mobile/MonacoUITests/Journeys/ProfileEditJourneyUITests.swift]
---

# Edit your profile

A signed-in member renames themselves on the Edit profile screen, which the settings round (Settings → Edit profile) opens, changes their face, and finds the name again after a relaunch. The photo rate limit, the onboarding nudge and the handle editor are in [profile/nudge](nudge.md), which keeps each journey inside the runner's 300 s budget. The design is [Flow 23](../../architecture/auth.md#handle).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Everything [auth/sign-in](../auth/sign-in.md) needs |
| P2 | Actor A has a handle and a display name |
| P3 | Before each scenario, `scripts/qa/journey.py` runs `apps/mobile/qa/journeys/profile/edit.setup.sh` with the scenario id. It sets A's `auth_state` to `AWAITING_SOCIALS`, so no one edits the database by hand |
| P4 | The simulator's photo library has at least one photo. A new simulator ships with sample photos. The journey runs on the `photos` SimSlim profile (`ci/profiles/photos.json`), which keeps Photos on |
| P5 | S4 starts from the name S1 saved. Run on its own, S4 first renames A to `Alfred {QA.run}` |

## Scenarios

### S1 Change the name

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S1.1 | tap | the Profile tab | | Within 15 s, `profile-card` shows the photo's pencil badge `profile-photo-edit` labelled "Edit profile photo" (#6488), `profile-display-name`, and `profile-handle` reading "@" and the handle, then "joined" and a month and year |
| S1.2 | tap, then tap | `profile-settings-row`, then `settings-edit-profile` | | "Edit profile" shows within 5 s as the screen title, with `profile-name-field` and `profile-name-save` disabled because nothing has changed. Back returns to the Profile tab |
| S1.3 | tap, tap, type, then tap | `profile-settings-row`, `settings-edit-profile`, `profile-name-field`, then `profile-name-save` | `Alfred {QA.run}` | `monaco-toast-banner` reads "Profile saved." within 10 s and `edit-profile-saved` shows. Back returns to the Profile tab, where `profile-display-name` reads "Alfred {QA.run}" |

### S2 An empty name is refused

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S2.1 | tap, tap, then tap | the Profile tab, `profile-settings-row`, then `settings-edit-profile` | | "Edit profile" shows within 5 s |
| S2.2 | clear | `profile-name-field` | | `profile-name-field` reports "Display name is required." within 2 s, and `profile-name-save` is disabled |
| S2.3 | tap | back | | "Edit profile" closes within 5 s, and `profile-display-name` reads what it read in S2.1: "Alfred {QA.run}" after S1 |

### S3 Change the photo

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S3.1 | tap, then tap | the Profile tab, then `profile-photo-edit` | | `face-picker-sheet` shows within 5 s with "Your face", eight `face-option-…` animals and `face-choose-photo` reading "Choose a photo" |
| S3.2 | tap, then tap | `face-choose-photo`, then the first image labelled "Photo, …" in the library | | `monaco-toast-banner` reads "Profile photo updated." within 20 s |

### S4 The name survives a relaunch

Starts signed in (auth/sign-in), from S1 (P5).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S4.1 | relaunch, then tap | the app, then the Profile tab | | Within 30 s, `profile-display-name` reads "Alfred {QA.run}" and `profile-photo-edit` shows |

## Ground truth

After a run, A's `users` row has `display_name = 'Alfred {QA.run}'` and a `photo_url`. `apps/mobile/qa/journeys/profile/edit.truth.sh` checks both, through `psql` on the host or in the Compose `monaco-postgres` container.

## Not covered

- Telling a library photo from an animal on the avatar. An animal is stored as a photo too, and the avatar always reads "Edit profile photo". The toast in S3.2 says which one was picked.
- Saving a new username. A handle is unique and can change only once per period, so a run would use up actor A's next change. `HandleAvailabilityCheckerTests` covers the checks, and the Edit profile sample scenarios cover the field states.
- Saving the bio and the "Show my 1M return" toggle. The journey leaves both alone so A's profile stays the same across runs. `ProfileBioRulesTests` and `AppSessionStoreTests` cover the limit and the save.
- A name the server rejects after the field accepts it. `SessionAPITests` covers the `display_name_invalid` mapping.
