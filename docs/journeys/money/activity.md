---
id: money/activity
title: Account activity
version: 2
milestone: M12
requires: [auth/sign-in]
actors: [A]
flows: []
xcuitest: [apps/mobile/MonacoUITests/Journeys/MoneyActivityJourney.swift, apps/mobile/MonacoUITests/Journeys/MoneyActivityJourneyUITests.swift]
---

# Account activity

A member opens Activity from Settings, sees their deposits, withdrawals and cabal moves, and opens one as a receipt with its status, time, a Solscan link and, for a cabal move, the cabal.

Old app (`c838bd24`): the per-cabal `Groups/TransactionDetailView.swift` list -> row -> receipt -> "View on Solscan" / open the cabal. The rewrite moves the list to Settings -> "Activity". Spec: **Account activity** in [screens.md](../../screens.md) (Money).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actor A has signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | `apps/mobile/qa/journeys/money/activity.setup.sh` ran right before the scenario, through `scripts/qa/seed.sh`. It marks A as done with onboarding. For S1 it writes a settled $1.23 deposit for A to `user_txns` with SQL, because only a real USDC transfer reaches the deposit poller, and hands its id to the test as `deposit-txn`. For S2, A creates the cabal `QA activity {QA.run}` through the API. The journey moves no money |

## Scenarios

### S1 Open a deposit receipt

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap, tap, then scroll to and tap | the Profile tab, `profile-settings-row`, then `settings-activity` | | The screen titled "Activity" shows `account-activity-row-<deposit-txn>`, which reads "Deposit" and "$1.23", within 15 s (old app: `TransactionDetailView.swift` list) |
| S1.2 | A | tap | `account-activity-row-<deposit-txn>` | | The receipt titled "Deposit" shows `account-txn-receipt-amount` with "$1.23", `account-txn-receipt-status` with "Done", `account-txn-receipt-time`, and `account-txn-receipt-solscan` "View on Solscan" within 10 s (old app: the receipt's Solscan link) |
| S1.3 | A | tap | `account-txn-receipt-done` "Done" | | The receipt closes and `account-activity-row-<deposit-txn>` shows within 5 s |

### S2 Open a cabal move

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap, tap, then scroll to and tap | the Profile tab, `profile-settings-row`, then `settings-activity` | | A row "Funded QA activity {QA.run}" shows within 15 s |
| S2.2 | A | tap | the row "Funded QA activity {QA.run}" | | The receipt shows `account-txn-receipt-cabal` with "QA activity {QA.run}" within 10 s |
| S2.3 | A | tap | `account-txn-receipt-cabal` | | The cabal screen shows, and `cabal-header-name` reads `QA activity {QA.run}` within 15 s (old app: open the cabal from the receipt) |

### S3 See a withdrawal

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S3.1 | A | tap, tap, then scroll to and tap | the Profile tab, `profile-settings-row`, then `settings-activity` | | A row "Withdrawal" shows within 15 s |

## Ground truth

`apps/mobile/qa/journeys/money/activity.truth.sh` reads `user_txns`. The deposit S1 seeded is still there, `settled`, for $1.23 into A's wallet: opening the receipt changed nothing.

## Known failures on staging

- S2.1 to S2.3: no route writes a `fund` row, so there is no "Funded …" row to open. Blocked by #651 (and the fund route, #608). Once it lands, the setup funds `QA activity {QA.run}` with $1 through the route, and the journey gains `funds: {A: 1}`.
- S3.1: no route writes a `withdrawal` row. Blocked by #652.

## Not covered

- Opening "View on Solscan". It leaves the app for Safari; S1.2 checks the link is there.
- Pending and failed rows ("Pending" amber, "Failed" red). No route reaches those states without moving money; `AccountActivityRowTests` covers the labels.
- The empty state "No activity yet". A's history is never empty after the deposit S1 seeds.
- Paging past 30 rows.
