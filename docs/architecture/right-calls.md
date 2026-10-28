# Right calls

**Status:** Decided 2026-10-26 (founder decision for the launch leaderboard). The store and the rule ship in #6434. The resolver is #6445, and backfill and the boards are #6452 and #6453.

## Decision

The launch leaderboard ranks members on right calls over a plain period, not on money. The score is the right-call rate with its record ("91% · 10 of 11 right"). A member needs at least five resolved votes in the period to rank. Turnout ("Voted 11 of 11") is the sub-line. There is no season and no streak, and a dollar figure is never the rank.

Every figure on every screen comes from one definition and one store, both owned by the `ranking` module: Home's leaderboard row, a cabal's Members tab, the profile standing line, the weekly Ledger and the leaderboard itself.

## Rule

A call is a judgement about the market, not about the cabal's outcome. Each eligible vote on a buy or sell proposal becomes a call. It is judged 24 hours after the proposal is decided, on the asset's price move from a reference price.

| Proposal | Your vote | Right when the end price compared with the reference price is |
| --- | --- | --- |
| Buy | Yes | higher |
| Buy | No | equal or lower |
| Sell | Yes | lower |
| Sell | No | equal or higher |

- **Direction comes from the raw prices.** `domain.Right` compares `money.Micros` with `end.Cmp(ref)`. A buy that rises by less than one basis point is still a right Yes. `domain.MoveBps` is for display and rounds toward zero.
- **Reference price.** For a proposal that filled, it is the fill price: `money.PricePerUnit(usdc_micros, token_base_units, decimals)`. The token side is `OutAmount` for a buy and `InAmount` for a sell, and `decimals` comes from the catalog asset of that mint. For every other decided proposal it is the market price at the decision, stored with the `ObservedAt` of the sample so the resolver can judge freshness.
- **Horizon.** `resolve_at = decided_at + 24h`. The end price is the accepted sample at `resolve_at`.
- **A cabal's call** is the decision itself. A passed proposal counts as the cabal's Yes, and a failed or expired one as its No. The same table judges it.
- **What counts.** Decided proposals: `passed`, `executed`, `execution_blocked`, `failed` and `expired`. A withdrawn or voided proposal voids every call on it, and a void call stays void when a decision arrives later. Only `buy` and `sell` proposals are calls, so a rule proposal never gets a row.
- **Pre-IPO assets** count like any other. They are judged on the secondary estimate, which `market` samples about once a day. An unchanged estimate is a move of 0, so the No side is right on a buy.
- **Turnout.** For a period, `voted` is the number of proposals in the user's cabals that opened in the period, where the user was among the frozen voters and cast a ballot. `eligible` counts the same proposals whether or not the user voted. Open, decided and resolved proposals count, and void ones do not. A member who undid a vote keeps their row with no choice, so they still count in `eligible` and not in `voted`. A member who never voted has no call, and so counts in turnout only.
- **Period attribution.** A call belongs to the period its `resolved_at` falls in. Periods are UTC calendar weeks (Monday to Sunday), UTC calendar months, and all time, each as `[start, end)`. Turnout uses `opened_at` instead, because an open proposal has no `resolved_at`.
- **Pending.** `PendingSymbols` lists the user's voted calls that are not resolved yet and opened before the period ends. They count in whichever period they resolve in.

## Store

Three tables, all written by the `ranking_calls` consumer. The consumer has one handler per event type, named `ranking.calls.<event>`, because `monacoctl backfill` replays one handler per type.

| Table | One row per | Holds |
| --- | --- | --- |
| `calls` | proposal | The opening facts (kind, symbol, mint, asset, `opened_at`), the decision (`decided_at`, outcome, the market reference and its observation time, `resolve_at`), the fill (price and time), and the resolution columns that #6445 fills. `state` is `open`, `decided`, `resolved` or `void` |
| `call_voters` | proposal and frozen voter | The last choice with its `choice_at`, and `right` once resolved. A member who has not voted has a row with no choice |
| `cabal_calls` | proposal | The cabal's side (`yes` or `no`) and `right` once resolved |

**Every piece of evidence is stored on its own, whatever order it arrives in.** The opening facts, each ballot, the fill and the decision each write their own columns with an upsert keyed by the proposal. Events reach the consumer in any order. A fill can arrive before the `proposal.passed` that caused it, and a redelivery can bring an older `vote.cast` after the decision. A store that only accepted a fill while the call was pending, or let any `vote.cast` overwrite the choice, would give a different answer for the same history. So:

- The opening columns of `calls` are nullable. A ballot, a fill or a decision creates the row when it comes first, and the opening event fills the rest later.
- `call_voters` has no foreign key to `calls`, so a ballot that arrives first is kept. A ballot only replaces a stored one when its `choice_at` is newer. A snapshot of a member who has not voted never erases a choice.
- A decision is recorded once, guarded by `decided_at IS NULL`. It stores the market price at the decision and the final ballots of every frozen voter, read from `governance`'s `Ballots` port. It never sets `state` over `void`.
- A fill is kept unless the call is resolved, and the newest fill by `filled_at` wins. The reference price is chosen when the call is resolved, so the fill is preferred whenever it has arrived by then.
- A void applies to any call that is not resolved.
- A failed or expired proposal is priced from the asset on its opening row, because those events carry only the proposal. When the opening row has not arrived, the market reference stays empty and the resolver voids the call as unpriced.

The records port, `ranking/port.Calls`, reads these tables for a period: `Records` per member (optionally within one cabal) and `CabalRecords` per cabal. A member in the request always has an entry, zero when there is nothing to count. A cabal's record carries no turnout.

Log: [log/right-calls.md](log/right-calls.md).
