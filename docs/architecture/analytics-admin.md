# Analytics & admin panel

**Status:** Decided 2026-09-26; ownership, panel and moderation defaults set 2026-09-27. The backend shape (modules, telemetry, deploy, rollout) follows [backend-platform.md](backend-platform.md), which wins where the two differ.

## Decision

Three kinds of numbers, each from the place that owns the truth:

| Kind | Examples | Source of truth | Tool |
| --- | --- | --- | --- |
| **System health** | Request duration, DB latency, poller lag, upstream errors | The `api` and `worker` processes | OTel traces, metrics and logs to Grafana Cloud; Sentry for panics and `KindInternal` |
| **Business** | Open proposals, deposits, platform volume, total value held, social graph size | Postgres (ledgers, proposals, followers) | Admin panel dashboards over SQL |
| **Product behavior** | DAU, retention, funnels, drop-off, rage taps, most-used flows | User actions in the app | PostHog Cloud, US region (iOS SDK + backend events) |

Money numbers never come from PostHog. Client analytics lose events (offline, ad-block, app killed); the ledger does not. PostHog gets money events from the **backend** only, for joining behavior to outcomes.

The admin panel is an internal tool for dashboards and a small set of moderation and safety actions, built on Retool first. Every action goes through the backend with a required reason, and is written to an audit log.

Ownership (default 2026-09-27): a new `analytics` module owns the PostHog export consumer and the business dashboards' queries. The `admin` module owns `admins`, `admin_actions`, `dead_letters` and the `/v1/admin/*` routes. Both are listed in [backend-platform.md](backend-platform.md#repository-layout).

## System health

The current backend exports Prometheus metrics (see [legacy/ops-observability.md](../legacy/ops-observability.md)): `monaco_http_request_duration_seconds` per route, money event counters and volume, upstream latency, poller lag, pending backlogs, alerts to Slack/Discord and Sentry.

