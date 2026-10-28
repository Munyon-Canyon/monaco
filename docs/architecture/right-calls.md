# Right calls

**Status:** Decided 2026-10-26 (founder decision on the launch leaderboard). The store and the rule are built by #6434; resolution (#6445), backfill (#6452) and boards (#6453) add their own sections. Owned by the `ranking` module ([backend-platform.md](backend-platform.md)). Boards that read it: [leaderboards.md](leaderboards.md).

## Decision

The launch leaderboard ranks members on right calls over a plain period (Week, Month, All time), not on money. A member's score is the right-call rate with its record, such as "91% · 10 of 11 right". A member needs at least five resolved votes in the period to rank, and turnout ("Voted 11 of 11") is the sub-line. There is no season and no streak, and dollars are never the rank.

Every eligible vote on a buy or sell proposal is a **call**. A call is a judgement about the market, not about the cabal's outcome: it is judged 24 hours after the proposal is decided, on the asset's price move from a reference price. All the figures in the app (Home's leaderboard row, the Members tab, the profile standing line, the weekly Ledger, the leaderboard itself) read one definition and one store.

## Rule

| Proposal | Your vote | Right when the end price compared with the reference price is |
| --- | --- | --- |
| Buy | Yes | higher |
| Buy | No | equal or lower |
| Sell | Yes | lower |
| Sell | No | equal or higher |

- **Direction comes from the raw prices.** `domain.Right` compares `money.Micros` (`end.Cmp(ref)`). It never uses rounded basis points: a buy that rises by less than one basis point is a right Yes. `move_bps` (`domain.MoveBps`, rounded toward zero, an error on a zero reference or an overflow) is for display only.
- **Reference price.** For a proposal that filled, the fill price: `money.PricePerUnit(usdc_micros, token_base_units, decimals)`, with the token side `OutAmount` for a buy and `InAmount` for a sell, and `decimals` from the catalog asset of that mint (`domain.FillPrice`). For every other decided proposal, the market price at the decision: the mint resolves to its `AssetID`, `PricesAsOf` gives the sample, and the price and the sample's `ObservedAt` are stored. #6445 picks between the two when it resolves.
- **Horizon.** `resolve_at = decided_at + 24 h`. The end price is the accepted sample at `resolve_at` (#6445).
- **A cabal's call** (the Cabals board) is the decision itself: passed counts as the cabal's Yes, failed or expired as its No, judged by the same table.
- **What counts.** Decided proposals only: `passed`, `executed`, `execution_blocked`, `failed`, `expired`. `withdrawn` and `voided` void every call on them, and a decision that arrives afterwards does not reopen a void call. Rule proposals (`kind: rule`) are not calls: the consumer skips any kind other than `buy` and `sell`. An eligible member who never voted has no call but counts in turnout.
- **Pre-IPO assets** count like any other. They are judged on the secondary estimate, which `market` samples about once a day, so an unchanged estimate is a move of 0 and the No side is right on a buy (founder decision Q13).
- **Turnout.** For a period, `eligible` is the number of proposals in the user's cabals whose voting opened (`created_at`) in the period and whose frozen voter set (`proposal_voters`) includes the user. `voted` is the subset where the user cast a ballot. Open, decided and resolved proposals all count; withdrawn and voided ones don't. A member who retracts a vote stays in `eligible` and drops out of `voted`.
- **Period attribution.** A call belongs to the period its `resolved_at` falls in. Periods are UTC calendar weeks (Monday to Sunday), UTC calendar months, and All time (founder decision Q14). `domain.PeriodContaining` returns the half-open `[Start, End)` window.
- **Voiding.** A void call keeps its `void_reason`: `withdrawn` for a withdrawn proposal, the event's reason for a voided one. A decided call that #6445 cannot price is voided there as `unpriced`.

## Store

Three tables, all written by the `ranking_calls` consumer (one handler per event type, named `ranking.calls.<event>`, so `monacoctl backfill` can replay one type at a time).

- `calls`, one row per proposal: the opening facts (`kind`, `symbol`, `mint`, `asset_id`, `opened_at`), the decision (`decided_at`, `outcome`, `market_ref_micros`, `market_ref_observed_at`, `resolve_at`), the fill (`fill_price_micros`, `filled_at`), and the resolution columns #6445 fills (`ref_price_micros`, `ref_source`, `end_price_micros`, `move_bps`, `resolved_at`). `state` is `open`, `decided`, `resolved` or `void`.
- `call_voters`, one row per frozen voter and proposal: `choice`, `choice_at` and `is_right`. A voter who has not voted has a row with a null choice, so turnout can count them. (`right` is a reserved word in Postgres, so the column is `is_right`, here and in `cabal_calls`.)
- `cabal_calls`, one row per decided proposal: the cabal's `side` and `is_right`.

**Every piece of evidence is stored on its own, whatever order it arrives in.** A `vote.cast`, a fill or a decision can arrive before `proposal.created`, and a replay can bring an older ballot after the decision. So each event writes its own columns with an upsert keyed by the proposal, there is no foreign key from `call_voters` to `calls`, and nothing is dropped because another event has not arrived:

- A ballot only replaces an older one (`choice_at` guard), and a null `choice_at` never overwrites a choice.
- A fill is kept whether or not the decision has arrived, and a fill after resolution changes nothing. A proposal can confirm several trades (one per chain leg, or a retried trade), so the newest fill wins: a fill replaces the stored one only when its `filled_at` is later, or equal with a higher price, so the stored fill is the same in any delivery order. The reference price is chosen at resolution, not when the fill or decision lands.
- A decision is recorded once (`WHERE decided_at IS NULL`) and never reopens a void call.
- A void is final unless the call is already resolved.

One order matters: a `proposal.failed` or `proposal.expired` event carries no mint, so when it arrives before `proposal.created` there is no asset to price and the market reference stays null. #6445 voids that call as `unpriced`.

Reads go through `ranking/port.Calls`: `Records` (a member's right, resolved, voted and eligible counts in a period, optionally scoped to one cabal, plus the symbols of voted calls that have not resolved yet) and `CabalRecords` (right and resolved per cabal). `governance/port.Ballots` supplies the frozen voter set joined to the votes, so the consumer never reads governance tables.
