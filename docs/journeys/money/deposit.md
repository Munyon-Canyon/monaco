---
id: money/deposit
title: Add money
version: 2
milestone: M12
requires: [auth/sign-in]
actors: [A]
flows: [05]
funds:
  A: 1
xcuitest: [apps/mobile/MonacoUITests/Journeys/MoneyDepositJourney.swift, apps/mobile/MonacoUITests/Journeys/MoneyDepositJourneyUITests.swift]
---

# Add money

A member opens Add money from Home, picks "Send crypto", chooses a network, sees the deposit address for it, copies it, and reads how a deposit reaches their account balance. Solana and Base each have their own address. The deposit itself is a USDC transfer on Solana that the runner makes before the run (see Journeys that move money in [App journeys](../README.md)). The rules are [Deposits and withdrawals](../../architecture/deposits-withdrawals.md#deposits-from-an-evm-chain).

Version 2 (M24, #4968): Add money lists "Card" first and "Send crypto" second, "Send crypto" opens "Choose a network" when the member can deposit on more than one network, and each network has its own address screen titled "Send USDC on <network>". It replaces version 1's "Crypto" chooser and its S2.

Old app (`c838bd24`): Home -> "Add money" -> `Deposit/DepositView.swift`: the address, "Copy", the copied toast, "How it works", and the deposit-received toast. Spec: **Add money** in [screens.md](../../screens.md) (Money).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actor A has signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend`. `EVM_DEPOSIT_NETWORKS` is unset or includes `base` (the default is `base`) |
| P3 | The runner sent A at least 1 USDC on Solana from the QA pot (`monacoctl qa fund`) or the Phantom MCP agent wallet to the address Add money shows, per `funds` |
| P4 | `apps/mobile/qa/journeys/money/deposit.setup.sh` ran right before the scenario. It marks A as done with onboarding and sets A's display name through `scripts/qa/seed.sh` |

## Scenarios

### S1 Copy the Solana deposit address

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap, then wait | the Home tab, then `platform-balance-value` | | The account balance reads at least "$1.00" within 30 s: the runner's deposit arrived (old app: Home balance) |
| S1.2 | A | tap | `home-add-money-link` "Add money" | | The screen titled "Add money" shows `deposit-card-row` "Card" above `deposit-crypto-row` "Send crypto" within 15 s |
| S1.3 | A | tap | `deposit-crypto-row` | | The screen titled "Choose a network" lists `deposit-network-solana` "Solana" first and `deposit-network-base` "Base" after it within 15 s |
| S1.4 | A | tap | `deposit-network-solana` | | The screen titled "Send USDC on Solana" shows "Your deposit address" and `deposit-address-value` within 15 s, and no `deposit-arrival-note` |
| S1.5 | A | tap | `deposit-address-copy-button` "Copy address" | | The toast "Address copied." shows within 5 s, and `deposit-network-warning` reads "Only send USDC on Solana to this address." (old app: `DepositView.swift` Copy and its toast) |
| S1.6 | A | scroll to | "How it works" | | The steps "Send USDC on Solana", "It lands in your balance" and "Fund a cabal" show, and `deposit-screen-balance-value` shows within 10 s (old app: `DepositView.swift` "How it works") |

### S2 Copy the Base deposit address

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap, tap, then tap | the Home tab, `home-add-money-link`, then `deposit-crypto-row` | | "Choose a network" shows `deposit-network-base` "Base" within 15 s |
| S2.2 | A | tap | `deposit-network-base` | | The screen titled "Send USDC on Base" shows `deposit-address-value` with an address that starts "0x" within 15 s. `deposit-network-warning` reads "Only send USDC on Base to this address." and `deposit-arrival-note` reads "Arrives in a few minutes" |
| S2.3 | A | tap | `deposit-address-copy-button` "Copy address" | | The toast "Address copied." shows within 5 s |

## Ground truth

`apps/mobile/qa/journeys/money/deposit.truth.sh` reads `user_txns` and `user_wallets`. A has at least one settled `deposit` row, the one the runner's transfer credited, and no `failed` deposit row from this run. A's `user_wallets` row has an `evm_address`: S2 made A's Base deposit address.

## Known failures on staging

None known.

## Not covered

- The toast "Deposit received: $25.00". It shows on whichever screen is open when a deposit lands, and the runner deposits before the run, so a step would wait on a person. `BalanceChangeTests` covers its copy.
- Tapping the address itself to copy it (`deposit-address-value`). It copies the same way as S1.5.
- A deposit on Base and its forwarding to Solana, with the "Arriving" line on the balance. It needs a Base transfer from outside Monaco, which the runner can't make, so [Verify multichain money](../../how-to/verify-multichain.md) covers it. `DepositArrivingTests` covers the line.
- Arbitrum and Ethereum. They show only when `EVM_DEPOSIT_NETWORKS` lists them, and their screens are Base's with another name. `DepositNetworksTests` covers them.
- The clipboard's content. XCUITest cannot read the simulator pasteboard without a system prompt.