The rewrite replaces that pipeline. `platform/observability` sends traces, metrics and logs over OTLP straight to Grafana Cloud (Tempo, Mimir, Loki). Grafana alert rules replace the Slack/Discord webhook. Sentry stays for panics and `KindInternal`. See [Deploy and observability](backend-platform.md#deploy-and-observability). Logs follow [Logs as evidence](backend-platform.md#logs-as-evidence), so an incident is reconstructed from Loki plus the `events` table, joined on `trace_id`.

The RFC already alerts on `poller_errors_total{poller,code}`, the dead-letter count, relayer SOL balance, `js.AccountInfo` usage and p95 per route. The rewrite also emits these as OTel instruments:

| Metric | Labels | Why |
| --- | --- | --- |
| `monaco_db_query_duration_seconds` | `query` (named statement), `outcome` | DB latency, slow-query spotting. One pgx tracer in `platform/db` rather than per call; the same tracer counts queries per request for the [Testing](backend-platform.md#testing) gate. |
| `monaco_db_pool_in_use`, `monaco_db_pool_wait_seconds` | | Pool exhaustion shows up here before it shows up as latency. |
| `monaco_open_proposals` | | Gauge on the backlog scrape. |
| `monaco_events_unpublished`, `monaco_bus_consumer_pending`, `monaco_bus_consumer_ack_pending`, `monaco_dead_letters` | `consumer` | Relay lag and per-consumer backlog on the event bus. Full list in [event-bus.md](event-bus.md#metrics). Synadia account usage comes from the worker's `js.AccountInfo` gauges ([NATS hosting and budget](backend-platform.md#nats-hosting-and-budget)). |
| `monaco_paused_cabals` | `reason` | External-deposit pauses and ops pauses, read from `funding`'s pause records. |

## Business metrics

Read-only SQL over a Postgres read replica (or the primary with a statement timeout until a replica exists). Shown in the admin panel.

- **Money:** deposits (count, USDC, split crypto vs card), fund-to-cabal count and USDC, platform volume (confirmed swap USDC, buys and sells), cash outs, withdrawals, total value held (sum of pot values plus platform balances), per day/week.
- **Governance:** open proposals, proposals created/passed/failed/expired/blocked per day, median time to pass, vote participation rate per cabal.
- **Social graph:** users, cabals, members per cabal, followers/following distribution, follows by source (`phone`, `x`, `cabal`, `feed`, `suggested`), contact-match acceptance rate, comments and replies per day, reply depth, users in each `auth_state` and `account_status`.
- **Safety:** paused cabals, external deposits and bounces, banned users and cabals, admin actions per day.

Metrics that need history (e.g. "members per cabal last month") come from the `events` table, not from current-state tables. `monacoctl replay --to <event_id>` rebuilds only the projections `projectionDurables` names (`system_pings`, `feed_*`, `ranking_triggers`, leaderboard display names and `cabal_activity`) as of a past event, so a history dashboard reads `events` itself ([Replay and seeded states](backend-platform.md#replay-and-seeded-states)). Dashboards are CQRS-lite reads through sqlc and never go through a command ([Patterns](backend-platform.md#patterns-and-where-each-earns-its-place)). They live in the `analytics` module and read across modules only through each module's read-only query port ([Dependency rules](backend-platform.md#dependency-rules-enforced-by-depguard)).

## Product analytics (PostHog)

**Identity.** `posthog.identify(<monaco user id>)` after `POST /v1/auth/session`. The id is the lowercase hyphenated UUID that the API returns, and server events use it as `distinct_id`. PostHog treats distinct ids as case-sensitive, so the app must not send Swift's uppercase `uuidString`. Person properties: `auth_state`, `login_provider`, `cabal_count`, `created_at`. **No email, phone, X handle or wallet address** in PostHog.

**Client events** (iOS SDK):

- Screen views on every screen (`$screen`).
- One event per step of every flow, named `<flow>_<step>`, with a shared `flow_id` per attempt so funnels do not mix attempts.
- `rage_tap`: emitted by the app when the same control is tapped 4+ times within 1 second, with the control's identifier and screen. Also `dead_tap` when a tap on a non-interactive element repeats. Built as a small tap tracker in the app rather than relying on autocapture.
- Session replay is off (default 2026-09-27). These are money screens.

**Server events** (backend, PostHog server API, sent by the `analytics` module's consumer on the event bus, with the event id as PostHog's `uuid` so redeliveries dedupe). These are the authoritative "it happened" for funnels that end in money or social actions. Each comes from one subject in the `events` registry:

| PostHog event | Bus subject ([Flows](backend-platform.md#flows)) |
| --- | --- |
| `user_signed_up` | `user.created` (flow 1) |
| `auth_state_changed` | `user.auth_state_changed` (flow 1) |
| `cabal_created` | `cabal.created` (flow 2) |
| `cabal_joined` | `cabal.member_joined` (flow 3) |
| `cabal_left` | `cabal.member_left` (flow 4) |
| `deposit_credited` | `deposit.credited` (flow 5) |
| `onramp_status_changed` | `onramp.status_changed` (flow 6) |
| `cabal_funded` | `cabal.funded` (flow 7) |
| `proposal_passed` | `proposal.passed` (flow 10) |
| `proposal_failed` | `proposal.failed` (flow 10) |
| `proposal_expired` | `proposal.expired` (flow 10) |
| `trade_executed` | `trade.confirmed` (flow 11) |
| `trade_blocked` | `trade.blocked` (flow 11) |
| `trade_failed` | `trade.failed` (flow 11) |
| `cash_out_completed` | `cashout.completed` (flow 14) |
| `cash_out_partial` | `cashout.partial` (flow 14) |
| `cash_out_failed` | `cashout.failed` (flow 14) |
| `withdrawal_sent` | `withdrawal.confirmed` (flow 15) |
| `follow_created` | `follow.created` (flow 20) |
| `comment_created` | `comment.created` (flow 21) |

`usdc_amount` is a decimal number of USDC for PostHog charts only. It rides on `deposit_credited`, `cabal_funded`, `cash_out_completed`, `cash_out_partial`, `withdrawal_sent`, `proposal_passed` and `trade_executed`. No event carries a wallet address, a transaction signature, a cabal name or a comment body.

`user_signed_up` sets the person properties `login_provider`, `created_at` and `auth_state`. `created_at` is in UTC and `auth_state` starts at `CREATED`. `auth_state_changed` sets `auth_state` to the new state. Both send their values in PostHog's `$set` property on the capture, so the server updates the person with no separate identify call.

`cabal_created`, `cabal_joined` and `cabal_left` set the person property `cabal_count` to the number of cabals the member belongs to, read through cabal's `CabalsOf` when the event is exported. A redelivered older event can set a stale count until the next membership event, which PostHog accepts because it holds behavior, not truth. `cabal_joined` also fires for a cabal's creator, with `via` set to `create`.

The RFC's flow table lists `analytics` as a consumer on every one of these flows (default 2026-09-27 added it to flows 7, 10, 11, 14, 20 and 21, and flows 3 and 4 follow for the join funnel and `cabal_count`), so `monacoctl flows check` holds each export to a test.

The PostHog call is an outbound HTTP call, so it sits behind a port with an anti-corruption adapter, circuit breaker and retry ([Patterns](backend-platform.md#patterns-and-where-each-earns-its-place)). A PostHog outage is a retryable `KindUnavailable` code: `bus.Dispatch` naks with backoff and never blocks another consumer.

**Flows to instrument.** Each gets a funnel in PostHog:

| Flow | Steps |
| --- | --- |
| Onboarding | login_started → login_completed → phone_shown → phone_verified / skipped → x_shown → x_linked / skipped → contacts_permission → follow_suggestions_shown → first_follow |
| Crypto deposit | deposit_opened → crypto_selected → address_copied → (server) deposit_credited |
| Card deposit | deposit_opened → card_selected → page_opened → privy_auth → fund_confirmed / cancelled → (server) deposit_credited |
| Join cabal | cabal_viewed → join_tapped → request_sent / joined → fund_sheet_opened → (server) cabal_funded |
| Propose | propose_opened → asset_selected → amount_entered → thesis_entered → submitted |
| Vote | proposal_viewed → vote_cast |
| Trade outcome | (server) proposal_passed → trade_executed / trade_blocked / trade_failed |
| Cash out | cash_out_opened → amount_entered → confirmed → (server) cash_out_completed / cash_out_partial / cash_out_failed |
| Withdraw | withdraw_opened → address_entered → confirmed → (server) withdrawal_sent |
| Feed | feed_opened → item_opened → comment_opened → comment_posted |
| Social | profile_viewed → follow_tapped; suggestion_shown → suggestion_followed / dismissed |
| Chat | chat_opened → message_sent |

The list of flows lives here; adding a screen means adding its steps here and in the app's event enum, so names never drift.

**Standard reports:** DAU/WAU/MAU, stickiness (DAU/MAU), D1/D7/D30 retention by signup week and by `login_provider`, funnel drop-off per flow, most-used flows, rage-tap hotspots by screen.

## Admin panel

### Access

- Separate web app at `admin.monacolabs.xyz`, not reachable from the consumer app.
- Login with Privy (Google), then the backend checks the user against an `admins` table (`user_id`, `role`: `viewer`, `moderator`, `operator`). No admin rights from a client flag or an env list baked into the frontend.
- Admin API under `/v1/admin/*`, same backend, separate middleware: admin role check, per-admin rate limit, all requests logged. Routes live in `api/openapi.yaml` like every other route. A failed role check is a `KindForbidden` code (403) from the `errs` table ([Errors](backend-platform.md#errors)). The `admin` module owns `admins`, `admin_actions` and the dead-letter queue ([Repository layout](backend-platform.md#repository-layout)).
- Each admin route lives in the HTTP adapter of the module that owns the data it changes (an ops pause in `funding`, a proposal void in `governance`), mounted under `/v1/admin/*` behind the shared admin middleware that `admin` exports. `admin` does not proxy other modules' commands.

### Actions

| Action | Role | Effect | Limits |
| --- | --- | --- | --- |
| **Pause / resume trading** (cabal) | operator | Writes a pause record with `reason = ops` through the `funding` module, which owns every pause (default 2026-09-27). Same record the external-deposit bounce uses ([deposits-withdrawals.md](deposits-withdrawals.md#direct-transfers-to-a-cabal-treasury-are-not-allowed)); `trading` and `treasury` read it through `funding`'s query port at check time: no executions, no funding, no cash outs. There is no `cabals.trading_paused_at` column. Global switch for all cabals too. | Resume only clears an `ops` pause, never an unresolved external-deposit pause. One operator is enough, so a pause is fast. |
| **Void a proposal** | moderator | Status `voided` with reason. Removed from voting; feed item shows "Voided by Monaco". | Allowed while `open` or `passed` with no live transaction. Once a swap is `submitted`, it cannot be voided; it finishes or fails. |
| **Ban a user** | moderator | `account_status` → `banned` ([auth.md](auth.md#account_status)). Middleware blocks funding, voting, proposing and commenting. Their open proposals voided. | They can still withdraw and cash out (decided 2026-09-27). The money is theirs. |
| **Ban a cabal** | operator, plus a second operator's approval | Cabal `status = banned`. Hidden from lists and feed; trading paused permanently. Then wind-down (default 2026-09-27): sell every holding and return USDC to each member pro rata through the normal cash-out path (flow 14). | Two-person approval (default 2026-09-27): the ban takes effect only when a second operator approves. Members' money always comes back; the wind-down moves it through the product path, not an admin transfer. |
| **Delete a comment** | moderator | Soft delete (`deleted_at`, `deleted_by = admin`). Renders as "Removed by Monaco". | |
| **Delete a proposal** | moderator | Soft delete: hidden from cabal and feed. | Executed proposals are linked to ledger rows and cannot be deleted, only hidden. |

Every action:

1. Requires a free-text reason.
2. Is a Command with an `IdempotencyKey`, handled by the module that owns the state (for example `governance`'s `VoidProposal`, flow 13), with the same guarded updates the product uses. The request context carries actor `admin` ([Context rules](backend-platform.md#context-rules)). The admin API adds no second path for changing state.
3. Appends `admin.action` (flow 26), plus the domain event such as `proposal.voided`, inside the same `uow.Do` as the state change.
4. The `admin` module's consumer on `admin.action` writes the `admin_actions` row (`admin_id`, `action`, `target_type`, `target_id`, `reason`, `before`, `after`, `created_at`) (default 2026-09-27). The `events` row is the atomic record; `admin_actions` is a projection of it, one relay hop behind. This keeps the module walls: the owning module never writes an `admin` table ([Dependency rules](backend-platform.md#dependency-rules-enforced-by-depguard)).
5. The usual consumers react to the same domain events. Only events in [What notifies](notifications.md#what-notifies-mvp) push: an ops pause pushes cabal members through `cabal.paused`; a void pushes nobody for MVP.

No admin action can move money. Moving funds stays in the ops runbooks ([ops-sweep-usdc.md](../legacy/ops-sweep-usdc.md), whose script was deleted with the legacy backend).

The `admin` module lands in [Rollout](backend-platform.md#rollout) step 6. The telemetry pipeline lands with the scaffold in step 1.

### Views

- Dashboards: the business metrics above.
- Lookups: user (profile, `auth_state` history, cabals, balances, recent actions), cabal (members, pot, holdings, proposals, pause state, external deposits), proposal (votes, comments, linked transaction), transaction (signature, explorer link, status history).
- Queues: external deposits pending bounce, stuck transactions, unpublished `events` rows older than 30 s, and dead letters (flow 27). `bus.Dispatch` writes a termed message and its error `code` to the `DEADLETTER` stream, and the `admin` module records each one in its `dead_letters` table with a resolve state (default 2026-09-27), so the queue outlives the stream's 30-day `MaxAge` ([NATS hosting and budget](backend-platform.md#nats-hosting-and-budget)). The redrive button runs the same path as `monacoctl deadletter retry`, back through `bus.Dispatch`, so `event_deliveries` still dedupes, and marks the row resolved on success.
- User-facing reporting of comments, proposals and users, and its moderation queue, is deferred past launch (default 2026-09-27).

### Build

Retool first (default 2026-09-27), on top of `/v1/admin/*`. The actions are already backend endpoints, so Retool is only a UI; nothing to host on Render. Data passes through Retool, so the admin API returns no more PII than a lookup needs. A custom app in `apps/admin` is the later option if Retool limits bite.

## Alternatives considered

| Alternative | Why not |
| --- | --- |
| Money metrics from PostHog | Client events drop. Ledger is the truth. |
| Admin actions as direct SQL | No audit trail, bypasses guarded updates and emits no events, so no consumer ever reacts. |
| Custom React admin app now | More work before launch for screens Retool gives for free. Revisit when Retool limits bite. |
| Admin routes inside the consumer app gated by a flag | Bigger attack surface on the public app, and a client flag is not an authorization check. |
| Hard deletes of user content | Breaks threads, feed history and ledger links. `users`, chat messages and `follows` soft delete through `deleted_at` (decided 2026-09-27), as do comments and proposals above. Ledger rows are never deleted. |
| Self-hosted PostHog | The RFC hosts nothing itself. PostHog Cloud, US region (default 2026-09-27). |
| PostHog autocapture only | Event names drift with UI changes and funnels break silently. Named step events per flow are stable. |

## Open questions

None.

Log: [log/analytics-admin.md](log/analytics-admin.md).
