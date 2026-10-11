---
id: money/withdraw
title: Withdraw
version: 3
milestone: M12
requires: [auth/sign-in]
actors: [A, B]
flows: []
funds:
  A: 1
  B: 1
xcuitest: [apps/mobile/MonacoUITests/Journeys/MoneyWithdrawJourney.swift, apps/mobile/MonacoUITests/Journeys/MoneyWithdrawJourneyUITests.swift]
---

# Withdraw

A member sends USDC from their account balance to an address outside Monaco. They open Withdraw from Home, pick a network, tap "Max", paste an address, check it on Confirm and tap "Withdraw". The rules are [Deposits and withdrawals](../../architecture/deposits-withdrawals.md), and a withdrawal to Base, Arbitrum or Ethereum is [Withdraw to an EVM wallet](../../architecture/deposits-withdrawals.md#withdraw-to-an-evm-wallet).

Version 3 (M24, #4973): Withdraw has a network picker (Solana, Base, Arbitrum, Ethereum), checks the address for the network picked, shows "You'll receive ~$X" for an EVM network, and Confirm names the network. S1 checks the picker starts on Solana and Confirm's "Network" row. S2 walks a Base withdrawal up to Confirm, then sends B's balance back on Solana.

Old app (`c838bd24`): Home -> Withdraw -> `Settings/WithdrawView.swift`: the amount entry with "Max", "Send to" with the address field and its inline errors, "Continue" -> the confirm step -> the toast. Spec: **Withdraw** under Money in [screens.md](../../screens.md).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and B have each signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | The runner funded A and B with 1 USDC each from the QA pot (`monacoctl qa fund`) or the Phantom MCP agent wallet, and exported `MONACO_QA_REFUND_ADDRESS` |
| P4 | `apps/mobile/qa/journeys/money/withdraw.setup.sh` ran right before each scenario. It marks the scenario's actor as done with onboarding and sets their display name |
| P5 | The backend reaches Privy, which quotes S2's Base estimate (`POST /v1/me/withdrawals/preview`) |

## Scenarios

### S1 Withdraw the whole balance to the agent wallet

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap | `home-withdraw-link` on Home (old app: Home -> "Withdraw") | | The title "Withdraw" and `amount-entry-helper` reading "available" and not "$0.00 available" within 15 s (screens.md: helper "$850.03 available") |
| S1.2 | A | type | `withdraw-address-field` | `not a solana address` | `withdraw-network-picker` has "Solana" selected, `withdraw-address-problem` shows within 5 s, the caveat "A Solana address that accepts USDC. Transfers can't be undone." shows, and `withdraw-continue-button` "Continue" is disabled (screens.md: "inline address errors") |
| S1.3 | A | tap, clear and type | "Max", then `withdraw-address-field` | `{QA.refund_address}` | `withdraw-address-problem` is gone and `withdraw-continue-button` is enabled within 5 s (old app: "Max" preset, `withdraw-continue-button`) |
| S1.4 | A | tap | `withdraw-continue-button` | | The title "Confirm", "You're withdrawing", the rows "To" with `{QA.refund_address}` unhyphenated, "Network" "Solana", "From" "Account balance", "Arrives" "About a minute", and the caption "Double-check the address. Transfers can't be undone." within 10 s |
| S1.5 | A | tap | `withdraw-confirm-button` "Withdraw" | | The toast "Withdrawing $<amount>. It lands in about a minute." within 10 s (screens.md: Toast "Withdrawing $50.00. It lands in about a minute.") |

### S2 Check a Base withdrawal, then withdraw on Solana

S2 stops at Confirm for Base and never taps "Withdraw" there: `{QA.base_address}` is `0x000000000000000000000000000000000000dEaD`, a valid Base address that nobody can spend from. The real Base withdrawal is in [Verify multichain money](../../how-to/verify-multichain.md).

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | B | tap, then tap | `home-withdraw-link` on Home, then "Base" in `withdraw-network-picker` | | The title "Withdraw" and `amount-entry-helper` reading "available" and not "$0.00 available" within 15 s. After "Base", the caveat "A Base address that accepts USDC. Transfers can't be undone." shows within 5 s |
| S2.2 | B | type | `withdraw-address-field` | `not a base address` | `withdraw-address-problem` reads "Check the address. It doesn't look like a Base address." within 5 s, and `withdraw-continue-button` is disabled |
| S2.3 | B | tap, clear and type | "Max", then `withdraw-address-field` | `{QA.base_address}` | `withdraw-address-problem` is gone, `withdraw-receive` reads "You'll receive ~$" within 15 s, and `withdraw-continue-button` is enabled |
| S2.4 | B | tap | `withdraw-continue-button` | | The title "Confirm" with the rows "To" with `{QA.base_address}` unhyphenated and "Network" "Base", and `withdraw-confirm-button` enabled, within 10 s. B does not tap it |
| S2.5 | B | back out, then run | Back to Home, then `MoneyWithdrawJourney.withdrawAll` | `{QA.refund_address}` | The steps of S1 pass for B on Solana, ending with the toast "Withdrawing $<amount>. It lands in about a minute." |

## Ground truth

`apps/mobile/qa/journeys/money/withdraw.truth.sh` reads `withdrawals`. A and B each have a row to `{QA.refund_address}` from the last 15 minutes, and B has no row to `{QA.base_address}`: S2 never sent on Base.

## Known failures on staging

None known.

## Not covered

- The "Not enough in your account balance." helper for an amount over the balance. `WithdrawFormTests` covers the copy.
- The member's own deposit address as a destination. `SolanaAddress` tests cover it.
- A Base withdrawal that really sends, and its "Withdrawal complete" toast. It needs a Base address Monaco can take the money back from, so [Verify multichain money](../../how-to/verify-multichain.md) covers it, sending to another member's Base deposit address.
- Arbitrum and Ethereum. They take the same path as Base with another name. `WithdrawNetworkTests` covers their copy.
- A mistyped checksum ("That address has a typo in it. Copy it again."). `WithdrawNetworkTests` covers it.
- The Solscan link on the account activity receipt once the withdrawal confirms. That belongs to the activity journey (#2138).
