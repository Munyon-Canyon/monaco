# Changelog

All notable changes to the Monaco backend. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Write one line per change a user or operator would notice, not one per commit.

## [Unreleased]

### Added

- The `system` module: `system_pings` and the `RecordPing` command, which writes a ping and appends `system.pinged` (now carrying `user_id`) in one transaction.
- New Go backend module at `apps/backend` with `api`, `worker` and `monacoctl` binaries that load typed boot config and fail fast on a bad value.
- One error code table (`internal/errs`) that drives HTTP status, retry and alert behaviour, and the `ErrorCode` enum in the OpenAPI spec.
- `api/openapi.yaml` as the HTTP contract. The server is generated from it, errors are `application/problem+json`, and every test response is validated against it.
- Money types with checked arithmetic, UUIDv7 ids and an injectable clock.
- JSON logs with context join keys, secret redaction and a registry of log messages. Traces through OpenTelemetry, carried across NATS.
- An events registry with golden payload contracts, and the events outbox tables.
- Atlas migrations (`monacoctl migrate`), sqlc queries, and NATS plus a tmpfs test Postgres in the local compose stack.
- An outbound HTTP client with deadlines, retries and a circuit breaker, and a scriptable fakes server for outside services.
- `testkit` with per-test database clones, fakes for the clock, ids and seeds, and `monacoctl bench db`.
- `flows.tsv` with `monacoctl flows check`, run by `just test backend` and CI.
- Lint gates: golangci-lint with per-module depguard walls, a ban on bare `go` statements, and a no-comments checker that also runs as an agent edit hook.
- Docs site built with MkDocs Material, with generated event, flow, error, log and API reference pages. `just gen docs` regenerates them and CI fails when they are stale.

### Changed

- CI runs only on ready PRs to `main`, reports one `ci / ci-ok` check, and caches Go builds from the nightly job only.

### Fixed

- Nothing yet.

### Removed

- The legacy backend, its migrations, its Go domain package and the reference trading bot.

### Security

- Nothing yet.
