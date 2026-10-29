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

## Resolution

The `ranking.calls_resolver` poller (`ResolveCalls`, every minute, 200 calls a batch) judges each call at `state = 'decided'` whose `resolve_at` has passed. A batch resolves in one transaction with its `call.resolved` events.

- **Reference.** The fill price (`ref_source = 'fill'`) when the proposal filled, else the market price stored at the decision (`ref_source = 'market'`). A call with neither, or with a zero reference, is voided `unpriced`.
- **End price.** The accepted sample at `resolve_at` (`PricesAsOf`). It counts only when it was observed at most 2 hours before `resolve_at`, or 36 hours for a Pre-IPO asset, whose estimate is sampled about once a day. Direction comes from the raw prices (`domain.Right`); `move_bps` is for display.
- **Unpriced.** With no accepted sample, the call waits for the next tick until `resolve_at` + 6 hours, then it is voided `unpriced`. A call priced late but inside that window resolves normally.
- **Resolved time.** `resolved_at = resolve_at`, not the tick time, so a late tick never moves a call into another period.
- **Writes.** `ResolveCall` is a guarded update on `state = 'decided'`, so a second run, or another worker, resolves nothing and appends nothing. The same transaction sets `is_right` on every voter with a choice and on the cabal's call.
- **Event.** `call.resolved` carries the proposal, cabal, kind, symbol, outcome, `resolved_at`, `move_bps`, the right and missed voter IDs and `cabal_right`. The boards (#6453) treat it as a trigger. Nothing pushes on it at launch.
- **Retraction.** `vote.retracted` (`ranking.calls.vote_retracted`) clears the voter's choice and keeps the row, so the member still counts as eligible in turnout but has no call. A `vote.cast` older than the retraction does not bring the choice back.

## Backfill

Proposals decided before the consumer shipped have no calls. `monacoctl backfill` replays their stored events through the `ranking_calls` handlers, and the resolver then judges the decided calls like any other. The founder runs it on staging and on production once after the deploy, because they hold that access. The PR's agent never runs it.

`monacoctl backfill` takes one handler and its one event type, so there is one command per handler. The consumer stamps `opened_at`, `decided_at` and the market reference from the event's stored `created_at`, not from the clock of the run, so a replay of old events dates each call by its decision. A `vote.retracted` that arrives before any ballot row writes the retraction itself, so an older `vote.cast` cannot bring the vote back. The store converges in any order, so the order below only saves work: the opening facts go first, the fills last.

```
monacoctl backfill --consumer ranking.calls.proposal_created --types proposal.created
monacoctl backfill --consumer ranking.calls.vote_cast --types vote.cast
monacoctl backfill --consumer ranking.calls.vote_retracted --types vote.retracted
monacoctl backfill --consumer ranking.calls.proposal_passed --types proposal.passed
monacoctl backfill --consumer ranking.calls.proposal_failed --types proposal.failed
monacoctl backfill --consumer ranking.calls.proposal_expired --types proposal.expired
monacoctl backfill --consumer ranking.calls.proposal_withdrawn --types proposal.withdrawn
monacoctl backfill --consumer ranking.calls.proposal_voided --types proposal.voided
monacoctl backfill --consumer ranking.calls.trade_confirmed --types trade.confirmed
```

As a loop, where `${h/_/.}` turns `proposal_created` into `proposal.created`:

```
for h in proposal_created vote_cast vote_retracted proposal_passed proposal_failed proposal_expired proposal_withdrawn proposal_voided trade_confirmed; do
  monacoctl backfill --consumer ranking.calls.$h --types ${h/_/.}
done
```

- **Nine commands.** The consumer has nine handlers, and `vote_retracted` is one of them. Without it a replayed `vote.cast` would bring back a vote that the member retracted, because the ballots snapshot at the decision never clears a stored choice.
- **Safe to run twice.** Each delivery is recorded in `event_deliveries`, so a live delivery and a replayed one never apply twice, and a second pass reports every event as `duplicate` and changes no row. `--since <event_id>` resumes after an event.
- **Resolution.** The resolver picks up the decided calls within a few ticks, 200 a batch. A call whose `resolve_at` is more than 7 days back accepts a sample observed up to 36 hours before `resolve_at`, because `market` keeps one sample a day after the first week. A call with no such sample is voided `unpriced` and is not re-priced from history.
- **`call.resolved` events.** Backfilled calls append one `call.resolved` event each, many of them for old `resolved_at` times. That is expected. The boards (#6453) treat the event as a trigger only and read the period from `resolved_at`.
- **Seeding QA.** `qa_flow_seed 19a ok` (`monacoctl flows seed 19a ok`) puts resolved calls in the current month on eight users: the signed-in user with exactly five, six others with five to eight, and one with two.

## Boards

The `ranking.call_board` poller (`RunCallBoard`) turns resolved calls into ranked boards: `people` and `cabals`, each for `week`, `month` and `all`. One run reads the six inputs (two boards over three periods), ranks them in memory with `domain.RankCalls`, and swaps every row of `call_board_entries` in one transaction with its `call_board_runs` row and the `ranking.call_board_written` event. A reader asks for one board and period with no loop over cabals. The money boards (flow 19) are unchanged.

- **Subjects.** People: every user with at least one eligible proposal opened in the period. Cabals: every cabal with at least one decided proposal in the period. Banned and deleted users and banned cabals are left out, the same exclusions as `IncludeOnBoard` and validity rule 3 in [Leaderboards](leaderboards.md). Subject cards come from the identity and cabal ports the valuation job reads.
- **Rate.** `rate_bps = floor(right × 10 000 ÷ resolved)`. A display rounds half up to a whole percent. `right` and `resolved` count calls whose `resolved_at` falls in the period.
- **Five-call minimum.** A subject ranks at `resolved >= 5`. Below that its row has `rank = NULL` and the reader shows `needed = 5 − resolved`.
- **Order.** `rate_bps` descending, then `right` descending, then turnout (`voted ÷ eligible`) descending, then the subject's `created_at` ascending, then its id. Ranks are dense from 1, with no shared ranks. Unranked rows sort by `resolved` descending.
- **Turnout.** A person's turnout is votes cast over eligible calls opened in the period. A cabal's turnout is the sum of votes cast over the sum of eligible voters across its proposals opened in the period.
- **Periods.** The current UTC week (Monday to Sunday), the current UTC month, and all time. Previous periods are not served at launch, but each closed week and month is kept. Every viewer sees the same UTC periods, not their own time zone. `period_start` records the start so a reader can refuse a stale period.
- **Trigger.** The job has its own `call_board_triggers` table, because the valuation job deletes every row of `ranking_triggers` up to its start. The consumer `ranking_call_board` inserts a row for `call.resolved`, `vote.cast`, `proposal.created`, `user.profile_updated` (when a name, handle or photo changed) and `cabal.updated`. The poller runs when 5 minutes have passed since the last run, or when the oldest trigger is at least a second old, and deletes its own triggers through the run's start. Neither job reads or deletes the other's triggers.
- **Hint.** `ranking.call_board_written` publishes the existing `global.leaderboards_updated` hint after the commit, so open apps refetch.
- **Reads.** `port.CallBoards` gives `CallStanding` (rank, how many are ranked, rate, record and `PeriodStart` for a member) and `CallLeader` (the rank 1 subject of a board). Two runs on unchanged inputs write identical ranks.
- **Baseline and movement.** `baseline_rank` is the subject's rank in the same board and period at the first run after 00:00 UTC today. That run writes the day's ranks to `call_board_baselines` before the movement step, so the baseline is the board as the day began. `movement = baseline_rank − rank`: positive is up, negative is down and 0 is `same`. A subject with no baseline rank, or no rank now, gets `movement_kind = 'new'` and no movement. On the first day of a period every baseline is empty, so every row is `new` and the app draws "–". Baselines older than 40 days are deleted.
- **Last call.** `last_call` is the subject's most recent resolved call since 00:00 UTC today, as `{proposal_id, kind, symbol, right}`, for "Up 1 · right on NVDA" and "Down 2 · missed TSLA". It is null when there is none.
- **Closing a period.** A week or month is closed by the first run at least 6 h after it ends (`unpricedAfter`, so late resolutions have landed). The run ranks that period's window with the same inputs and keeps every row in `call_board_closed`, unranked ones with `rank = NULL`, for good. `ON CONFLICT DO NOTHING` makes a second run add nothing, and the run does not touch a closed period again. `port.CallBoards.ClosedTops(subject, board, period)` returns every closed period in which the subject ranked #1, newest first, with its `Right` and `Resolved` counts.
- **Flow 19b.** The `ranking.call_board` poller, `RunCallBoard` and the `ranking.call_board_written` event are flow 19b, with an `ok` test and a `crash:before-commit` test. After the restart the rows, baselines, closed rows and the one event are the same.
- **Not here.** The HTTP route that serves movement is #6472, and a closed-period board view in the app is phase 2.

## The Ledger

The Ledger is one member's week, built once and stored in `weekly_ledgers (user_id, week)`. The row holds the JSON payload as built; `GET /v1/me/ledgers/latest` and `GET /v1/me/ledgers/{week}` return it unchanged and never recompute. A second build for the same `(user, week)` keeps the first payload (`ON CONFLICT DO NOTHING`). `weekly_ledger_skips` records members who got no Ledger that week.

**Window.** The 7 days ending at the delivery time, Sunday 17:00 UTC: `[Sun 17:00 − 7 d, Sun 17:00)`. The week is the ISO week of the Monday inside it (`2026-W42`), labelled with that Monday and the following Sunday.

**Fields.**

- `voted`, `eligible`: proposals opened in the window, as the call board counts them.
- `right`, `resolved`, `calls`: the member's calls resolved in the window, newest first.
- `best_call`: the member's right call with the largest move toward their side. `miss`: their wrong call with the largest move against them. Ties go to the later decision, then the proposal id. `move_bps` is signed toward the member's side, so a right call is at least 0 and a miss at most 0.
- `sharpest`: the best right-call rate in the window among members of the member's cabals, the member included. It needs 2 resolved calls. Ties go to more right calls, then name.
- `top_reply`: the most-reacted reply on the window's resolved calls in the member's cabals, from the social read.
- `markers`: the best call (`Called AMZN · Sat`) and each buy that filled in the member's cabals in the window (`B`).
- `cabals`: the member's cabals, by name for now.

A member with no cabal and no eligible proposal gets no Ledger.

`stake_return_bps` and `series` are always null and empty for now. The window loader that fills them, and the ordering of `cabals` by equity at the window end, land in the follow-up.
