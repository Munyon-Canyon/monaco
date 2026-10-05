---
id: money/deposit
title: Add money
version: 1
milestone: M12
requires: [auth/sign-in]
actors: [A]
flows: [05]
funds:
  A: 1
xcuitest: [apps/mobile/MonacoUITests/Journeys/MoneyDepositJourney.swift, apps/mobile/MonacoUITests/Journeys/MoneyDepositJourneyUITests.swift]
---

# Add money

A member opens Add money from Home, sees their deposit address, copies it, and reads how a deposit reaches their account balance. The deposit itself is a USDC transfer on Solana that the runner makes before the run (see Journeys that move money in [App journeys](../README.md)).

Old app (`c838bd24`): Home -> "Add money" -> `Deposit/DepositView.swift`: the address, "Copy", the copied toast, "How it works", and the deposit-received toast. Spec: **Add money** in [screens.md](../../screens.md) (Money).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actor A has signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | The runner sent A at least 1 USDC on Solana from the QA pot (`monacoctl qa fund`) or the Phantom MCP agent wallet to the address Add money shows, per `funds` |
| P4 | `apps/mobile/qa/journeys/money/deposit.setup.sh` ran right before the scenario. It marks A as done with onboarding and sets A's display name through `scripts/qa/seed.sh` |

## Scenarios

### S1 Copy the deposit address

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap, then wait | the Home tab, then `platform-balance-value` | | The account balance reads at least "$1.00" within 30 s: the runner's deposit arrived (old app: Home balance) |
| S1.2 | A | tap | `home-add-money-link` "Add money" | | The screen titled "Add money" shows "Your deposit address" and `deposit-address-value` within 15 s (old app: Home -> Add money) |
| S1.3 | A | tap | `deposit-address-copy-button` "Copy address" | | The toast "Address copied." shows within 5 s, and "Only send USDC on Solana to this address." is on screen (old app: `DepositView.swift` Copy and its toast) |
| S1.4 | A | scroll to | "How it works" | | "Send USDC to the address above from an exchange or another app.", "Your account balance updates a few seconds after it arrives." and "Fund a cabal to move it into the pot and grow your slice." show, and `deposit-screen-balance-value` shows within 10 s (old app: `DepositView.swift` "How it works") |

### S2 Choose card or crypto

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap, then tap | the Home tab, then `home-add-money-link` "Add money" | | The screen titled "Add money" offers "Card" and "Crypto" within 10 s (screens.md: #650 makes Add money a chooser) |

## Ground truth

`apps/mobile/qa/journeys/money/deposit.truth.sh` reads `user_txns`. A has at least one settled `deposit` row, the one the runner's transfer credited, and no `failed` deposit row from this run.

## Known failures on staging

- S2.1: Add money has no "Card" (Apple Pay or card) and "Crypto" chooser. Blocked by #650.

## Not covered

- The toast "Deposit received: $25.00". It shows only when a deposit lands while Add money is on screen, and the runner deposits before the run, so a step would wait on a person. `BalanceChangeTests` covers its copy.
- Tapping the address itself to copy it (`deposit-address-value`). It copies the same way as S1.3.
- The clipboard's content. XCUITest cannot read the simulator pasteboard without a system prompt.
