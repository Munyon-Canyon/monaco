# Feed & comments

**Status:** Decided 2026-09-26 for MVP scope. Reconciled with [backend-platform.md](backend-platform.md) 2026-09-27: owned by the `social` module, built in [Rollout](backend-platform.md#rollout) step 6, realtime through the RFC's [SSE hub](backend-platform.md#sse-hub). Visibility decided 2026-09-27; the remaining open points took defaults the same day.

## Decision

The feed is one table, **FeedObject**, holding every kind of item the app shows in the feed. Reading the feed is a single paginated `GET` over that table, with filters, search and sort as query parameters. There is no ranking model for MVP: the default order is newest first. Users can comment on any feed item and reply to other comments, using one comment system, **FeedComment**, for every item kind, proposals included.

The `social` module owns the feed, its comments and its mutes ([Repository layout](backend-platform.md#repository-layout)). Feed items are written by the module's `feed` consumer as side effects of events from other modules, never by clients directly. The consumer runs through `bus.Dispatch`, so a redelivered event is a no-op through `event_deliveries` ([event-bus.md](event-bus.md#consumers-and-handlers)). Comments are the only thing users write to the feed. Creating one is [flow 21](backend-platform.md#flows).

## In the app

The feed is a fifth bottom tab, **Feed**, between Home and Cabals (default 2026-09-29; see #535).

## Item kinds

| Kind | Source | Created when | MVP |
| --- | --- | --- | --- |
| `proposal` | `governance` events | A proposal is created in any cabal. Its row is updated on status change (passed, executed, blocked, failed, expired, withdrawn, voided). | Yes |
| `trade` | `trading` events | A cabal swap confirms. | Yes |
| `price_move` | `market` module | A stock's change since the previous close crosses a threshold (e.g. AAPL +10% on the day). | Yes |
| `cabal_created` | `cabal` events | A cabal is created. | Yes |
| `member_joined` | `cabal` events | A user joins a cabal. | Yes |
| `reply` | `social` comments | A comment, top-level or nested, is posted on a `proposal` item. Written in the comment's own transaction and deleted with the comment. Comments on other kinds make none. | Yes |
| `threshold` | `governance` events | Older items; no longer written. A proposal that passed used to insert one (`proposal.passed`), and it stayed "Execution pending" because the `proposal` item, not this one, advanced. | Yes |
| `result` | `governance` events | A proposal closes: one row per proposal, written at `proposal.passed`, `proposal.failed` or `proposal.expired`, then patched by its trade events (#6515); every write re-renders title and body. It stores the outcome (`passed` or `failed`), the shape that says who heads the post (`cabal`, or the proposing `member` for a sale and a Pre-IPO buy), the caller (the proposer of a passed proposal) and the final tally with the votes it needed. A rule-change proposal has none. | Yes |
| `news` | News provider | A news item tagged to a stock in the catalog. | No, deferred |

**Every cabal is public in the feed** (decided 2026-09-27). There is no private-cabal flag. Proposals and trades from all cabals appear, not only cabals the viewer belongs to. `cabal_created` and `member_joined` items carry no money amounts, and funding and cash-outs never become feed items (default 2026-09-27), so the feed does not show what a member put in or took out.

## Table shape

```
feed_objects
  id             uuid      UUIDv7
  kind           text      trade | proposal | cabal_created | member_joined | price_move | reply | threshold | result
  ref_type       text      swaps | proposals | cabals | cabal_members | asset_price_moves | feed_comments, one per kind
  ref_id         uuid      source row
  cabal_id       uuid?     null for price_move and news
  cabal_name     text?     snapshot, for search
  actor_id       uuid?     proposer, or null for system items
  asset_id       uuid?     stock the item is about
  symbol         text?     snapshot, for the symbol filter and search
  title          text      rendered headline, e.g. "Alpha Cabal bought $500 of AAPLx"
  body           text?     thesis, news summary
  payload        jsonb     kind-specific snapshot: amounts, % move, tally, status
  status         text?     mirrors the source for proposals
  comment_count  int
  search         tsvector  generated from title, body, symbol, cabal name
  created_at     timestamptz
  updated_at     timestamptz

feed_comments
  id, feed_object_id, author_id, parent_comment_id?, reply_to_user_id?, body (1 to 1000 chars), created_at,
  deleted_at?, deleted_by?, deleted_by_kind (author | admin, default author)

feed_mutes
  user_id, target_type (kind | cabal | asset | user | item), target_id, label?, created_at
```

Unique on `(ref_type, ref_id, kind)` so a redelivered event cannot create a duplicate item.

`payload` is a snapshot so the list renders from one table without joins. Amounts in it are integer base units (`money.Micros`), and percentages are integer basis points, never floats ([Money and types](backend-platform.md#money-and-types)). The server renders `title` and every display string, so the app only formats ([Thin client](backend-platform.md#thin-client)). When the source changes (a proposal passes or executes), the `feed` consumer updates the feed row's `status` and `payload`. Tapping an item fetches the live source detail.

The source rows (`swaps`, `proposals`, `cabals`, `cabal_members`, `asset_price_moves`) belong to other modules. A `trade` item points at trading's `swaps` row, not at a ledger row. `news` joins `kind` when news ships. `ref_type` and `ref_id` are identifiers only. The feed never joins those tables; it reads what the event payload carried, or a module's read-only query port ([Dependency rules](backend-platform.md#dependency-rules-enforced-by-depguard)).

## Writing feed items

All through the `feed` consumer on the event bus. Each source module appends its event in the same transaction as its change through `uow.Do` ([Patterns](backend-platform.md#patterns-and-where-each-earns-its-place)), so a feed item exists if and only if its source change committed. Subjects follow the RFC's [flows table](backend-platform.md#flows):

- **Proposal created** → `proposal.created` (flow 9) → insert `proposal` item.
- **Proposal status change** → `proposal.{passed,failed,expired}` (flow 10), `proposal.{withdrawn,voided}` (flow 13), `proposal.executed` and `proposal.execution_blocked`, and `trade.failed` (flow 11) → update that item. `governance` emits `proposal.executed` and `proposal.execution_blocked` when it consumes `trade.confirmed` and `trade.blocked` (platform default 2026-09-27), so the feed reads proposal status from `governance` alone.
- **Trade confirmed** → `trade.confirmed` (flow 11) → insert `trade` item.
- **Price move** → the `market` price poller (flow 18, every 120 s) compares each asset's new sample with its previous close on every tick. There is no second poller. The window is the change since the previous close only, with no rolling intraday window (default 2026-09-27). Crossing a threshold (proposal: ±5% and ±10%) appends an `asset.price_moved` event, once per asset, per threshold, per day (dedupe key `(asset_id, threshold, trading_day)`). The `feed` consumer inserts the item. It sends no push for MVP ([notifications.md](notifications.md#what-notifies-mvp)). This event must be stored, because a feed item is durable work. The `price.tick` message is core NATS only (flow 18) and cannot drive it. `asset.price_moved` is in flow 18's row of the flows table (market → social feed).
- **Cabal created** → `cabal.created` (flow 2) → insert `cabal_created` item.
- **Member joined** → `cabal.member_joined` (flow 3) → insert `member_joined` item.
- **Proposal closed** → `proposal.passed`, `proposal.failed` or `proposal.expired` (flow 10) → `social.feed.proposal_passed`, `social.feed.proposal_failed` and `social.feed.proposal_expired` advance the `proposal` item and write the proposal's one `result` item in the same transaction. The final tally and the votes it needed come from governance's `Tallies` port (the proposal's frozen threshold, never the cabal's current rule and never the price-move `threshold_bps`). Every write to a result row goes through one projection (`writeResult`): lock the row, merge the new facts into the payload, re-render `title`, `body` and `status`, upsert. A redelivery, or a reopened proposal that closes again, keeps one row and any fill keys already on it. No `threshold` item is written.
- **News** (deferred) → an ingest job pulls from a provider and inserts items tagged by `asset_id`.

The RFC also routes `cabal.member_left`, `cabal.funded`, `cashout.*` and `user.profile_updated` to the feed (flows 4, 7, 14, 23). None of them creates an item. Profile and cabal-name changes refresh the snapshots in `title`, `payload` and `search`; the rest keep the viewer's cabal list current for `scope=mine`.

## Reading the feed

```
GET /v1/feed?kind=proposal,trade&cabal_id=…&symbol=AAPL&q=earnings
            &scope=all|mine|following|network|others&status=open&sort=new|top&cursor=…&limit=30
```

| Param | Does |
| --- | --- |
| `kind` | Include only these kinds. |
| `cabal_id`, `symbol` | Scope to one cabal or one stock. |
| `scope` | `all` (default), `mine` (cabals I am in), `following` (users I follow, [followers.md](followers.md); there are no cabal follows in MVP), `network` (the cabals I am in plus the users I follow, one keyset-paged list; an item that is both appears once) or `others` (every cabal I am not in, never a price move). Mutes and hidden cabals apply to every scope. |
| `status` | `open` only: keeps the items whose `status` is `open`, such as open proposals; use it with `kind=proposal`. Keyset paging is unchanged, so a client can page every open item. Any other value is `invalid_input`. |
| `q` | Full-text search over the `search` column (Postgres `tsvector`, GIN index). Symbol and cabal-name matches rank first. |
| `sort` | `new` (default, `created_at desc`) or `top`: comment count, then recency, over the items from the last 24 h (default 2026-09-27). Votes do not change `top`; a proposal post's `tally` is read from governance on each request, and every `vote.cast` publishes the `global` feed hint. |
| `cursor` | Keyset pagination on `(created_at, id)`, or on `(comment_count, created_at, id)` for `sort=top`. A cursor works only for the sort that wrote it. No offset paging. |

Each item in the response carries `title`, `detail` and `tone` (`neutral`, `positive` or `negative`), all rendered by the server from `payload`, and never the raw `payload`. A `proposal` item also carries `tally` (`yes`, `no`, `voters`, `needed`) and `expires_at`, read from governance once per page; every other kind has both null. `GET /v1/feed/{id}` returns one item plus `visible`, whether it passes the filters sent with the request. The cursor is opaque base64 of the keyset position. `q` filters with `websearch_to_tsquery` under both the `english` and `simple` configurations and keeps the newest-first order.

This is a CQRS-lite query: it reads `feed_objects` directly through sqlc and bypasses the domain ([Patterns](backend-platform.md#patterns-and-where-each-earns-its-place)). `scope=mine` needs the viewer's cabals. The `social` module keeps that membership from `cabal.member_joined` and `cabal.member_left`, which it already consumes, or reads it through the `cabal` module's query port.

**Filtering out things a user does not like.** A per-user mute list, `feed_mutes (user_id, target_type, target_id)`, where the target is a kind, a cabal, a stock or a user. The feed query excludes muted targets. Hiding a single item is a mute on that item's id.

Indexes: `(created_at desc, id)`, `(cabal_id, created_at desc)`, `(asset_id, created_at desc)`, `(kind, created_at desc)`, GIN on `search`.

## Comments

- Every feed item resolves to one **thread item**, and replies live on it. A `proposal` item is its own thread. A `threshold` item, a `reply` item and a `result` item whose `payload.shape` is `cabal` (or absent) share their proposal's thread: `ThreadOf` reads `payload.proposal_id` and finds the proposal item, and answers `feed_item_pending` while that item is missing. A `result` with `shape = member` (a member's sale or Pre-IPO buy) and every other kind are their own thread.
- Who may reply is decided on the thread item (`CanReply`, `reply_rule` on `comment.created`). The founder's 10-27 rules: a proposal's thread is its cabal's discussion, so only members of that cabal reply (`comment_members_only`, rule `members`); anyone else who can see the post reads the replies and reacts. Every other thread takes replies from anyone who can see it (rule `anyone`). A reply names the comment it answers (`parent_comment_id` on the request) and the server stores it as `reply_to_comment_id` on every reply. Placement follows F: when the post's author answers (the proposer, on a proposal; a system item with no actor never folds), the answer folds under the answered comment's top-level comment, one indent in. Anyone else's reply is a top-level comment in time order. Every reply returns `reply_to`, a quote with the answered comment's id, its `root_comment_id`, the author's name (`Someone` for a deleted account), a 120-scalar excerpt (null when the comment is deleted or removed, or its author is blocked by the reader), `is_deleted` and `is_folded`. Loading rule: the list is oldest first and each top-level comment comes with all its folded answers, so a quoted comment's root is always on the same or an earlier page; the app expands the root from the quote, and for an anchor that arrives from outside the page (a push or Catch up) it pages forward until that `root_comment_id` is loaded, at most 10 pages, then lands at the top of the thread with a toast. `comment.created` carries the answered comment's author, root, deleted flag and excerpt, and `comment_reply` pushes the author of the answered comment.
- Who may react is `CanReact`: anyone who can see the item. It is the one place a later rule (blocks, banned cabals) plugs in, and `PUT /v1/feed/{id}/reactions/{emoji}` calls it.
- A comment posted through a `threshold`, `reply` or cabal-shaped `result` card is stored on the proposal item, which carries the thread's `comment_count`. `GET /v1/feed/{id}/comments` on such a card returns the proposal thread's page, and `ReplyCounts` counts every reply in that thread.
- A reply's parent must belong to the same thread item (composite key, same pattern as today's `proposal_comments`).
- `CreateComment` is a command with an `Idempotency-Key`, like every mutating call ([Thin client](backend-platform.md#thin-client)). One `uow.Do` inserts the comment, increments `feed_objects.comment_count` and appends `comment.created`. Its consumers are `notify`, `analytics` and the live hint (flow 21).
- Refusals are `errs` codes, and each is an outcome in flow 21's file, `packages/flows/backend/21.tsv` ([Errors](backend-platform.md#errors)): an empty or over-long body, an unknown feed item, a parent on another item.
- A comment on a proposal's thread also inserts a `reply` item in the same transaction (flow 21's `CreateComment`): the actor is the commenter, `body` is the comment text, `ref_id` is the comment id, and the payload carries the proposal's figures and `proposal_id`. A comment on a `reply` or `threshold` card writes to the proposal's thread, so it too inserts a `reply` item. Deleting or removing the comment deletes its `reply` item and the item's likes. Older comments and already-passed proposals are not backfilled.
- Authors can delete their own comments. Deletion is soft (`deleted_at`) so reply threads keep their shape; the body renders as "deleted".
- **Moderation.** Users can report a comment. Admins remove it through an audited `admin.action` (flow 26). Each user has a comment rate limit. Blocking users ships with it ([followers.md](followers.md)).
- A reply to your comment, or a comment on your proposal, is a notification candidate (see below). `comment.created` carries what the push copy needs: `item_kind` and `item_label`/`item_noun` for the post replied on, `thread_feed_object_id`, the thread item's `item_actor_id` and `item_actor_name`, `cabal_name` and `reply_rule`. The label and noun come from `domain/feed/render.go`, the one place display strings come from.
- **Proposal comments move here.** The proposal detail screen reads the FeedComments of that proposal's feed item. Nothing migrates from `proposal_comments`. The new backend starts on an empty database (decided 2026-09-27), and the old table goes with the old backend at [Rollout](backend-platform.md#rollout) step 7.

## A member's activity

`GET /v1/users/{id}/activity` serves a profile's Activity tab: what the member voted on, proposed, replied to, and which months they topped the People board. It is a separate read from the feed, for two reasons.

- **Votes never enter the Feed.** A `voted` kind in `feed_objects` would put every vote into the Feed tab, which Feed A does not draw. The read merges four sources at request time instead: `VotesBy` and `ProposalsBy` from governance, the member's live `feed_comments`, and the closed months from ranking's `ClosedTops` (`call_board_closed`, one row per month the member ranked first).
- **Sides stay private.** A `voted` row carries no `choice` unless the caller is the member, because a vote's side is private by default (founder decision). The feed and the proposal read never expose it either.

Social reads governance through the `governance/port.MemberActivity` port and ranking through `CallBoards`, both picked from the module set in `Wire`, never by importing the module (governance imports social). A missing pick answers `upstream_unavailable`.

The page is ordered `(at desc, kind_rank, source_id desc)` and paged by one opaque cursor over that tuple, so rows with the same timestamp are neither lost nor repeated across a page boundary. Each source returns the rows strictly after the cursor; a batch that visibility filtering empties (banned cabal, removed comment, unknown proposal) is followed by the source's next batch, up to five. The page stops at the earliest point a capped source has read to, so a filtered source never lets a later row of another source jump ahead of it. A blocked pair, either way, gets an empty page.

## Reactions

- Any feed item takes emoji reactions, one row per user and emoji in `feed_reactions`, at most 10 distinct emoji per user per item ([Comments](#comments) covers replies). `domain.ParseEmoji` accepts exactly one emoji.
- `PUT /v1/feed/{id}/reactions/{emoji}` adds one and `DELETE /v1/feed/{id}/reactions/{emoji}` removes the caller's, the emoji percent-encoded in the path and no body. The emoji follows the id because `PUT /v1/feed/{id}/reactions` would overlap `PUT /v1/feed/likes/{id}` in the router. Both are idempotent and return the item's `ReactionSummary`, so the app reconciles its optimistic chip. The 11th distinct emoji is `invalid_input` with `reason=reaction_limit`.
- `ReactFeedItem` inserts the row and appends `reaction.added` in one `uow.Do`; `UnreactFeedItem` appends `reaction.removed` only when a row went. A repeat appends nothing. Both hint `global.feed` after the commit. Nothing consumes the events: a reaction never pushes (flow 21b).
- The like routes are ❤️ reactions. `PUT|DELETE /v1/feed/likes/{id}` call the same handlers and no longer write `feed_likes`.
- There is no route that lists who reacted.

## GIFs

- GIFs come from GIPHY on the device, not from a server proxy. The app target carries the GIPHY iOS SDK (`GiphyUISDK`), configured once at launch, and reads results through the `GifProvider` port that `MonacoCore` declares. `MonacoCore` never imports the SDK. The app supplies `GiphyGifProvider`, `UnavailableGifProvider` when the build has no key, and `SampleGifProvider` in harnesses and tests.
- The key is the client key `GIPHY_SDK_KEY`, read at build time by `scripts/ensure-ios-privy-config.sh` the way Privy's client ID is: `.env.local`, `.env.staging` and `.env.production` (else the staging key) become `GIPHY_SDK_KEY_DEV`, `_STAGING` and `_PRODUCTION`, the environment picks one, and `Info.plist` carries it. A missing key only warns, and GIFs show as unavailable.
- Trending and search ask GIPHY's `v1/gifs/trending` and `v1/gifs/search` through the SDK's own HTTP client (`GiphyCore.shared.httpRequest`), 25 results a page, because SDK 2.3 has no trending or search call. The rating is `pg-13` (`GPHRatingType.ratedPG13`), set on every request.
- A result is kept only when it has a still, a preview and an original rendition and all three URLs are on `media*.giphy.com` or `i.giphy.com` over https. The server repeats the host rule when it stores a reply (#6533).
- The sheet must show "Powered by GIPHY" (`GifPage.attribution`) whenever results are visible.
- The server stores only GIPHY media URLs on replies (#6533). There is no GIF route and no Render variable.

## Realtime

Target: new items and new comments appear without a pull to refresh.

Decided by the RFC: the app holds one Server-Sent Events connection, `GET /v1/stream`, for every live screen ([Thin client](backend-platform.md#thin-client)). There is no feed-specific stream. After the `feed` consumer commits a new or updated item or a comment, it publishes a core NATS hint under `hint.>` carrying the item id. The api holds one subscription to `hint.>` and routes each hint in memory to the connections registered under its key ([SSE hub](backend-platform.md#sse-hub)). The app then fetches the item through the normal `GET`, which applies scope and mutes. The stream carries ids only, so the list query stays the one source of truth and the filter logic lives in one place.

Hints are droppable. A lost hint costs one missed re-fetch, and the reconnect fetch below covers it.

Fallback when the stream drops: the app re-fetches from its newest cursor on reconnect. It never polls on a timer.

Every feed hint goes under the hub's `global` key, which every connection joins (platform default 2026-09-27). Every cabal is public, so any connection may learn that a feed item changed; the `GET` still applies scope and mutes. The hub's `user:<id>` and `cabal:<id>` keys stay for hints that only concern one person or one cabal.

## Notifications

Which feed events push in MVP is decided in [notifications.md](notifications.md#what-notifies-mvp): proposal created, a voted proposal passing, and a reply to your comment. Price moves do not push.

The `feed` consumer (in `social`) and the `notify` module each have their own durable consumer off the same event, so neither depends on the other. Which of these push in MVP is decided in [notifications.md](notifications.md) (defaults 2026-09-27), not here.

## Alternatives considered

| Alternative | Why not |
| --- | --- |
| Build the feed at read time with a `UNION` over proposals, transactions and prices | Slow, hard to paginate, and search and mutes have to be written once per source. It would also read other modules' tables, which the module walls forbid. One table with snapshots is one indexed query. |
| Ranking model or recommendation service for MVP | No data yet to rank on. Chronological with filters first; add ranking as a new `sort` value later without changing the table. |
| Separate comment systems per kind (keep `proposal_comments`) | Two thread implementations, two notification paths. One system covers every kind. |
| WebSockets | Two-way isn't needed; the client only receives. SSE is plain HTTP and is the RFC's one live channel. |
| A feed-only SSE stream (`/v1/feed/stream`) | Earlier draft. The RFC uses one `/v1/stream` per app for every hint, so one connection and one reconnect path. |
| Private-cabal flag with members-only items | Every cabal is public (decided 2026-09-27). A flag would split the feed query and the hint routing for no current user. |
| External search (Elasticsearch, Typesense) | Postgres full-text is enough at current scale. Revisit if search quality or volume demands it. |

## Open questions

None at the moment.

Log: [log/feed.md](log/feed.md).
