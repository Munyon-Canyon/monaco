# Right calls log

Dated record of changes to [right-calls.md](../right-calls.md). Add one line per change, newest last.

- 2026-10-28: Added the calls store and the rule: `calls`, `call_voters` and `cabal_calls`, the `ranking_calls` consumer, and `ranking/port.Calls`. The call columns use `is_right`, since `right` is a reserved word in Postgres (#6434).
- 2026-10-28: A call keeps its newest fill, not the last one delivered. `RecordCallFill` replaces the stored fill only when `filled_at` is later, or equal with a higher price, so several confirmed trades for one proposal leave the same fill in any order. The alternative, a volume-weighted price over every leg, needs per-leg storage and was not taken (#6697).
- 2026-10-28: Added the Resolution section: the `ranking.calls_resolver` poller, the `call.resolved` event, flow 19a, and the `ranking.calls.vote_retracted` handler (#6445).
- 2026-10-28: Added the Backfill section: nine `monacoctl backfill` commands, one per `ranking_calls` handler, and `backfillSampleAge` (36 h) as the accepted sample age for a call whose `resolve_at` is more than 7 days back, since `market` keeps one sample a day after a week (#6452).
- 2026-10-28: The calls consumer reads decision time from the event's `created_at` instead of the delivery clock, and `vote.retracted` upserts the voter row, so a backfill dates calls by their decision and converges in any order (#6452).
