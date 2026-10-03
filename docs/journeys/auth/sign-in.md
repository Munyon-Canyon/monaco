---
id: auth/sign-in
title: Sign in
version: 1
milestone: M9
requires: []
actors: [A]
xcuitest: [apps/mobile/MonacoUITests/Journeys/SignInFlow.swift, apps/mobile/MonacoUITests/Journeys/SignInFlowUITests.swift]
---

# Sign in

A member signs in with a one-time code, by text or by email, and lands on the tab bar. Signing up is the same flow: Privy creates the account on the first code. The design is [Login](../../architecture/auth.md#login).

The format of this doc is in [App flows](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | The app is installed from a Debug build and shows the login form. A test that finds a saved session signs out first with S3 |
| P2 | Actor A's account is a Privy test login from `apps/mobile/qa/flows/accounts.tsv`: a fixed phone, email and code |
| P3 | The simulator can reach `auth.privy.io`, and the backend is running (`just migrate db`, then `just run backend`). At S1.4 the app opens a backend session with `POST /v1/auth/session` before the tab bar shows, and without the backend it stops on "Your account didn't load" |

The channel is text message unless the run sets `MONACO_QA_CHANNEL=email`. For email, read `sms` as `email` in every identifier, `smsPhoneField` as `emailAddressField`, and `{A.phone}` as `{A.email}`.

## Scenarios

### S1 Sign in

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S1.1 | tap, only when the field is not already showing | "Text message" in the Sign-in method control | | `smsPhoneField` shows within 5 s |
| S1.2 | type, then tap | `smsPhoneField`, then `smsSendCodeButton` | `{A.phone}` | The button reads "Send code" and is enabled before the tap |
| S1.3 | type | `smsCodeField` | `{A.code}` | The field shows within 20 s of S1.2. The sixth digit submits the code. Continue (`smsVerifyButton`) is not tapped |
| S1.4 | wait | the tab bar | | Home, Feed, Cabals, Stocks and Profile tabs show within 30 s, and `smsCodeField` is gone |

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
| S3.1 | tap | the Profile tab | | `profileSignOutButton` shows within 15 s |
| S3.2 | tap | `profileSignOutButton` | | The login form shows within 30 s and the tab bar is gone. No dialog asks first |
| S3.3 | relaunch | the app | | The login form shows within 30 s |

## Ground truth

None yet. Since #1485 the app calls `POST /v1/auth/session` at S1.4, so the check to add is: a `users` row exists for the actor's Privy user, read with `GET /v1/me`. Add it as `apps/mobile/qa/flows/auth/sign-in.truth.sh` and bump the version.

## Not covered

- A wrong code. Privy may lock a test login after repeated wrong codes, and the three test logins are shared by the team.
- Send a new code, and Change number.
- Apple and Google login (#541).
- The first-run handle screen (#643, #693). It gets its own flow, which requires this one.
