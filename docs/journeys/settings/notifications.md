---
id: settings/notifications
title: Turn on notifications
version: 2
milestone: M9
requires: [auth/sign-in]
actors: [B]
flows: [03]
xcuitest: [apps/mobile/MonacoUITests/Journeys/SettingsNotificationsJourney.swift, apps/mobile/MonacoUITests/Journeys/SettingsNotificationsJourneyUITests.swift]
---

# Turn on notifications

A member who has not decided on notifications sees "Off" in Settings. They join their first cabal, and the push pre-prompt asks them to turn notifications on. They do, answer the iOS permission alert, and Settings then reads "On" and opens the iOS notification settings. The design is the Settings "Notifications" row in the [Profile tab](../../screens.md#profile-tab) section and the pre-prompt from #2143. The old app at `c838bd24` had neither, so every step is new in the spec.

The format of this doc is in [App journeys](../README.md). The Old app column names the old app's tap or element for each step, or says the step is new in the spec.

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Everything [auth/sign-in](../auth/sign-in.md) needs |
| P2 | `scripts/qa/journey.py` reinstalls the app before every run, so notification permission starts undecided and the pre-prompt has not been shown |
| P3 | Before S2, `apps/mobile/qa/journeys/settings/notifications.setup.sh` declines every pending invite to B, makes a dev host create an open cabal named `QA push {QA.run}`, and invites B by handle. It hands the test the cabal's name as `cabalName` |
| P4 | The scenarios run in order in one run: S2 answers the permission alert, and S3 starts from that answer |
| P5 | The test answers the iOS permission alert through springboard (`com.apple.springboard`), tapping "Allow". It taps no coordinates |

## Scenarios

### S1 Settings reads Off before asking

Starts signed in (auth/sign-in), with permission undecided (P2).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S1.1 | tap, then tap | the Profile tab, then `profile-settings-row` | | The "Settings" screen shows within 10 s | None, new in spec |
| S1.2 | wait | `settings-notifications` | | Within 5 s the row reads "Notifications" and "Off" | None, new in spec |

### S2 The pre-prompt after the first cabal

Starts signed in (auth/sign-in), with an invite to `cabalName` waiting (P3).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S2.1 | tap | the Cabals tab | | `cabal-invite-row` for `cabalName` shows within 15 s | None, new in spec |
| S2.2 | tap | `cabal-invite-accept` | | The toast "You're in." shows within 10 s. Once it goes, `push-pre-prompt` shows within 15 s with `push-pre-prompt-title` reading "Know when your cabal votes and trades", "We'll tell you when a vote opens, passes, or a trade fills.", `push-pre-prompt-turn-on` ("Turn on notifications") and `push-pre-prompt-not-now` ("Not now") | None, new in spec |
| S2.3 | tap | `push-pre-prompt-turn-on` | | The iOS alert asking to send notifications shows, the test taps "Allow" on it through springboard (P5), and `push-pre-prompt` closes within 15 s | None, new in spec |

### S3 Settings reads On and opens iOS settings

Starts from S2 (P4).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S3.1 | tap, then tap | the Profile tab, then `profile-settings-row` | | Within 10 s `settings-notifications` reads "Notifications" and "On" | None, new in spec |
| S3.2 | tap | `settings-notifications` | | The iOS Settings app is in the foreground within 15 s | None, new in spec |

## Ground truth

After a run, B is a member of the cabal named `QA push {QA.run}`, the join that showed the pre-prompt. `apps/mobile/qa/journeys/settings/notifications.truth.sh` checks it through `apps/mobile/qa/journeys/psql.sh`.

## Known failures on staging

None.

## Not covered

- "Not now" on the pre-prompt. The prompt shows once per install, and S2 turns notifications on. `PushPrePromptTests` covers "Not now" and the show-once rule.
- "Don't Allow" on the iOS alert, and the row reading "Off" after a denial. iOS asks once per install, so one run can answer only one way.
- The device token in `device_tokens` and a remote push arriving. Whether a simulator gets an APNs token depends on the host Mac, so a run cannot promise one.
