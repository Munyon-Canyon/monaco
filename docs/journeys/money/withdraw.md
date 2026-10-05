---
id: money/withdraw
title: Withdraw
version: 2
milestone: M12
requires: [auth/sign-in]
actors: [A]
flows: []
funds:
  A: 1
xcuitest: [apps/mobile/MonacoUITests/Journeys/MoneyWithdrawJourney.swift, apps/mobile/MonacoUITests/Journeys/MoneyWithdrawJourneyUITests.swift]
---

# Withdraw

A member sends USDC from their account balance to a Solana address outside Monaco. They open Withdraw from Home, tap "Max", paste an address, check it on Confirm and tap "Withdraw". The rules are [Deposits and withdrawals](../../architecture/deposits-withdrawals.md).

Old app (`c838bd24`): Home -> Withdraw -> `Settings/WithdrawView.swift`: the amount entry with "Max", "Send to" with the address field and its inline errors, "Continue" -> the confirm step -> the toast. Spec: **Withdraw** under Money in [screens.md](../../screens.md).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actor A has signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | The runner funded A's member wallet with 1 USDC from the Phantom MCP agent wallet, and exported `MONACO_QA_REFUND_ADDRESS` |
| P4 | `apps/mobile/qa/journeys/money/withdraw.setup.sh` ran right before the scenario. It marks A as done with onboarding and sets A's display name |

## Scenarios

### S1 Withdraw the whole balance to the agent wallet

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap | `home-withdraw-link` on Home (old app: Home -> "Withdraw") | | The title "Withdraw" and `amount-entry-helper` reading "available" and not "$0.00 available" within 15 s (screens.md: helper "$850.03 available") |
| S1.2 | A | type | `withdraw-address-field` | `not a solana address` | `withdraw-address-problem` shows within 5 s, the caveat "A Solana address that accepts USDC. Transfers can't be undone." shows, and `withdraw-continue-button` "Continue" is disabled (screens.md: "inline address errors") |
| S1.3 | A | tap, clear and type | "Max", then `withdraw-address-field` | `{QA.refund_address}` | `withdraw-address-problem` is gone and `withdraw-continue-button` is enabled within 5 s (old app: "Max" preset, `withdraw-continue-button`) |
| S1.4 | A | tap | `withdraw-continue-button` | | The title "Confirm", "You're withdrawing", the rows "To" with `{QA.refund_address}` unhyphenated, "From" "Account balance", "Arrives" "About a minute", and the caption "Double-check the address. Transfers can't be undone." within 10 s |
| S1.5 | A | tap | `withdraw-confirm-button` "Withdraw" | | The toast "Withdrawing $<amount>. It lands in about a minute." within 10 s (screens.md: Toast "Withdrawing $50.00. It lands in about a minute.") |

## Ground truth

`apps/mobile/qa/journeys/money/withdraw.truth.sh` reads back the `withdrawals` row A's run created to `{QA.refund_address}` in the last 15 minutes.

## Known failures on staging

None known.

## Not covered

- The "Not enough in your account balance." helper for an amount over the balance. `WithdrawFormTests` covers the copy.
- The member's own deposit address as a destination. `SolanaAddress` tests cover it.
- The Solscan link on the account activity receipt once the withdrawal confirms. That belongs to the activity journey (#2138).
