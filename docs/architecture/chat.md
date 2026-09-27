# Chat

**Status:** Decided 2026-09-27. The core shape was proposed 2026-09-26: store first then publish on Ably, one level of threads with "also send to channel", and a per-member seen watermark. Reconciled with [backend-platform.md](backend-platform.md) 2026-09-27: chat is flow 22 in [`flows.tsv`](backend-platform.md#flows), owned by the `social` module, built in [Rollout](backend-platform.md#rollout) step 6, and keeps Ably for delivery as the RFC decides. The remaining open points took defaults 2026-09-27.

## Decision

Every cabal has one internal chat, visible only to its members. Chat messages live in **CabalChat**, the `cabal_messages` table (today's `group_messages`, renamed under the RFC's [`cabal` everywhere](backend-platform.md#decided) rule). The database is the source of truth. Ably is only the delivery pipe that makes chat real time.

The `social` module owns chat: its tables, the send and seen commands, the Ably token route and the Ably publish ([Repository layout](backend-platform.md#repository-layout)). Membership belongs to the `cabal` module, so `social` checks it through the `cabal` module's read-only query port and never reads `cabal_members` directly ([Dependency rules](backend-platform.md#dependency-rules-enforced-by-depguard)).

1. **Send.** The user types a message and presses enter. The app calls `POST /v1/cabals/{id}/messages`. The backend inserts the row into CabalChat and commits. Only after the commit does it publish a `message.created` event on the cabal's Ably channel. Every other member with the chat open gets the event and renders the message right away.
2. **Threads.** Any top-level message can have a thread. A reply is a CabalChat row whose `parent_id` points at the top-level message. Threads are one level deep, as in Slack.
3. **Also send to channel.** When replying in a thread, the user can tick "Also send to channel". The reply stays in the thread and also shows in the main channel, with a "replied to a thread" label that links back.
4. **Seen by N.** A `social`-owned `chat_seen` row per member per cabal holds a `last_seen_at` timestamp. The app bumps it while the member has the chat open. For the most recent message, "seen by N" is the count of other members whose `last_seen_at` is at or after that message's `created_at`. That is one indexed count query, with no per-message read rows.

## Why

- **Store, then publish.** A message that shows on screens but never reached the database would vanish on the next reload. Committing first means every event on Ably refers to a message that exists. If a publish is lost, the clients' reconnect fetch picks the message up anyway (see [Catching up](#catching-up-after-a-disconnect)).
- **Ably, not our own sockets.** Ably handles fan-out, reconnects, connection state recovery and presence across flaky mobile networks. The RFC's [SSE hub](backend-platform.md#sse-hub) carries droppable "re-fetch" hints; chat wants the message itself delivered, resumed without loss after a short drop, and presence. Today's 4 s poll (`apps/mobile/Monaco/Features/Groups/GroupChatView.swift`) feels laggy for chat, and it costs one request per open screen every 4 s.
- **Only the backend publishes.** Clients get subscribe-only Ably tokens. A client cannot put a message on the channel without going through the API, so there is nothing to spoof, membership checks happen once, and every message has already been validated and stored.
- **A watermark, not read receipts.** Per-message receipts grow as messages × members, and every open screen would write rows. One timestamp per member answers "who has seen the latest message" with a single count, and one row update per member keeps it current. The trade-off: we can only say "seen" for messages up to the watermark, not exactly which messages a member scrolled past. For "seen by N" on the latest message, that is enough.

## How it works

### Tables

CabalChat, which extends today's `group_messages`:

```
cabal_messages                  (CabalChat)
  id               uuid         UUIDv7
  cabal_id         uuid         -> cabals
  author_id        uuid         -> users
  body             text
  created_at       timestamptz
  parent_id        uuid?        -> cabal_messages.id; null for a top-level message
  also_in_channel  boolean      default false; only true when parent_id is set
  reply_count      int          default 0; on top-level messages, denormalized
  last_reply_at    timestamptz? on top-level messages, denormalized
  proposal_id      uuid?        set on a proposal card message
  deleted_at       timestamptz? soft delete
```

Constraints:

- A parent must be a top-level message in the same cabal: `parent.parent_id IS NULL` and `parent.cabal_id = cabal_id`. The insert enforces this, because a CHECK constraint cannot look at another row.
- `CHECK (also_in_channel = false OR parent_id IS NOT NULL)`.
- Indexes:
  - `(cabal_id, created_at DESC, id DESC)`, which exists today on `group_messages`, for the channel list.
  - `(parent_id, created_at, id)` for threads.

`parent_id` replaces the `thread_root_id` sketched in [proposals.md](proposals.md#future-referencing-a-proposal-from-cabal-chat). Because threads are one level deep, the parent is the root. A proposal card message is an ordinary top-level message with `proposal_id` set, so its thread uses the same column (default 2026-09-27). It reaches open screens through the same `message.created` event, whose payload carries `proposal_id`. Proposal comments and chat threads stay separate streams ([proposals.md](proposals.md)).

The seen watermark, owned by `social`:

```
chat_seen
  cabal_id      uuid
  user_id       uuid
  last_seen_at  timestamptz
  PRIMARY KEY (cabal_id, user_id)
```

Index: `(cabal_id, last_seen_at)`. The earlier draft put `last_chat_seen_at` on `group_members`. That table belongs to the `cabal` module, and `social` cannot write it. A member who leaves loses their row: the `social` consumer of `cabal.member_left` (flow 4) hard-deletes it. That is fine, since a seen watermark is not user content (decided 2026-09-27).

### API

The RFC renames routes from `/v1/groups` to `/v1/cabals` at the iOS cutover, one PR per module ([Decided](backend-platform.md#decided)). The legacy `/v1/groups/{id}/messages` routes live only as long as the old backend. Every mutating call takes an `Idempotency-Key` ([Thin client](backend-platform.md#thin-client)).

| Route | Change |
| --- | --- |
| `POST /v1/cabals/{id}/messages` | `PostChatMessage` command. Body gains optional `parent_id` and `also_in_channel`. Inserts, commits, then publishes to Ably. Idempotent on `Idempotency-Key`, so a retried send cannot post twice. Today's `POST /v1/groups/{id}/messages` does not take one. |
| `GET /v1/cabals/{id}/messages?before=&after=&limit=` | Channel view: `parent_id IS NULL OR also_in_channel`. Each row carries `reply_count`, `last_reply_at`, and for an also-in-channel reply, its `parent_id`. `after` is new and is used for the reconnect catch-up. |
| `GET /v1/cabals/{id}/messages/{messageId}/thread?before=&limit=` | New. Returns the parent and its replies, oldest first. |
| `POST /v1/cabals/{id}/chat/seen` | New. `MarkChatSeen` command. Bumps the caller's `last_seen_at`. See [Seen by N](#seen-by-n). |
| `GET /v1/cabals/{id}/chat/seen?message_id=` | New. Returns `{ count, members: [...] }` for the "seen by" sheet. The count is also included inline on the newest message in the channel `GET`. |
| `DELETE /v1/cabals/{id}/messages/{messageId}` | New. `DeleteChatMessage` command. The author soft-deletes their own message. See [Deleting](#deleting). |
| `POST /v1/realtime/token` | New. Returns an Ably token request for the caller. See [Ably](#ably). |

Refusals (not a member, parent in another cabal, parent is itself a reply, empty body) are `errs` codes and outcomes on flow 22 ([Errors](backend-platform.md#errors)).

### Send flow

1. The app shows the message right away as pending, keyed by the idempotency key.
2. `POST /v1/cabals/{id}/messages`. In one `uow.Do` ([Patterns](backend-platform.md#patterns-and-where-each-earns-its-place)) the backend:
   - checks the caller is a member, through the `cabal` query port;
   - checks the parent rules, if `parent_id` is set;
   - inserts the row;
   - for a reply, updates the parent: `reply_count = reply_count + 1, last_reply_at = now()`;
   - appends the `events` row `chat.message_posted` (ids only, no body).
3. After the commit, the backend publishes `message.created` on `cabal:{cabal_id}` with the full message (id, author, body, created_at, parent_id, also_in_channel). For a reply it also publishes `thread.updated` with the parent's new `reply_count` and `last_reply_at`.
4. The handler returns `201` with the stored message. The app swaps its pending row for the real one. When its own `message.created` echo arrives, the app drops it because it already has that id.
5. If the Ably publish fails, the handler still returns `201` and logs the decision as a registered message with the message id and cause ([Logs as evidence](backend-platform.md#logs-as-evidence)). The message is stored, and the other clients will see it on their next catch-up fetch.

The relay publishes `chat.message_posted` on the NATS event bus ([event-bus.md](event-bus.md)). Side effects that must not be lost are bus consumers of that event. `notify` pushes for @mentions and for replies in threads you started or replied in, and nothing else (default 2026-09-27). There is no digest for other messages. `analytics` can take the event too. See [notifications.md](notifications.md).

The Ably publish in step 3 is a deliberate exception to the rule that side effects are bus consumers. It stays inside the `social` module, so it crosses no module wall. Routing it through a consumer would add a hop to every message for no gain: a lost chat event is harmless because the reconnect fetch covers it. Clients never connect to NATS, so Ably stays the client transport.

### Ably

- **Channel:** one per cabal, `cabal:{cabal_id}`.
- **Event names:**
  - `message.created`
  - `thread.updated`
  - `seen.updated`
  - `message.deleted`
- **Auth:** the backend holds `ABLY_API_KEY` in `.env.local` / `.env.production` via dotenvx, read once at boot through `platform/config`. `POST /v1/realtime/token` returns an Ably `TokenRequest`. Its `clientId` is the user's id. Its capability is `subscribe` on `cabal:{id}` for each cabal the user belongs to, and nothing else. Typing indicators and online presence are not in MVP (default 2026-09-27), so the token grants no `presence`. The TTL is 15 minutes (default 2026-09-27), so a member who leaves loses the channel within 15 minutes with no revocation call. The Ably SDK calls this endpoint through its `authCallback` whenever it needs a new token.
- **Publishing:** the backend uses the Ably Go SDK (`github.com/ably/ably-go`) over REST, with no persistent connection. It sits behind a port in `social/app`, with the SDK in `social/adapters` as an anti-corruption layer ([Patterns](backend-platform.md#patterns-and-where-each-earns-its-place)). The call gets a deadline from config and the RFC's circuit breaker. The `testkit` fake has `Fail` and `FailOnce`, so step 5 is reachable in tests. The `cmd/fakes` server has an Ably REST fake (platform default 2026-09-27), so `verify-backend` drives flow 22 against real binaries ([Verification skill](backend-platform.md#verification-skill)).
- **iOS:** the app uses the `ably-cocoa` SDK (Swift Package Manager). It attaches to the channel when the chat screen appears and detaches when the screen disappears or the app goes to the background.

The Ably SDK is a new direct outside dependency for the app. Up to now the app talks only to the Monaco API and Privy. Ably carries no product decisions, only the delivery of rows the API already stored.

### Catching up after a disconnect

Ably resumes a dropped connection without losing messages for about 2 minutes. After a longer gap, or on a cold open, the app doesn't rely on Ably for history:

- **On chat open:** `GET /messages` for the newest page. Then attach to Ably.
- **On Ably `attached`, after a reconnect:** `GET /messages?after=<newest id the app holds>` to fill the gap.
- **Always:** merge the results and dedupe by message id.

So the API is always the history, and Ably only reports what happened since the last fetch.

### Threads

- The channel shows top-level messages, plus replies sent with "also send to channel". A top-level message with replies shows "N replies · last reply 2m ago", driven by `reply_count` and `last_reply_at`.
- Tapping that opens the thread view, which loads `GET .../thread` and listens on the same Ably channel for `message.created` events whose `parent_id` matches.
- A reply sent with `also_in_channel` shows in both places. In the channel it has a "replied to a thread: <parent snippet>" header that opens the thread.
- Replies to a reply are not possible. Replying to a message in a thread just posts to the same thread.

### Deleting

Soft delete only (decided 2026-09-27), with no editing (default 2026-09-27). The author calls `DELETE /v1/cabals/{id}/messages/{messageId}`. One `uow.Do` sets `deleted_at`. The row stays, so threads, `reply_count` and "seen by" keep their shape. Reads filter `deleted_at IS NULL`, with one exception. A deleted top-level message that has replies comes back as a placeholder with no body, rendered as "deleted", so its thread keeps a parent. After the commit the backend publishes `message.deleted { id }` on the cabal's Ably channel. Admin removal goes through the same column and an audited `admin.action` (flow 26).

### Seen by N

**Writing.** The app calls `POST /v1/cabals/{id}/chat/seen`:

- when the chat screen appears;
- when a `message.created` arrives for the channel while the screen is visible and the app is in the foreground, debounced to one call per 2 s;
- when the app returns to the foreground with the chat screen open.

That is enough to say "continuously while in the chat". The watermark only needs to move when there is something new to have seen, so a periodic heartbeat adds writes and no information.

The server writes its own clock, never the client's, and never moves the watermark backwards:

```sql
INSERT INTO chat_seen (cabal_id, user_id, last_seen_at)
VALUES ($1, $2, $3)                  -- $3 from the injected clock.Clock
ON CONFLICT (cabal_id, user_id) DO UPDATE
SET last_seen_at = GREATEST(chat_seen.last_seen_at, EXCLUDED.last_seen_at)
RETURNING last_seen_at;
```

`created_at` on messages comes from the same injected clock, so the two compare on one clock. A member cannot have seen a message before it committed: the seen call starts after the client received the message, and that happens after the commit.

After the update, the backend recomputes "seen by N" for the newest channel message and publishes `seen.updated { message_id, count }`, so open screens replace the label without doing the comparison themselves ([Thin client](backend-platform.md#thin-client)). It publishes at most once per member per 5 s, to keep the channel quiet when someone sits in the chat during a busy stretch.

**Reading.**

```sql
SELECT count(*)
FROM chat_seen
WHERE cabal_id = $1
  AND user_id <> $2                 -- the message author
  AND last_seen_at >= $3;           -- the message's created_at
```

The "seen by" sheet uses the same `WHERE` and selects user ids; names come from the `identity` query port. Only current members count, because a member's row goes when they leave. The label shows only on the most recent channel message.

"Seen" covers the channel only (default 2026-09-27). Opening the chat screen moves the watermark; thread replies not sent to the channel have no seen state.

**Unread badge** (default 2026-09-27). The cabal row shows an unread count: channel messages with `created_at` after the caller's `last_seen_at`. The server computes it. The cabal list belongs to the `cabal` module, which gets the count from `social`'s query port, so the list is still one `GET` ([Thin client](backend-platform.md#thin-client)).

## Alternatives considered

| Alternative | Why not |
| --- | --- |
| Keep polling every 4 s | Laggy for chat and wasteful: a request every 4 s per open screen even when nobody is talking. The RFC allows timer polling only as an SSE-reconnect fallback. |
| Chat over the RFC's SSE hub (`/v1/stream`) | Hints there are ids and droppable by design. Chat would need its own resume, presence and per-message delivery on top. The RFC's flow 22 keeps Ably. |
| Our own WebSocket server | We'd have to run sticky connections, reconnect handling and fan-out across API instances ourselves. |
| Clients publish straight to Ably, and the backend stores from an Ably webhook or integration | Messages would show before they are stored, validated or checked for membership. Spoofing and a lost webhook both become our problem. Store first. |
| Publish to Ably from a bus consumer | An extra hop (relay, JetStream, consumer) on every message, and a lost chat event does no harm (see [Send flow](#send-flow)). |
| Clients subscribe to NATS directly | Needs per-user NATS credentials and exposes internal subjects. Ably already handles mobile reconnects and token auth. |
| Per-message read receipts (`message_reads` table) | Rows grow as messages × members, and every open screen writes rows. The product only needs "seen by N" on the latest message. |
| Seen watermark as `last_seen_message_id` | Needs a join to compare positions, and it breaks when a message is deleted. A timestamp compares directly with `created_at`. |
| Seen watermark on `cabal_members` | Earlier draft. That table belongs to the `cabal` module; `social` cannot write it. |
| Nested threads (replies to replies) | Harder to follow on a phone, and Slack's one level has proven enough. One level also lets the parent act as the root, so there is no separate root column. |
| A separate thread table | Threads are just messages. One table keeps a single insert path and a single Ably event shape. |

## Open questions

None at the moment.

## Log

- 2026-09-27: Decided: `cabal_messages` soft deletes through `deleted_at`. `chat_seen` stays a `social` table, and its rows are hard-deleted when a member leaves.
- 2026-09-27: Defaults applied: seen covers the channel only; Ably token TTL 15 minutes, `subscribe` only; no typing or presence in MVP; soft delete only, no editing (`deleted_at`, `DELETE` route, `message.deleted`); pushes for @mentions and replies in your threads, no digest; unread badge on the cabal row, computed by the server; proposal cards are messages with `proposal_id` through `message.created`. `cmd/fakes` has an Ably fake. All open questions closed.
- 2026-09-27: Reconciled with [backend-platform.md](backend-platform.md). Owner is the `social` module, flow 22, rollout step 6. Tables and routes renamed to `cabal_messages`, `cabal_id` and `/v1/cabals/{id}`. Event renamed `chat.message_created` to `chat.message_posted`, appended inside `uow.Do`. The seen watermark moves from `group_members.last_chat_seen_at` to a `social`-owned `chat_seen` table, cleared on `cabal.member_left`. Membership checks use the `cabal` query port. `seen.updated` carries the server-computed count. Ably sits behind a port with a `testkit` fake; `cmd/fakes` needs an Ably endpoint. Corrected the claim that today's send route is idempotent. Closed the "one transport or two" question: the RFC keeps Ably for chat and the SSE hub for everything else.
- 2026-09-26: Message insert also appends `chat.message_created` to `events` for bus consumers (mention pushes, analytics). Ably publish stays direct ([event-bus.md](event-bus.md)).
- 2026-09-26: Proposed store-then-publish on Ably, one-level threads via `parent_id` with `also_in_channel`, and `group_members.last_chat_seen_at` for "seen by N".
