---
id: money/fund-cabal
title: Fund this cabal
version: 10
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

Version 10 (UI/UX v5, #7937, #7952): the keypad line is two sentences ("From your cash. Your share goes from 35% to 36%."), and A's first fund into a cabal ends on the full-screen welcome `FirstFundView` ("You’re in.", `first-fund-done`) over the settled keypad. Version 9 (F launch, #6576): Fund is the Phantom keypad (`amount-keypad-*`): a source pill, a big amount, one line, the chips "+$1 · +$10 · +$20 · Max" and plain keys. There is no yield note and no success sheet: the primary button turns into the settled "✓ $1 to <cabal>" in place.

Old app (`c838bd24`): Cabal -> "Add money" -> `Deposit/FundCabalView.swift`: the amount, the presets, "Deposit $X.00", the toast, and the sweep status. Spec: **Fund this cabal** in [screens.md](../../screens.md) (Money).

Frames: `docs/design/v5/screens/09-fund-cabal.png` and `docs/design/v5/screens/15-first-fund.png`.

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
| S1.3 | A | wait | `amount-keypad-line` | | Within 5 s `amount-keypad-line` reads "From your cash. Your share is " and a percent with a full stop (nothing typed yet; A holds no share in the new cabal, so it reads "0%."), or exactly "From your cash" when the share is unavailable; it contains no "·" and no "→" |
| S1.4 | A | tap | `amount-keypad-1`, `amount-keypad-0`, `amount-keypad-0` | `100` | `amount-keypad-limit-line` reads "… available · … over" within 5 s; the chips and `amount-keypad-action` are gone |
| S1.5 | A | tap | `amount-keypad-delete` three times, then `amount-keypad-1` | `1` | `amount-keypad-line` reads "From your cash. Your share goes from " … " to " … "%." within 5 s, and `amount-keypad-action` reads "Fund $1" within 5 s |

### S2 Add to the pot

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | open | the cabal `QA fund {QA.run}` (`openCabal(named:)`) | | `cabal-header-name` reads `QA fund {QA.run}` within 15 s |
| S2.2 | A | tap, then tap | `cabal-action-fund`, then `amount-keypad-1` | `1` | `amount-keypad-action` "Fund $1" is enabled within 15 s |
| S2.3 | A | tap | `amount-keypad-action` | | Within 120 s either `first-fund` shows (A's first fund into this cabal, `FirstFundView`) or `amount-keypad-settled` reads "✓ $1 to QA fund {QA.run}". For this journey it is `first-fund`: the step fails if the keypad settles without it, because the seed guarantees a first fund. The settled button may sit under the cover, so the step does not read `amount-keypad-line` behind it |
| S2.4 | A | read, then tap | `first-fund-card`, then `first-fund-done` | | `first-fund-card` reads "You’re in." and "$1 in QA fund {QA.run}" followed by A's share, and contains no "of the treasury"; with A alone in the cabal it has no "with …" line, there is no `first-fund-vote` (no open vote) and no text "You put in" (the stat grid is gone). Done returns to the cabal page within 15 s, and `cabal-pot-value` ("Treasury") reads "$1.00" within 30 s |

## Ground truth

`apps/mobile/qa/journeys/money/fund-cabal.truth.sh` reads `cabals`, `cabal_members` and `user_txns`. A is a member of `QA fund {QA.run}`. Every `fund` row for A into that cabal is for $1 (`-1000000` wallet micros) and none is `failed`.

## Known failures on staging

- S2.4: "of the treasury" is no longer app copy; "You put in" is no longer app copy. Blocked by #6198.
## Not covered

- "Add money first" for a member with nothing in their balance (`fund-cabal-needs-money`). The journey's actor is funded; `FundCabalStageTests` covers the empty stage.
- The "· $300.00 pending" helper while a fund or withdrawal is on its way. That needs a fund to be running, which S2 cannot start today.
- The faces row and "Your first vote" on the welcome card. A is alone in a cabal with no open vote; `FirstFundModelTests` covers `othersLine` and the vote row.
- The pause notice on a paused cabal. It belongs to #651.
