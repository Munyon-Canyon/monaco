# Cabals

**Status:** Decided 2026-09-27

## Decision

The `cabal` module owns cabals, members, access requests and treasury wallets ([Table ownership](backend-platform.md#table-ownership)). Its commands are flows 2 to 4 in [Flows](backend-platform.md#flows): `CreateCabal`, `JoinCabal`, `RequestAccess`, `InviteMember`, `DecideAccess`, `LeaveCabal`, plus `UpdateCabal`. The user-facing rules are in [product.md](../product.md#cabals) and the tables in [data-model.md](data-model.md#decision).

- A cabal has two roles, `creator` and `member`. The creator decides access requests and is the only one who edits the cabal.
- Joining is direct in an `open` cabal and goes through a request the creator decides in a `request` cabal.
- Invites come in two kinds. An **invite code** is a random token, never the cabal id. A **direct invite** targets one user by handle and lives 7 days.
- Leaving is refused while the member holds share units, while the last member leaves a pot worth more than zero, and while the creator leaves other members behind.
- The treasury wallet is created before the create transaction opens, idempotently.

## Why

- **Every M10 ticket needs the same answers.** Without one file, each backend ticket would pick its own invite lifetime, approver and leave guard.
- **The cabal id is not a secret.** Every cabal is public ([data-model.md](data-model.md#decision)), so ids appear in feeds and links. Today's app uses the id as the invite code, which makes any cabal's invite guessable from its URL.
- **Two roles are enough for the MVP.** Product names one person who approves requests, the creator. A third role adds a permission matrix nobody has asked for.
- **Both directions in one table.** A request and an invite are the same fact, a pending link between a user and a cabal, differing only in who started it. One table gives one uniqueness rule and one poller.

## How it works

### Rules

The creator sets these at `CreateCabal`. `UpdateCabal` can change them later ([Edit](#edit)).

| Rule | Values | Default |
| --- | --- | --- |
| `join_mode` | `open` \| `request` | none, the creator picks |
| `voter_mode` | `all` \| `list` | none, the creator picks |
| `threshold` | `majority` \| `unanimous` | none, the creator picks |
| `proposal_expiry_seconds` | 3600, 86400 or 604800, the three the app offers today | none, the creator picks |
| `slippage_bps` | 1 to 300 ([trade-execution.md](trade-execution.md#stage-2-trade-engine)) | 100 |

- **Voter list.** With `voter_mode = list`, the voters are the members whose `cabal_members` row can vote. The list always contains the creator, so it never empties while members remain. A member who joins later is not a voter until the creator adds them. With `voter_mode = all`, every member votes.
- **Name.** 3 to 40 characters after trimming whitespace. Names are not unique.
- **Validation.** Every rule is checked in the command. A value outside its set is refused with an `Invalid`-kind code.

### Roles

| Role | Can |
| --- | --- |
| `creator` | Everything a member can, plus decide access requests, invite in a `request` cabal, revoke any invite, and run `UpdateCabal`. |
| `member` | Read the cabal, chat, invite in an `open` cabal, revoke their own invites, leave. Proposing and voting depend on the voter set, not the role. |

There is exactly one creator per cabal, and the role never moves. There is no admin role in the MVP ([Open questions](#open-questions)).

### Join and access requests

| `join_mode` | What a signed-in non-member does | Result |
| --- | --- | --- |
| `open` | `JoinCabal` | Joins at once. Emits `cabal.member_joined`. |
| `request` | `RequestAccess` | Inserts a `cabal_access_requests` row with `direction = request`, `status = pending`. Emits `cabal.access_requested`. |

`DecideAccess` resolves a pending request:

| Actor | Decision | Status | Effect |
| --- | --- | --- | --- |
| creator | approve | `approved` | Inserts the member row in the same Unit of Work. Emits `cabal.access_decided` and `cabal.member_joined`. |
| creator | deny | `denied` | Emits `cabal.access_decided`. |
| requester | revoke | `revoked` | Emits `cabal.access_decided`. |

- Requests do not expire. They stay `pending` until someone decides them.
- One pending row per (cabal, user), in either direction: a partial unique index on `(cabal_id, user_id) WHERE status = 'pending'`. A second `RequestAccess` from the same user returns the existing request. A `RequestAccess` while an invite is pending is refused; the user accepts the invite instead.
- A denied or revoked user can request again, which inserts a new row.
- `JoinCabal` on a `request` cabal and `RequestAccess` on an `open` cabal are refused with a `Blocked`-kind code, so the app always calls the command that matches the mode it read.
- A member calling `JoinCabal` or `RequestAccess` gets the existing membership back, not an error.

### Invites

**Invite code.**

- A random 10-character Crockford base32 token (50 bits), stored on `cabals.invite_code` with a unique index. It is generated at `CreateCabal`.
- Only members see it. Non-members never read it from any route.
- It is never the cabal id. Pasting the code resolves the cabal, and then the normal path for its `join_mode` runs: `open` joins, `request` files a request. The code grants no extra access.

**Direct invite.**

- `InviteMember` targets a user by handle, resolved through `identity`'s query port. It inserts a `cabal_access_requests` row with `direction = invite`, `status = pending`, the inviter's id and `expires_at = now() + 7 days`. Emits `cabal.access_requested`.
- Who may invite: any member in an `open` cabal, only the creator in a `request` cabal.
- Inviting an existing member is refused. Inviting a user with a pending request in a `request` cabal is refused; the creator approves the request instead.
- The invitee accepts or declines through `DecideAccess`. Accepting joins directly in either `join_mode` (`approved`, plus `cabal.member_joined`). Declining sets `denied`.
- The inviter or the creator can revoke a pending invite (`revoked`).
- A worker poller sets `expired` on pending invites past `expires_at`, one guarded update per row (`WHERE status = 'pending'`), and emits `cabal.access_decided` with decision `expired`. An accept that races the poller loses on the same guard and is refused.

### Edit

- `UpdateCabal` is creator only. It changes the name, the picture and any rule in [Rules](#rules), the voter list included.
- A rules change applies to proposals created afterwards. Open proposals keep the voter set frozen at their creation ([proposals.md](proposals.md#data-model)) and their `expires_at`.
- Two rules are still read live today. The vote path reads `threshold` ([proposals.md](proposals.md#vote-path)) and the trade engine reads `slippage_bps` ([trade-execution.md](trade-execution.md#stage-2-trade-engine)) through this module's query port when they run. Until governance copies both onto the proposal at creation, a change to either reaches open proposals ([Open questions](#open-questions)).
- Switching `join_mode` from `request` to `open` leaves pending requests as they are. The creator can still decide them, and the requester can now join directly; that `JoinCabal` sets their pending request to `approved` in the same Unit of Work.
- Emits `cabal.updated` with the fields that changed.

### Leave

`LeaveCabal` is refused, with a `Blocked`-kind code, in these cases:

| Case | Code |
| --- | --- |
| The leaver holds share units in this cabal. The app routes them to cash out first ([Cash out](deposits-withdrawals.md#cash-out)), so from the user's side they cash out as they leave. | `LeaveHoldsShares` |
| The leaver is the last member and the pot value is above zero. | `LeaveLastMemberPotNotEmpty` |
| The leaver is the creator and other members remain. | `LeaveCreatorWithMembers` |

- Share units and pot value come from `treasury`'s query port (`PotValue`, [data-model.md](data-model.md#decision)). `cabal` never reads treasury tables.
- A successful leave deletes the member row and emits `cabal.member_left`. If the leaver was on the voter list, they leave it too; the creator stays on it, so the list never empties while members remain.
- The last member leaving an empty pot, the creator included, leaves a cabal with no members. The cabal row and its treasury wallet stay.

### Treasury wallet

- `CreateCabal` creates the treasury synchronously through `platform/chain`, as an app-owned Privy wallet ([backend-platform.md](backend-platform.md#table-ownership)).
- The wallet call is idempotent by a key derived from the caller's `Idempotency-Key`, so a retried create gets the same wallet back.
- The call runs **before** the create Unit of Work opens. The Unit of Work then inserts the `cabals` row, the creator's `cabal_members` row, the `treasury_wallets` row and the events, in one transaction.
- A create that fails after the wallet exists leaves an unused wallet. The command logs it once with the Privy wallet id. A retry with the same `Idempotency-Key` reuses it.
- A Privy outage fails the create with an `Unavailable`-kind code and writes nothing.

### Events

Every payload carries `v` ([event-bus.md](event-bus.md)). Timestamps live on the event row.

| Event | Emitted by | Payload |
| --- | --- | --- |
| `cabal.created` | `CreateCabal` | `cabal_id`, `creator_id`, `name`, `join_mode`, `voter_mode`, `threshold`, `proposal_expiry_seconds`, `slippage_bps`, `treasury_address` |
| `cabal.member_joined` | `CreateCabal` (the creator), `JoinCabal`, `DecideAccess` approve or accept | `cabal_id`, `user_id`, `role`, `via` (`create` \| `open` \| `request` \| `invite`), `request_id` when via a request or invite |
| `cabal.access_requested` | `RequestAccess`, `InviteMember` | `request_id`, `cabal_id`, `user_id` (the requester or invitee), `direction` (`request` \| `invite`), `actor_id` (the requester or inviter), `expires_at` for an invite |
| `cabal.access_decided` | `DecideAccess`, the expiry poller | `request_id`, `cabal_id`, `user_id`, `direction`, `decision` (`approved` \| `denied` \| `revoked` \| `expired`), `actor_id` (absent for `expired`) |
| `cabal.member_left` | `LeaveCabal` | `cabal_id`, `user_id`, `was_voter` |
| `cabal.updated` | `UpdateCabal` | `cabal_id`, `actor_id`, `changes`: each changed field with its new value, from `name`, `picture_url`, `join_mode`, `voter_mode`, `voter_ids`, `threshold`, `proposal_expiry_seconds`, `slippage_bps` |

`cabal.member_joined` fires for the creator at create too, so every consumer that tracks membership sees one join per member. The feed skips `via = create`, since `cabal.created` already makes its item.

## Alternatives considered

| Alternative | Why not |
| --- | --- |
| Cabal id as invite code | Guessable. Every cabal is public, so the id is in feed items and links. |
| A third `invite` join mode, where only invited users join | Product names two modes, open and by request. Direct invites already cover inviting into a `request` cabal. |
| An admin role between creator and member | No MVP need. Product gives approval to the creator alone. |
| Invites in their own table | [data-model.md](data-model.md#decision) puts both directions in `cabal_access_requests`, which keeps one pending-row rule and one poller. |
| Requests expire like invites | A request waits on the creator, not the requester. Expiring it punishes the requester for the creator's delay. |

## Open questions

- **Freezing threshold and slippage.** [Edit](#edit) says a rules change applies to later proposals, but [proposals.md](proposals.md#vote-path) counts votes against the cabal's current threshold and the trade engine reads the current slippage. Closing the gap means `ProposeTrade` copies both onto the proposal, which is a governance change.
- **Admin role.** Whether a creator can name co-admins who decide requests and invite. Not in the MVP.
- **Invite code rotation.** Whether the creator can rotate a leaked code. Not in the MVP; the code grants no access beyond the join mode, so a leak in a `request` cabal only produces requests.

## Log

- 2026-09-27: Created. Rules, roles, join and request, invite code and direct invite, edit, leave guards, treasury wallet creation and events decided (#525).
