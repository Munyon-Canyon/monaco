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
- `monacoctl agents`, the tooling that dispatches, checks and lands agent work: `check` (the stage 0 run before a push, with a budget per row in `.monaco/agents.toml`, Go under `-tags faultpoints` with `-p` capped by the running owners, and Swift when `packages/mobile-core` or `openapi.yaml` changes), `forecast`, `verify-plan` (opus or sonnet only), `verdict`, `watch`, `land-stack` (relands an ejected stack whole in one call), `timeline` and `handoff`.
- Test lint: the `wallclock` and `testwait` nogo analyzers fail wall-clock waits and poll loops in tests, and the `testmain` analyzer makes `testkit.Main` the only `TestMain` body. Each test run names its databases with its own prefix and refuses to drop any other.
- A `gate-changes` PR check and Claude Code hook that warn when a change weakens a test gate (coverage excludes, allowed mutants, baselines, goldens, lint exclusions, skipped or deleted tests).
- `govulncheck` in the backend nightly, and a summary line for each mutation run.
- `platform/chain` adapters. `chain/privy` verifies access tokens and Svix webhooks, reads users, finds or creates a member's one wallet, creates app-owned wallets idempotently and signs under the app authorization key. `chain/solana` reads balances, signature statuses, mint configs and inbound SPL transfers, and sends transactions. `chain/relayer` builds relayer-paid `TransferChecked` transfers that are fully signed before broadcast. `testkit/chainfake` stands in for all three in module tests.
- Config keys `PRIVY_*`, `SOLANA_RPC_URL`, `SOLANA_USDC_MINT` and `RELAYER_PRIVATE_KEY`, and the `privy_unavailable`, `rpc_unavailable`, `relayer_underfunded` and `invalid_address` error codes.
- `monacoctl replay --into <database_url> [--to <event_id>] [--verify]` rebuilds every projection into a fresh database from the `events` table and diffs it against the source. It refuses the source database and the dev container. `replay.RegisterLedgerCheck` is the hook for money modules.
- `monacoctl backfill --consumer <handler> --types <t1,t2> [--since <event_id>]` runs one handler over past events. `event_deliveries` dedupes it, so a second run applies nothing.
- `monacoctl events export [--aggregate <type>:<id>] [--anonymize]` writes the `events` table as seed-scenario JSONL. `--anonymize` replaces the actor and `pii`-tagged fields with deterministic pseudonyms, so the output still seeds.
- `monacoctl deadletter list` and `monacoctl deadletter retry <seq> | --all --consumer <durable>`. A retried event that a handler already applied is a no-op.
- `monacoctl verify [flow <id> | all] [--outcome X] [--crash-at P]` runs each flow script against the real api, worker and fakes binaries on a throwaway stack. It checks convergence and invariants under fixed budgets and writes evidence to `apps/backend/.verify/`. `--crash-at P` crashes the worker at faultpoint `P` and checks the flow still converges. `MONACO_BUS_API_RELAY` and `MONACO_BUS_ACK_WAIT` are new config for it.
- The stage 2 `e2e` CI job. The merge queue runs `monacoctl verify all` and `monacoctl verify all --crash-at after-publish` on every backend change, `ci-ok` requires it, and the evidence uploads as `verify-evidence`. The backend nightly also runs `monacoctl verify all`.
- The `Changelog (checkpoint into main)` check, which fails a checkpoint PR into `main` that changes `apps/backend` without an entry here.
- `monacoctl garden report [--skip-mutation]`, which lists dead code, candidate lints, surviving mutants and generator drift in `garden-report.md`. The nightly `gardener` workflow runs it with `--skip-mutation` and keeps the result in one open `gardener` issue.

### Changed

- Migrations are named by timestamp; databases that applied the old names keep working.
- `.golangci.yml` is generated from a base file plus each module's `lint.yml`, and messages, tools and consumers each register from one file.
- `monacoctl flows check` also checks each flow's trigger, command and consumers against the code.
- CI splits into a PR stage and a full merge-queue stage, runs each job only when its inputs change (every job for a path no filter owns), adds a `ready` job for tidy and generated files, and lints PR titles, bodies and commit subjects. Feature branch PRs land through a merge queue, and `main` merges back into the feature branch after a checkpoint.
- Test budget: each package warns at 10 s and fails at 20 s, and the laptop run of `just test backend` has 90 s.
- api and worker refuse to boot in staging and production when the relayer holds 0.001 SOL or less (`relayer_underfunded`).
- `monacoctl gen consumer` wires the module's `Bus` into the consumer it generates, so a consumer that publishes hints or events needs no hand edits.
- `monacoctl flows check` calls a flow `verified` when every outcome has a registered verify script. Flow 00 is `verified`.
- The PR size check fails a committed binary unless it sits under `testdata/` or has a media extension. The `large-pr` label does not override it.
- CI skips PRs based on Graphite's temporary `graphite-base/` branches. `agents status` runs queue in one concurrency group per PR instead of cancelling each other. The flake job reruns changed tests with `-tags faultpoints`.

### Fixed

- Three flaky bus tests that hung, leaked a drain goroutine or read duplicates, and a race between stream creates and the shared test nats-server's cleanup.
- The scripts CI job, which failed on every PR once `scripts/cloud-setup.sh` came over from `main`.
- Flakes that ejected merge queue entries: `ETXTBSY` in the monacoctl migrate and bench tests, and the bus apply stream test on repeated runs.

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
