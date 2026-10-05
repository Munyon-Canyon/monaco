---
id: money/fund-cabal
title: Fund this cabal
version: 1
milestone: M12
requires: [auth/sign-in]
actors: [A]
flows: []
funds:
  A: 2
xcuitest: [apps/mobile/MonacoUITests/Journeys/MoneyFundCabalJourney.swift, apps/mobile/MonacoUITests/Journeys/MoneyFundCabalJourneyUITests.swift]
---

# Fund this cabal

A member opens a cabal, taps "Add money", picks an amount from their account balance, and adds it to the pot. The money rules are in [Architecture](../../architecture.md).

Old app (`c838bd24`): Cabal -> "Add money" -> `Deposit/FundCabalView.swift`: the amount, the presets, "Add $X to the pot", the toast, and the sweep status. Spec: **Fund this cabal** in [screens.md](../../screens.md) (Money).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actor A has signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | The runner sent A 2 USDC on Solana from the QA pot (`monacoctl qa fund`) or the Phantom MCP agent wallet, per `funds`, so A's account balance is more than $1 and less than $100 |
| P4 | `apps/mobile/qa/journeys/money/fund-cabal.setup.sh` ran right before the scenario. Through `scripts/qa/seed.sh` it marks A as done with onboarding, and A creates the open cabal `QA fund {QA.run}` through the API |

## Scenarios

### S1 Pick an amount

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `QA fund {QA.run}` | `cabal-header-name` reads `QA fund {QA.run}` within 15 s |
| S1.2 | A | tap | `cabal-action-fund` "Add money" | | The screen titled "Fund this cabal" shows `amount-entry-field`, the presets "$25", "$50", "$100" and "Max", and `amount-entry-helper` reads "… available" within 15 s (old app: `FundCabalView.swift` amount and presets) |
| S1.3 | A | wait | `fund-cabal-treasury-note` | | "The money leaves your account balance and joins the QA fund {QA.run} pot. Your slice grows by the same amount." and "To add money to this cabal, use Fund. Sending USDC straight to the treasury will be returned and pauses the cabal's trading." show within 5 s |
| S1.4 | A | tap | the preset "$100" | | `amount-entry-helper` reads "Not enough in your account balance." within 5 s |
| S1.5 | A | tap, then type | `amount-entry-field` | clear, then `1` | The button reads "Add $1 to the pot" within 5 s (old app: `FundCabalView.swift` "Add $X to the pot") |

### S2 Add to the pot

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `QA fund {QA.run}` | `cabal-header-name` reads `QA fund {QA.run}` within 15 s |
| S2.2 | A | tap, tap, then type | `cabal-action-fund`, then `amount-entry-field` | `1` | The button "Add $1 to the pot" is enabled within 15 s |
| S2.3 | A | tap | "Add $1 to the pot" | | The toast "Funding this cabal…" shows within 10 s, then "Added $1 to QA fund {QA.run}." within 120 s (old app: `FundCabalView.swift` toast and sweep status) |

## Ground truth

`apps/mobile/qa/journeys/money/fund-cabal.truth.sh` reads `cabals`, `cabal_members` and `user_txns`. A is a member of `QA fund {QA.run}`. Every `fund` row for A into that cabal is for $1 (`-1000000` wallet micros) and none is `failed`.

## Known failures on staging

- S2.2 and S2.3: "Add $1 to the pot" is disabled under "Funding opens soon.", so the pot cannot grow. Blocked by #608 (the fund route) and #651 (the Fund screen's submit).

## Not covered

- "Add money first" for a member with nothing in their balance (`fund-cabal-needs-money`). The journey's actor is funded; `FundCabalStageTests` covers the empty stage.
- The "· $300.00 funding" helper while a fund is in flight. That needs a fund to be running, which S2 cannot start today.
- The pause notice on a paused cabal. It belongs to #651.
