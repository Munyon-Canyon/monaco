---
id: governance/propose-buy
title: Propose a buy and vote it through
version: 4
milestone: M13
requires: [auth/sign-in]
actors: [A, B]
flows: [09, 10]
xcuitest: [apps/mobile/MonacoUITests/Journeys/GovernanceProposeBuyJourney.swift, apps/mobile/MonacoUITests/Journeys/GovernanceProposeBuyJourneyUITests.swift]
---

# Propose a buy and vote it through

A voter opens a cabal with a $3.00 pot, proposes a $1.00 buy of GOOGL, and the cabal's two voters vote it through. The passed vote hands the trade to the trade engine, which a stub answers, so the proposal shows "Buying" and the journey stops there. The rules are [Proposals](../../architecture/proposals.md).

This version extends version 1 (propose only) instead of adding a second id. `governance/vote` covers voting on a seeded proposal, ballot changes and See all; this journey covers the path from the propose chooser to the hand-off.

Old app (`c838bd24`): Cabal -> Propose -> `ProposeChooserView` -> `ProposeBuyView` -> `ProposeAmountView` -> `ProposeReviewView` -> Send -> toast. Spec: "Propose" in [screens.md](../../screens.md#propose-613).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and B have each signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). The backend runs with `TRADE_ENGINE=stub`: `scripts/qa/journey.py run` starts it that way unless `TRADE_ENGINE` is set, and the config refuses `stub` outside local dev |
| P3 | `apps/mobile/qa/journeys/governance/propose-buy.setup.sh` ran before S1. It marks A and B as done with onboarding, sets their display names to the `name` column of `accounts.tsv` and expires every open proposal either can vote on. It runs `monacoctl dev seed-scenario cabal-with-funded-pot` for A and B: the open cabal `QA <run>` that A created and B joined, with the ledger crediting A $2.00 and B $1.00. It hands the test `cabalName`. S2 continues from S1's proposal, so its setup changes nothing |

With two voters and "Majority", a buy needs both yes votes.

## Scenarios

### S1 A proposes a buy

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `{cabalName}` | `cabal-header-name` reads `{cabalName}`, and `cabal-action-propose` is enabled with no "Only voters can propose" caption, within 15 s |
| S1.2 | A | tap | `cabal-action-propose` | | The chooser titled "Propose" shows `propose-kind-buy` "Buy a stock" with "Your cabal votes on it first", and "Sell something the cabal owns" with "Nothing to sell yet", within 10 s |
| S1.3 | A | tap, type, then tap | `propose-kind-buy`, `monaco-search-field`, then the `propose-buy-stock-<symbol>` row for GOOGL | `GOOGL` | The screen titled "Buy" shows "Search Apple, Tesla, NVDA…" and "Popular" within 10 s. Then `propose-amount-screen` shows within 10 s. The helper stays empty until an amount is typed |
| S1.4 | A | type | `amount-entry-field` | `5` | the message "More than the pot has" shows under the field within 5 s, and `propose-amount-review` is disabled |
| S1.5 | A | type, tap, type, then tap | `amount-entry-field`, `propose-amount-add-reason`, `propose-amount-reason`, then `propose-amount-review` | `1`, then `Journey buy {QA.run}` | After the `1`, `amount-entry-helper` reads "The pot has $3.00" within 10 s. After the reason and Review, the screen titled "Review" (`propose-review-screen`) reads "Buy $1.00 of GOOGL" with the rows "Cabal gets", "Price", "Pot" and "Who votes", and the section "Why buy" with `Journey buy {QA.run}`, within 10 s |
| S1.6 | A | tap | `propose-review-send` | | The button reads "Sending…", then the flow closes back to the cabal screen (`cabal-header-name` reads `{cabalName}`) and the toast "Proposal sent to {cabalName}" shows, within 10 s |

### S2 The cabal votes it through

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | B | tap | the Home tab | | "Needs your vote" shows a `proposal-card-<id>` for GOOGL that reads "Closes in", `Journey buy {QA.run}` and "0 of 2 voted · 2 yes to pass" within 10 s |
| S2.2 | B | tap | "Yes" on the card | | The toast "Vote in" shows. The card reads "✓ You voted yes" and "1 of 2 voted · 2 yes to pass" within 10 s |
| S2.3 | A | tap, then tap | the Home tab, then the header of the `proposal-card-<id>` (its `proposal-closes-in` row) | | The Proposal screen shows "Votes", "Proposed by" with A's name and "<B's name> voted yes" within 15 s. A taps "Yes" on the card and the toast "Vote in" shows |
| S2.4 | A | wait | `proposal-tracker` | | The Status tracker reads "Buying, step 2 of 3" and the `proposal-status-chip` on the proposal's card reads "Buying" within 10 s. The stub never confirms the trade, so the journey does not wait for "Done" or "Bought" |

## Ground truth

`apps/mobile/qa/journeys/governance/propose-buy.truth.sh` reads `proposals`, `swaps` and `event_deliveries` for the cabal named in the hand-off. After S2: the cabal has one `buy` proposal for GOOGL by A worth 1 USDC and with the reason `Journey buy {QA.run}`; it is `passed`; it has no `swaps` row; and `event_deliveries` holds exactly one row with handler `trading.engine` and code `stubbed` for its `proposal.passed` event. It skips the check when the run never seeded the cabal.

## Known failures on staging

None known.

## Not covered

- The trade itself, "Holdings", "Activity" and the refund. They are M14 Trading (#2705); this journey stops at the stub.
- "Done" or "Bought" on the proposal. The stub claims the delivery and never writes a swap.
- Voting from the cabal's own card, ballot changes and See all. `governance/vote` covers them.
- The preview errors other than "More than the pot has" ("Can't buy <name> right now. Try a smaller amount or another stock."). Flow 09's outcomes cover them in `monacoctl flows check`.
- A member who is not a voter. `cabal-action-propose-caption` "Only voters can propose" is covered by `CabalActionsSlot`'s sample harness.
