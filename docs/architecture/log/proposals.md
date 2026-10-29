# Proposals log

Dated record of changes to [proposals.md](../proposals.md). Add one line per change, newest last.

- 2026-10-07: A failed swap blocks its passed proposal with `status_reason` `swap_failed` (governance consumes `trade.failed`). `RetryTrade` still accepts that proposal, and governance consumes `trade.retry_requested` to reopen it to `passed` and emit `proposal.reopened` (#3472).
- 2026-09-29: The propose-time pot check reads `treasury`'s `PotValue(ctx, cabalID)` query port, not market (default; see #535).
- 2026-09-27: Default 2026-09-27 (reversible): governance consumes `agent.enabled`, `agent.paused` and `agent.removed` to mark agent proposals `executed`. Closes the last open question.
- 2026-09-27: Applied the 2026-09-27 decisions. Decided 2026-09-27: every cabal is public; the new backend starts on an empty database, so nothing migrates from `proposal_comments`. Default 2026-09-27 (reversible): voters can change their ballot while `open`; the voter set is frozen at creation in `proposal_voters`; only the proposer can withdraw; feed comments and the chat thread stay separate; governance emits `proposal.executed` and `proposal.execution_blocked`; non-members are view-only on other cabals' proposals until moderation; proposal cards reach chat as `message.created` with `proposal_id`; the linked swap lives in trading's `swaps` table. Still open: how agent proposals reach `executed`.
- 2026-09-27: Reconciled with [backend-platform.md](../backend-platform.md). Owned by the `governance` module; reads cabal, market and trading state through query ports. `group_id` renamed `cabal_id`; routes move to `/v1/cabals/{id}/proposals` (legacy `/v1/groups` until Rollout step 7). Commands named (`ProposeTrade`, `CastVote`, `WithdrawProposal`, `VoidProposal`) and `proposal.withdrawn` added. `executed` and `execution_blocked` set by governance consumers of `trade.confirmed` and `trade.blocked`. Agent proposals no longer apply in the tally transaction; the `agents` module consumes `proposal.passed`. `agent_revoke` renamed `agent_remove`. `status_reason` holds an `errs` code. Comments and chat belong to `social`. Integer money types, state-machine pattern, expiry under the poller lock, Rollout step 4.
- 2026-09-26: Outbox rows replaced by `events` rows delivered over the NATS event bus ([event-bus.md](../event-bus.md)).
- 2026-09-26: Added `voided` status (admin action).
- 2026-09-26: Comments moved to FeedComment on the proposal's feed item (feed decision).
- 2026-09-26: Initial decision. Proposals are simple CRUD with votes, comments and status; execution checks only on threshold met; future chat references planned.
- 2026-10-07: The pass threshold is copied onto the proposal at creation, so a cabal rules change no longer moves an open vote (#3464).
- 2026-10-09: Governance's reopen on `trade.retry_requested` is skipped when the swap that was retried has already been followed by a newer swap that failed, so the proposal stays `execution_blocked` instead of ending `passed` with a failed swap (#4749).
- 2026-10-11: A ballot is refused with a `Blocked` code once the proposal's `expires_at` has passed, even if the expiry poller has not reached it yet (#4750).
- 2026-10-21: `network` is accepted on a sell. It names the location the proposal sells from, and a sell without it keeps selling the Solana holding (#5851).
- 2026-10-28: Undoing a ballot appends `vote.retracted`, which analytics exports as `vote_retracted`, and flow 10a covers the route (#6439).
- 2026-10-28: A proposal stores the pot at open, the time it left `open` and the voter who decided it, and the detail and vote responses return the treasury share, close times, reply count and the caller's place in the vote (#6438).
- 2026-10-28: The proposal detail returns the live price, the buy's share estimate and Since, and the swap's fee and fill time (#6448).
- 2026-10-28: A proposal stores the asset's price when the vote passed, and the detail returns it with the proposer's name and photo (#6457).
- 2026-10-28: `GET /v1/me/market-facts` answers a watched stock with its Monaco fact, ranked vote open, held, called (#6447).
- 2026-10-28: The proposal detail keeps each voter's side private until the proposal closes in a cabal whose rule was on when it opened, and gives every voter a name and photo (#6468).
- 2026-10-29: A voter who has not decided can ask for a reminder an hour before close with `PUT /v1/proposals/{id}/reminder`, clear it with `DELETE`, and see `reminder_set` on the detail. Voting drops the reminder (#6478).
- 2026-10-29: A poller raises `proposal.close_reminder_due` once for a member who asked for a close reminder and still has not voted, with the live tally (#6492).
