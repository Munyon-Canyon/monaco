# Analytics & admin panel log

Dated record of changes to [analytics-admin.md](../analytics-admin.md). Add one line per change, newest last.

- 2026-09-29: Admin routes live in the owning module's HTTP adapter behind shared admin middleware from `admin` (default; see #535).
- 2026-09-27: Decided: `users`, chat messages and `follows` soft delete through `deleted_at`; other tables keep their current delete behavior; ledger rows are never deleted. Narrowed the hard-delete alternative to match.
- 2026-09-27: Closed the last open question (default 2026-09-27): PostHog Cloud runs in the US region.
- 2026-09-27: Applied decisions. Decided: banned users can withdraw and cash out. Defaults: new `analytics` module owns the PostHog export and dashboards; ops pause is a `funding` pause record with `reason = ops`, no `trading_paused_at`; `admin_actions` written by an `admin` consumer of `admin.action`; `dead_letters` table with resolve state next to the `DEADLETTER` stream; `analytics` added to flows 7, 10, 11, 14, 20, 21 in the RFC; Retool first; banned-cabal wind-down through cash out; two-person approval for cabal bans only; PostHog Cloud; session replay off; user reporting deferred. Open question left: PostHog region.
- 2026-09-27: Reconciled with [backend-platform.md](../backend-platform.md): system health moves from Prometheus and Slack/Discord to OTel over OTLP into Grafana Cloud with Grafana alerts, Sentry kept for panics and `KindInternal`; PostHog server events mapped to bus subjects, with `analytics` missing from six `flows.tsv` consumer cells; admin actions are Commands with actor `admin` inside `uow.Do`; dead letters come from the `DEADLETTER` stream with redrive through `bus.Dispatch`; rollout step 6. Added open questions on analytics ownership, where `admin_actions` is written, and panel hosting.
- 2026-09-26: Outbox rows replaced by `events` rows delivered over the NATS event bus ([event-bus.md](../event-bus.md)).
- 2026-09-26: Initial decision. Prometheus for system health, Postgres for business metrics, PostHog for behavior; admin panel with audited, reason-required actions through the normal backend services.
