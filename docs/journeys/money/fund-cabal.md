---
id: money/fund-cabal
title: Fund this cabal
version: 7
milestone: M12
requires: [auth/sign-in]
actors: [A]
flows: []
funds:
  A: 2
xcuitest: [apps/mobile/MonacoUITests/Journeys/MoneyFundCabalJourney.swift, apps/mobile/MonacoUITests/Journeys/MoneyFundCabalJourneyUITests.swift]
---

# Fund this cabal

A member opens a cabal, taps "Deposit", picks an amount from their account balance, and deposits it into the pot. The money rules are in [Architecture](../../architecture.md).

Old app (`c838bd24`): Cabal -> "Add money" -> `Deposit/FundCabalView.swift`: the amount, the presets, "Deposit $X.00", the toast, and the sweep status. Spec: **Fund this cabal** in [screens.md](../../screens.md) (Money).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actor A has signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | The runner sent A 2 USDC on Solana from the QA pot (`monacoctl qa fund`) or the Phantom MCP agent wallet, per `funds`, so A's account balance is more than $1 and less than $100 |
| P4 | `apps/mobile/qa/journeys/money/fund-cabal.setup.sh` ran right before the scenario. Through `scripts/qa/seed.sh` it marks A as done with onboarding, and A creates the cabal `QA fund {QA.run}` through the API |

## Scenarios

### S1 Pick an amount

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `QA fund {QA.run}` | `cabal-header-name` reads `QA fund {QA.run}` within 15 s |
| S1.2 | A | tap | `cabal-action-fund` "Deposit" | | The screen titled "Deposit to <cabal>" shows `amount-entry-field`, the presets "$25", "$50", "$100" and "Max", and `amount-entry-helper` reads "… available" within 15 s (old app: `FundCabalView.swift` amount and presets) |
| S1.3 | A | wait | `fund-cabal-yield-note` | | "Your cabal earns yield on its USDC. Cash it holds earns 3.5% APY." shows within 5 s |
| S1.4 | A | tap | the preset "$100" | | `amount-entry-helper` reads "Not enough in your account balance." within 5 s |
| S1.5 | A | tap, then type | `amount-entry-field` | clear, then `1` | The button reads "Deposit $1.00" within 5 s (screens.md: the primary reads "Deposit $250.00") |

### S2 Add to the pot

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `QA fund {QA.run}` | `cabal-header-name` reads `QA fund {QA.run}` within 15 s |
| S2.2 | A | tap, tap, then type | `cabal-action-fund`, then `amount-entry-field` | `1` | The button "Deposit $1.00" is enabled within 15 s |
| S2.3 | A | tap, then tap | "Deposit $1.00", then `fund-cabal-done-button` "Done" | | The sheet `fund-cabal-success-sheet` titled "Deposit confirmed" shows within 120 s; Done closes it and the cabal screen shows, then the toast "Added $1 to QA fund {QA.run}." within 120 s (screens.md: the glass sheet "Deposit confirmed" with "Done") |

## Ground truth

`apps/mobile/qa/journeys/money/fund-cabal.truth.sh` reads `cabals`, `cabal_members` and `user_txns`. A is a member of `QA fund {QA.run}`. Every `fund` row for A into that cabal is for $1 (`-1000000` wallet micros) and none is `failed`.

## Known failures on staging

None known.

## Not covered

- "Add money first" for a member with nothing in their balance (`fund-cabal-needs-money`). The journey's actor is funded; `FundCabalStageTests` covers the empty stage.
- The "· $300.00 pending" helper while a fund or withdrawal is on its way. That needs a fund to be running, which S2 cannot start today.
- The pause notice on a paused cabal. It belongs to #651.
