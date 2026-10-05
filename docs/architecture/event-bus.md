# Event bus (NATS)

**Status:** Decided 2026-09-26. Amended 2026-09-27 to match [backend-platform.md](backend-platform.md), which wins where the two differ, and to apply the 2026-09-27 defaults (see [Log](log/event-bus.md)). Not built; lands in [Rollout](backend-platform.md#rollout) step 2.

## Decision

Modules talk through events on NATS, not through direct calls. A module that changes state publishes a fact ("this proposal passed", "this trade confirmed"). Modules that care subscribe to the subjects they need and run a handler. The publisher does not know who listens. In the Go rewrite this is the only cross-module path: Go channels stay inside one handler ([backend-platform.md](backend-platform.md#decision), rules 2 and 4).

Three rules make this safe for money:

1. **The `events` table is the source of truth.** Every state change appends one `events` row in the same DB transaction as the change. The row is written whether or not NATS is up.
2. **The `events` table is also the outbox.** A relay publishes each committed row to NATS JetStream and marks it published. There is no separate `outbox` / `TxnOutbox` table.
3. **Durable work uses JetStream; live UI hints use core NATS.** Anything that must happen (a push, a feed item, a trade execution, a position refresh) is a durable JetStream consumer with explicit acks. Core NATS pub/sub, which drops messages when nobody is listening, carries only "something changed, re-fetch" hints to connected clients and the batched price tick.

NATS is Synadia Cloud in production, on the free plan first ([NATS hosting and budget](backend-platform.md#nats-hosting-and-budget)).

```
 API handler / worker          Postgres            relay           NATS JetStream           durable consumers
┌────────────────────┐      ┌──────────────┐   Publish     ┌───────────────────┐   pull,     ┌─────────────────┐
│ BEGIN              │      │ events       │   WithMsgID   │ stream EVENTS     │   explicit  │ trade-engine    │
│  guarded update    │ ───► │ (log+outbox) │ ────────────► │ subjects events.> │   ack       │ notifications   │
│  INSERT events row │      │ published_at │  (event id)   │ dedupe on msg id  │ ──────────► │ feed            │
│ COMMIT (uow.Do)    │      └──────────────┘               └───────────────────┘             │ positions       │
└────────────────────┘             ▲                                                         │ leaderboard     │
                                   │  a handler's own state changes append new events rows   │ analytics, ...  │
                                   └──────────────────────────────────────────────────────── └────────┬────────┘
                                                                                                      │ core NATS
                                                                                                      ▼
                                                                          hint.> → api SSE hub → /v1/stream
```

## Why

- **Decoupling.** Today every new side effect means editing the code that caused it. With a bus, the trade path emits `trade.confirmed` once. Notifications, feed, positions, leaderboard and analytics each subscribe on their own. Adding referral qualification later means adding a consumer, not touching the swap code.
- **No dual write.** Publishing to NATS straight from the request handler after commit loses the event if the process dies between commit and publish. Publishing before commit announces a change that may roll back. Writing the event row inside the transaction and relaying it afterwards removes both failure modes. This is the same guarantee the outbox design gave; NATS replaces the polling worker as the delivery path.
- **One log, not two.** The earlier design had an `events` table (audit) and an `outbox` table (delivery) with overlapping rows. Merging them means every delivered message is an audit row, and every audit row gets delivered.
- **NATS over Kafka.** One small Go binary, a first-party Go client (`github.com/nats-io/nats.go`), JetStream persistence and per-consumer acks built in, and an embeddable server for tests. At Monaco's volume (thousands of events a day, not millions a second) Kafka's partitions, ZooKeeper/KRaft and ops cost buy nothing.
- **JetStream over core pub/sub for work.** Core NATS is at-most-once: a subscriber that is restarting, slow, or not yet connected misses the message, and `ChanSubscribe` drops messages when its Go channel is full (slow consumer). That is fine for a UI hint and not fine for a push notification or a trade. JetStream stores the message and redelivers until the handler acks.

## How it works

### Writing an event

Every domain write that other modules care about goes through the Unit of Work ([Patterns](backend-platform.md#patterns-and-where-each-earns-its-place)). `uow.Do` opens the transaction, the closure runs the guarded update and `Tx.Events.Append`, and commit wakes the relay. No code outside `platform/db` calls `Begin` or `Commit`.

```go
err := uow.Do(ctx, func(ctx context.Context, tx Tx) error {
    ok, err := tx.Proposals.MarkPassed(ctx, id)                  // UPDATE … WHERE id = $1 AND status = 'open'
    if err != nil || !ok { return err }                          // !ok: someone else won
    return tx.Events.Append(ctx, events.ProposalPassed{ProposalID: id, ...})
})
```

Event payloads are Go types in the `events` package, one file per aggregate, with a subject registry ([Decision](backend-platform.md#decision), rule 5). A test asserts every event type has a registered subject.

`events` columns:

| Column | Notes |
| --- | --- |
| `id` | UUIDv7. Time-ordered, and used as the NATS message id (`Nats-Msg-Id`) on every publish. |
| `aggregate_type`, `aggregate_id` | `proposal` / `<uuid>`, `cabal_txn` / `<uuid>`, `user` / `<uuid>`, … |
| `type` | `proposal.passed`, `trade.confirmed`, … Also the NATS subject suffix. |
| `payload` | JSONB. The facts a consumer needs without a lookup: ids, amounts, before/after status. Every payload carries a `v` field. A handler accepts the current and the previous version (default 2026-09-27). Money events carry every amount the ledger holds, so `monacoctl replay --verify` can check the ledger ([data-model.md](data-model.md#double-entry-ledgers)). |
| `actor_type`, `actor_id` | `user`, `admin`, `system`, `agent`. For `user`, `admin` and `agent` the id is that actor's id. For `system` it is `poller.<name>` on an event a poller appended, for example `poller.market.catalog`. |
| `trace_parent` | The W3C `traceparent` of the request that wrote the row, or null when tracing is off. The relay publishes after the request is gone, so the trace reaches the consumer only if the row stores it ([context rule 9](backend-platform.md#context-rules)). |
| `created_at` | Commit order is not guaranteed by this; see [Ordering](#ordering). |
| `published_at` | Null until the relay gets a JetStream ack. Partial index `WHERE published_at IS NULL`. |

Rows are append-only. The only update is setting `published_at`.

### Relay

The relay lives in `internal/platform/bus` and runs in both the `api` and the `worker` binary (default 2026-09-27). Each process's relay wakes on its own commits, so an event from an HTTP command publishes a few milliseconds after commit. Publishing within a batch is sequential; parallelism comes from more processes and `SKIP LOCKED`.

1. Wake on the commit signal from `uow.Do` (a buffered Go channel of size 1, so a burst of commits coalesces into one wake-up) or every 1 s as a fallback.
2. `SELECT … FROM events WHERE published_at IS NULL ORDER BY id LIMIT 100 FOR UPDATE SKIP LOCKED`, so two relays never publish the same batch at once.
3. For each row, `bus.Publish` to the row's registered subject with `Nats-Msg-Id` set to the row id, and wait for the `PubAck`.
4. `UPDATE events SET published_at = now() WHERE id = ANY($acked)`, commit.

If the process dies after step 3 and before step 4, the next run publishes the same rows again. JetStream drops them, because the stream's `Duplicates` window (2 minutes) remembers the `Nats-Msg-Id`. A row stuck longer than the window is still safe, because every handler is idempotent on the event id (below).

A full stream rejects the publish (`DiscardNew`). The rows stay unpublished and the relay retries, so a storage limit delays events and never drops them.

The 1 s poll only catches rows a crash left behind.

### Streams

Two streams, `EVENTS` and `DEADLETTER`. Their config is declared in code and applied by `monacoctl bus apply` in the deploy's pre-deploy step. `api` and `worker` never create streams, and a module gets a consumer, never a stream. The full config and the limits it is sized against are in [NATS hosting and budget](backend-platform.md#nats-hosting-and-budget). The settings that shape this design:

| Setting | `EVENTS` | Why |
| --- | --- | --- |
| `Subjects` | From the `events` registry | A new event cannot add a stream. |
| `Retention` | `LimitsPolicy` (default) | Many consumers read each message; `WorkQueuePolicy` would allow only one. |
| `MaxAge` | 7 days | The stream is transport. The permanent log is Postgres. A consumer that falls further behind is rebuilt from the table ([Replay](#replay)). |
| `MaxBytes`, `Discard` | 2 GiB, `DiscardNew` | A full stream rejects the relay's publish and the outbox holds the rows. `DiscardOld` would drop events no consumer has read. |
| `Duplicates` | 2 minutes | Dedupe window for relay re-publishes. |
| `Storage` | `FileStorage` | Survives a NATS restart. |
| `Replicas` | 1 | 3 once the account moves to the Starter plan. Nothing else changes. |

`DEADLETTER` takes `deadletter.>`, 512 MiB, 30 days. It holds messages `bus.Dispatch` terminated.

At boot, `api` and `worker` compare each stream's live config with the declared one, field by field, over the settings in the table above. A missing stream or any difference stops boot with `not_found`, naming the stream and the fields, for example `stream EVENTS: config differs from the declared one in Subjects; run monacoctl bus apply`. Boot never applies the config itself. The usual local cause is a stream created before a new event was added, so its `Subjects` lack the new subject and every publish on it fails with `nats: no response from stream`. To fix it locally, run `just build backend`, then `scripts/with-dotenv-local.sh bin/monacoctl bus apply` from the repo root, then `just run backend` again. In a deploy, the pre-deploy `monacoctl bus apply` runs before the new instances boot, so they see the updated config.

Subjects are `events.<aggregate>.<verb>`: `events.proposal.created`, `events.proposal.passed`, `events.trade.confirmed`, `events.deposit.credited`, `events.follow.created`. Wildcards let a consumer take a whole area (`events.proposal.*`). Aggregate names follow the `cabal` naming ([Decided](backend-platform.md#decided)): `events.cabal.funded`, never `events.group.*`.

### Consumers and handlers

Each module gets one durable pull consumer on `EVENTS`, named after the module and registered by its `module.go` and read with `Consume()` in the `worker` binary. Pull consumers are what the `jetstream` package recommends; they avoid the slow-consumer drops of push subscriptions. Consumers are registered by durable name, and a duplicate name panics at boot.

```go
jetstream.ConsumerConfig{
    Durable:        "notify",
    FilterSubjects: []string{"events.trade.*", "events.proposal.*", "events.follow.created", ...},
    DeliverPolicy:  jetstream.DeliverNewPolicy,
    AckPolicy:      jetstream.AckExplicitPolicy,
    MaxDeliver:     10,
    BackOff:        []time.Duration{30 * time.Second, time.Minute, 5 * time.Minute, 15 * time.Minute},
    MaxAckPending:  64,
}
```

A module with several handlers still has one durable. `bus.Dispatch` routes each message to the handlers registered for its subject, and each handler has its own name, such as `treasury.positions` (the names in the flow files' `consumers` column).

`MaxAckPending` 64 is the cross-process concurrency bound. In-process pools must not exceed it ([Concurrency rules](backend-platform.md#concurrency-rules), rule 8).

`BackOff` overrides `AckWait`, so its first entry is the ack deadline for every first delivery. It is the timeout for a handler that crashed or hung, which is why it starts at 30 s rather than 1 s: a shorter deadline would redeliver a message to a second process while the first is still working on it. Past the last entry, the last interval repeats until `MaxDeliver`.

Several worker processes share one durable consumer name, so JetStream hands each message to one of them. That is the load-balancing that queue groups give in core NATS.

`bus.Dispatch` is the one wrapper every handler goes through:

Every error it sees is an `errs.Error`, and the code table decides the verdict: `Retryable` naks, anything else terms ([Errors](backend-platform.md#surfacing)). Panics are recovered here and become `KindInternal`.

1. Decode the payload into the typed event for the subject. A payload that does not decode is terminated and alerted: redelivering it will never help.
2. Read the event id from the `Nats-Msg-Id` header the relay set. In one DB transaction: `INSERT INTO event_deliveries (handler, event_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, keyed by the handler name, not the durable (default 2026-09-27). If no row was inserted, this handler already handled the event: commit, `msg.Ack()`, return. Otherwise run the handler's DB writes in the same transaction and commit. The row records `code = "ok"`; a nak or a term writes no row, so a retry runs the handler again and a terminated event can be replayed (decided 2026-09-27, #472). This is what makes redelivery harmless. Two handlers in one module dedupe independently. A daily job deletes rows older than 30 days, which outlives the 7-day stream and the replay window (default 2026-09-27).
3. Handler success → `msg.Ack()`.
4. Retryable code (APNs 5xx, Jupiter timeout, DB serialization failure) → `msg.NakWithDelay(d)`, with `d` from the handler's own schedule keyed on `msg.Metadata().NumDelivered` (1 s, 5 s, 30 s, 2 min, …), or from `Retry-After` when the service sent one (the `Retry-After` override is not implemented yet; the schedule alone decides, #472). Plain `msg.Nak()` is avoided because it redelivers immediately. A nak does not use `BackOff`, which only covers deliveries that were never acked or naked.
5. Long work (a swap waiting on Jupiter `/execute` for up to 2 minutes, or a cash-out payout retrying Solana RPC) calls `msg.InProgress()` every 10 s, which resets the ack deadline, so the message is not redelivered mid-trade. When the worker sets a shorter ack wait with `MONACO_BUS_ACK_WAIT`, as `monacoctl verify` does with 100 ms, `bus.KeepAlive` sends it every half ack wait instead.
6. Any other code → `msg.TermWithReason(...)`, publish the message and its error to `DEADLETTER`, and alert on `KindInternal`.

The trade engine in the `trading` module is the one exception to step 2's single transaction. It cannot hold a DB transaction open across a 2-minute swap, so its idempotency comes from its own claim and a partial unique index on `swaps.proposal_id` ([trade-execution.md](trade-execution.md#stage-2-trade-engine)). It registers with `bus.HandleOwn`, which runs it outside the delivery transaction, and writes its `event_deliveries` row itself with `Delivery.Record` when it blocks the trade or the swap leaves `created`. `bus.Heartbeat` gives it the message's `InProgress`. Replay and seeding never run such a handler. `trading` owns the `swaps` table; `treasury` writes the ledger from `trade.confirmed` ([data-model.md](data-model.md#double-entry-ledgers)).

After `MaxDeliver` attempts JetStream stops redelivering and publishes an advisory on `$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.EVENTS.<consumer>`. The worker's advisory subscriber sends each one to `DEADLETTER`. The `admin` module keeps a `dead_letters` table fed from that stream, with resolve state and history past the stream's 30 days (default 2026-09-27). The admin panel shows it as a queue ([analytics-admin.md](analytics-admin.md#views)), and `monacoctl deadletter retry` replays an entry ([Flows](backend-platform.md#flows), row 27). That is the one recovery path for stuck work, trades included.

Handlers never call an outside service (APNs, PostHog) inside that transaction. They write a row that records the intent, such as a `notifications` row with `delivered_at` null, commit, then send from the row and set `delivered_at`. A crash after the send and before `delivered_at` can send twice, so the send carries a key the receiver dedupes on: `apns-collapse-id` for APNs (already `trade-<txn_id>` in [notifications.md](notifications.md#sending-go)), and the event id as PostHog's `uuid`.

### Who publishes, who subscribes

The flow files, `packages/flows/backend/<id>.tsv`, are the source of truth for which command emits which event and which consumers react. [backend-platform.md](backend-platform.md#flows) renders it. This file no longer keeps its own table.

Names changed from the 2026-09-26 table, to match the flows:

| Was | Now |
| --- | --- |
| `cash_out.completed` | `cashout.started`, `cashout.completed` / `.partial` / `.failed` |
| `withdrawal.sent` | `withdrawal.submitted`, `.confirmed`, `.failed` |
| `chat.message_created` | `chat.message_posted` |
| `referral.attributed` only | `referral.attributed` at sign-up, then `referral.qualified` at the first cabal funding of $10 or more |
| `cabal.funded` alone | `cabal.fund_submitted`, then `cabal.funded` |
| `trade.blocked` sets the proposal to `execution_blocked` in the engine's transaction | `trading` emits `trade.blocked`; `governance` consumes it and emits `proposal.execution_blocked` |
| Publishers named by service ("Proposal service", "Swap layer") | Publishers are modules (`governance`, `trading`, `treasury`, `funding`, …) |

Rows added to the flow registry on 2026-09-27 (defaults):

| Event | Published by | Consumers |
| --- | --- | --- |
| `proposal.executed`, `proposal.execution_blocked` | `governance`, consuming `trade.confirmed` / `trade.blocked` | `social` (feed), `notify` |
| `asset.price_moved` | `market` | `social` (feed), `notify` |
| `user.nudge_due` | `identity` (daily onboarding job) | `notify` |
| `referral.attributed` | `referrals` | `social` |
| `deposit.credited`, `withdrawal.confirmed` | `funding` | adds `treasury`, which writes `user_txns` |
| `deposit.credited` | `funding` | adds `identity`, which sets `users.first_deposit_at` on the first deposit of at least $10, and drops `referrals` (decided 2026-09-27) |

A new `analytics` module exports to PostHog. It consumes flows 7, 10, 11, 14, 20 and 21 in addition to those that already listed analytics (default 2026-09-27).

The trade engine is a consumer like any other. A passed proposal no longer writes a row addressed to it. The tally emits `proposal.passed`, and the trade engine picks it up. Agent intents emit `agent.intent_created` and reach the same engine ([Flows](backend-platform.md#flows), row 17). The 15 s proposal safety-net poller is dropped (default 2026-09-27). A stuck `proposal.passed` ends in `DEADLETTER`, and `monacoctl deadletter retry` replays it.

### Ordering

JetStream delivers in stream order, but a message that is not acked is redelivered after later ones, and `MaxAckPending > 1` lets a consumer work on several at once. Handlers therefore never assume order. They apply state with guarded updates (`WHERE status = $expected`) or compare versions, so an older status never overwrites a newer one. When a handler's precondition is missing, for example the `feed` consumer gets `proposal.passed` while `proposal.created` is still waiting on a retry and there is no feed row to update yet, it naks with a delay instead of acking, and the event lands once the earlier one has. A consumer that genuinely needs strict order per aggregate sets `MaxAckPending: 1` and accepts the throughput cost.

### Live updates (core NATS)

Handlers that change something a connected client is looking at publish a hint on core NATS after their DB commit. Hint subjects sit under `hint.>` and name the key they are for, for example `hint.cabal.42.updated`. The hub keys are `cabal:<id>`, `user:<id>` and `global`. Every connection joins `global`, which carries feed and leaderboard hints (default 2026-09-27).

Each `api` process holds one subscription, `hint.>`, and routes in memory through the SSE hub to the phones registered under that key on `/v1/stream` ([SSE hub](backend-platform.md#sse-hub)). The hub's `Register` is the authz check: a phone in cabal 7 never gets cabal 42's hint. Lost hints are fine: the payload is an id, the client fetches through the normal `GET`, and it re-fetches from its cursor on reconnect. The hub counts drops in a metric. This replaces the Postgres `NOTIFY feed` plan in [feed.md](feed.md#realtime), which does not survive a pooled or replicated database.

Prices do not go through `EVENTS`. The price poller publishes one batched `price.tick` per poll on core NATS, every 120 s (decided 2026-09-27), and it is not stored as an event ([NATS hosting and budget](backend-platform.md#nats-hosting-and-budget)).

Chat keeps Ably for client delivery ([chat.md](chat.md)). Clients never connect to NATS.

### Replay

- **A new consumer** starts at new messages (`DeliverNewPolicy` above), so shipping a consumer does not replay a week of events into it. To backfill, `monacoctl` reads the `events` table for the types it needs and calls the handler directly through the same `bus.Dispatch` wrapper, so dedupe still holds.
- **A consumer more than 7 days behind** (stream `MaxAge`) is reset the same way.
- **Analytics history** (members per cabal last month, `auth_state` funnels) reads the `events` table, not NATS.
- **Projections and test seeds.** `monacoctl replay --to <event_id>` rebuilds every projection into a fresh database from the `events` table, and `testkit` seeds tests from named event sequences ([Replay and seeded states](backend-platform.md#replay-and-seeded-states)). Replay never calls Jupiter, Privy or RPC.

### Local development and tests

Local dev and tests never touch Synadia.

- The compose file (`apps/backend/deployments/`) gains a `nats` service: `nats:2-alpine` with `-js -sd /data`, port `4222`, a volume, and a health check on the monitoring port `8222`. `just run backend` starts it with Postgres; `just reset` wipes its volume and `just reset db` keeps it.
- `NATS_URL` joins `.env.local` (`nats://localhost:4222`) and `.env.production`. Only `platform/bus` and `testkit` open a connection, once per process, named `monaco-api` or `monaco-worker`.
- `just test backend` runs one in-process `nats-server` per test package in `TestMain` and one stream per test, with `AckWait` 100 ms ([Keeping it fast](backend-platform.md#keeping-it-fast), [Isolating test state](backend-platform.md#isolating-test-state)). No external NATS needed.
- Every consumer's tests run through the chaos dispatcher, which duplicates, reorders, delays and redelivers by seed ([Keeping it deterministic](backend-platform.md#keeping-it-deterministic)). That turns the ordering and redelivery rules above into checks.

### Metrics

Replaces the outbox metrics in [analytics-admin.md](analytics-admin.md#system-health). Exported through OTel to Grafana Cloud ([Deploy and observability](backend-platform.md#deploy-and-observability)), next to the worker's `js.AccountInfo` gauges for storage and stream counts:

| Metric | Source |
| --- | --- |
| `monaco_events_unpublished`, `monaco_events_oldest_unpublished_seconds` | `events WHERE published_at IS NULL` on the backlog scrape. Relay lag. |
| `monaco_bus_consumer_pending{consumer}` | `ConsumerInfo.NumPending`. Messages not yet delivered. |
| `monaco_bus_consumer_ack_pending{consumer}` | `ConsumerInfo.NumAckPending`. Delivered, not yet acked. |
| `monaco_bus_handler_duration_seconds{consumer,subject,outcome}` | `bus.Dispatch`. |
| `monaco_dead_letters{consumer}` | `dead_letters` rows not yet resolved. |

Alert when relay lag passes 30 s, any consumer's pending count grows for 5 minutes, or account storage passes 80%.

## Alternatives considered

| Alternative | Why not |
| --- | --- |
| **Kafka** | Built for a scale we do not have. Heavy to run, and consumer-group rebalances add latency and ops work. |
| **Core NATS pub/sub only** (`Subscribe` / `ChanSubscribe`) | At-most-once. A restarting handler misses pushes and feed items, and a trade could never start. Kept only for live hints. |
| **Publish to NATS directly after commit, no relay** | Dual write. Crash between commit and publish loses the event forever. |
| **Separate `outbox` table next to `events`** | Two rows for one fact, and two tables to keep in step. The `published_at` column does the same job. |
| **Postgres-only outbox worker** (previous decision) | Works, but a single polling loop owns dispatch to every handler, retries and dead letters are hand-built, and fan-out to many consumers means one row per consumer. JetStream provides per-consumer acks, backoff, dedupe and redelivery. |
| **Postgres `LISTEN/NOTIFY` for live hints** | Tied to one database connection, does not cross a connection pooler, lost on failover. NATS already runs. |
| **Clients subscribe to NATS directly** | Exposes internal subjects and needs per-user NATS auth. SSE from the API reuses existing auth, and chat already has Ably. |
| **One NATS subscription per connected phone** | Hits the 100-subscription limit at 100 phones and delivers each cabal hint once per member to the same process. One `hint.>` subscription and an in-memory hub instead. |
| **Relay in `worker` only** | Commits from `api` would wait for the 1 s poll. A relay in each process costs nothing extra, since `SKIP LOCKED` already keeps relays apart. |
| **Version in the subject** (`events.v2.trade.confirmed`) | Every version bump changes consumer filters. A `v` field keeps subjects stable. |
| **A 15 s trade safety-net poller** | A second recovery path beside dead letters. One path is easier to test and to operate. |
| **Self-hosted JetStream** | One node on one disk on a PaaS. Synadia runs the cluster ([NATS hosting and budget](backend-platform.md#nats-hosting-and-budget)). |

## Open questions

None at the moment.

Log: [log/event-bus.md](log/event-bus.md).
