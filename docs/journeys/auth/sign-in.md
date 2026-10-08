---
id: auth/sign-in
title: Sign in
version: 7
milestone: M9
requires: []
actors: [A]
flows: [01]
xcuitest: [apps/mobile/MonacoUITests/Journeys/SignInJourney.swift, apps/mobile/MonacoUITests/Journeys/SignInJourneyUITests.swift]
---

# Sign in

A member signs in with a one-time code, by text or by email, and lands on the tab bar. Signing up is the same journey. Privy creates the account on the first code. The design is [Login](../../architecture/auth.md#login).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | The app is installed from a Debug build and shows the login form. A test that finds a saved session signs out first: with S3 from the tab bar (Profile, then Settings), or with `onboarding-handle-step-sign-out` or `onboarding-phone-step-sign-out` from a first-run step |
| P2 | Actors A to C use the Privy test logins in `apps/mobile/qa/journeys/accounts.tsv` |
| P3 | The simulator can reach `auth.privy.io`. The dev database is migrated (`just migrate db`). `journey.py` starts the local backend, which answers `GET http://127.0.0.1:8080/healthz` |
| P4 | Actor A's `users` row has a handle and an `auth_state` past `CREATED`, so the first-run gate opens the tab bar and not the handle or phone step. On a fresh dev database, sign in once, then run `update users set handle = 'qa_alfred', auth_state = 'ONBOARDING_COMPLETED' where privy_user_id = '<A.privy_user_id>'` |

The channel is text message unless the run sets `MONACO_QA_CHANNEL=email`. For email, read `sms` as `email` in every identifier, `smsPhoneField` as `emailAddressField`, and `{A.phone}` as `{A.email}`.

## Scenarios

### S1 Sign in

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S1.1 | tap, only when the field is not already showing | "Text message" in the Sign-in method control | | `smsPhoneField` shows within 5 s |
| S1.2 | type, then tap | `smsPhoneField`, then `smsSendCodeButton` | `{A.phone}` | The button reads "Send code" and is enabled before the tap |
| S1.3 | type | `smsCodeField` | `{A.code}` | The field shows within 20 s of S1.2. The sixth digit submits the code. Continue (`smsVerifyButton`) is not tapped |
| S1.4 | wait | the session-opening screen | | The backend session opens. Within 30 s the tab bar or the Find friends step shows, not the handle or phone step |
| S1.5 | tap, only when the Find friends step shows | `friends-not-now` (Not now) | | The tab bar shows within 30 s and `smsCodeField` is gone. An account without a linked phone goes straight to the tab bar and the step does nothing |
| S1.6 | wait | the tab bar | | Home, Feed, Cabals, Stocks and Profile tabs show |

### S2 The session survives a relaunch

Starts signed in (S1).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S2.1 | relaunch | the app | | |
| S2.2 | wait | the tab bar | | The Home tab shows within 30 s. The login form never shows |

### S3 Sign out

Starts signed in (S1).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S3.1 | tap, then tap | the Profile tab, then `profile-settings-row` | | The "Settings" screen shows within 10 s, and `profileSignOutButton` shows within 15 s, the last row of Settings. The push pre-prompt (`push-pre-prompt`) is answered "Not now" first when it shows, and the Profile tab is tapped a second time when `profile-header` has not shown within 5 s |
| S3.2 | tap | `profileSignOutButton` | | The dialog "Sign out of Monaco?" shows within 5 s |
| S3.3 | tap | `profile-sign-out-confirm` | | The login form shows within 30 s and the tab bar is gone |
| S3.4 | relaunch | the app | | The login form shows within 30 s |

## Ground truth

After S1, the local database has a `users` row for actor A. `apps/mobile/qa/journeys/auth/sign-in.truth.sh` checks its configured Privy user ID.

The row also exists after an earlier run, so a run that never reaches the backend still passes this check. The backend log line `POST /v1/auth/session` with status 200 during the run is the stronger sign, and `just reset db` gives a fresh database.

## Not covered

- A wrong code. Privy may lock a test login after repeated wrong codes, and the three test logins are shared by the team.
- Send a new code, and Change number.
- Apple and Google login (#541).
- The first-run handle, phone and socials steps (#643, #693, #694). An account without a handle, or still in `CREATED`, lands on one after the backend session opens, which P4 rules out.
