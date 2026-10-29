---
id: settings/notifications
title: Turn on notifications
version: 6
milestone: M9
requires: [auth/sign-in]
actors: [B]
flows: [03, 32]
xcuitest: [apps/mobile/MonacoUITests/Journeys/SettingsNotificationsJourney.swift, apps/mobile/MonacoUITests/Journeys/SettingsNotificationsJourneyUITests.swift]
---

# Turn on notifications

A member who has not decided on notifications sees "Off" in Settings. They cast their first vote, and once the vote's toast ends the push permission sheet "Hear how votes end?" asks them to turn notifications on. They do, answer the iOS permission alert, and Settings then reads "On" and opens the iOS notification settings. They then turn "Decision requests" off, and Settings keeps it off after they leave and come back. The design is the Settings "Notifications" row in the [Profile tab](../../screens.md#profile-tab) section and the pre-prompt from #2143. The old app at `c838bd24` had neither, so every step is new in the spec.

The format of this doc is in [App journeys](../README.md). The Old app column names the old app's tap or element for each step, or says the step is new in the spec.

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Everything [auth/sign-in](../auth/sign-in.md) needs |
| P2 | `scripts/qa/journey.py` reinstalls the app before every run, so notification permission starts undecided and the pre-prompt has not been shown |
| P3 | Before S2, `apps/mobile/qa/journeys/settings/notifications.setup.sh` expires B's open proposals, and makes a dev host create a cabal named `QA push {QA.run}` with B as a voter and one open proposal B has never voted on (a fresh install, P2). It hands the test the cabal's name as `cabalName` and the proposal's id as `proposalID` |
| P4 | The scenarios run in order in one run: S2 answers the permission alert, S3 starts from that answer, and S4 starts from S3 |
| P5 | The test answers the iOS permission alert through springboard (`com.apple.springboard`), tapping "Allow". It taps no coordinates |

## Scenarios

### S1 Settings reads Off before asking

Starts signed in (auth/sign-in), with permission undecided (P2).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S1.1 | tap, then tap | the Profile tab, then `profile-settings-row` | | The "Settings" screen shows within 10 s | None, new in spec |
| S1.2 | wait | `settings-notifications` | | Within 5 s the row reads "Notifications" and "Off" | None, new in spec |

### S2 The permission sheet after the first vote

Starts signed in (auth/sign-in), with an open proposal in `cabalName` that B has not voted on (P3). The sheet comes after the vote's toast ends (#6554).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S2.1 | tap, then hold | the Home tab, then `home-vote-yes-<proposalID>` (`pickAndHold("yes")`) | | The vote is cast and a toast shows within 10 s | None, new in spec |
| S2.2 | wait | `push-pre-prompt` | | While the vote's toast shows, `push-pre-prompt` does not. Within 15 s of the toast closing it shows the title "Hear how votes end?", a sample result push, "Get each result when a vote closes. No more vote reminders after you vote.", `push-pre-prompt-turn-on` ("Turn on notifications") and `push-pre-prompt-not-now` ("Not now") (`push-pre-prompt-title` is shadowed by the sheet's `push-pre-prompt`) | None, new in spec |
| S2.3 | tap | `push-pre-prompt-turn-on` | | The iOS alert asking to send notifications shows, the test taps "Allow" on it through springboard (P5), and `push-pre-prompt` closes within 15 s | None, new in spec |

### S3 Settings reads On and opens iOS settings

Starts from S2 (P4).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S3.1 | tap, then tap | the Profile tab, then `profile-settings-row` | | Within 10 s `settings-notifications` reads "Notifications" and "On" | None, new in spec |
| S3.2 | tap | `settings-notifications` | | The iOS Settings app is in the foreground within 15 s | None, new in spec |

### S4 A category switch saves

Starts from S3 (P4). B has never changed a category, so both are on.

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S4.1 | tap, then tap | the Profile tab, then `profile-settings-row` | | The "Settings" screen shows, and within 10 s `settings-category-decisions-switch` and `settings-category-replies-switch` both read on and can be tapped. No text reads "Notification choices open soon." | None, new in spec |
| S4.2 | tap | `settings-category-decisions-switch` | | Within 10 s the switch reads off and `settings-category-replies-switch` still reads on | None, new in spec |
| S4.3 | tap, then tap | the back button, then `profile-settings-row` | | Within 10 s `settings-category-decisions-switch` reads off and `settings-category-replies-switch` reads on | None, new in spec |

## Ground truth

After a run, B is a member of the cabal named `QA push {QA.run}`, the cabal whose vote showed the pre-prompt, and B's `decision_requests` preference is off while `replies_and_mentions` is on or has no row. `apps/mobile/qa/journeys/settings/notifications.truth.sh` checks both through `apps/mobile/qa/journeys/psql.sh`.

## Known failures on staging

None.

## Not covered

- "Not now" and "Deliver quietly" on the sheet. The prompt shows once per install, and S2 turns notifications on. `PushPrePromptTests` covers "Not now" and the show-once rule.
- "Don't Allow" on the iOS alert, and the row reading "Off" after a denial. iOS asks once per install, so one run can answer only one way.
- A failed save rolling the switch back with an error toast. A run cannot make the save fail on purpose. `NotificationPreferencesModelTests` covers the rollback.
- The "Replies and mentions" switch and turning a category back on. S4 turns one category off, and the model treats both the same.
- The device token in `device_tokens` and a remote push arriving. Whether a simulator gets an APNs token depends on the host Mac, so a run cannot promise one.
