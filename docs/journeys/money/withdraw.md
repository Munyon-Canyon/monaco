---
id: money/withdraw
title: Withdraw
version: 8
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

A member sends USDC from their account balance to an address outside Monaco. They open Withdraw from Settings, type an address, tap "Max" on the keypad and tap "Withdraw $…". It is one screen with no Continue and no review step. The rules are [Deposits and withdrawals](../../architecture/deposits-withdrawals.md), and a withdrawal to Base, Arbitrum or Ethereum is [Withdraw to an EVM wallet](../../architecture/deposits-withdrawals.md#withdraw-to-an-evm-wallet).

Version 3 (M24, #4973): Withdraw has a network picker (Solana, Base, Arbitrum, Ethereum), checks the address for the network picked, shows "You'll receive ~$X" for an EVM network, and Confirm names the network. S1 checks the picker starts on Solana and Confirm's "Network" row. S2 walks a Base withdrawal up to Confirm, then sends B's balance back on Solana.

Version 6 (F launch, #6576): Withdraw is one screen: the address field with paste and scan, the network chips (`withdraw-network-<id>`), then the Phantom keypad. The settled button reads "✓ $… to <short address>". `MoneyWithdrawJourney.withdrawAll(to:)` is S1.1, S1.3 and S1.4.

Old app (`c838bd24`): Home -> Withdraw -> `Settings/WithdrawView.swift`: the amount entry with "Max", "Send to" with the address field and its inline errors, "Continue" -> the confirm step -> the toast. Spec: **Withdraw** under Money in [screens.md](../../screens.md).

Version 8 (UI/UX v5, #7982): the fee line reads "$… fee · arrives in about …" with no "USDC", no "≈" and no network name. A valid address shows as the short row `withdraw-address-ending` ("Address ending …"), and a tap on it opens the full address (S1.5). Only the Solana and Base chips are offered.

Frames: `docs/design/v5/screens/09-withdraw.png`.

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
| S1.1 | A | tap | the Profile tab, `profile-settings-row`, then `settings-withdraw` | | `amount-keypad-source` reads "Cash $…" and not "$0.00" within 15 s; `withdraw-address-field` is empty and editable with `withdraw-paste` and `withdraw-scan`; `withdraw-network-solana` is selected, and `withdraw-network-solana` and `withdraw-network-base` are the only `withdraw-network-*` chips |
| S1.2 | A | type | `withdraw-address-field` | `not a solana address` | `withdraw-address-problem` shows within 5 s and no `amount-keypad-action` shows |
| S1.3 | A | clear and type, then tap | `withdraw-address-field`, then `amount-keypad-preset-Max` | `{QA.refund_address}` | `withdraw-address-problem` is gone. After the keyboard closes, `withdraw-address-ending` reads "Address ending " and the last four characters of `{QA.refund_address}`, and the full address is not on screen. After Max, `amount-keypad-line` reads "$… fee · arrives in about " or "No fee · arrives in about " within 15 s, with no "USDC", no "≈" and no "Solana"; `amount-keypad-amount` is above "$0" |
| S1.5 | A | tap | `withdraw-address-ending` | | Runs only in the full journey, not in `MoneyWithdrawJourney.withdrawAll`. Within 5 s `withdraw-address-field` shows with the full `{QA.refund_address}` as its value and no `withdraw-address-problem`. The keyboard closes and the row reads "Address ending …" again |
| S1.4 | A | tap | `amount-keypad-action` "Withdraw $…" | | The button reads "Withdrawal processing · Pending" until the backend confirms, then `amount-keypad-settled` reads "✓ $… to" and the short address within 120 s, and `amount-keypad-line` reads "Sent just now · arrives on Solana in about a minute". There is no review step |

### S2 Check a Base withdrawal, then withdraw on Solana

S2 never sends on Base: `{QA.base_address}` is `0x000000000000000000000000000000000000dEaD`, a valid Base address that nobody can spend from. The real Base withdrawal is in [Verify multichain money](../../how-to/verify-multichain.md).

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | B | tap, tap, tap, then tap | the Profile tab, `profile-settings-row`, `settings-withdraw`, then `withdraw-network-base` | | `amount-keypad-source` reads "Cash $…" within 15 s. After "Base", the address placeholder reads "USDC address on Base" within 5 s, `withdraw-network-base` is selected, and `amount-keypad-line` reads "… fee · arrives in about …". The line names no network, so the chip and the placeholder carry the switch |
| S2.2 | B | type | `withdraw-address-field` | `not a base address` | `withdraw-address-problem` reads "Check the address. It doesn't look like a Base address." within 5 s, and no `amount-keypad-action` shows |
| S2.3 | B | clear and type, then tap | `withdraw-address-field`, then `amount-keypad-preset-Max` | `{QA.base_address}` | `withdraw-address-problem` is gone, `withdraw-address-ending` reads "Address ending dEaD", `amount-keypad-line` reads "$… fee · arrives in about " with no "≈" and no "USDC" within 15 s, and `amount-keypad-action` shows. B does not tap it |
| S2.4 | B | tap, then run | `amount-keypad-close`, then `MoneyWithdrawJourney.withdrawAll` | `{QA.refund_address}` | The steps S1.1, S1.3 and S1.4 pass for B on Solana, ending with the settled button |

## Ground truth

`apps/mobile/qa/journeys/money/withdraw.truth.sh` reads `withdrawals`. A and B each have a row to `{QA.refund_address}` from the last 15 minutes, and B has no row to `{QA.base_address}`: S2 never sent on Base.

## Known failures on staging

None known.

## Not covered

- The over-the-balance line "You can send $… · $… over". `WithdrawFormTests` covers the copy.
- The member's own deposit address as a destination. `SolanaAddress` tests cover it.
- A Base withdrawal that really sends, and its "Withdrawal complete" toast. It needs a Base address Monaco can take the money back from, so [Verify multichain money](../../how-to/verify-multichain.md) covers it, sending to another member's Base deposit address.
- Arbitrum and Ethereum. They take the same path as Base with another name. `WithdrawNetworkTests` covers their copy.
- A mistyped checksum ("That address has a typo in it. Copy it again."). `WithdrawNetworkTests` covers it.
- The Solscan link on the account activity receipt once the withdrawal confirms. That belongs to the activity journey (#2138).
