---
id: onboarding/first-run
title: First run
version: 1
milestone: M9
requires: [auth/sign-in]
actors: [C]
flows: [01a, 01b, 01c, 01d]
xcuitest: [apps/mobile/MonacoUITests/Journeys/FirstRunJourney.swift, apps/mobile/MonacoUITests/Journeys/FirstRunJourneyUITests.swift]
---

# First run

A new member signs in, picks a handle, and skips the phone and X steps to reach Home. Home then nudges them to add a number. They try a number that belongs to someone else, relaunch and stay past the gate, then link a number of their own. A new dev user links a fake X account and the nudge goes away. The design is [Navigation](../../screens.md#navigation), and the states come from [`auth_state`](../../architecture/auth.md#auth_state).

The format of this doc is in [App journeys](../README.md). The Old app column names the old app's tap or element at `c838bd24` for each step, or says the step is new in the spec. The old first run was one screen, `Onboarding/OnboardingNameView.swift`.

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Everything [auth/sign-in](../auth/sign-in.md) needs. Actor C signs in by email, whatever `--channel` says, so C's Privy user starts with no phone |
| P2 | Before S1, `apps/mobile/qa/journeys/onboarding/first-run.setup.sh` makes C a new member: C's `users` row gets `handle = null`, `auth_state = 'CREATED'`, no handle-change time and no phone. A first run has no row yet, and signing in makes one |
| P3 | Before S1, the setup script frees `{L.phone}`, the Privy test number kept for linking. When C's Privy user holds it, the script unlinks it through the Privy API. When a user whose handle starts with `dev_` holds it, the script deletes that Privy user. Any other holder stops the run. It also clears the phone on the `users` row that held it |
| P4 | Before S1, the setup script hands the test actor A's handle as `takenHandle`, a handle that is always taken |
| P5 | Actor B has signed in by text at least once, so `{B.phone}` belongs to B's Privy user |
| P6 | Before S2, S3 and S4, the setup script puts C where S1 leaves it: handle `qa_cayman`, `AWAITING_PHONE`, no phone, and `{L.phone}` free. So each scenario also runs alone once S1 has run one time. The test signs C in by email when the app is signed out |
| P7 | Before S5, the setup script makes a new dev user with `bin/monacoctl dev token --user new` and moves it to `AWAITING_SOCIALS`, the state it reaches after linking a phone. It hands the test the dev token as `devToken` and the user id as `devUserID`. A dev user has no Privy session in the app, so it cannot link a phone (Not covered) |

## Scenarios

### S1 Pick a handle and skip both links

Starts signed out, with C a new member (P2).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S1.1 | launch, then sign in by email | the login form, as in [auth/sign-in](../auth/sign-in.md) | `{C.email}`, `{C.code}` | `onboarding-handle-step` shows within 30 s, not the tab bar, with "Pick your handle" and `handle-step-subtext` reading "This is how people find you on Monaco." | Gate `.nameSetup` opened `OnboardingNameView`, "What should friends call you?" |
| S1.2 | type | `handle-step-field` | `takenHandle` (P4) | `handle-step-status` reads "That handle is taken." within 5 s, and `onboarding-handle-step-continue` is disabled | None, new in spec (#693) |
| S1.3 | clear, then type | `handle-step-field` | `qa_cayman` | `handle-step-status` reads "@qa_cayman is available" within 5 s, and `onboarding-handle-step-continue` is enabled | None, new in spec (#693) |
| S1.4 | tap | `onboarding-handle-step-continue` | | `onboarding-phone-step` shows within 10 s with "Add your number" and "We match your contacts to find friends. Your number stays private." | `onboarding-continue` ("Continue") |
| S1.5 | tap | `phone-step-skip` | | `onboarding-socials-step` shows within 10 s with "Connect X" and "Find people you follow on Monaco." | None, new in spec (#694) |
| S1.6 | tap | `socials-step-skip` | | The tab bar shows on Home within 10 s, and `onboarding-nudge` reads "Add your number to find friends" | "Continue" went straight to the tabs |

### S2 A number linked to someone else

Starts from S1 (P6).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S2.1 | tap | `onboarding-nudge-open` | | A sheet shows `onboarding-phone-step` within 5 s with "Add your number", and `phone-step-skip` reads "Not now" | None, new in spec (#694) |
| S2.2 | type, then tap | `phone-step-number-field`, then `phone-step-send-code` | `{B.phone}` | `phone-step-sent-to` reads "Code sent to" and the number within 20 s | None, new in spec (#694) |
| S2.3 | type | `phone-step-code-field` | `{B.code}` | The sixth digit submits the code. `phone-step-caption` reads "This number is linked to another account." within 20 s, and `phone-step-skip` reads "Not now" | None, new in spec (#694) |
| S2.4 | tap | `phone-step-skip` | | The sheet closes within 5 s, and `onboarding-nudge` still reads "Add your number to find friends" | None, new in spec (#694) |

### S3 The gate stays open after a relaunch

Starts from S1 (P6).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S3.1 | relaunch | the app | | The tab bar shows on Home within 30 s, never `onboarding-handle-step` or `onboarding-phone-step`, and `onboarding-nudge` reads "Add your number to find friends" | The gate skipped `.nameSetup` once the name was set |
| S3.2 | tap | the Profile tab | | `profile-handle` reads "@qa_cayman" within 15 s | `profile-header` on the Profile tab |

### S4 Link a number

Starts from S1 (P6), with `{L.phone}` free (P3).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S4.1 | tap, then tap | the Home tab, then `onboarding-nudge-open` | | A sheet shows `onboarding-phone-step` within 5 s with "Add your number" | None, new in spec (#694) |
| S4.2 | type, then tap | `phone-step-number-field`, then `phone-step-send-code` | `{L.phone}` | `phone-step-sent-to` reads "Code sent to" and the number within 20 s | None, new in spec (#694) |
| S4.3 | type | `phone-step-code-field` | `{L.code}` | The sixth digit submits the code. The sheet closes within 20 s, `monaco-toast-banner` reads "Number added.", and `onboarding-nudge` reads "Connect X to find people you follow" | None, new in spec (#694) |

### S5 Link X as a new dev user

Starts with a new dev user at `AWAITING_SOCIALS` (P7). The app launches with `MONACO_DEV_TOKEN` set to `devToken` and `MONACO_FAKE_X=1`, so "Connect X" calls the dev route instead of opening x.com.

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S5.1 | launch, then sign out | the app, then `profileSignOutButton` when C is still signed in | | `devSignInButton` shows on the login form within 30 s | None, new in spec (#694) |
| S5.2 | tap | `devSignInButton` | | The tab bar shows on Home within 30 s, and `onboarding-nudge` reads "Connect X to find people you follow" | None, new in spec (#694) |
| S5.3 | tap | `onboarding-nudge-open` | | A sheet shows `onboarding-socials-step` within 5 s with "Connect X", and `socials-step-skip` reads "Not now" | None, new in spec (#694) |
| S5.4 | tap | `socials-step-connect` | | The sheet closes within 20 s with no web sheet, `monaco-toast-banner` reads "X connected.", and `onboarding-nudge` does not show on Home | None, new in spec (#694) |

## Ground truth

After a run, C's `users` row has `handle = 'qa_cayman'`, `phone_e164 = {L.phone}` and `auth_state = 'AWAITING_SOCIALS'`. The dev user from S5 (`devUserID`) has an `x_username` that starts with `dev_x_` and `auth_state = 'ONBOARDING_COMPLETED'`. `apps/mobile/qa/journeys/onboarding/first-run.truth.sh` checks both rows through `apps/mobile/qa/journeys/psql.sh`.

## Known failures on staging

None.

## Not covered

- The old app's name step, "What should friends call you?" with "Shown on votes, leaderboards and in chat." and "Add a photo (optional)". The spec's first run is handle, phone and X ([Navigation](../../screens.md#navigation)), and the name and photo moved to the Profile tab, which [profile/edit](../profile/edit.md) covers.
- An X link through the real OAuth sheet on x.com. Only a person can complete it.
- A dev user linking a phone. The dev sign-in opens a backend session but no Privy session, and Privy refuses a link without one ("Missing auth token."). So C links the number in S4 and a dev user links X in S5.
- The busy labels "Saving…", "Linking…" and "Connecting…". The local backend answers before a poll sees them. `OnboardingFlowTests` covers the activity states.
- A wrong code. `OnboardingFlowTests` covers the inline error.
