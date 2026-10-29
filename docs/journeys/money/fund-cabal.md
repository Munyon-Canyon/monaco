---
id: money/fund-cabal
title: Fund this cabal
version: 9
milestone: M12
requires: [auth/sign-in]
actors: [A]
flows: []
funds:
  A: 2
xcuitest: [apps/mobile/MonacoUITests/Journeys/MoneyFundCabalJourney.swift, apps/mobile/MonacoUITests/Journeys/MoneyFundCabalJourneyUITests.swift]
---

# Fund this cabal

A member opens a cabal, taps "Fund", types an amount on the keypad from their cash, and funds the pooled treasury with it. The money rules are in [Architecture](../../architecture.md).

Version 9 (F launch, #6576): Fund is the Phantom keypad (`amount-keypad-*`): a source pill, a big amount, one line, the chips "+$1 · +$10 · +$20 · Max" and plain keys. There is no yield note and no success sheet: the primary button turns into the settled "✓ $1 to <cabal>" in place.

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
| S1.1 | A | open | the cabal `QA fund {QA.run}` (`openCabal(named:)`) | | `cabal-header-name` reads `QA fund {QA.run}` within 15 s |
| S1.2 | A | tap | `cabal-action-fund` "Fund" | | `amount-keypad-screen` shows within 15 s; `amount-keypad-source` names the cabal and reads "Cash $…"; `amount-keypad-amount` reads "$0"; the chips `amount-keypad-preset-+$1`, `-+$10`, `-+$20` and `-Max` show; no `amount-keypad-action` |
| S1.3 | A | wait | `amount-keypad-line` | | The line reads "From your cash · your share …" within 5 s |
| S1.4 | A | tap | `amount-keypad-1`, `amount-keypad-0`, `amount-keypad-0` | `100` | `amount-keypad-limit-line` reads "… available · … over" within 5 s; the chips and `amount-keypad-action` are gone |
| S1.5 | A | tap | `amount-keypad-delete` three times, then `amount-keypad-1` | `1` | `amount-keypad-action` reads "Fund $1" within 5 s |

### S2 Add to the pot

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | open | the cabal `QA fund {QA.run}` (`openCabal(named:)`) | | `cabal-header-name` reads `QA fund {QA.run}` within 15 s |
| S2.2 | A | tap, then tap | `cabal-action-fund`, then `amount-keypad-1` | `1` | `amount-keypad-action` "Fund $1" is enabled within 15 s |
| S2.3 | A | tap, then tap | `amount-keypad-action`, then `amount-keypad-close` | | `amount-keypad-settled` reads "✓ $1 to QA fund {QA.run}" and `amount-keypad-line` reads "Added just now · your share is now …%" within 120 s. Close returns to the cabal page, and `cabal-pot-value` ("Treasury") reads "$1.00" within 30 s. No toast: the settled button replaces it while the screen is open |

## Ground truth

`apps/mobile/qa/journeys/money/fund-cabal.truth.sh` reads `cabals`, `cabal_members` and `user_txns`. A is a member of `QA fund {QA.run}`. Every `fund` row for A into that cabal is for $1 (`-1000000` wallet micros) and none is `failed`.

## Known failures on staging

None known.

## Not covered

- "Add money first" for a member with nothing in their balance (`fund-cabal-needs-money`). The journey's actor is funded; `FundCabalStageTests` covers the empty stage.
- The "· $300.00 pending" helper while a fund or withdrawal is on its way. That needs a fund to be running, which S2 cannot start today.
- The pause notice on a paused cabal. It belongs to #651.
