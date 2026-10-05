---
id: settings/delete-account
title: Delete your account
version: 2
milestone: M9
requires: []
actors: [C]
flows: [01e]
xcuitest: [apps/mobile/MonacoUITests/Journeys/SettingsDeleteAccountJourney.swift, apps/mobile/MonacoUITests/Journeys/SettingsDeleteAccountJourneyUITests.swift]
---

# Delete your account

A member opens Settings, reads what deleting their account removes, checks the cash-out and withdraw checklist, backs out once, and then deletes the account. A second member who still sits in a cabal sees that cabal on the checklist. The design is Settings and "Delete account" in the [Profile tab](../../screens.md#profile-tab) section, flow `01e`, and [`AccountCopy`](https://github.com/Munyon-Canyon/monaco/blob/staging/packages/mobile-core/Sources/MonacoCore/AccountCopy.swift). The old app at `c838bd24` had no way to delete an account, so every step is new in the spec.

The format of this doc is in [App journeys](../README.md). The Old app column names the old app's tap or element for each step, or says the step is new in the spec.

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Each scenario deletes or inspects a throwaway account, never a Privy test login. Before each scenario, `apps/mobile/qa/journeys/settings/delete-account.setup.sh` makes a new dev user with `bin/monacoctl dev token --user new`, moves it to `ONBOARDING_COMPLETED` with the `dev_` handle it was made with, and names it `QA delete {QA.run}` through the API. It hands the test the dev token as `devToken` and the user id as `devUserID` |
| P2 | The dev user has no platform balance and no cabal, except in S3 |
| P3 | Before S3, the setup script also creates a cabal named `QA delete pot {QA.run}` with the dev user's token, so the dev user is its only member. It hands the test the cabal's id as `cabalID` and its name as `cabalName` |
| P4 | The test launches the app with `MONACO_DEV_TOKEN` set to `devToken`. The simulator is actor C's, so C may still be signed in from another journey; the first step signs C out |

## Scenarios

### S1 Read the checklist and back out

Starts with a new dev user (P1, P2).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S1.1 | launch, sign out when signed in, then tap | the app, `profileSignOutButton` and `profile-sign-out-confirm` when C is signed in, then `devSignInButton` | | The tab bar shows on Home within 30 s | None, new in spec |
| S1.2 | tap, then tap | the Profile tab, then `profile-settings-row` | | The "Settings" screen shows within 10 s, and `settings-delete-account` reads "Delete account" | None, new in spec |
| S1.3 | tap | `settings-delete-account` | | Within 10 s, the "Delete account" screen shows, and `delete-account-explainer` reads "Deleting your account removes your name, photo, phone and X from Monaco. Your handle stays reserved. Your transaction history stays, because cabal records need it. This can't be undone." | None, new in spec |
| S1.4 | wait | `delete-account-step-cash-out`, `delete-account-step-withdraw` | | Within 15 s, "Cash out of every cabal" reads "Done" and "No cabal holds money of yours.", and "Withdraw your balance" reads "Done" and "$0.00" | None, new in spec |
| S1.5 | tap | `delete-account-button` | | Within 5 s the confirm shows "Delete your Monaco account?", with `delete-account-confirm` ("Delete"). On iOS 27 the confirm is a popover that shows no Cancel button | None, new in spec |
| S1.6 | tap | outside the dialog | | The confirm closes within 5 s, and `delete-account-explainer` still shows | None, new in spec |

### S2 Delete the account

Starts with a new dev user (P1, P2).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S2.1 | launch, sign out when signed in, tap, then open | the app, `devSignInButton`, then the Profile tab, `profile-settings-row` and `settings-delete-account` | | `delete-account-explainer` shows within 30 s, and "Withdraw your balance" reads "Done" | None, new in spec |
| S2.2 | tap, then tap | `delete-account-button`, then `delete-account-confirm` | | Within 20 s, `monaco-toast-banner` reads "Your account was deleted.", and the login form shows with `devSignInButton` | None, new in spec |

### S3 A cabal on the checklist

Starts with a new dev user who is the only member of a cabal (P3).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S3.1 | launch, sign out when signed in, tap, then open | the app, `devSignInButton`, then the Profile tab, `profile-settings-row` and `settings-delete-account` | | `delete-account-explainer` shows within 30 s | None, new in spec |
| S3.2 | wait | `delete-account-cabal-{cabalID}` | | Within 15 s, "Cash out of every cabal" does not read "Done", and the row reads `cabalName` | None, new in spec |
| S3.3 | wait | `delete-account-step-cash-out` | | The step shows the member's slice in `cabalName`, not "Your slice in each cabal shows up here soon." | None, new in spec |

## Ground truth

After a run, S2's dev user is deleted: its `users` row has `account_status = 'deleted'`, `deleted_at` set and an empty `display_name`, and it keeps its handle. S1's and S3's dev users are not deleted. `apps/mobile/qa/journeys/settings/delete-account.truth.sh` reads the user ids the setup script wrote to the hand-off file and checks those rows through `apps/mobile/qa/journeys/psql.sh`.

## Known failures on staging

| Step | What fails | Blocked by |
| --- | --- | --- |
| S3.3 | The cash-out step reads "Your slice in each cabal shows up here soon.", because no route serves the member's slice | #2136 |

## Not covered

- A delete the backend refuses with "Cash out of every cabal first." or "Withdraw your balance first.". It needs a stake or a balance, which only real USDC makes. Flow `01e` covers both refusals with fixture data, and `DeleteAccountModelTests` covers the highlighted step.
- Tapping a cabal row or the "Account balance" row on the checklist. They open `CashOutRoute` and `WithdrawRoute`, which `money/cash-out` and `money/withdraw` cover.
- "Deleting…" on the button. The local backend answers before a poll sees it.
- "Couldn't load your account." with "Try again". The local backend cannot be made to fail one read on cue; `DeleteAccountModelTests` covers it.
