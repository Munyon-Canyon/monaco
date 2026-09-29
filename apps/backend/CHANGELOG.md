# Changelog

All notable changes to the Monaco backend. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Write one line per change a user or operator would notice, not one per commit, under `### Added`, `### Changed`, `### Fixed`, `### Removed` or `### Security`.

A checkpoint PR into `main` must change this file, and `## [Unreleased]` must hold its entries. The `Changelog (checkpoint into main)` check (`scripts/check-changelog.py`) fails it otherwise. Ticket PRs into the feature branch don't need an entry. At each checkpoint, rename `## [Unreleased]` to `## [checkpoint N] - <date>`, the date the checkpoint merges into `main`, and open a new `## [Unreleased]` above it that holds only an empty `### Added` heading, where `monacoctl gen module` adds its line. The check accepts that rename when the new checkpoint section has entries.

## [Unreleased]

### Added

- The `system` module: `system_pings` and the `RecordPing` command, which writes a ping and appends `system.pinged` (now carrying `user_id`) in one transaction.
- `POST /v1/system/pings` and `GET /v1/system/pings/{id}`, and the `system.echo` consumer that marks a ping echoed and sends its user a `ping_echoed` hint once the update commits. `db.Tx.AfterCommit` runs a callback only after its transaction commits.
- The acceptance scenario DSL (`internal/testkit/scenario`), which drives the api and worker components in one process over HTTP and waits on consumer commits and SSE hints instead of sleeping; `testkit.Seed`, which replays a named event sequence from `internal/testkit/scenarios` through the consumers; and the flow 00 script in `internal/testkit/flows`.
- Flow 00 (`RecordPing`) in `flows.tsv` at `built`, with one acceptance test per outcome. `monacoctl flows check` accepts a handler name as well as a durable in the consumers column.
- The module registry. The api boots every module through it in a fixed order and composes their routes in httpx; the worker serves health on `:8081`, runs the pollers and shuts down in order.
- `monacoctl gen module|command|query|consumer|provider|flow`, generators for the paved paths, each checked against golden output.
- The Jupiter Swap API v2 client (order, quote, execute) and Price API v3 client, with the `jupiterfake` test port.
- `monacoctl agents`, the tooling that dispatches, checks and lands agent work: `check` (the 60 s stage 0 run before a push), `forecast`, `verify-plan`, `verdict`, `watch`, `land-stack`, `timeline` and `handoff`.
- Test lint: the `wallclock` and `testwait` nogo analyzers fail wall-clock waits and poll loops in tests, and the `testmain` analyzer makes `testkit.Main` the only `TestMain` body. Each test run names its databases with its own prefix and refuses to drop any other.
- A `gate-changes` PR check and Claude Code hook that warn when a change weakens a test gate (coverage excludes, allowed mutants, baselines, goldens, lint exclusions, skipped or deleted tests).
- `govulncheck` in the backend nightly, and a summary line for each mutation run.

### Changed

- Migrations are named by timestamp; databases that applied the old names keep working.
- `.golangci.yml` is generated from a base file plus each module's `lint.yml`, and messages, tools and consumers each register from one file.
- `monacoctl flows check` also checks each flow's trigger, command and consumers against the code.
- CI splits into a PR stage and a full merge-queue stage, runs each job only when its inputs change (every job for a path no filter owns), adds a `ready` job for tidy and generated files, and lints PR titles, bodies and commit subjects. Feature branch PRs land through a merge queue, and `main` merges back into the feature branch after a checkpoint.
- Test budget: each package warns at 10 s and fails at 20 s, and the laptop run of `just test backend` has 90 s.

### Fixed

- Three flaky bus tests that hung, leaked a drain goroutine or read duplicates, and a race between stream creates and the shared test nats-server's cleanup.

## [checkpoint 2] - 2026-09-27

### Added

- The events relay: an outbox drain in api and worker publishes committed events from Postgres to JetStream and reports its backlog.
- `bus.Dispatch` with the consumer registry, `event_deliveries` dedupe, nak backoff and dead letters to `DEADLETTER` after the last delivery, plus `bus.KeepAlive` and trace extraction into consumers.
- `Idempotency-Key` replay on the `idempotency_keys` table. Anonymous keys are scoped, and an abandoned in-flight key is taken over.
- Bearer-token auth middleware with a `DevVerifier`, and `monacoctl dev token`.
- `GET /v1/stream`, served by an SSE hub that routes `hint.>` messages to registered connections.
- Test gates: the `just test backend` time budget (`monacoctl test-report`), 100% merged coverage (`monacoctl coverage` and `coverage.exclude`), allocation and query-count baselines (`testkit.AssertAllocs`), and `just test mutation` with gremlins and `mutants.allow`.
- The nightly backend suites (100k rapid runs, fuzz per target, seed sweeps and the tests `-short` skips) and a PR mutation job.

## [checkpoint 1] - 2026-09-27

### Added

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
