# Proposals log

Dated record of changes to [proposals.md](../proposals.md). Add one line per change, newest last.

- 2026-09-29: The propose-time pot check reads `treasury`'s `PotValue(ctx, cabalID)` query port, not market (default; see #535).
- 2026-09-27: Default 2026-09-27 (reversible): governance consumes `agent.enabled`, `agent.paused` and `agent.removed` to mark agent proposals `executed`. Closes the last open question.
- 2026-09-27: Applied the 2026-09-27 decisions. Decided 2026-09-27: every cabal is public; the new backend starts on an empty database, so nothing migrates from `proposal_comments`. Default 2026-09-27 (reversible): voters can change their ballot while `open`; the voter set is frozen at creation in `proposal_voters`; only the proposer can withdraw; feed comments and the chat thread stay separate; governance emits `proposal.executed` and `proposal.execution_blocked`; non-members are view-only on other cabals' proposals until moderation; proposal cards reach chat as `message.created` with `proposal_id`; the linked swap lives in trading's `swaps` table. Still open: how agent proposals reach `executed`.
- 2026-09-27: Reconciled with [backend-platform.md](../backend-platform.md). Owned by the `governance` module; reads cabal, market and trading state through query ports. `group_id` renamed `cabal_id`; routes move to `/v1/cabals/{id}/proposals` (legacy `/v1/groups` until Rollout step 7). Commands named (`ProposeTrade`, `CastVote`, `WithdrawProposal`, `VoidProposal`) and `proposal.withdrawn` added. `executed` and `execution_blocked` set by governance consumers of `trade.confirmed` and `trade.blocked`. Agent proposals no longer apply in the tally transaction; the `agents` module consumes `proposal.passed`. `agent_revoke` renamed `agent_remove`. `status_reason` holds an `errs` code. Comments and chat belong to `social`. Integer money types, state-machine pattern, expiry under the poller lock, Rollout step 4.
- 2026-09-26: Outbox rows replaced by `events` rows delivered over the NATS event bus ([event-bus.md](../event-bus.md)).
- 2026-09-26: Added `voided` status (admin action).
- 2026-09-26: Comments moved to FeedComment on the proposal's feed item (feed decision).
- 2026-09-26: Initial decision. Proposals are simple CRUD with votes, comments and status; execution checks only on threshold met; future chat references planned.
