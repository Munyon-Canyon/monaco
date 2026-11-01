---
id: money/cash-out
title: Cash out
version: 5
milestone: M15
requires: [auth/sign-in]
actors: [A]
flows: []
funds:
  A: 2
xcuitest: [apps/mobile/MonacoUITests/Journeys/MoneyCashOutJourney.swift, apps/mobile/MonacoUITests/Journeys/MoneyCashOutJourneyUITests.swift]
---

# Cash out

A member with money in Monaco opens a cabal's "Cash out", picks 25%, cashes out, and lands back on the cabal with a toast. Then they withdraw their account balance to a wallet.

Version 4 (F launch, #6576): no F frame draws Cash out (founder decision Q26). It keeps today's `AmountEntry` with the restyled keys and the presets "25%", "50%" and "All", and its review step. S2 is `MoneyWithdrawJourney.withdrawAll`, the one-screen Withdraw.

Old app (`c838bd24`): the cabal screen -> "Sell" -> `Redeem/SellCabalView.swift` with "25%", "50%", "All" and "Cash out $X", which pops back with a toast. Then Withdraw to wallet. Spec: "Money: Cash out" (`CashOutRoute`) and "Withdraw" (`WithdrawRoute`) in [screens.md](../../screens.md).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actor A has signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | Whoever runs the journey sent A 2 USDC from the QA pot (`monacoctl qa fund`) or the Phantom MCP agent wallet, as [Journeys that move money](../README.md#journeys-that-move-money) says, and set `MONACO_QA_REFUND_ADDRESS` |
| P4 | `apps/mobile/qa/journeys/money/cash-out.setup.sh` ran right before the scenario, through `scripts/qa/seed.sh`. For S1, A creates the cabal `QA cash out {QA.run}` through the API. No route funds a cabal on staging (#651), so A has no slice in it yet |

## Scenarios

### S1 Cash out part of a slice

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | wait | `platform-balance-value` on Home | | The account balance reads more than "$0.00" within 60 s: the run is funded |
| S1.2 | A | tap, type, tap, then tap | the Cabals tab, `cabals-search-field`, the `cabals-search-result-<id>`, then `cabal-treasury-cash-out` | `QA cash out {QA.run}` | The "Cash out your share" screen shows `cash-out-amount` with "25%", "50%" and "All", and `cash-out-explainer` reads "We sell this much of your slice and move the cash to your account balance. You stay in the cabal." within 10 s |
| S1.3 | A | wait | the helper under `cash-out-amount` | | The helper reads "$… current share value" within 10 s |
| S1.4 | A | tap | "25%" | | `cash-out-review` is enabled and reads "Review cash-out" within 5 s |
| S1.5 | A | tap, then tap | `cash-out-review`, then `cash-out-submit` | | The toast "Cashing out $…. The final amount is known when it settles" shows and the outcome page (`cash-out-progress`) shows within 10 s |

### S2 Withdraw to a wallet

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | run | `MoneyWithdrawJourney.withdrawAll` (Settings -> `settings-withdraw`, address, `amount-keypad-preset-Max`, `amount-keypad-action`) | `{QA.refund_address}` | `amount-keypad-settled` reads "✓ $… to" and the short address within 10 s, and `amount-keypad-line` reads "Sent just now · arrives on Solana in about a minute" |

## Ground truth

`apps/mobile/qa/journeys/money/cash-out.truth.sh` reads the database. A is still a member of `QA cash out {QA.run}`: a cash out keeps you in the cabal. No cash out ever reached the database for that cabal, because the screen cannot submit yet (#657). Once #657 lands, the check must find one `cash_out` row in `cabal_activity`, and this journey goes up a version.

## Known failures on staging

- S1.3, S1.4 and S1.5: the screen shows "Your slice shows up here soon." and "Cash outs open soon.", the chips are disabled, and "Cash out" never submits. Cashing out a slice and the balance growing are blocked by #657 and #653.

## Not covered

- "Too small to cash out" under $0.10, and "Nothing to cash out yet" / "Fund this cabal first. Your slice shows up here." for a member with no slice. Both need the live screen (#657).
- Inline address errors on Withdraw. `money/withdraw` covers them.
