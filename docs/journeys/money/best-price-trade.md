---
id: money/best-price-trade
title: Buy at the best price
version: 1
milestone: M24
requires: [auth/sign-in]
actors: [A]
flows: [09, 10, 11, 30]
funds:
  A: 2
xcuitest: [apps/mobile/MonacoUITests/Journeys/MoneyBestPriceTradeJourney.swift, apps/mobile/MonacoUITests/Journeys/MoneyBestPriceTradeJourneyUITests.swift]
---

# Buy at the best price

A funds a cabal where they are the only voter, proposes a $1.00 buy of Apple, and sees the price Monaco found across every venue that lists it: on Review, on the proposal and on the trade. A votes it through and the trade engine buys on the winning venue, which may be on another chain. Last, A cashes out and withdraws what is left to the wallet that funded the run. The rules are [Routing across chains](../../architecture/trade-execution.md#routing-across-chains), and the copy rules are "Best price across chains" in [Product](../../product.md).

Apple is the stock because it has the most enabled listings at launch: Solana (two issuers), Ethereum and Base ([Stock listings](../../how-to/stock-listings.md)). Which one wins changes with the market, so the steps check that a comparison shows and the trade completes, not which venue wins. [Verify multichain money](../../how-to/verify-multichain.md) records a run where an EVM venue wins.

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actor A has signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`) and holds Apple's listings in `asset_listings` (the catalog's listings sync, #4969). The backend runs with `TRADE_ENGINE=live`: `scripts/qa/journey.py run` starts it that way for a journey with `funds`, so the vote's trade really buys on mainnet |
| P3 | Whoever runs the journey sent A 2 USDC from the QA pot (`monacoctl qa fund`) or the Phantom MCP agent wallet, as [Journeys that move money](../README.md#journeys-that-move-money) says, and set `MONACO_QA_REFUND_ADDRESS` (`{QA.refund_address}`) |
| P4 | `apps/mobile/qa/journeys/money/best-price-trade.setup.sh` ran before S1. It marks A as done with onboarding, sets A's display name, and expires every open proposal A can vote on. A creates the cabal `QA best price {QA.run}` through the API with "Majority", A its only member and voter. It hands the test `cabalName`. The later scenarios continue from the earlier ones, so their setup changes nothing |
| P5 | A trade routed to another chain bridges out and back, which takes minutes. Run `scripts/qa/journey.py run money/best-price-trade --timeout 1800` |

## Scenarios

### S1 Fill the pot

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap, type, tap, then tap | the Cabals tab, `cabals-search-field` and its result, `cabal-action-fund`, `amount-entry-field`, then "Add $2 to the pot" | `{cabalName}`, `2` | The toast "Added $2 to {cabalName}." shows within 60 s, then `cabal-pot-value` under "In the pot" reads "$2.00" within 30 s |

### S2 Propose Apple and compare prices

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap, tap, type, then tap | `cabal-action-propose`, `propose-kind-buy`, `monaco-search-field`, then the `propose-buy-stock-<symbol>` row for AAPL | `AAPL` | `propose-amount-screen` shows within 10 s |
| S2.2 | A | type, then tap | `amount-entry-field`, then `propose-amount-review` | `1` | `propose-review-screen` reads "Buy $1.00 of AAPL" within 10 s, and its "Price" section shows `best-price-row` within 30 s, reading "<price> a share" and "Best price across <N> venues" with N at least 2 |
| S2.3 | A | tap | `best-price-row` | | `best-price-sheet` shows within 10 s. Its first row is `best-price-winner` with "Best price", and at least one `best-price-venue` row follows. Every row is named "<exchange> on <network>", such as "Jupiter on Solana" or "Uniswap on Base", and no row names an issuer ("xStocks", "Ondo", "Coinbase") |
| S2.4 | A | tap, then tap | `best-price-done`, then `propose-review-send` | | The sheet closes, the flow closes back to the cabal screen, and the toast "Proposal sent to {cabalName}" shows, within 10 s |

### S3 Vote it through

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S3.1 | A | tap, then tap | the Home tab, then the header of the `proposal-card-<id>` for AAPL (its `proposal-closes-in` row) | | The Proposal screen shows `best-price-row` reading "Best price across <N> venues", N at least 2, within 15 s |
| S3.2 | A | tap | "Yes" on the proposal | | The toast "Vote recorded." shows within 10 s |
| S3.3 | A | wait | `proposal-tracker` | | The Status tracker reads "Bought, step 3 of 3, done" and `proposal-status-chip` reads "Bought" within 900 s |

### S4 See the trade

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S4.1 | A | tap, type, tap, then scroll to | the Cabals tab, `cabals-search-field` and its result, then "Holdings" | `{cabalName}` | `cabal-holding-AAPL` and `cabal-holdings-cash` show within 30 s |
| S4.2 | A | scroll to, then tap | "Activity" (`cabal-activity`), then the `cabal-activity-row-<id>` that reads "Bought" | | The screen titled "Transaction" shows `cabal-txn-status`, which does not read "In progress", and `best-price-row` reading "Best price across <N> venues", N at least 2, within 30 s |

### S5 Refund

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S5.1 | A | tap, tap, then tap | `cabal-action-cash-out`, "All", then `cash-out-submit` | | A toast starting "Cashing out $" shows within 10 s, then one starting "Cashed out $" within 900 s, and `cabal-pot-value` reads "$0.00" within 60 s. Selling Apple may bridge back from another chain |
| S5.2 | A | run | `MoneyWithdrawJourney.withdrawAll` | `{QA.refund_address}` | The steps of `money/withdraw` S1 pass, ending with a toast starting "Withdrawing $" |
| S5.3 | A | wait | the Home tab | | A toast starting "Withdrawal complete: $" shows within 120 s, and `platform-balance-value` reads "$0.00" within 60 s |

## Ground truth

`apps/mobile/qa/journeys/money/best-price-trade.truth.sh` reads `proposals`, `swaps`, `trade_routes`, `trade_steps`, `cabal_positions` and `user_txns` once, after S5. The cabal has one `buy` proposal of Apple by A worth 1 USDC, and it is `executed`. Exactly one `confirmed` `buy` swap has that proposal as `source_id`. It has one `trade_routes` row whose `comparison` ranks at least two quotes and whose `winner_listing_id` is one of them, and none of its `trade_steps` is `pending`, `submitted` or `failed`. After the refund the cabal holds no units of the asset it bought, and A has a `settled` `cash_out` in `user_txns`. It skips the check when the run never made the cabal.

## Known failures on staging

None known.

## Not covered

- Which venue wins. It follows the market, so a step can't pin it. `router_test.go` in the market module pins the choice and the minimum edge (`TestRouterKeepsSolanaUnlessAnEVMRouteBeatsItByTheEdge`), and the runbook records a real EVM win.
- The pot while a trade bridges ("in transit"). The bridge leg lasts seconds to minutes and the test can't hold it open. The runbook's step 4 checks it, and flow 30 is the multichain path's backend flow.
- A routed trade that fails partway. It needs a failing bridge or swap, which mainnet does not give on demand. The partial-failure table in [Routing across chains](../../architecture/trade-execution.md#routing-across-chains) says what happens.
- A sell's comparison in dollars. `BestPriceSheetTests` covers the sheet for a sell.
