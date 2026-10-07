---
id: cabals/activity
title: Cabal activity
version: 3
milestone: M14
requires: [auth/sign-in]
actors: [A]
flows: []
xcuitest: [apps/mobile/MonacoUITests/Journeys/CabalsActivityJourney.swift, apps/mobile/MonacoUITests/Journeys/CabalsActivityJourneyUITests.swift]
---

# Cabal activity

A member reads a cabal's Activity section, opens "See all", opens the receipt of a confirmed trade, and retries a failed one.

Old app (`c838bd24`): the cabal screen's `GroupActivitySection.swift` -> "See all" -> `GroupActivityListView.swift` -> a row -> `TransactionDetailView.swift` with "Retry" on a failed trade. Spec: `CabalActivitySlot` in [screens.md](../../screens.md#cabal-screen-cabalroute), slot 12, and its rows open `TransactionRoute`.

The format of this doc is in [App journeys](../README.md).

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
| S1.1 | A | tap, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `QA activity {QA.run}` | `cabal-header-name` reads `QA activity {QA.run}` within 15 s |
| S1.2 | A | scroll to | `cabal-activity` | | The section header reads "Activity" with "See all" (`cabal-activity-see-all`), and the newest row `cabal-activity-row-{failed_trade}` reads "Bought Apple" with the red status "Failed" within 15 s |
| S1.3 | A | tap | `cabal-activity-see-all` | | The "Activity" screen lists all six rows (`cabal-activity-row-<id>`), "Money added" among them, within 10 s |
| S1.4 | A | tap | `cabal-activity-row-{confirmed_trade}` | | The "Transaction" receipt shows within 10 s: `cabal-txn-amount` "$25.00", `cabal-txn-status` "Done", and `cabal-txn-solscan` "View on Solscan" |

### S2 Retry a failed trade

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `QA activity {QA.run}` | `cabal-header-name` reads `QA activity {QA.run}` within 15 s |
| S2.2 | A | scroll to, then tap the leading glyph of | `cabal-activity-row-{failed_trade}`, clear of its inline "Retry" | | The "Transaction" receipt shows `cabal-txn-status` "Failed" within 10 s |
| S2.3 | A | tap | "Retry" | | The receipt offers "Retry" on the failed trade within 5 s (old app: `TransactionDetailView.swift` `transaction-detail-retry`; screens.md: "Failed · Retry") |

## Ground truth

`apps/mobile/qa/journeys/cabals/activity.truth.sh` reads `cabal_activity` for `QA activity {QA.run}`. The run only reads, so the cabal still has its six rows, the confirmed buy is still `confirmed`, and the failed buy is still `failed`: opening a receipt changes nothing.

## Known failures on staging

- S2.3: the receipt has no "Retry", and the activity row reads "Failed" without "· Retry". Retrying a failed trade is blocked by #654.

## Not covered

- The amber "Pending" status. A pending row turns confirmed or failed on its own, so a seeded one is not a stable state to read.
- "Scout · Bought Nvidia", an agent's trade. Agents have no route on staging.
- Paging past the first page of `GET /v1/cabals/{id}/activity` in the "See all" list. Six rows fit on one page.
- The empty state "Nothing yet" / "Money added and trades show up here.". `ActivityModelTests` in `packages/mobile-core` covers it.
