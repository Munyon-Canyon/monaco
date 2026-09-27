# Feed & comments

**Status:** Decided 2026-09-26 for MVP scope. Reconciled with [backend-platform.md](backend-platform.md) 2026-09-27: owned by the `social` module, built in [Rollout](backend-platform.md#rollout) step 6, realtime through the RFC's [SSE hub](backend-platform.md#sse-hub). Visibility decided 2026-09-27; the remaining open points took defaults the same day.

## Decision

The feed is one table, **FeedObject**, holding every kind of item the app shows in the feed. Reading the feed is a single paginated `GET` over that table, with filters, search and sort as query parameters. There is no ranking model for MVP: the default order is newest first. Users can comment on any feed item and reply to other comments, using one comment system, **FeedComment**, for every item kind, proposals included.

The `social` module owns the feed, its comments and its mutes ([Repository layout](backend-platform.md#repository-layout)). Feed items are written by the module's `feed` consumer as side effects of events from other modules, never by clients directly. The consumer runs through `bus.Dispatch`, so a redelivered event is a no-op through `event_deliveries` ([event-bus.md](event-bus.md#consumers-and-handlers)). Comments are the only thing users write to the feed. Creating one is flow 21 in [`flows.tsv`](backend-platform.md#flows).

## Item kinds

| Kind | Source | Created when | MVP |
| --- | --- | --- | --- |
| `proposal` | `governance` events | A proposal is created in any cabal. Its row is updated on status change (passed, executed, blocked, failed, expired, withdrawn, voided). | Yes |
| `trade` | `trading` events | A cabal swap confirms. | Yes |
| `price_move` | `market` module | A stock's change since the previous close crosses a threshold (e.g. AAPL +10% on the day). | Yes |
| `cabal_created` | `cabal` events | A cabal is created. | Yes |
| `member_joined` | `cabal` events | A user joins a cabal. | Yes |
| `news` | News provider | A news item tagged to a stock in the catalog. | No, deferred |

**Every cabal is public in the feed** (decided 2026-09-27). There is no private-cabal flag. Proposals and trades from all cabals appear, not only cabals the viewer belongs to. `cabal_created` and `member_joined` items carry no money amounts, and funding and cash-outs never become feed items (default 2026-09-27), so the feed does not show what a member put in or took out.

## Table shape

```
feed_objects
  id             uuid      UUIDv7
  kind           text      proposal | trade | price_move | news
  ref_type       text      proposals | cabal_txns | assets | news_items
  ref_id         uuid      source row
  cabal_id       uuid?     null for price_move and news
  actor_id       uuid?     proposer, or null for system items
  asset_id       uuid?     stock the item is about
  title          text      rendered headline, e.g. "Alpha Cabal bought $500 of AAPLx"
  body           text?     thesis, news summary
  payload        jsonb     kind-specific snapshot: amounts, % move, tally, status
  status         text?     mirrors the source for proposals
  comment_count  int
  search         tsvector  generated from title, body, symbol, cabal name
  created_at     timestamptz
  updated_at     timestamptz

feed_comments
  id, feed_object_id, author_id, parent_comment_id?, body (1 to 1000 chars), created_at, deleted_at?
```

Unique on `(ref_type, ref_id, kind)` so a redelivered event cannot create a duplicate item.

`payload` is a snapshot so the list renders from one table without joins. Amounts in it are integer base units (`money.Micros`), and percentages are integer basis points, never floats ([Money and types](backend-platform.md#money-and-types)). The server renders `title` and every display string, so the app only formats ([Thin client](backend-platform.md#thin-client)). When the source changes (a proposal passes or executes), the `feed` consumer updates the feed row's `status` and `payload`. Tapping an item fetches the live source detail.

The source rows (`proposals`, `cabal_txns`, `assets`) belong to other modules. `ref_type` and `ref_id` are identifiers only. The feed never joins those tables; it reads what the event payload carried, or a module's read-only query port ([Dependency rules](backend-platform.md#dependency-rules-enforced-by-depguard)).

## Writing feed items

All through the `feed` consumer on the event bus. Each source module appends its event in the same transaction as its change through `uow.Do` ([Patterns](backend-platform.md#patterns-and-where-each-earns-its-place)), so a feed item exists if and only if its source change committed. Subjects follow the RFC's [flows table](backend-platform.md#flows):

- **Proposal created** → `proposal.created` (flow 9) → insert `proposal` item.
- **Proposal status change** → `proposal.{passed,failed,expired}` (flow 10), `proposal.{withdrawn,voided}` (flow 13), `proposal.executed` and `proposal.execution_blocked`, and `trade.failed` (flow 11) → update that item. `governance` emits `proposal.executed` and `proposal.execution_blocked` when it consumes `trade.confirmed` and `trade.blocked` (platform default 2026-09-27), so the feed reads proposal status from `governance` alone.
- **Trade confirmed** → `trade.confirmed` (flow 11) → insert `trade` item.
- **Price move** → a price-move poller in the `market` module (every 5 min in market hours) compares each asset's mark with its previous close. The window is the change since the previous close only, with no rolling intraday window (default 2026-09-27). Crossing a threshold (proposal: ±5% and ±10%) appends an `asset.price_moved` event, once per asset, per threshold, per day (dedupe key `(asset_id, threshold, trading_day)`). The `feed` consumer inserts the item; `notify` can take the same event. This event must be stored, because a feed item is durable work. The `price.tick` message is core NATS only (flow 18) and cannot drive it. `asset.price_moved` has its own row in the flows table (market → social feed, notify).
- **Cabal created** → `cabal.created` (flow 2) → insert `cabal_created` item.
- **Member joined** → `cabal.member_joined` (flow 3) → insert `member_joined` item.
- **News** (deferred) → an ingest job pulls from a provider and inserts items tagged by `asset_id`.

The RFC also routes `cabal.member_left`, `cabal.funded`, `cashout.*`, agent lifecycle events and `user.profile_updated` to the feed (flows 4, 7, 14, 16, 23). None of them creates an item. Profile and cabal-name changes refresh the snapshots in `title`, `payload` and `search`; the rest keep the viewer's cabal list current for `scope=mine`.

## Reading the feed

```
GET /v1/feed?kind=proposal,trade&cabal_id=…&symbol=AAPL&q=earnings
            &scope=all|mine|following&sort=new|top&cursor=…&limit=30
```

| Param | Does |
| --- | --- |
| `kind` | Include only these kinds. |
| `cabal_id`, `symbol` | Scope to one cabal or one stock. |
| `scope` | `all` (default), `mine` (cabals I am in), `following` (users I follow, [followers.md](followers.md); there are no cabal follows in MVP). |
| `q` | Full-text search over the `search` column (Postgres `tsvector`, GIN index). Symbol and cabal-name matches rank first. |
| `sort` | `new` (default, `created_at desc`) or `top`: `comment_count` plus votes, over items from the last 24 h (default 2026-09-27). |
| `cursor` | Keyset pagination on `(created_at, id)`. No offset paging. |

This is a CQRS-lite query: it reads `feed_objects` directly through sqlc and bypasses the domain ([Patterns](backend-platform.md#patterns-and-where-each-earns-its-place)). `scope=mine` needs the viewer's cabals. The `social` module keeps that membership from `cabal.member_joined` and `cabal.member_left`, which it already consumes, or reads it through the `cabal` module's query port.

**Filtering out things a user does not like.** A per-user mute list, `feed_mutes (user_id, target_type, target_id)`, where the target is a kind, a cabal, a stock or a user. The feed query excludes muted targets. Hiding a single item is a mute on that item's id.

Indexes: `(created_at desc, id)`, `(cabal_id, created_at desc)`, `(asset_id, created_at desc)`, `(kind, created_at desc)`, GIN on `search`.

## Comments

- Any feed item can be commented on, with one exception. Non-members can view another cabal's proposal but not comment on it until moderation ships (default 2026-09-27); the refusal is an `errs` code on flow 21. Replies reference `parent_comment_id`. The app shows one level of nesting; deeper replies attach to the top-level comment and mention the person they reply to.
- A reply's parent must belong to the same feed item (composite key, same pattern as today's `proposal_comments`).
- `CreateComment` is a command with an `Idempotency-Key`, like every mutating call ([Thin client](backend-platform.md#thin-client)). One `uow.Do` inserts the comment, increments `feed_objects.comment_count` and appends `comment.created`. Its consumers are `notify`, `analytics` and the live hint (flow 21).
- Refusals are `errs` codes, and each is an outcome on flow 21 in `flows.tsv` ([Errors](backend-platform.md#errors)): an empty or over-long body, an unknown feed item, a parent on another item.
- Authors can delete their own comments. Deletion is soft (`deleted_at`) so reply threads keep their shape; the body renders as "deleted".
- **Moderation** ships before non-member commenting opens (default 2026-09-27). Users can report a comment. Admins remove it through an audited `admin.action` (flow 26). Each user has a comment rate limit. Blocking users ships with it ([followers.md](followers.md)).
- A reply to your comment, or a comment on your proposal, is a notification candidate (see below).
- **Proposal comments move here.** The proposal detail screen reads the FeedComments of that proposal's feed item. Nothing migrates from `proposal_comments`. The new backend starts on an empty database (decided 2026-09-27), and the old table goes with the old backend at [Rollout](backend-platform.md#rollout) step 7.

## Realtime

Target: new items and new comments appear without a pull to refresh.

Decided by the RFC: the app holds one Server-Sent Events connection, `GET /v1/stream`, for every live screen ([Thin client](backend-platform.md#thin-client)). There is no feed-specific stream. After the `feed` consumer commits a new or updated item or a comment, it publishes a core NATS hint under `hint.>` carrying the item id. The api holds one subscription to `hint.>` and routes each hint in memory to the connections registered under its key ([SSE hub](backend-platform.md#sse-hub)). The app then fetches the item through the normal `GET`, which applies scope and mutes. The stream carries ids only, so the list query stays the one source of truth and the filter logic lives in one place.

Hints are droppable. A lost hint costs one missed re-fetch, and the reconnect fetch below covers it.

Fallback when the stream drops: the app re-fetches from its newest cursor on reconnect. It never polls on a timer.

Every feed hint goes under the hub's `global` key, which every connection joins (platform default 2026-09-27). Every cabal is public, so any connection may learn that a feed item changed; the `GET` still applies scope and mutes. The hub's `user:<id>` and `cabal:<id>` keys stay for hints that only concern one person or one cabal.

## Notifications (TBD)

Some feed events should also notify. Candidates, pending the Notifications decision:

- A proposal is created in a cabal you belong to.
- A proposal you voted on passes, executes or is blocked.
- Someone replies to your comment or comments on your proposal.
- A stock your cabal holds crosses a price-move threshold.

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

## Log

- 2026-09-27: Decided every cabal is public in the feed, with no private flag. Defaults applied: non-members view but do not comment on other cabals' proposals until moderation ships; moderation is report, admin removal and a per-user rate limit; `cabal_created` and `member_joined` items with no money amounts, and no funding or cash-out items; price move is change since previous close; `top` is comments plus votes over 24 h; news deferred. Proposal status comes from `governance`'s `proposal.executed` and `proposal.execution_blocked`. `asset.price_moved` has a flows row. Feed hints use the SSE hub's `global` key. Notification choices point at notifications.md. `proposal_comments` no longer migrates, since the new backend starts empty. All open questions closed.
- 2026-09-27: Reconciled with [backend-platform.md](backend-platform.md). Owner is the `social` module, rollout step 6. Event subjects match the RFC flows table (`proposal.withdrawn`, `trade.failed`; `proposal.executed` dropped for `trade.confirmed`). Price moves come from `market`. `CreateComment` is a command with `Idempotency-Key` and `comment.created` in the same `uow.Do`. Realtime moves from `live.feed` and `/v1/feed/stream` to `hint.>` and the RFC's single `/v1/stream` hub. Payload amounts are integer micros; server renders display strings. Added open questions on new item kinds and global hint routing.
- 2026-09-26: Feed items written by the `feed` consumer on the NATS event bus; realtime hints moved from Postgres `NOTIFY` to core NATS `live.feed` ([event-bus.md](event-bus.md)). Price moves emit `asset.price_moved`.
- 2026-09-26: Initial decision. One FeedObject table with proposal, trade, price-move and (later) news kinds. Single filterable, searchable GET. FeedComment on every item, replacing proposal comments. Realtime via SSE leaning; notifications TBD.
