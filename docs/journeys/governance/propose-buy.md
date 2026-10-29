---
id: governance/propose-buy
title: Propose, vote and trade
version: 9
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

A funds the Treasury with $2.00 and B with $1.00 on the amount keypad. A proposes a $1.00 buy of GOOGL, the cabal's two voters vote it through by pick and hold, and the trade engine buys for real. The Treasury tab then shows GOOGL under Positions and Treasury history shows "Bought". Last, A and B cash out their slices and withdraw what is left to the wallet that funded the run. The rules are [Proposals](../../architecture/proposals.md).

This version extends version 1 (propose only) and version 5 (stop at the stub) instead of adding a second id. `governance/vote` covers voting on a seeded proposal, ballot changes and See all; this journey covers the path from the propose chooser to the hand-off.

Flow: Cabal -> Propose -> stock search -> amount keypad -> review sheet -> Send to vote. Spec: "Propose" in [screens.md](../../screens.md#propose-613).

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
| S1.1 | A | run | `MoneyFundCabalJourney.fundOnKeypad` from the cabal page (`openCabal(named:)`, then `cabal-action-fund`) | `{cabalName}`, `2` | The keys `amount-keypad-2` are tapped, then `amount-keypad-action` "Fund $2", and `amount-keypad-settled` shows within 120 s. Close (`amount-keypad-close`) returns to the cabal page |
| S1.2 | B | run | the same | `{cabalName}`, `1` | The same for $1, then `cabal-pot-value` under "Treasury" reads "$3.00" within 30 s |

### S2 A proposes a buy

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | run | `openCabal(named:)` | `{cabalName}` | `cabal-header-name` reads `{cabalName}`, and `cabal-action-propose` is enabled with no "Only voters can propose" caption, within 15 s |
| S2.2 | A | tap | `cabal-action-propose` | | The Propose action sheet (`propose-chooser-sheet`) shows `propose-kind-buy` "Buy stock", `propose-kind-sell` "Sell a holding" with "No holdings to sell" and disabled, and `propose-chooser-cancel` "Cancel", within 10 s |
| S2.3 | A | tap, type, then tap | `propose-kind-buy`, `monaco-search-field`, then the `propose-buy-stock-<symbol>` row for GOOGL | `GOOGL` | The screen titled "Buy stock" shows the search field within 10 s. Then `amount-keypad-screen` shows within 10 s |
| S2.4 | A | tap | `amount-keypad-5` | `5` | `amount-keypad-limit-line` shows within 5 s, and there is no `amount-keypad-action` |
| S2.5 | A | tap, tap, then tap | `amount-keypad-delete`, `amount-keypad-1`, then `amount-keypad-action` | `1` | The sheet `propose-review-screen` shows within 10 s: the stock row `propose-review-market-order` reads "Buy · market order", the cabal row names `{cabalName}`, `propose-review-thesis` shows, and `propose-review-stats` shows "Shares", "Price", "Of treasury" and "To pass" |
| S2.6 | A | tap | `propose-review-send` | | `propose-review-sent` shows within 10 s. Close returns to the cabal page (`cabal-header-name` reads `{cabalName}`) within 10 s |

### S3 The cabal votes it through

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S3.1 | B | tap | the Home tab | | Under "Your vote", a `proposal-card-<id>` reads "✓ 0 · ✕ 0" within 10 s |
| S3.2 | B | pick and hold | `home-vote-yes-<id>` (`homeVoteCardPickAndHold`) | | The toast "Voted Yes. 1st of 2." shows within 10 s with an Undo action (`monaco-toast-action`). After the toast ends, B answers the permission sheet with "Not now" (`answerVotePermissionSheet(turnOn: false)`) |
| S3.3 | A | tap, then pick and hold | the Home tab and the `proposal-card-<id>`, then `proposal-vote-yes` (`pickAndHold("yes")`) | | The toast "Voted Yes. Passed 2–0." shows within 10 s with no Undo action |
| S3.4 | A | wait | `proposal-vote-status` | | The status line reads "Passed 2–0" and "filled at $" within 120 s (it reads "buying …" first) |

### S4 See the trade

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S4.1 | A | run, then tap | `openCabal(named:)`, then `cabal-tab-treasury` | `{cabalName}` | `cabal-holding-GOOGL` (the Positions row) and `cabal-holdings-cash` show within 30 s |
| S4.2 | A | tap | `cabal-treasury-history` | | `cabal-treasury-history-screen` shows, a `cabal-activity-row-<id>` reads "Bought" within 30 s, and no row reads "Pending" or "Failed" |

### S5 Refund

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S5.1 | A | tap, tap, then tap | `cabal-action-cash-out`, "All", then `cash-out-submit` | | A toast starting "Cashing out $" shows within 10 s, then one starting "Cashed out $" within 120 s. The amount follows the GOOGL price, so the steps read it from the button |
| S5.2 | B | tap, tap, then tap | the same cash out of "All" | | The same two toasts within 10 s and 120 s, then `cabal-pot-value` reads "$0.00" within 60 s |
| S5.3 | A | run | `MoneyWithdrawJourney.withdrawAll` | `{QA.refund_address}` | The steps of `money/withdraw` pass, ending with a toast starting "Withdrawing $" |
| S5.4 | A | wait | the Home tab | | A toast starting "Withdrawal complete: $" shows within 120 s, and `platform-balance-value` reads "$0.00" within 60 s |
| S5.5 | B | run | `MoneyWithdrawJourney.withdrawAll` | `{QA.refund_address}` | The same as S5.3 |
| S5.6 | B | wait | the Home tab | | The same as S5.4 |

## Ground truth

`apps/mobile/qa/journeys/governance/propose-buy.truth.sh` reads `proposals`, `swaps`, `cabal_activity`, `cabal_positions`, `user_positions` and `user_txns` once, after S5. S5's cash outs sell the GOOGL again, so it counts swaps per proposal and positions per asset, never the cabal's totals. The cabal has one `buy` proposal for GOOGL by A worth 1 USDC; it is `executed`. Exactly one `swaps` row has `source_kind` `proposal`, that proposal as `source_id` and action `buy`, and it is `confirmed`. That swap has one `confirmed` `buy` row in `cabal_activity` for the asset it bought. After the refund the cabal's `cabal_positions` units of that asset are 0, A's and B's `share_units` add up to 0, and A and B each have a `settled` `cash_out` in `user_txns`. It skips the check when the run never made the cabal.

## Known failures on staging

- S2.5 cannot type the thesis `Journey buy {QA.run}`: F's Propose flow has no thesis field (`propose-review-thesis` is a text row, and `ProposeAmountScreen` only seeds a thesis). The step checks the row shows, and the ground truth no longer matches the reason. Follow-up against the PV ticket (#6573).
- The ticket names `cabal-position-GOOGL`; the app's Positions row is `cabal-holding-GOOGL`, which S4.1 uses.
- The status line has no step for the tracker (`proposal-tracker`), which F replaced.

## Not covered

- Voting ballot changes and the votes list. `governance/vote` covers them.
- The preview errors other than "More than the pot has" ("Can't buy <name> right now. Try a smaller amount or another stock."). Flow 09's outcomes cover them in `monacoctl flows check`.
- A member who is not a voter. `cabal-action-propose-caption` "Only voters can propose" is covered by `CabalActionsSlot`'s sample harness.
- A trade that fails or is blocked ("Couldn't buy"). It needs a failing swap, which the live engine does not give on demand.
- The cash-out and withdrawn amounts. They follow the GOOGL price, so the steps read them from the screen and the ground truth checks the zero balances.
- The chain side of the refund. `money/withdraw` covers the withdrawal; this journey checks the balances in the app.
- Cabal holding (`CabalHoldingRoute`). No journey covers it yet: it needs per-holding average buy and sell prices, realized and unrealized P&L, a position value series, period P&L, a `symbol` filter on `GET /v1/cabals/{id}/activity` and a per-trade unit price, none of which the backend serves. S4 checks the Holdings rows on the cabal screen only.
