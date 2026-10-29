---
id: cabals/activity
title: Cabal activity
version: 9
milestone: M14
requires: [auth/sign-in]
actors: [A]
flows: []
xcuitest: [apps/mobile/MonacoUITests/Journeys/CabalsActivityJourney.swift, apps/mobile/MonacoUITests/Journeys/CabalsActivityJourneyUITests.swift]
---

# Cabal activity

A member opens "Treasury history" from the cabal's Treasury tab, reads its rows, opens the receipt of a confirmed trade, and retries a failed one.

Old app (`c838bd24`): the cabal screen's `GroupActivitySection.swift` -> "See all" -> `GroupActivityListView.swift` -> a row -> `TransactionDetailView.swift` with "Retry" on a failed trade. Spec: `CabalActivitySlot` in [screens.md](../../screens.md#cabal-screen-cabalroute), slot 12, and its rows open `TransactionRoute`.

The format of this doc is in [App journeys](../README.md).

On the F cabal page the activity list moved: the Treasury tab ends in a "Treasury history" row (`cabal-treasury-history`) that opens the screen with `cabal-activity` and the rows (#6511). There is no "See all" on the cabal page.

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actor A has signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | `apps/mobile/qa/journeys/cabals/activity.setup.sh` ran right before the scenario, through `scripts/qa/seed.sh`. A creates the cabal `QA activity {QA.run}` through the API. The setup then writes six `cabal_activity` rows in SQL, oldest first: four confirmed "Money added" rows of $5.00, one confirmed buy of Apple (`AAPLx`) for $25.00 with a transaction signature, and one failed buy of Apple for $10.00. S2 reuses the cabal S1 seeded in the same run. The setup hands the two buy ids to the test as `confirmed_trade` and `failed_trade` |

The rows come from SQL because no route writes `cabal_activity` without a real swap. Flow 11 (Execute trade) is `planned` and has no seeder in `internal/testkit/flows/`, and the testkit scenario `cabal-with-confirmed-trade` has fixed ids, so it cannot seed a per-run cabal on the dev database. The setup copies that scenario's trade: the `AAPLx` mint, $25.00 in.

## Scenarios

### S1 Read the activity and open a receipt

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | call | `openCabal(named:)` | `QA activity {QA.run}` | `cabal-header-name` reads `QA activity {QA.run}` within 15 s |
| S1.2 | A | call, scroll to, then tap | `openCabalTab("Treasury")`, then `cabal-treasury-history` | | The "Treasury history" screen shows `cabal-activity`, and the newest row `cabal-activity-row-{failed_trade}` reads "Bought Apple" with the red status "Failed" within 15 s |
| S1.3 | A | wait | the rows on "Treasury history" | | All six rows (`cabal-activity-row-<id>`) show, "Money added" among them, within 10 s |
| S1.4 | A | tap | `cabal-activity-row-{confirmed_trade}` | | The "Transaction" receipt shows within 10 s: `cabal-txn-amount` "$25.00", `cabal-txn-status` "Done", and `cabal-txn-solscan` "View on Solscan" |
| S1.5 | A | tap | the back button of the "Transaction" bar | | The "Treasury history" screen shows within 10 s |
| S1.6 | A | scroll to, then tap | a "Money added" row on "Treasury history" | | The "Transaction" receipt shows within 10 s |
| S1.7 | A | tap | the back button of the "Transaction" bar | | "Treasury history" shows within 10 s, the receipt is gone, and no sheet, alert, popover or menu is up |
| S1.8 | A | tap | a "Money added" row on "Treasury history" again | | The "Transaction" receipt shows within 10 s |
| S1.9 | A | tap | the back button of the "Transaction" bar | | The same as S1.7: the history, no prompt |
| S1.10 | A | tap, then tap | the back button of the "Treasury history" bar, then of the cabal screen | | `cabal-header-name` shows, then the Cabals tab shows within 10 s and no `cabal-header-name` is left, so there was one cabal screen on the path |

### S2 Retry a failed trade

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | call | `openCabal(named:)` | `QA activity {QA.run}` | `cabal-header-name` reads `QA activity {QA.run}` within 15 s |
| S2.2 | A | call, then tap | `openCabalTab("Treasury")`, `cabal-treasury-history`, then `cabal-activity-row-{failed_trade}` | | The "Transaction" receipt shows `cabal-txn-status` "Failed" within 10 s |
| S2.3 | A | wait | `cabal-txn-retry` | | The receipt offers "Retry" on the failed trade within 5 s (screens.md: "Failed · Retry") |

## Ground truth

`apps/mobile/qa/journeys/cabals/activity.truth.sh` reads `cabal_activity` for `QA activity {QA.run}`. The run only reads, so the cabal still has its six rows, the confirmed buy is still `confirmed`, and the failed buy is still `failed`: opening a receipt changes nothing.

## Known failures on staging

- S2.3: the receipt has no "Retry" (`cabal-txn-retry`), and the activity row reads "Failed" without "· Retry". The app shows Retry only for a swap that `GET /v1/swaps/{id}` reports as `retryable`, and the seeded failed row has no swap behind it. Retrying a failed trade is blocked by #654.

## Not covered

- The amber "Pending" status. A pending row turns confirmed or failed on its own, so a seeded one is not a stable state to read.
- Paging past the first page of `GET /v1/cabals/{id}/activity` on "Treasury history". Six rows fit on one page.
- The empty state "Nothing yet" / "Money added and trades show up here.". `ActivityModelTests` in `packages/mobile-core` covers it.
