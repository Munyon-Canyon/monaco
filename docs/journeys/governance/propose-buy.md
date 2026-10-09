---
id: governance/propose-buy
title: Propose, vote and trade
version: 6
milestone: M14
requires: [auth/sign-in]
actors: [A, B]
flows: [09, 10, 11]
funds:
  A: 2
  B: 1
xcuitest: [apps/mobile/MonacoUITests/Journeys/GovernanceProposeBuyJourney.swift, apps/mobile/MonacoUITests/Journeys/GovernanceProposeBuyJourneyUITests.swift]
---

# Propose, vote and trade

A funds the pot with $2.00 and B with $1.00 through the app. A proposes a $1.00 buy of GOOGL, the cabal's two voters vote it through, and the trade engine buys for real. The cabal then shows GOOGL under "Holdings" and "Bought" under "Activity". Last, A and B cash out their slices and withdraw what is left to the wallet that funded the run. The rules are [Proposals](../../architecture/proposals.md).

This version extends version 1 (propose only) and version 5 (stop at the stub) instead of adding a second id. `governance/vote` covers voting on a seeded proposal, ballot changes and See all; this journey covers the path from the propose chooser to the hand-off.

Old app (`c838bd24`): Cabal -> Propose -> `ProposeChooserView` -> `ProposeBuyView` -> `ProposeAmountView` -> `ProposeReviewView` -> Send -> toast. Spec: "Propose" in [screens.md](../../screens.md#propose-613).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and B have each signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). The backend runs with `TRADE_ENGINE=live`: `scripts/qa/journey.py run` starts it that way for a journey with `funds`, so the vote's trade really buys on mainnet |
| P3 | Whoever runs the journey sent A 2 USDC and B 1 USDC from the QA pot (`monacoctl qa fund`) or the Phantom MCP agent wallet, as [Journeys that move money](../README.md#journeys-that-move-money) says, and set `MONACO_QA_REFUND_ADDRESS` (`{QA.refund_address}`) |
| P4 | `apps/mobile/qa/journeys/governance/propose-buy.setup.sh` ran before S1. It marks A and B as done with onboarding, sets their display names to the `name` column of `accounts.tsv` and expires every open proposal either can vote on. A creates the cabal `QA buy {QA.run}` through the API with "Majority" and every member a voter, and B joins it. It hands the test `cabalName`. The later scenarios continue from the earlier ones, so their setup changes nothing |
| P5 | The run needs a longer budget than the runner's 300 s default: funding alone takes about 114 s and the trade and the refund wait up to 120 s each. Run `scripts/qa/journey.py run governance/propose-buy --timeout 900`, and `scripts/qa/journey.py mutants governance/propose-buy --timeout 900` for the seeded bugs |

With two voters and "Majority", a buy needs both yes votes.

## Scenarios

### S1 Fill the pot

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap, type, tap, then tap | the Cabals tab, `cabals-search-field` and its result, `cabal-action-fund`, `amount-entry-field`, then "Add $2 to the pot" | `{cabalName}`, `2` | The toast "Added $2 to {cabalName}." shows within 60 s |
| S1.2 | B | tap, type, tap, then tap | the same path | `{cabalName}`, `1` | The toast "Added $1 to {cabalName}." shows within 60 s, then `cabal-pot-value` under "In the pot" reads "$3.00" within 30 s |

### S2 A proposes a buy

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `{cabalName}` | `cabal-header-name` reads `{cabalName}`, and `cabal-action-propose` is enabled with no "Only voters can propose" caption, within 15 s |
| S2.2 | A | tap | `cabal-action-propose` | | The chooser titled "Propose" shows `propose-kind-buy` "Buy a stock" with "Your cabal votes on it first", and "Sell something the cabal owns" with "Nothing to sell yet", within 10 s |
| S2.3 | A | tap, type, then tap | `propose-kind-buy`, `monaco-search-field`, then the `propose-buy-stock-<symbol>` row for GOOGL | `GOOGL` | The screen titled "Buy" shows "Search Apple, Tesla, NVDA…" and "Popular" within 10 s. Then `propose-amount-screen` shows within 10 s. The helper stays empty until an amount is typed |
| S2.4 | A | type | `amount-entry-field` | `5` | the message "More than the pot has" shows under the field within 5 s, and `propose-amount-review` is disabled |
| S2.5 | A | type, tap, type, then tap | `amount-entry-field`, `propose-amount-add-reason`, `propose-amount-reason`, then `propose-amount-review` | `1`, then `Journey buy {QA.run}` | After the `1`, `amount-entry-helper` reads "The pot has $3.00" within 10 s. After the reason and Review, the screen titled "Review" (`propose-review-screen`) reads "Buy $1.00 of GOOGL" with the rows "Cabal gets", "Price", "Pot" and "Who votes", and the section "Why buy" with `Journey buy {QA.run}`, within 10 s |
| S2.6 | A | tap | `propose-review-send` | | The button reads "Sending…", then the flow closes back to the cabal screen (`cabal-header-name` reads `{cabalName}`) and the toast "Proposal sent to {cabalName}" shows, within 10 s |

### S3 The cabal votes it through

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S3.1 | B | tap | the Home tab | | "Needs your vote" shows a `proposal-card-<id>` for GOOGL that reads "Closes in", `Journey buy {QA.run}` and "0 of 2 voted · 2 yes to pass" within 10 s |
| S3.2 | B | tap | "Yes" on the card | | The toast "Vote in" shows. The card reads "✓ You voted yes" and "1 of 2 voted · 2 yes to pass" within 10 s |
| S3.3 | A | tap, then tap | the Home tab, then the header of the `proposal-card-<id>` (its `proposal-closes-in` row) | | The Proposal screen shows "Votes", "Proposed by" with A's name and "<B's name> voted yes" within 15 s. A taps "Yes" on the card and the toast "Vote in" shows |
| S3.4 | A | wait | `proposal-tracker` | | The Status tracker reads "Bought, step 3 of 3, done" and the `proposal-status-chip` reads "Bought" within 120 s |

### S4 See the trade

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S4.1 | A | tap, type, tap, then scroll to | the Cabals tab, `cabals-search-field` and its result, then "Holdings" | `{cabalName}` | `cabal-holding-GOOGL` and `cabal-holdings-cash` show within 30 s |
| S4.2 | A | scroll to | "Activity" (`cabal-activity`) | | A `cabal-activity-row-<id>` reads "Bought" within 30 s, and no row of Activity reads "Pending" or "Failed" |

### S5 Refund

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S5.1 | A | tap, tap, then tap | `cabal-action-cash-out`, "All", then `cash-out-submit` | | A toast starting "Cashing out $" shows within 10 s, then one starting "Cashed out $" within 120 s. The amount follows the GOOGL price, so the steps read it from the button |
| S5.2 | B | tap, tap, then tap | the same cash out of "All" | | The same two toasts within 10 s and 120 s, then `cabal-pot-value` reads "$0.00" within 60 s |
| S5.3 | A | run | `MoneyWithdrawJourney.withdrawAll` | `{QA.refund_address}` | The steps of `money/withdraw` S1 pass: Home `home-withdraw-link`, "Max", the address, "Continue", Confirm with "You're withdrawing", the whole address, "From" "Account balance" and "Arrives" "About a minute", then "Withdraw" and a toast starting "Withdrawing $" |
| S5.4 | A | wait | the Home tab | | A toast starting "Withdrawal complete: $" shows within 120 s, and `platform-balance-value` reads "$0.00" within 60 s |
| S5.5 | B | run | `MoneyWithdrawJourney.withdrawAll` | `{QA.refund_address}` | The same as S5.3 |
| S5.6 | B | wait | the Home tab | | The same as S5.4 |

## Ground truth

`apps/mobile/qa/journeys/governance/propose-buy.truth.sh` reads `proposals`, `swaps`, `cabal_activity`, `cabal_positions`, `user_positions` and `user_txns` once, after S5. S5's cash outs sell the GOOGL again, so it counts swaps per proposal and positions per asset, never the cabal's totals. The cabal has one `buy` proposal for GOOGL by A worth 1 USDC and with the reason `Journey buy {QA.run}`; it is `executed`. Exactly one `swaps` row has `source_kind` `proposal`, that proposal as `source_id` and action `buy`, and it is `confirmed`. That swap has one `confirmed` `buy` row in `cabal_activity` for the asset it bought. After the refund the cabal's `cabal_positions` units of that asset are 0, A's and B's `share_units` add up to 0, and A and B each have a `settled` `cash_out` in `user_txns`. It skips the check when the run never made the cabal.

## Known failures on staging

None known.

## Not covered

- Voting from the cabal's own card, ballot changes and See all. `governance/vote` covers them.
- The preview errors other than "More than the pot has" ("Can't buy <name> right now. Try a smaller amount or another stock."). Flow 09's outcomes cover them in `monacoctl flows check`.
- A member who is not a voter. `cabal-action-propose-caption` "Only voters can propose" is covered by `CabalActionsSlot`'s sample harness.
- A trade that fails or is blocked ("Couldn't buy"). It needs a failing swap, which the live engine does not give on demand.
- The cash-out and withdrawn amounts. They follow the GOOGL price, so the steps read them from the screen and the ground truth checks the zero balances.
- The chain side of the refund. `money/withdraw` covers the withdrawal; this journey checks the balances in the app.
