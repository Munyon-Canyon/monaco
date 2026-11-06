# Chat

**Status:** Decided 2026-09-27. The core shape was proposed 2026-09-26: store first then publish on Ably, one level of threads with "also send to channel", and a per-member seen watermark. Reconciled with [backend-platform.md](backend-platform.md) 2026-09-27: chat is [flow 22](backend-platform.md#flows), owned by the `social` module, built in [Rollout](backend-platform.md#rollout) step 6, and keeps Ably for delivery as the RFC decides. The remaining open points took defaults 2026-09-27.

## Decision

Every cabal has one internal chat, visible only to its members. Chat messages live in **CabalChat**, the `cabal_messages` table (today's `group_messages`, renamed under the RFC's [`cabal` everywhere](backend-platform.md#decided) rule). The database is the source of truth. Ably is only the delivery pipe that makes chat real time.

The `social` module owns chat: its tables, the send and seen commands, the Ably token route and the Ably publish ([Repository layout](backend-platform.md#repository-layout)). Membership belongs to the `cabal` module, so `social` checks it through the `cabal` module's read-only query port and never reads `cabal_members` directly ([Dependency rules](backend-platform.md#dependency-rules-enforced-by-depguard)).

1. **Send.** The user types a message and presses enter. The app calls `POST /v1/cabals/{id}/messages`. The backend inserts the row into CabalChat and commits. Only after the commit does it publish a `message.created` event on the cabal's Ably channel. Every other member with the chat open gets the event and renders the message right away.
2. **Threads.** Any top-level message can have a thread. A reply is a CabalChat row whose `parent_id` points at the top-level message. Threads are one level deep, as in Slack.
3. **Also post in the cabal chat.** When replying in a thread, the user can tick "Also post in the cabal chat". The reply stays in the thread and also shows in the main channel, with a "Replied to a thread" label that links back.
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
  author_id        uuid?        -> users; null on a system row
  kind             text         user | proposal | system; default user
  moment           jsonb?       the payload of a system row
  dedupe_key       text?        unique per cabal; makes a re-delivered moment write once
  body             text?        null on a message that is only an attachment
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

`parent_id` replaces the `thread_root_id` sketched in [proposals.md](proposals.md#future-referencing-a-proposal-from-cabal-chat). Because threads are one level deep, the parent is the root. A proposal card message is an ordinary top-level message with `proposal_id` set, so its thread uses the same column (default 2026-09-27). It reaches open screens through the same `message.created` event, whose payload carries `proposal_id`. A proposal card's thread reads the proposal's own comments instead ([Proposal threads](#proposal-threads)).

The seen watermark, owned by `social`:

```
chat_seen
  cabal_id      uuid
  user_id       uuid
  last_seen_at  timestamptz
  PRIMARY KEY (cabal_id, user_id)
```

Index: `(cabal_id, last_seen_at)`. The earlier draft put `last_chat_seen_at` on `group_members`. That table belongs to the `cabal` module, and `social` cannot write it. A member who leaves loses their row: the `social` consumer of `cabal.member_left` (flow 4) hard-deletes it. That is fine, since a seen watermark is not user content (decided 2026-09-27). A member who joins or rejoins gets a row seeded at their newest message at the time they joined (the `cabal.member_joined` handler on the same durable), so their unread count starts at 0 (2026-11-01).

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
   - parses mentions: each `@handle` in the body that matches a current cabal member's handle, looked up through the `identity` query port, becomes a mentioned user ID. Unknown handles, non-members and the author are dropped. The port reads run before the transaction opens, because each takes its own pool connection, and a failure still stops the post before any write;
   - for a reply, reads the thread participants: the distinct authors of the parent and its live replies who are still members, minus the author, capped at 200;
   - appends the `events` row `chat.message_posted` with ids only, no body: the message, cabal, author and parent IDs, `mentioned_user_ids` and `thread_participant_ids`. Both arrays are always present.
3. After the commit, the backend publishes `message.created` on `cabal:{cabal_id}` with the full message (id, author, body, created_at, parent_id, also_in_channel). For a reply it also publishes `thread.updated` with the parent's new `reply_count` and `last_reply_at`.
4. The handler returns `201` with the stored message. The app swaps its pending row for the real one. When its own `message.created` echo arrives, the app drops it because it already has that id.
5. If the Ably publish fails, the handler still returns `201` and logs the decision as a registered message with the message id and cause ([Logs as evidence](backend-platform.md#logs-as-evidence)). The message is stored, and the other clients will see it on their next catch-up fetch.

The relay publishes `chat.message_posted` on the NATS event bus ([event-bus.md](event-bus.md)). Side effects that must not be lost are bus consumers of that event. `notify` pushes for @mentions, to the users in `mentioned_user_ids`, and for replies to the users in `thread_participant_ids`, and nothing else (default 2026-09-27). There is no digest for other messages. `analytics` can take the event too. See [notifications.md](notifications.md).

The Ably publish in step 3 is a deliberate exception to the rule that side effects are bus consumers. It stays inside the `social` module, so it crosses no module wall. Routing it through a consumer would add a hop to every message for no gain: a lost chat event is harmless because the reconnect fetch covers it. Clients never connect to NATS, so Ably stays the client transport.

### Ably

- **Channel:** one per cabal, `cabal:{cabal_id}`.
- **Event names:**
  - `message.created`
  - `thread.updated`
  - `seen.updated`
  - `message.deleted`
- **Auth:** the backend holds `ABLY_API_KEY` in `.env.local` / `.env.production` via dotenvx, read once at boot through `platform/config`. `POST /v1/realtime/token` returns an Ably `TokenRequest`. Its `clientId` is the user's id. Its capability is `subscribe` on `cabal:{id}` for each cabal the user belongs to, and nothing else. Typing indicators and online presence are not in MVP (default 2026-09-27), so the token grants no `presence`. The TTL is 5 minutes (15 until 2026-10-21). A token request is checked against membership only when it is minted, so the `social_realtime_revoke` consumer revokes every token of the user at Ably (`POST /keys/{keyName}/revokeTokens`, target `clientId:{user id}`) on `cabal.member_left`, on `user.account_status_changed` to anything but `active`, and on `user.deleted`. A client that ignores the detach and keeps its old token is cut off at once, and the SDK asks this endpoint for a new token, which lists only the cabals the user still belongs to. A failed revocation naks and is retried. The Ably key must have "Revocable tokens" enabled in the Ably dashboard, and only tokens issued after that are revocable. A token request minted before the change and redeemed after the revocation is not caught, so the exposure is at most the 2 minutes Ably accepts a token request for plus the 5 minute TTL. The Ably SDK calls this endpoint through its `authCallback` whenever it needs a new token.
- **Publishing:** the backend uses the Ably Go SDK (`github.com/ably/ably-go`) over REST, with no persistent connection. It sits behind a port in `social/app`, with the SDK in `social/adapters` as an anti-corruption layer ([Patterns](backend-platform.md#patterns-and-where-each-earns-its-place)). The call gets a deadline from config and the RFC's circuit breaker. The `testkit` fake has `Fail` and `FailOnce`, so step 5 is reachable in tests. The `cmd/fakes` server has an Ably REST fake (platform default 2026-09-27), so `verify-backend` drives flow 22 against real binaries ([Verification skill](backend-platform.md#verification-skill)).
- **iOS:** the app uses the `ably-cocoa` SDK (Swift Package Manager). It attaches to the channel when the chat screen appears and detaches when the screen disappears or the app goes to the background. The app keeps one connection until sign-out, so a cabal joined after the token was issued fails its attach for lack of capability. The app then asks for one fresh token and attaches again, once per chat open, so a member who was removed is not retried in a loop.

The Ably SDK is a new direct outside dependency for the app. Up to now the app talks only to the Monaco API and Privy. Ably carries no product decisions, only the delivery of rows the API already stored.

### Catching up after a disconnect

Ably resumes a dropped connection without losing messages for about 2 minutes. After a longer gap, or on a cold open, the app doesn't rely on Ably for history:

- **On chat open:** `GET /messages` for the newest page. Then attach to Ably.
- **On Ably `attached`, after a reconnect:** `GET /messages?after=<newest id the app holds>` to fill the gap.
- **Always:** merge the results and dedupe by message id.

So the API is always the history, and Ably only reports what happened since the last fetch.

### Threads

- The channel shows top-level messages, plus replies sent with "also send to channel". A top-level message with replies shows "N replies · last reply 2m ago", driven by `reply_count` and `last_reply_at`.
- A top-level message with replies also names who replied last: `last_repliers` holds up to 3 `ChatAuthor`, newest reply first and one per person, for the thread line's faces (#8393). It leaves out deleted replies, Monaco's note and authors the caller blocked, and is `[]` with no replies, on a reply and on a deleted message. One batched query reads a page's messages with `reply_count > 0`. `thread.updated` carries the same field as unfiltered `ChatAuthor` objects, since a broadcast cannot filter per subscriber, and the app drops the faces of authors it has blocked.
- Tapping that opens the thread view, which loads `GET .../thread` and listens on the same Ably channel for `message.created` events whose `parent_id` matches.
- A reply sent with `also_in_channel` shows in both places. In the channel it has a "Replied to a thread: <parent snippet>" header that opens the thread.
- Replies to a reply are not possible. Replying to a message in a thread just posts to the same thread.

### Deleting

Soft delete only (decided 2026-09-27), with no editing (default 2026-09-27). The author calls `DELETE /v1/cabals/{id}/messages/{messageId}`. One `uow.Do` sets `deleted_at`. The row stays, so threads and "seen by" keep their shape. Deleting a reply also lowers the parent's `reply_count` by one in the same transaction, once, so the count is the number of live replies. Reads filter `deleted_at IS NULL`, with one exception. A deleted top-level message with a nonzero `reply_count` comes back as a placeholder with no body, rendered as "deleted", so its thread keeps a parent. After the commit the backend publishes `message.deleted { id }` on the cabal's Ably channel, and for a reply also `thread.updated` with the parent's lowered `reply_count`. Admin removal goes through the same column and an audited `admin.action` (flow 26).

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

After the update, the backend recomputes "seen by N" for the newest channel message and publishes `seen.updated { message_id, count }`, so open screens replace the label without doing the comparison themselves ([Thin client](backend-platform.md#thin-client)). It publishes at most once per member per 5 s, to keep the channel quiet when someone sits in the chat during a busy stretch, except that a report always publishes when a channel message is newer than that member's last publish.

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

### Reactions

Any member reacts to a live chat message, top level or reply, with one emoji, the way iOS Messages draws a tapback: the emoji sits on the corner of the message with a count. Routes are `PUT` and `DELETE /v1/cabals/{id}/messages/{message_id}/reactions/{emoji}` (flow 22a), and each returns the message's `ReactionSummary`. The `ReactChatMessage` and `UnreactChatMessage` commands mirror the comment reactions of [feed.md](feed.md#reactions): one emoji parsed by `ParseEmoji`, at most 10 distinct emoji per member per message (an 11th is `invalid_input` with `reason=reaction_limit`), and a repeat of either call changes nothing.

- **Rows.** `chat_message_reactions (message_id, user_id, emoji)` is the key, with `cabal_id` repeated so the foreign key to `cabal_messages (cabal_id, id)` makes a message of another cabal impossible. A deleted account cascades its rows away.
- **Checks.** The caller must be a member (`not_cabal_member`). The message must be live and in the cabal, otherwise `chat_message_not_found`, and the command holds a `FOR SHARE` lock on the row so a concurrent delete cannot slip a reaction in.
- **Events.** A real change appends `chat.reaction_added` or `chat.reaction_removed` in the same transaction, carrying the message, cabal, reactor, emoji and the message author. No notification consumes them.
- **Wire.** Every `ChatMessage` the channel, thread and post routes return has `reactions` (at most 20, most used first and then earliest, `mine` for the caller) and `reaction_count`, read for the whole page by one query. A deleted message has none.
- **Realtime.** After the commit the command publishes `reaction.updated` on `cabal:<id>` with `{message_id, reactions: [{emoji, count}], total}`. It has no `mine` because the channel is shared, so the client keeps its own. A failed publish is logged as degraded and does not fail the request.
- **Leaving.** The `social_chat_seen_cleanup` durable also deletes the leaver's reactions in that cabal on `cabal.member_left`, so a left member's reactions stop counting.

### Attachments

A chat message carries at most one attachment: a GIF, a stock or a quote of a proposal's thesis (flow 22b). A poll is a fourth kind, described under [Polls](#polls), and photos never. The shapes are the ones replies on a feed item already use ([feed.md](feed.md)): the `GifAttachmentInput` and `StockAttachmentInput` request schemas, the `GifAttachment`, `StockAttachment` and `ThesisQuoteAttachment` response schemas under `CommentAttachment`, and the same `domain.Attachment` parsers.

- **Rows.** `chat_message_attachments (message_id, cabal_id, kind, payload)` has one row per message. The foreign key to `cabal_messages (cabal_id, id)` cascades. `cabal_messages.body` is nullable, with `body IS NULL OR char_length(body) BETWEEN 1 AND 2000`. `PostChatMessage` insists on a body or an attachment, otherwise `chat_body_invalid`. A blank body beside an attachment is dropped.
- **Request.** `POST /v1/cabals/{id}/messages` takes an optional `body` and an optional `attachment` keyed by `kind`. An unknown kind is `invalid_input`. A `thesis_quote` names a `proposal_id` and an optional `start` and `end`, since a chat message is not on a proposal thread.
- **Thesis rule.** The server cuts the text from the proposal's feed item, read in the same module, and never trusts the client. A proposal of another cabal, an unknown one, or one without a thesis is `invalid_input` with `reason=no_thesis`. A range outside the thesis is `reason=invalid_range`. The stored payload keeps the cut text and the proposer's name.
- **Stock.** The server resolves `asset_id` from the catalog and stores a snapshot (symbol, name, logo). Price and change are read live for each page, as on a reply. An unknown asset is `invalid_input` with `reason=asset_not_found`.
- **Command.** The attachment is resolved before the transaction and inserted in the same transaction as the message, which appends `chat.message_posted` once. The event carries ids only, so `notify` is unchanged.
- **Wire.** `ChatMessage.attachment` is the `CommentAttachment` shape, absent when there is none and on a deleted message. A message that is only an attachment reads back with `body` `GIF`, `Shared a stock` or `Quoted the thesis`, so an older app build draws a readable bubble. A message with text keeps its own text. A page of messages loads its attachments with one query, and the `message.created` event carries the attachment too.

### Polls

A poll asks the cabal something that is not a trade (flow 22c). It mirrors the reply polls of [feed.md](feed.md#comments) with chat keys. A member posts one as the `poll` attachment of `POST /v1/cabals/{id}/messages`, with or without text: the `PollAttachmentInput` of the feed (a question up to 100 characters, 2 to 4 distinct options of up to 40, `closes_in_hours` 1 to 168, default 24) parsed by `domain.ParsePoll`. A poll message with no text reads back with the question as its `body`.

- **Rows.** `chat_poll_options (message_id, position, label)` hangs off `chat_message_attachments`, and `chat_poll_votes (message_id, user_id, position)` holds one vote per member, with the same two indexes as `comment_poll_votes`. The attachment payload keeps the question and `closes_at`. The options and the payload are written in the transaction that stores the message. A deleted account cascades its votes away.
- **Vote.** `PUT /v1/cabals/{id}/messages/{message_id}/poll/vote` with `{position}` casts or moves the caller's vote, and `DELETE` on the same path retracts it. Both return the feed's `PollState` (counts and `my_position`). Only a member can vote (`not_cabal_member`). A message that is unknown, deleted, in another cabal or without a poll is `chat_message_not_found`, a poll past `closes_at` is `poll_closed`, and a position the poll lacks is `invalid_input`. A vote appends `chat.poll_voted` in the same transaction. A retraction appends no event, as on the feed.
- **Wire.** A poll message's `attachment` is the `PollAttachment`, viewer-relative: each option has its `count`, and `my_position` is the caller's. A page of messages loads its options and counts with one query (`ListChatPolls`). Who voted for what is never returned.
- **Realtime.** After the commit a vote or a retraction that changed a row publishes `poll.updated` on `cabal:<id>` with `{message_id, options: [{position, votes}], total_votes}`. It has no `mine` because the channel is shared. A failed publish is logged as degraded and does not fail the request.
- **Leaving.** The `social_chat_seen_cleanup` durable also deletes the leaver's poll votes in that cabal on `cabal.member_left`, so a left member's votes stop counting.

### Pins

A cabal pins up to 3 messages so a member opening the chat sees the house rules first (flow 22f). `cabal_messages` gains `pinned_at` and `pinned_by`, a check that both are null or both set, and a partial index on `(cabal_id, pinned_at DESC)` over the pinned rows.

- **Who may pin.** A cabal admin (role creator or admin, read from the cabal port's `MemberView`) can pin any message, and an author can pin their own. A system row has no author, so only an admin pins it. Anyone else gets `chat_pin_forbidden`. Only a member can pin (`not_cabal_member`).
- **What can be pinned.** A live top-level message. A message that is unknown, deleted or in another cabal is `chat_message_not_found`, and a reply is `invalid_input` with `reason=reply`. A fourth pin is `chat_pin_limit`; the count is taken under a per-cabal advisory lock, so two pins at once cannot pass the limit.
- **Who may unpin.** An admin, or the member who pinned it, otherwise `chat_pin_forbidden`. Pinning a pinned message and unpinning an unpinned one change nothing: no write, no event, no publish.
- **Routes.** `PUT /v1/cabals/{id}/messages/{message_id}/pin` and `DELETE` on the same path return the updated `ChatMessage`. `GET /v1/cabals/{id}/chat/pins` returns `{messages}`, newest pin first, at most 3, for members only.
- **Wire.** Every `ChatMessage` carries `pinned_at` and `pinned_by` (a `ChatAuthor`), both null when the message is not pinned.
- **Deleting.** `SoftDeleteChatMessage` clears the pin in the same statement, so a deleted message leaves the list.
- **Events and realtime.** A change appends `chat.message_pinned` or `chat.message_unpinned` (`{v, message_id, cabal_id, actor_id}`) in the same transaction. After the commit it publishes `pin.updated` on `cabal:<id>` with `{message_id, pinned}`, and the client re-reads the pins route. A failed publish is logged as degraded and does not fail the request. A pin sends no push and writes no system line.

### Moments

A moment is the cabal speaking in its own chat: "Big Ballers was created", "Kai joined", "Bought NVDA at $196.75". It is a `cabal_messages` row with `kind = 'system'`, no author, no body and a `moment` payload (flow 22d). `kind` is `user` (the default, so every earlier row reads as `user`), `proposal` (a new proposal's card row, see [Proposal messages](#proposal-messages)) or `system`. A check constraint fixes the shape of each kind: a `system` row has a null `author_id`, a null `body`, a `moment` and no `parent_id`, and the other kinds have an author and no `moment`. A `user` row with neither a body nor an attachment stays refused by the command, not the database.

- **Payload.** `domain.ChatMoment` is a closed set built by constructors: `cabal_created {}`, `member_joined {user_id}` and `trade_filled {side, symbol, asset_id, price_micros, usdc_micros, filled_at, swap_id, proposal_id?}`, with `price_micros` the fill price per share in USD micros. It holds no free text. An unknown kind is `invalid_input` on write and is skipped on read, so a later build can add kinds.
- **Write.** `WriteChatMoment{CabalID, Moment, At, DedupeKey}` is internal: no route calls it, and later tickets call it from consumers. A unique index on `(cabal_id, dedupe_key)` makes a re-delivered event write once, and the second call returns the stored row and changes nothing. The command appends no `chat.message_posted` event, so a moment sends no push, no mention and no notification. It is the one chat write that appends no event at all, which is why flow 22d lists none.
- **Realtime.** After the commit a new row publishes `moment.created` on `cabal:<id>` with the wired message. `message.created` stays for `user` and `proposal` rows, so an older build ignores moments. A replay publishes nothing.
- **Read.** `GET /v1/cabals/{id}/messages` and the thread route leave system rows out unless the request sets `moments=true`, so a cabal with only user messages reads as before apart from the new `kind` field. On the wire a system row has the nil UUID as `author.id`, a null handle, an empty name and a null `body`. `member_joined` carries the joined member as a `ChatAuthor` read when the page is built, so a rename shows.
- **Writers.** The durable `social_chat_moments` (`adapters/chat_moments.go`) turns events into rows through the same write, inside the handler's transaction. `social.chat_moment.cabal_created` on `cabal.created` writes `cabal_created` at the event time under the key `created`. `social.chat_moment.member_joined` on `cabal.member_joined` writes `member_joined` under `joined:<user_id>:<request_id>`, or the event id when the join has no request, so a rejoin adds a second row. `social.chat_moment.trade_filled` on `trade.confirmed` writes `trade_filled` for a buy or sell under `fill:<swap_id>`, with the fill price per share worked out from the USDC amount and the share amount using the asset's decimals, and `proposal_id` only when the swap came from a proposal. A redelivery hits the key and writes and publishes nothing.
- **Unread and seen-by.** `UnreadCounts` and the seen-by queries skip `system` rows, so "Kai joined" never lights the Chat tab dot. Seen-by of a system row is `chat_message_not_found`.
- **Moderation.** A member cannot delete or report a system row: both are `invalid_input` with `reason=system_message`. An admin removes one through the existing admin route.

### Proposal messages

A new proposal appears in the chat as its proposer's message (flow 22e).

- **Write.** The `social_chat_proposals` durable reacts to `proposal.created` with `social.chat.proposal_created`. It reads the thesis through the governance port and inserts one `cabal_messages` row in the delivery's transaction: `kind = 'proposal'`, `author_id` the proposer, `proposal_id` set, `created_at` the event time and `dedupe_key = 'proposal:<proposal_id>'`, so a redelivery writes once. A proposal that is gone is acknowledged and writes nothing.
- **Body.** The body is the thesis trimmed and cut to 280 characters. It is null when the proposal has no thesis, and always null for a rule change.
- **Realtime and events.** After the commit the row publishes `message.created` with the wired message. It appends no `chat.message_posted` event, because the proposal already pushes through its own notify path. A redelivery publishes nothing.
- **Wire.** The row reads as `kind: proposal` with `proposal_id`, the proposer as `author` and the `body`. The card's tally, status and closing time are not in the message; the app loads them by proposal id.
- **Rules that hold.** The row counts toward unread for the other members and not for the proposer. It takes reactions and replies like any message. A withdrawn or voided proposal keeps its row. Proposals created before this shipped have no row.

### Proposal threads

A proposal card's thread is the proposal's own replies, so a reply reads the same on the proposal page and in Chat (#8391). The store is `feed_comments` on the proposal's feed item, not `cabal_messages` rows with a `parent_id`. The chat only maps them.

- **Read.** `GET /v1/cabals/{id}/messages/{card}/thread` on a `kind = proposal` card finds the proposal's feed item and lists its live user comments in one flat list, oldest first, ordered by `created_at, id`. Top-level comments and folded answers sit side by side, and `before` is a comment id meaning "older than it". A proposal with no feed item yet returns the card with no replies. Deleted and removed comments, Monaco's closing note and comments by authors the caller blocked are left out. Every other message reads as before.
- **Mapping.** Each comment is a `ChatMessage` with `id` and `comment_id` both the comment id, `kind: user`, `parent_id` the card, `also_in_channel: false`, `reply_count: 0` and the comment's body, attachment (GIF, stock, poll, thesis quote), reactions and `created_at`. `reply_to` carries the quote above a reply that answers another comment. The app sends a comment-backed row's reactions, deletes and reports to `/v1/feed/comments/{comment_id}`; the chat routes still refuse that id as `chat_message_not_found`.
- **Count.** A card's `reply_count` and `last_reply_at` are the proposal's live user comments and the newest one's time, read in one batched query for the page's proposal ids in the channel read, the pins list, the pin and unpin responses and the thread parent. `cabal_messages.reply_count` is no longer read for a proposal card.
- **Last repliers.** A card's `last_repliers` are the authors of the proposal's live user comments, newest first and one per person, read by the same batched query as chat threads, so they match `GET /v1/proposals/{id}/comments`. The `thread.updated` for a comment carries them too.
- **Realtime.** After a comment on a proposal's feed item commits, and the proposal has a card, the cabal's channel gets `message.created` (the mapped reply) and `thread.updated` (the card's new count) for a new comment, `message.deleted` and `thread.updated` for a delete or an admin removal, and `reaction.updated` with the comment id for a reaction change. Each publish runs from `tx.AfterCommit`, and a failure is logged as `social.chat_publish_failed` and never fails the write. A system note publishes nothing.
- **Write.** `POST /v1/cabals/{id}/messages` with a `parent_id` that is a proposal card does not insert a chat reply (#8392). It runs `CreateComment` on the proposal's feed item in the same transaction, so the member check, the 1 to 1000 scalar body, the text filter, mentions, the attachment rules, the comment events and their pushes are those of `POST /v1/proposals/{id}/comments`, and it returns `201` with the comment mapped as above. A body over 1000 scalars is `invalid_input` with `reason=comment_too_long` (the chat limit stays 2000 for every other message), a chat poll becomes the comment's poll, and a proposal with no feed item yet is `feed_item_pending`, which is retryable. No chat row exists for a reply sent with the switch off. No chat replies under proposal cards existed on staging on 2026-11-05, so nothing was moved.
- **Also send to chat.** With `also_in_channel: true` the same transaction inserts one channel copy: `kind = user`, the replier as author, the comment's body (or the attachment's display text and no attachment), `parent_id` the card, `also_in_channel = true` and `comment_id` the comment (`cabal_messages.comment_id`, unique where set). The copy appears on channel reads with its own `id` and `comment_id` set, takes the comment's `reactions` and `reaction_count` (the app reacts through the comment routes, and `reaction.updated` carries the comment id), and is left out of the card's `reply_count`, which counts the comment once. It counts toward `unread_count` like any channel message and publishes its own `message.created`. Its `chat.message_posted` carries no mentions and lists only thread participants the comment's pushes do not reach (not the proposer and not anyone the comment mentions), so each person gets one push per reply. Deleting the comment, or a moderator removing it, soft-deletes the copy and publishes `message.deleted` with the copy's id. Account deletion leaves comments and chat messages in place, so it leaves the copy too.
- **Old builds** read the same list through the thread route.

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

Log: [log/chat.md](log/chat.md).
