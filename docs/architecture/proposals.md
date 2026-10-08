# Proposals

**Status:** Decided 2026-09-26. Reconciled 2026-09-27 with [backend-platform.md](backend-platform.md), which wins where the two conflict. Owned by the `governance` module and built in [Rollout](backend-platform.md#rollout) step 4 (flows 9, 10 and 13). The old backend has most of it; see [Gap between this and the code](#gap-between-this-and-the-code).

## Decision

A proposal is a plain CRUD resource in the `governance` module. It owns its votes, carries a single status, and knows nothing about Solana or Jupiter. The only link to trading is one event: when a vote pushes the proposal over its threshold, the proposal flips to `passed` and emits `proposal.passed` for the [trade engine](trade-execution.md) in the `trading` module. Nothing about execution is checked on any other vote.

Governance never imports another module. It reads other modules' state through their read-only query ports and changes it only through events ([dependency rules](backend-platform.md#dependency-rules-enforced-by-depguard)).

Later, cabal chat messages can reference a proposal and open a thread on it. The proposal does not change for that; chat points at the proposal, not the other way round.

## Why

- **Proposals are the social object, trades are the money object.** Keeping the proposal dumb (create, read, list, vote, status) means the UI, the feed, notifications and chat can all read it without touching money code.
- **Execution checks are expensive and only matter once.** Reading on-chain treasury balance, fetching a fresh quote and checking slippage costs RPC and Jupiter calls. Running them on every vote wastes calls and gives voters a moving answer. They run once, when the threshold is met, inside the trade engine.
- **One owner per rule.** Governance decides *whether the cabal agreed*. The trade engine decides *whether the trade can happen right now*. A passed proposal that fails a pre-trade check is still a proposal the cabal agreed to; it records why it did not execute.

## Data model

```
proposals 1 ──< proposal_voters    voter set frozen at creation
proposals 1 ──< votes              (proposal_id, voter_id) primary key
proposals 1 ─── feed_objects 1 ──< feed_comments  (social module; comments live on the proposal's feed item)
proposals 1 ──< swaps              trading module; at most one live (non-failed) row
proposals 1 ──< cabal chat messages   future: chat messages that reference a proposal
```

Only `proposals`, `proposal_voters` and `votes` are governance tables. The others belong to other modules and reference the proposal by id.

**`proposals`**

| Column | Notes |
| --- | --- |
| `id`, `cabal_id`, `proposer_id` | Branded UUIDv7 ids ([Money and types](backend-platform.md#money-and-types)). |
| `kind` | `buy`, `sell`, or an agent action (`agent_add`, `agent_pause`, `agent_resume`, `agent_remove`) |
| `symbol`, `mint` | The asset's display symbol and its Solana mint address. Both are carried on `proposal.created` and `proposal.passed`. |
| `usdc_micros` / `token_amount` | Trade amount. Exactly one is set: `usdc_micros` for a buy, `token_amount` (the mint's base units) for a sell. On `proposal.passed`, `usdc_micros` is a `money.Micros` and `token_amount` is a `uint64` encoded as a JSON string. Immutable once the first vote is cast. |
| `thesis` | Proposer's short reason, shown on the card. |
| `quote_out_amount` | Quote at creation. Reference price for the slippage check at execution (see [trade-execution.md](trade-execution.md#stage-2-trade-engine)). |
| `threshold` | `majority` or `unanimous`, copied from the cabal's rules in the `ProposeTrade` transaction. Immutable. Vote tallies and the `need` in proposal reads use this value, never the cabal's current rule. |
| `status` | See below. |
| `status_reason` | The `errs` code from `trade.blocked` when `execution_blocked`; null otherwise. The user-facing message comes from the [code table](backend-platform.md#errors). |
| `void_reason` | The reason given when the proposal is `voided`; null otherwise. It is carried as `reason` on `proposal.voided`. |
| `expires_at`, `created_at`, `updated_at` | |

**`proposal_voters`**: `proposal_id`, `voter_id`. The cabal's voter set, copied from the cabal module's query port in the `ProposeTrade` transaction. Members who join or leave mid-vote do not change it, so the number of yes votes a proposal needs is fixed when it opens.

**`votes`**: `proposal_id`, `voter_id`, `choice` (`yes`/`no`), `cast_at`. One ballot per voter per proposal. A voter may change their ballot while the proposal is `open` (upsert on the primary key); ballots are frozen once status leaves `open`.

**Comments**: a proposal's comments are the FeedComments on its `proposal` feed item ([feed.md](feed.md#comments)), owned by the `social` module. Nothing migrates from the old `proposal_comments` table: the new backend starts on an empty database. Comments stay open after the proposal closes so people can discuss the result. Comments and the proposal's chat thread stay separate streams.

## Status

```
open ──► passed ──► executed
  │         └─────► execution_blocked
  ├──► failed
  ├──► expired
  ├──► withdrawn
  └──► voided   (also from passed, with no live swap)
```

| Status | Set by | Meaning |
| --- | --- | --- |
| `open` | `ProposeTrade` | Accepting votes. |
| `passed` | Tally | Threshold met. `proposal.passed` emitted in the same transaction. |
| `failed` | Tally | Enough `no` votes that `passed` is now impossible. Emits `proposal.failed`. |
| `expired` | Expiry sweep | `expires_at` reached while still `open`. Emits `proposal.expired`. |
| `withdrawn` | `WithdrawProposal` | The proposer withdrew before anyone else voted. Only the proposer can withdraw. Emits `proposal.withdrawn`. |
| `executed` | Governance consumer of `trade.confirmed`, or of `agent.enabled` / `.paused` / `.removed` for agent kinds | The linked swap confirmed. Emits `proposal.executed`, which the feed may use. |
| `execution_blocked` | Governance consumer of `trade.blocked` and `trade.failed` | A pre-trade check failed, or the swap failed (`status_reason` `swap_failed`). `status_reason` holds the code. Emits `proposal.execution_blocked`. Terminal; voters re-propose if they still want it. Only `swap_failed` is retryable: `RetryTrade` accepts it, and governance's consumer of `trade.retry_requested` moves it back to `passed` and emits `proposal.reopened`. |
| `voided` | Admin `VoidProposal` | Removed by Monaco with a reason. Only from `open`, or `passed` with no live swap (asked through trading's query port). Emits `proposal.voided`. See [analytics-admin.md](analytics-admin.md#actions). |

The status is a Go type with a `transitions` table and a pure `Next(from, event)` ([State machine](backend-platform.md#patterns-and-where-each-earns-its-place)). Every transition is a guarded update (`WHERE status = $expected`), the same pattern as `swaps`, so a racing vote, expiry sweep and trade-event consumer can never double-transition. Every transition appends its `events` row in the same `uow.Do`.

For agent proposals (`agent_*` kinds), `passed` emits `proposal.passed` like any other kind. The `agents` module consumes it, applies the change, and emits `agent.enabled`, `agent.paused` or `agent.removed` ([flow 16](backend-platform.md#flows)). Governance does not write agent tables. Governance consumes `agent.enabled`, `agent.paused` and `agent.removed`, sets the proposal to `executed` and emits `proposal.executed`, the same way it handles `trade.confirmed`. [Flow 16](backend-platform.md#flows) lists `governance` among its consumers for this.

## Vote path

`CastVote` ([flow 10](backend-platform.md#flows)), from `POST /v1/proposals/{id}/votes` with an `Idempotency-Key`. One `uow.Do`:

1. Lock the proposal row. Refuse with a `Blocked` code if not `open`, or `Forbidden` if the caller is not in the proposal's frozen voter set (`proposal_voters`).
2. Upsert the ballot.
3. Count yes and no against the frozen voter set and the proposal's own frozen `threshold` (majority or unanimous), read from the locked row. This is a pure domain function over integers, no network.
4. If still undecided, commit and return. **No execution check.**
5. If `failed`, guarded update to `failed`, append `proposal.failed`, commit.
6. If `passed`, guarded update to `passed`, append `proposal.passed`, commit. The trade engine picks it up over the bus. The HTTP response returns `passed` immediately. Execution is asynchronous, and the client sees `executed` or `execution_blocked` on the proposal detail after an SSE hint or a push.

Expiry is a worker poller over `open` proposals past `expires_at` (indexed), with the same guarded update. It runs under the poller advisory lock ([Deploy rule 1](backend-platform.md#deploy-and-observability)) and reads time from the injected `clock.Clock`.

## CRUD surface

Every cabal is public, so anyone signed in can read a cabal's proposals. Non-members are view-only: they cannot vote (they are not in the voter set), and they cannot comment on another cabal's proposals until moderation ships ([feed.md](feed.md)).

Routes follow the [`cabal` naming](backend-platform.md#decided). Every mutating route takes an `Idempotency-Key` ([Thin client](backend-platform.md#thin-client)). The legacy `/v1/groups/{id}/…` routes stay only until the iOS cutover in Rollout step 7.

| Route | Command | Does |
| --- | --- | --- |
| `POST /v1/cabals/{id}/proposals` | `ProposeTrade` ([flow 9](backend-platform.md#flows)) | Create; emits `proposal.created`. Advisory checks only: asset in catalog and Jupiter can route, through the market module's query port; amount ≤ pot, against `treasury`'s `PotValue(ctx, cabalID)` on its query port ([leaderboards.md](leaderboards.md#2-positions-from-the-ledger)). |
| `GET /v1/cabals/{id}/proposals` | query | List, filter by status, newest first, keyset paginated. |
| `GET /v1/proposals/{id}` | query | Detail: proposal, tally, the caller's ballot, who voted, comment count, linked swap and its state (from trading's query port). |
| `DELETE /v1/proposals/{id}` | `WithdrawProposal` ([flow 13](backend-platform.md#flows)) | Proposer withdraws; nobody else can. Allowed only while `open` and before any other member has voted. Soft delete to `withdrawn`. |
| `POST /v1/proposals/{id}/votes` | `CastVote` | Cast or change a ballot. |
| `GET/POST /v1/proposals/{id}/comments` | `CreateComment` (social module, [flow 21](backend-platform.md#flows)) | Comment thread on the proposal's feed item. Members only until moderation ships. |

There is no `PATCH` for trade parameters. Changing what the cabal is voting on after votes are cast would invalidate those votes; withdraw and re-propose instead.

The acceptance tests, outcome codes and crash points for flows 9, 10 and 13 live in their [flow files](backend-platform.md#flows), not here.

## Future: referencing a proposal from cabal chat

Goal: in cabal chat, a member can drop a proposal into the conversation, it renders as a live card (status, tally, vote buttons), and replies to it form a thread.

Planned shape, not built yet:

- The cabal chat messages table (today `group_messages`) gains nullable `proposal_id` (same `cabal_id` enforced) and nullable `parent_id` (see [chat.md](chat.md#threads)). Chat lives in the `social` module, so `proposal_id` is a reference, not a cross-module foreign key that governance maintains.
- A message with `proposal_id` renders as the proposal card. Replies set `parent_id` to that message.
- When a proposal is created, the `social` module consumes `proposal.created` and posts a system message into the cabal's chat with `proposal_id` set, so every proposal has a chat anchor by default. The card reaches clients as an ordinary `message.created` carrying `proposal_id`.
- The proposal's FeedComments and its chat thread stay separate. Feed comments are the public discussion; the chat thread is the members' conversation.

## Alternatives considered

| Alternative | Why not |
| --- | --- |
| Run pre-trade checks on every vote to show "would execute" live | Cost and noise. Balance and price move; voters would see a flickering answer. Checks run once, at pass. |
| Execute synchronously in the vote request that crosses the threshold | Ties the last voter's request to a multi-second swap and loses the trade on crash. See [trade-execution.md](trade-execution.md#alternatives-considered). |
| Store execution state only on the swap row, no `executed` / `execution_blocked` on the proposal | Every list query would ask trading to render a card. A status written by governance's consumers of trade events keeps list reads one-table. |
| Apply agent proposals in the tally transaction | Governance would write the `agents` module's tables, which `depguard` forbids. The agents module consumes `proposal.passed` instead. |
| Editable proposals | Invalidates cast votes. Withdraw and re-propose instead. |
| Merge a proposal's feed comments and its chat thread into one stream | Mixes the public discussion with the members' conversation. Kept separate (default 2026-09-27). |
| Voter set resolved live at tally time (the old backend) | Joins and leaves mid-vote move the threshold under voters. Frozen at creation instead. |
| Chat threads as a separate table from chat messages | More tables for the same shape. A `parent_id` on messages is enough. |

## Gap between this and the code

The rewrite replaces the old backend rather than refactoring it ([backend-platform.md](backend-platform.md)). What the old backend did before M7 deleted it (checked 2026-09-27 at `242c2609`; `apps/backend/internal/app/governance.go` and legacy migrations `000005`, `000009`, `000013`, `000014`, `000015`):

- `proposals`, `votes`, `proposal_comments`, `group_messages` tables. Statuses `open`, `passed`, `failed`, `expired`.
- Tally after every vote via `domain.TallyProposal` with guarded `UpdateProposalStatusTx`. Cheap and network-free on undecided votes.
- Execution trigger is the 15 s proposal execute poller scanning `passed` proposals, not an event.
- Create, list, detail, vote, comment routes under `/v1/groups/{id}/proposals` and `/v1/proposals/{id}`.

Nothing migrates: the new backend starts on an empty database at cutover. What the rewrite adds in Rollout step 4: `proposal_voters`; statuses `executed`, `execution_blocked`, `withdrawn`, `voided`; `status_reason` and `quote_out_amount`; `proposal.*` events from each transition; the withdraw route; `/v1/cabals` routes. The chat reference (`proposal_id` on chat messages, the system message) waits for the Chat decision.

## Open questions

None at the moment.

Log: [log/proposals.md](log/proposals.md).
