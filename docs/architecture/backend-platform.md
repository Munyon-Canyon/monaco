# Backend platform (Go rewrite)

**Status:** Proposed 2026-09-26. Builds on [event-bus.md](event-bus.md), [trade-execution.md](trade-execution.md), [data-model.md](data-model.md). Ignores the current `apps/backend` code on purpose.

## Decision

1. **Stay on Go.** No Rust rewrite.
2. **One modular monolith, two entrypoints.** One module, one container image, `cmd/api` (HTTP + SSE) and `cmd/worker` (JetStream consumers, pollers, relay). Modules talk through NATS events, never through each other's packages. Splitting a module into its own service later is a deploy change, not a code change.
3. **Clean Architecture, three rings, enforced by lint.** `domain` (pure) ← `app` (use cases, ports) ← `adapters` (Postgres, NATS, Jupiter, Privy, HTTP). Import direction is checked in CI by `depguard`, not by review.
4. **Go channels are in-process only. NATS is the only cross-module path.** "NATS channel based" means JetStream subjects between modules, and bounded Go-channel pipelines inside one handler. Never a Go channel as a substitute for the bus.
5. **Contracts are generated, not hand-written.** OpenAPI 3.1 spec is the source of truth for the iOS contract (Go server stubs via `oapi-codegen`, Swift client via `swift-openapi-generator`). SQL is the source of truth for rows (`sqlc`). Event payloads are Go types in one `events` package with a subject registry.
6. **Harsh `golangci-lint` v2, no inline `//nolint`, no comments.** Exceptions live in `.golangci.yml` with a reason. Hand-written Go carries only machine-read comments. Custom `forbidigo` rules encode Monaco-specific bans (floats for money, `time.Now` in domain, `context.Background` outside `main`).

## Why

- **Go over Rust.** The hot path is Jupiter `/execute` (up to 2 min), Solana RPC and Privy signing. Latency is network-bound. Volume is thousands of events a day. Rust buys memory safety and speed we do not need and costs compile time, hiring, and agent throughput. NATS's first-party client and embeddable server are Go, so tests run a real JetStream in-process.
- **Monolith over microservices.** One team, one database, money invariants that span a transaction (state change plus `events` row). Separate services would force a distributed transaction or a second outbox per service. Module boundaries plus NATS give the decoupling without the ops.
- **Three rings, not five.** Classic Clean Architecture in Go tends to grow `entity → usecase → interactor → repository → gateway → presenter` layers with one-implementation interfaces at every seam. That doubles files and hides the call path. Three rings keep the rule that matters (domain imports nothing) and nothing else.
- **Lint over docs.** Agents copy whatever the surrounding code does. A rule in a Markdown file gets skipped. A `depguard` failure does not.

## Repository layout

Takes `cmd/`, `internal/`, `api/`, `migrations/`, `deployments/`, `scripts/`, `docs/` from [golang-standards/project-layout](https://github.com/golang-standards/project-layout). Skips `pkg/` (nothing is imported from outside this module), `vendor/`, `src/`, `init/`, `web/`. That repo is a community convention, not an official standard; `internal/` is the only part the compiler enforces.

```
apps/backend/
├── AGENTS.md                      hard rules for agents (short; points at lints)
├── CHANGELOG.md                   Keep a Changelog, human-written
├── .golangci.yml
├── go.mod                         go 1.25+, toolchain pinned
├── api/
│   └── openapi.yaml               source of truth for HTTP contract
├── cmd/
│   ├── api/main.go                wiring only: config → adapters → app → http
│   ├── worker/main.go             wiring only: config → adapters → consumers
│   └── monacoctl/main.go          ops CLI: replay, dead-letter retry, backfill
├── internal/
│   ├── platform/                  cross-cutting mechanism, no business rules
│   │   ├── config/                env → typed Config, validated once at boot
│   │   ├── clock/                 Clock interface; real + fake
│   │   ├── db/                    pgxpool, UnitOfWork, tx helpers
│   │   ├── bus/                   JetStream setup, Dispatch wrapper, relay
│   │   ├── concurrency/           Pool, Pipeline, FanOut helpers (generic)
│   │   ├── httpx/                 middleware, error → problem+json, SSE hub
│   │   ├── observability/         slog, OTel tracer/meter, NATS header propagation
│   │   └── money/                 Micros, TokenAmount, share math (math/big)
│   ├── events/                    every event type + subject registry (one file per aggregate)
│   ├── modules/
│   │   ├── identity/              auth session, users, profile
│   │   ├── cabal/                 cabals, members, access requests, rules
│   │   ├── governance/            proposals, votes, tally, expiry
│   │   ├── trading/               trade engine, swap state machine, sweeper
│   │   ├── treasury/              ledgers (cabal_txns, user_txns), fund, cash out, positions
│   │   ├── funding/               deposits poller, withdrawals, onramp sessions, bounce
│   │   ├── market/                assets catalog, prices, provider strategies
│   │   ├── agents/                agent keys, intents, budget enforcement
│   │   ├── social/                follows, feed, comments, chat bridge
│   │   ├── ranking/               valuation snapshots, leaderboards
│   │   ├── notify/                notifications, device tokens, APNs
│   │   ├── referrals/
│   │   └── admin/                 admin actions, dead-letter queue
│   └── testkit/                   fakes, fixtures, embedded NATS, Postgres container
├── migrations/                    atlas versioned SQL, forward-only
├── flows.tsv                      every flow, its outcomes, and its test status (see Flows)
├── queries/                       sqlc .sql files, one dir per module
├── deployments/                   compose, Dockerfile, NATS stream config
└── test/
    └── e2e/                       black-box: real binary, real PG + NATS, fake externals
```

Each module has the same four directories. Nothing else.

```
internal/modules/governance/
├── domain/        Proposal, Vote, Tally(), status machine. Pure. No ctx, no I/O, no errors from infra.
├── app/           commands + queries (use cases). Defines the ports it needs.
├── adapters/      postgres repo (sqlc), http handlers (oapi-codegen strict server), consumers
└── module.go      New(deps) → *Module; registers routes and consumers
```

### Dependency rules (enforced by `depguard`)

| Package | May import | May not import |
| --- | --- | --- |
| `*/domain` | stdlib, `platform/money`, `events` | `app`, `adapters`, `platform/db`, `platform/bus`, `pgx`, `nats`, `net/http`, other modules |
| `*/app` | own `domain`, `events`, `platform/{money,clock}` | `adapters`, `pgx`, `nats`, `net/http`, other modules' `app` or `domain` |
| `*/adapters` | own `app` + `domain`, `platform/*`, drivers | other modules' packages |
| module A | module B | never. Cross-module effects go through an event. Cross-module reads go through a read-only query port that module B exports in `module.go`. |
| `cmd/*` | everything | nothing imports `cmd` |

## Patterns, and where each earns its place

| Pattern | Where | Shape |
| --- | --- | --- |
| **Unit of Work** | Every write that emits an event. Required by the events-as-outbox rule. | `uow.Do(ctx, func(ctx context.Context, tx Tx) error)`. `Tx` exposes the module's repos and `Events.Append`. Commit wakes the relay. No `Begin/Commit` anywhere else (lint: `forbidigo` on `pgx.Tx.Commit` outside `platform/db`). |
| **Command** | Every user or agent intent: `ProposeTrade`, `CastVote`, `FundCabal`, `CashOut`, `Withdraw`, `SubmitAgentIntent`, `CreateCabal`. | Typed struct with `IdempotencyKey`, one `Handle(ctx, cmd) (Result, error)` per command. HTTP handler parses → builds command → calls handler. Same command can come from HTTP, an agent key, or `monacoctl`. |
| **Query (CQRS-lite)** | Every screen read. | Reads hit projections (`cabal_positions`, `leaderboard_entries`, feed) directly via sqlc, bypassing domain. Writes go through commands. No shared "service" object doing both. |
| **Strategy** | Asset issuers (xStocks, Tessera, PreStocks) and swap venue (Jupiter today). | `type AssetProvider interface { Catalog(ctx); Quote(ctx, Asset, Micros) (Quote, error) }`. Best-price buy runs every provider that lists the asset (fan-out, below) and picks the min. `Venue` interface for execution. |
| **Registry (creational)** | Providers keyed by `Issuer`; consumers keyed by durable name; events keyed by subject. | Built once in `cmd/*/main.go`. Duplicate key panics at boot. Tests assert every `events` type has at least one registered subject. |
| **Smart constructor (creational)** | Every branded type: `UserID`, `CabalID`, `Micros`, `SharesUnits`, `Mint`, `SolanaAddress`. | `ParseX(raw) (X, error)` at the boundary, unexported fields so the zero value can't be forged outside the package. |
| **Functional options (creational)** | Clients with many knobs: Jupiter, Privy, NATS, HTTP server. | `jupiter.New(baseURL, jupiter.WithTimeout(d), jupiter.WithRetry(p))`. Not for domain types. |
| **State machine** | Proposal status, swap `created → submitted → confirmed|failed`, onramp session, cash-out job. | Status is a Go type with a `transitions` table; `Next(from, event) (to, error)` is pure. Adapters apply it as a guarded `UPDATE … WHERE status = $from`. `exhaustive` lint fails on a missed case. |
| **Anti-corruption layer** | Jupiter, Privy, xStocks, Helius, APNs responses. | Adapter maps wire JSON to domain types. Wire types never leave the adapter package. |
| **Circuit breaker + retry** | Outbound HTTP only. | `sony/gobreaker` + capped exponential backoff with jitter. Never inside a DB transaction. |

Patterns deliberately left out: abstract factory, builder for domain objects, generic repository (`Repository[T]`), dependency-injection frameworks (wire/fx/dig). Constructors wired by hand in `main.go` are readable in one file.

## Context rules

1. `ctx context.Context` is the first parameter of anything that does I/O or may block. Domain functions take no context (they do no I/O).
2. Never store a context in a struct (`containedctx`). Never pass `nil` (`staticcheck SA1012`).
3. `context.Background()` / `TODO()` only in `main`, tests and `platform/bus` consumer roots (`forbidigo`).
4. Every outbound call gets a deadline at the adapter: RPC 5 s, Privy 10 s, Jupiter quote 5 s, `/execute` 2 min. Set in config, not literals.
5. HTTP request context dies with the client. Work that must finish after the response (nothing, ideally; the event does it) uses `context.WithoutCancel(ctx)` plus its own timeout, never `Background`.
6. Values in context are limited to request-scoped metadata: trace span, actor (`user`/`agent`/`admin`/`system`), request id. Typed unexported keys. Never dependencies, never optional parameters.
7. Cancellation propagates into every goroutine. Every `select` that sends or receives also selects `<-ctx.Done()`.
8. Use `context.WithCancelCause` / `context.Cause` so logs say why work stopped.
9. Trace context crosses NATS in message headers (`otel` propagator on publish and in `bus.Dispatch`).

## Concurrency rules

Where concurrency pays in Monaco, and the pattern for each:

| Job | Pattern | Bound |
| --- | --- | --- |
| Price poller across N assets and 3 providers | Fan-out/fan-in with `errgroup.WithContext` + `SetLimit` | 8 in flight per provider |
| Deposit poller across all member wallets | Bounded worker pool reading wallet ids from a channel | 16 workers, RPC rate limiter shared |
| Leaderboard valuation per cabal | Pipeline: load holdings → value (needs prices) → write snapshot, each stage a goroutine, buffered channels | buffer 64, workers per stage 4 |
| Notification fan-out to device tokens | Worker pool, APNs HTTP/2 client shared | 32 |
| Best-price quote across issuers | Fan-out, first-error cancels, collect all | number of providers |
| Relay publish batch | Sequential (order and ack matter); parallelism comes from multiple processes and `SKIP LOCKED` | 1 |

Hard rules:

1. No bare `go f()` in business code. Use `platform/concurrency` helpers or `errgroup`. `forbidigo` flags `go ` statements outside `platform/` and `cmd/` (custom analyzer if `forbidigo` can't match syntax; see Lint).
2. Every goroutine has an owner that waits for it. Tests run `go.uber.org/goleak` in `TestMain`.
3. The sender closes the channel. Receivers never close.
4. Every channel is bounded. Unbuffered or explicit size; no "big enough" 10000 buffers.
5. Backpressure over dropping. Drop only for live SSE hints, and count drops in a metric.
6. Shared mutable state goes behind one goroutine or a mutex in the same struct, never both. Prefer ownership over locks.
7. `go test -race` in CI, always.
8. JetStream `MaxAckPending` is the cross-process concurrency bound. In-process pools must not exceed it.

`platform/concurrency` exports exactly three generic helpers, each ~40 lines and fully tested:

```go
func Pool[In, Out any](ctx context.Context, workers int, in <-chan In, fn func(context.Context, In) (Out, error)) (<-chan Out, <-chan error)
func Stage[In, Out any](ctx context.Context, in <-chan In, buf int, fn func(context.Context, In) (Out, error)) <-chan Result[Out]
func FanOut[T, R any](ctx context.Context, limit int, items []T, fn func(context.Context, T) (R, error)) ([]R, error)
```

## Money and types

- USDC and token amounts are `money.Micros` / `money.BaseUnits` (unsigned 64-bit branded ints). Share math multiplies then divides through `math/big` and rounds down, in favour of the pot, in one function.
- `float32`/`float64` are banned in `domain`, `app`, `platform/money` (`forbidigo` on `float64` identifiers there). Display formatting is the client's job from integer micros plus decimals.
- IDs are UUIDv7 branded per aggregate. `CabalID` cannot be passed as `UserID`.
- Enums are named string types with an exhaustive `switch` (`exhaustive` lint with `default-signifies-exhaustive: false`).

## Errors

One error type, one closed list of codes, one table that maps each code to everything a boundary needs. Every edge case is a row in that table and a row in [`flows.tsv`](#flows), so "did we handle X" is a lookup, not a code read.

### Shape

```go
type Kind uint8

const (
	KindInvalid      Kind = iota + 1 // caller's input; 400; never retry
	KindUnauthorized                 // 401
	KindForbidden                    // 403
	KindNotFound                     // 404
	KindConflict                     // idempotency replay mismatch, version clash; 409; never retry
	KindBlocked                      // domain rule refused (insufficient funds, vote closed, pot cap); 422; never retry
	KindUnavailable                  // upstream down or timed out; 503; retry
	KindInternal                     // bug or invariant broken; 500; alert; term
)

type Error struct {
	Code  Code        // closed enum, e.g. CodeInsufficientFunds
	Kind  Kind        // derived from Code via the table; never set by hand
	Op    string      // "treasury.FundCabal"
	Attrs []slog.Attr // cabal_id, amount, provider; never secrets
	Err   error       // wrapped cause, nil at the origin
}
```

- `Code` is a string enum in `internal/errs/codes.go`. The table `codes[Code] = {Kind, Retryable, Alert, Message}` is the single source of truth. `exhaustive` fails a switch that misses a code. A test asserts every `Code` constant has a table row and every row has a constant.
- The OpenAPI `ErrorCode` enum is generated from that table by `monacoctl gen errors` and checked fresh in CI, so the Swift client's `switch` and the Go table cannot drift.
- `Kind` decides the HTTP status, the bus verdict (ack, nak with delay, term) and whether to alert. Nothing else reads `Kind`; the `codes` table is the only map.
- Domain returns `errs.New(CodeInsufficientFunds, op, attrs...)`. Adapters wrap upstream failures with `errs.Wrap(err, CodeJupiterUnavailable, op, attrs...)`. Sentinel `var ErrX = errors.New(...)` is banned (`forbidigo` on `errors\.New` outside `errs`), so an error without a code cannot exist.
- `errors.Is` and `errors.As` are the only comparisons (`errorlint`). `errs.CodeOf(err)` returns `CodeInternal` for any non-`*errs.Error`, which is the fallback that makes a missed wrap visible as a 500 and an alert, not a silent 200.

### Surfacing

Each error is logged once, at the boundary that stops it, with the same `trace_id` the client received. Never log-and-return.

| Boundary | What it does with the error |
| --- | --- |
| HTTP (`httpx.Problem`) | RFC 9457 `application/problem+json`: `code`, `message` (from the table), `trace_id`, `retryable`. `4xx` logs at `Info`, `5xx` at `Error`. `KindInternal` also emits an alert. The response never carries `Err` or `Attrs`. |
| Bus (`bus.Dispatch`) | `Retryable` → nak with backoff. Otherwise term, write the message and the error to `DEADLETTER`, alert on `KindInternal`. `event_deliveries` row records the code. |
| Poller tick | Log with `Attrs`, count `poller_errors_total{poller,code}`, continue the loop. A tick never aborts the poller. |
| `cmd/*` boot | Fail fast with the code. No partial boot. |

Lint that keeps it to one log: `slog.Error` and `slog.Warn` are allowed only in `platform/httpx`, `platform/bus`, `platform/observability` and `cmd/` (the `forbidigo` pattern with path exclusions in the lint config). Modules return errors; they do not narrate them.

Panics are recovered at exactly three roots: HTTP middleware, `bus.Dispatch`, and the poller tick wrapper. Each converts the panic to `KindInternal` with the stack in `Attrs`, then follows the row above. In tests, the recovered panic is re-raised so the test fails.

### Reaching every branch

Error handling that no test reaches is decoration. The rules that make each branch reachable:

- Every port fake in `testkit` has `Fail(op string, err error)` and `FailOnce`. An `if err != nil` in `app` that a test cannot trigger through a fake is a design smell, not a coverage exclusion.
- `errcheck` runs with `check-blank: true` and `check-type-assertions: true`. `_ = f()` does not compile past lint.
- `nilerr` and `nilnil` are on: returning `nil` when `err != nil` is a lint failure.
- Mutation testing (below) flips `err != nil` to `err == nil` and removes `return err` lines. A surviving mutant is an error branch nothing checked.
- Every `KindInternal` in production is a bug. The alert links the trace, and the fix PR adds the code that should have caught it, so the `Internal` count trends to zero.

### Outcomes as a map

Each flow lists its outcomes in `flows.tsv`: the success path, every `Code` it can return, and every crash point (`after-sign`, `after-execute`, `before-commit`, `after-publish`). One acceptance scenario per outcome, named `TestFlow07_FundCabal_InsufficientFunds`. `monacoctl flows check` reads `go test -json` output and fails CI when an outcome in the TSV has no test that ran and passed. That is the codepath map: the file is the list, the test names are the proof, and the check is what keeps them equal.

## Logs as evidence

Three records exist, and each answers a different question. The `events` table in Postgres is what happened to state, and it is the source of truth. JetStream is how that reached the consumers. Logs are what each process did and did not do, and why, including every branch that ended with no state change. When something goes wrong, the logs must be enough to reconstruct how the current state came to be without reading code.

### Rules

1. **Structured only, stable names.** slog with `attr-only`, `static-msg`, `context: all` (`sloglint`). The message is an identifier, `treasury.fund.rejected`, never a sentence with values in it. Values are attrs. A message name is registered in `internal/observability/msgs.go` next to the attrs it requires, and a test fails on a call site that logs an unregistered name or omits a required attr. That registry is also the log catalog page in the docs.
2. **Every line carries the join keys.** `trace_id`, `span_id`, `request_id` or `event_id`, `actor`, `module`, `op`. The context logger adds them; a handler never types them. `sloglint context: all` means a call site cannot get a logger without the context.
3. **Log the decision, not the step.** One line where a branch chooses: a guard refused (the code and the numbers it compared, `have=4_000_000 need=5_000_000`), a retry was scheduled (`attempt=3 delay=30s cause=JupiterUnavailable`), a consumer skipped a duplicate (`event_id delivery=2`), a poller tick found nothing. Inaction is evidence. "Deposit poller ran at 10:04:10, scanned 212 wallets, found 0" is the line that proves a missing deposit was not the poller's fault.
4. **Money lines carry before and after.** Any line about a balance, share count or position has `before`, `after`, `delta`, `asset`, `cabal_id`. The ledger can be rebuilt from logs alone, and `monacoctl replay --verify` checks that it matches.
5. **Truthful means logged after commit.** A line that says "funded" before the transaction commits lies when the commit fails. `uow.Do` itself logs `tx.committed` or `tx.rolled_back` with the cause and the event ids it appended, so every write gets its terminal line without the handler doing it. Inside a transaction closure only `Debug` is allowed. That rule has no lint yet; the `money-change` skill checklist carries it until one exists.
6. **Levels mean one thing each.** `Debug` is step detail, off in production. `Info` is a decision or an outcome. `Warn` is degraded but handled (retry, fallback provider, stale price used). `Error` is a boundary only, one per failure, per the Errors section.
7. **Nothing secret, nothing personal.** The slog handler in `platform/observability` redacts by attr key (`phone`, `email`, `token`, `key`, `seed`, `signature`) and by value pattern (base58 secrets, JWTs). A golden test feeds each pattern through the handler and asserts the output. New attr keys that carry PII go on the list in the same PR.

### Replay and seeded states

The `events` table plus deterministic consumers make state replayable. `monacoctl replay --to <event_id>` rebuilds every projection (`cabal_positions`, `leaderboard_entries`, feed) into a fresh database from the event log, so "what did the leaderboard show at 14:02" is a command, not archaeology. Replay reads events only; it never calls Jupiter, Privy or RPC, which is why consumers keep side effects behind ports.

The same mechanism seeds tests. `testkit/scenarios/` holds named event sequences (`cabal-with-pending-trade`, `member-mid-cashout`, `treasury-after-bounce`), each produced by running the real commands once and exporting the resulting `events` rows. `testkit.Seed(t, db, "cabal-with-pending-trade")` replays one into the test's database in milliseconds. Crash-point and acceptance tests start from a seeded state instead of building it by hand. `monacoctl events export --cabal <id> --anonymize` pulls a real cabal's history so a production bug reproduces locally from the same state.

Logs ship to Grafana Loki over the same OTLP pipeline as traces, with `trace_id` as the link between a log line, its trace, and the `events` row. Retention is 30 days in Loki; the `events` table is forever.

## Lint

`golangci-lint` v2, run in CI and a pre-commit hook. Baseline for greenfield: every finding blocks.

```yaml
version: "2"
run:
  timeout: 5m
  tests: true
linters:
  default: none
  enable:
    # correctness
    - govet
    - staticcheck
    - errcheck
    - ineffassign
    - unused
    - nilerr
    - nilnil
    - bodyclose
    - sqlclosecheck
    - rowserrcheck
    - durationcheck
    - makezero
    - copyloopvar
    - exhaustive
    - gosec
    - musttag
    - asasalint
    - reassign
    # errors
    - errorlint
    - wrapcheck
    - errname
    - err113
    # context and concurrency
    - contextcheck
    - containedctx
    - fatcontext
    - noctx
    - spancheck
    # architecture
    - depguard
    - forbidigo
    - gochecknoglobals
    - gochecknoinits
    - ireturn
    - interfacebloat
    # complexity
    - cyclop
    - gocognit
    - funlen
    - nestif
    - maintidx
    # style that prevents bugs
    - revive
    - gocritic
    - unparam
    - unconvert
    - usestdlibvars
    - intrange
    - perfsprint
    - prealloc
    - sloglint
    - predeclared
    - exptostd
    # tests
    - testifylint
    - thelper
    - tparallel
    - paralleltest
    - usetesting
  settings:
    cyclop: { max-complexity: 12 }
    gocognit: { min-complexity: 15 }
    funlen: { lines: 70, statements: 40 }
    nestif: { min-complexity: 4 }
    exhaustive: { default-signifies-exhaustive: false }
    sloglint: { attr-only: true, context: all, static-msg: true, no-global: all }
    interfacebloat: { max: 5 }
    wrapcheck:
      ignore-package-globs: ["github.com/monaco/backend/internal/*"]
    forbidigo:
      analyze-types: true
      forbid:
        - pattern: '^(fmt\.Print.*|print|println)$'
          msg: use slog
        - pattern: '^context\.(Background|TODO)$'
          msg: only main, tests and bus roots create root contexts
        - pattern: '^time\.(Now|Since|Until)$'
          msg: inject platform/clock.Clock
        - pattern: '^os\.Getenv$'
          msg: read config through platform/config at boot
        - pattern: '^float(32|64)$'
          msg: money is integer base units (platform/money)
        - pattern: '^log\.'
          msg: use slog
        - pattern: '^(pgxpool\.New|pgx\.Connect|sql\.Open|nats\.Connect)$'
          msg: platform/db, platform/bus and testkit own connections
        - pattern: '^errors\.New$'
          msg: every error carries a Code; use errs.New
        - pattern: '^slog\.(Error|Warn)$'
          msg: modules return errors; boundaries log them once
        - pattern: '^uuid\.(MustParse|Must)$'
          msg: fixtures take ids from the injected generator
    depguard:
      rules:
        domain:
          files: ["**/domain/**"]
          deny:
            - { pkg: "github.com/jackc/pgx", desc: "domain is pure" }
            - { pkg: "github.com/nats-io", desc: "domain is pure" }
            - { pkg: "net/http", desc: "domain is pure" }
            - { pkg: "github.com/monaco/backend/internal/platform/db", desc: "domain is pure" }
        app:
          files: ["**/app/**"]
          deny:
            - { pkg: "github.com/jackc/pgx", desc: "declare a port" }
            - { pkg: "github.com/nats-io", desc: "emit via Tx.Events" }
            - { pkg: "net/http", desc: "use cases are transport-free" }
  exclusions:
    rules:
      - path: _test\.go
        linters: [funlen, err113, wrapcheck]
      - path: main_test\.go
        linters: [gochecknoglobals]
      - path: cmd/
        linters: [forbidigo]
      - path: internal/platform/(db|bus|httpx|observability)/
        linters: [forbidigo]
      - path: internal/errs/
        linters: [forbidigo]
      - path: internal/testkit/
        linters: [forbidigo]
formatters:
  enable: [gofumpt, gci, golines]
  settings:
    gci: { sections: [standard, default, localmodule] }
    golines: { max-len: 120 }
```

No `godox` or `nolintlint`: the comment ban below covers both, and inline `//nolint` is banned. A lint exception is an `exclusions.rules` entry in this file with a path and a reason, so every exception sits in one reviewed place. `revive`'s `exported` rule stays off and the v2 `comments` exclusion preset stays on, so no linter asks for the doc comments the ban removes.

`forbidigo` narrows by file only through `exclusions`, so the domain-only float and clock bans need path rules; expect to tune that on day one. Cross-module import bans (module A importing module B) need one `depguard` rule per module, generated by a `scripts/gen-depguard.go` so a new module can't forget it. Tuning numbers (`cyclop` 12, `funlen` 70) are starting points; raise per-package with a reason, never globally.

Also in CI:

| Check | Tool |
| --- | --- |
| Vulnerabilities | `govulncheck ./...` |
| Generated code is fresh | `sqlc diff`, `oapi-codegen` + `git diff --exit-code` |
| Migrations lint | `atlas migrate lint` on new SQL (no table rewrites, no `NOT NULL` without default on large tables, no destructive change without a `-- atlas:nolint destructive` line the reviewer sees) |
| Flows | `monacoctl flows check` (see [Flows](#flows)) |
| OpenAPI lint and breaking changes | `vacuum lint`, `oasdiff breaking` against `main` |
| No comments | `monacoctl lint comments` (see [No comments](#no-comments)) |
| Changelog touched | fail if `apps/backend/**` changed and `CHANGELOG.md` didn't, unless PR has `no-changelog` label |
| Race + leaks | `go test -race`, `goleak` |
| Coverage and mutation | see [Testing](#testing) CI gates |

## Testing

**Target: 100% statement coverage, and mutation testing alongside it so the 100% means something.** Agents write code many times faster than people, so spend the saved time torturing the code: unit, acceptance, QA, property, fuzz, mutation, performance and jitter tests. The constraint is that `just test backend` stays fast and deterministic, so development speed doesn't drop.

### What counts toward 100%

- One merged profile. Unit and integration tests use `-coverpkg=./...`. E2E and QA runs build the real binaries with `go build -cover` and write to `GOCOVERDIR`. `go tool covdata merge` combines everything, so `cmd/*/main.go` wiring and HTTP adapters are covered by the tests that really exercise them.
- Excluded paths are fixed and listed in one file (`coverage.exclude`): generated code (`*.gen.go`, sqlc output, oapi-codegen output) and `internal/testkit`. Nothing else.
- No ignore pragmas. Code a test can't reach gets deleted or made unrepresentable by a type change. An error branch that only fires on infra failure gets reached through a fake port that returns the error.
- Go has no branch coverage, and statement coverage can be gamed. Mutation testing closes that gap: a surviving mutant means a line ran but nothing checked its result.

### Test types

| Type | Scope | Tooling | Runs in |
| --- | --- | --- | --- |
| Unit | `domain` pure functions, state machines, share math | table tests, `t.Parallel`, no I/O | `just test backend` |
| Property | invariants over random inputs: shares never mint value, rounding favours the pot, entries sum to zero per asset, both headers on a `transfer_id` always share a status | `pgregory.net/rapid` | `just test backend` (100 cases), nightly (100k) |
| Model-based (stateful property) | random action sequences (fund, trade, cash out, crash, redeliver) run against the real app layer and a tiny in-memory model; states compared after each step | `rapid.StateMachine`-style actions | `just test backend` (short), nightly (long) |
| Fuzz | every parser at a boundary: HTTP bodies, event payloads, Jupiter/Privy/Helius responses, config | native `go test -fuzz`, corpus committed | seeds in `just test backend`; 10 min per target nightly |
| App / integration | each command and query against real Postgres | template-DB clone per test (below), fake ports, fake clock | `just test backend` |
| Adapter | sqlc repos, HTTP handlers, consumers, relay | real Postgres + embedded NATS, `httptest` fakes replaying recorded fixtures | `just test backend` |
| Bus semantics | redelivery, dedupe, term, dead letter, `InProgress`, out-of-order | embedded NATS plus the chaos dispatcher (below) | `just test backend` |
| Contract | server responses match `openapi.yaml`; event payloads match golden JSON | `kin-openapi` validator middleware in every HTTP test; golden files with `-update` | `just test backend` |
| Acceptance | one scenario per Flows row, in business language | Go scenario DSL (`scenario.New(t).Given(...).When(...).Then(...)`) over HTTP against the in-process app | `just test backend` |
| E2E | same scenarios against real `api` + `worker` binaries | compose: PG + NATS + fake externals; `-cover` binaries | PR CI |
| QA | agent-driven: `verify-backend flow <n>` with evidence in the PR | `verify-backend` skill CLI | every backend PR |
| Crash-point | panic at named points (after sign, after `/execute`, before commit, after publish), restart, assert convergence | `faultpoint` hooks compiled in under a build tag | PR CI |
| Jitter / concurrency | pools, pipelines, relay, consumers under random delays and interleavings | `testing/synctest` + seeded delay injection + `-race` | `just test backend` (fixed seeds), nightly (seed sweep) |
| Performance (deterministic) | allocations per op on hot paths; query count per request | `testing.AllocsPerRun`, a query-counting pgx tracer | `just test backend` |
| Performance (timing) | benchmarks compared against `main`; load on the full stack | `b.Loop` + `benchstat`; `vegeta` against the e2e stack | nightly, and on PRs labelled `perf` |
| Mutation | all non-generated packages | `gremlins`, diff-scoped on PRs, full nightly | PR CI (changed packages), nightly |
| Leak | every package | `goleak.VerifyTestMain` | always |

### Keeping it fast

Budget: `just test backend` under 60 s on a laptop, and the required PR checks under 6 min wall clock. The budget is a CI gate, not a hope: `testkit` records each package's wall time from `go test -json`, `monacoctl test-report` prints the ten slowest tests, and a package over 10 s or a run over 60 s fails. A test that breaks the budget gets fixed or moved to nightly, never skipped.

Where the 60 s goes. The clone row is measured; the rest are estimates until Rollout step 1 measures them on the scaffold and sets the gate from real numbers:

| Cost | Estimate | What keeps it there |
| --- | --- | --- |
| Compile and link about 15 test binaries with `pgx` and embedded `nats-server` | 15 to 25 s cold, near zero warm | Go's build cache. Never `-count=1` locally; `just test backend` lets the test cache skip unchanged packages, and `-shuffle=on` still reseeds. |
| Postgres | 1 to 2 s | One tuned Postgres already running from `just run` (`fsync=off`, `synchronous_commit=off`, `full_page_writes=off`, data on tmpfs). Migrations run once into `monaco_tmpl`. `just test backend` starts it only if it is down. |
| Template clone per test | Measured 2026-09-27 on the Compose Postgres 16 (Apple silicon, 26-table 10 MB template): create 27 ms, drop 19 ms, 13 ms per test amortized across 8 sessions with the tuned settings. A `TRUNCATE` of the same 26 tables is 31 ms, so a clone is cheaper than the alternative. With default `fsync`, drop is 169 ms, which is why the settings matter. 500 tests cost about 7 s of database time spread across the parallel run. CI gets its own number from `monacoctl bench db` in Rollout step 1. | `CREATE DATABASE t_<id> TEMPLATE monaco_tmpl` via `peterldowney/pgtestdb`, drop in `t.Cleanup` off the critical path. Per-test rollback isn't an option because the Unit of Work commits and the relay reads committed rows. |
| Embedded NATS | Measured 2026-09-27 (M2, `GOMAXPROCS=4`, `nice 19`, nats-server v2.14.5): server start 27 ms, stop under 1 ms, 1.7 MiB heap. 32 parallel tests, each creating its own stream and consumer and moving 50 messages, took 114 ms of test time (3.6 ms per test amortized, 12.7 ms per test body). Plus about 0.2 s of binary load per NATS-importing package on a cold test cache. `-race` roughly triples the test phase. | One `nats-server` per package in `TestMain`, one stream per test. |
| Property, model-based, fuzz seeds, jitter | Measured 2026-09-27 on pure domain code: 3 invariants × 100 cases, a model test of 20 steps × 100 runs, 60 fuzz seeds and fixed-seed synctest jitter took 10 to 30 ms in-process together. The package costs about 0.5 s, nearly all of it the fixed per-binary start, and about 1.5 s with `-race`. A model test against the real app layer pays one `testkit.Reset` (31 ms) per run, so 100 runs is about 3 s: those run with fewer runs under `-short`. | `just test backend` passes `-rapid.checks=500 -rapid.steps=40`, because rapid divides both by its own `-short` factor (5 and 2) to land on 100 cases and about 20 attempted steps. Corpus seeds only. Fixed jitter seeds. Nightly runs 100,000 cases and `-fuzz` per target. |
| Acceptance, one per `flows.tsv` outcome, in-process | 5 to 10 s | HTTP against the in-process app, no binaries, no compose. |

Not in `just test backend`: E2E (real binaries, compose), mutation, crash-point, timing benchmarks. Those are PR CI or nightly.

PR CI is three required jobs, each with its own budget: lint plus unit plus integration (under 3 min, sharded by package), E2E (under 4 min), and mutation on changed packages (under 10 min, or the PR is too big and gets split). Nightly runs everything unbounded.

- **No sleeps.** `time.Sleep` in tests is banned by `forbidigo`. Time-dependent code takes the injected `clock.Clock`, and goroutine timing uses `testing/synctest`, where virtual time advances instantly once every goroutine is blocked. Measured: the full 1 s, 5 s, 30 s, 2 min, 10 min backoff schedule (12m36s of fake time) runs in 19 to 40 µs, and an 8-worker pool covering 12.5 s of fake time in 1 to 4 ms.
- **Bus timers are real time.** synctest and `clock.Clock` cannot speed up timers inside `nats-server`. Measured: redelivery takes exactly `AckWait` plus 1.5 ms, the max-deliveries advisory takes `MaxDeliver × AckWait`, and proving that `Term` stops redelivery costs the whole wait window. So bus-semantics tests run with `AckWait` 100 ms from `testkit.NATS`, one sample each, in parallel. That is about 0.5 s of mostly idle wall time per package. `testkit.NATS` rejects an `AckWait` over 250 ms.
- **Everything parallel.** `t.Parallel` is required (`paralleltest` and `tparallel` lint). That only works because every test owns its database and its stream, below.
- **Run what changed.** On PRs, mutation and long property runs cover only packages affected by the diff (`go list -deps` against changed files).

### Isolating test state

Isolation is by database, not by row. A test never shares a table with another test, so there is nothing to scope, prefix, or clean up.

- `testkit.DB(t)` is the only way a test gets Postgres. It clones the template, returns a pool bound to `t`, registers the drop in `t.Cleanup`, and names the database after `t.Name()` plus a random suffix. Two tests cannot see each other's rows because they are in different databases.
- `testkit.NATS(t)` is the same for the bus: one stream and one consumer set per test, named after `t.Name()`, deleted in `t.Cleanup`.
- Fixtures take `t` and build into that test's database: `testkit.NewCabal(t, db, testkit.WithMembers(3))`. IDs come from the injected UUIDv7 generator, never literals.
- A property or model-based test runs every iteration in the same database, and calls `testkit.Reset(t, db)` (`TRUNCATE ... RESTART IDENTITY`) between iterations, because 100 clones per test would eat the budget.

Lint that makes the shared-state leak a compile-time failure:

| Leak | Lint |
| --- | --- |
| A test opens its own connection to a shared database | `forbidigo` bans `pgxpool\.New`, `pgx\.Connect`, `sql\.Open`, `nats\.Connect` outside `platform/db`, `platform/bus`, `testkit`. |
| A package-level `*pgxpool.Pool`, `*testkit.DB` or `*nats.Conn` shared across tests | `gochecknoglobals` stays on for `_test.go`. `TestMain` may hold only the embedded server handle, allowed by an `exclusions.rules` entry scoped to `main_test.go`. |
| A test reads `DATABASE_URL` and hits whatever is there | `forbidigo` already bans `os.Getenv`. `testkit` reads the test URL from `platform/config` once in `TestMain`. |
| Hard-coded IDs that collide across tests | `forbidigo` bans `uuid\.MustParse` and `uuid\.Must` in `_test.go`. |
| A test that is not parallel and hides an order dependency | `paralleltest`, `tparallel`, `-shuffle=on`. |
| Two tests with the same name in a package, so two databases get the same name | `testkit.DB` panics on a duplicate `t.Name()` within a run. |
| Leftover state after a failure | `t.Cleanup` drops the database even on failure. `testkit` `TestMain` drops any `t_*` database older than one hour at start, so a killed run cannot fill the disk. |

### Keeping it deterministic

- **Every nondeterminism source is injected:** clock, UUID generator, random source and Jupiter/Privy responses. `forbidigo` already bans `time.Now`. Add `math/rand` globals and `uuid.New` outside `platform/`.
- **Seeds are printed and replayable.** `go test -shuffle=on` catches order dependence. Rapid, fuzz, jitter and the chaos dispatcher each log their seed on failure. For rapid the flag is `-rapid.seed=<n>`; measured with shrinking off, it reproduced the exact raw counterexample 3 of 3 times, also under `-short -race -shuffle=on`. Rapid saves each failure to `testdata/rapid/<Test>/*.fail` and replays it first on the next run. Those files are committed as regression cases in the fix PR. CI runs with `-rapid.nofailfile` so a CI failure never writes to the tree.
- **Chaos dispatcher.** A test-only `bus.Dispatch` wrapper that, driven by a seed, duplicates messages, reorders them within a window, delays acks past `AckWait` and redelivers after a simulated crash. Every consumer's tests run through it. This makes the "handlers never assume order, redelivery is harmless" rule from [event-bus.md](event-bus.md) something a test checks, instead of something the doc claims.
- **No flake quarantine.** A flake is a bug with a seed. It gets fixed or reverted, never retried into green.

### Test code quality

- **Builders for fixtures** (`testkit.NewCabal(t, testkit.WithMembers(3), testkit.WithPot(100_00))`), the same functional-options pattern as production code. No shared mutable fixtures.
- **Assertions check effects:** ledger balances, `events` rows, HTTP status and body, published messages. They never check that a mock method was called. `testifylint` and a review rule back this up.
- **Every bug fix lands with the test that failed first**, at the lowest layer that can catch it, plus a lint rule when the bug class is mechanical.

### CI gates

| Gate | Threshold |
| --- | --- |
| Merged statement coverage | 100% of non-excluded statements |
| Mutation efficacy on changed packages | 100% of mutants killed, or listed in `mutants.allow` with a reason (equivalent mutants only; reviewed like a lint exclusion) |
| Allocations and query counts | no increase over the recorded baseline without a baseline update in the same PR |
| `-race`, `goleak`, `-shuffle=on` | zero findings |
| Nightly benchmarks | alert on a >10% `benchstat` regression with p < 0.05 |

## Flows

Each flow is a command, the events it emits, the consumers that react, and every outcome it can end in. The table below is the target state of the rewrite, not the current code. Most of rows 1 to 17 exist in the old backend in some form; 18 to 27 are partly new. Nothing here is "future reference": every row becomes a test before its module's rollout step closes.

### `flows.tsv` is the source of truth

The table below is a render. The file is `apps/backend/flows.tsv`, one line per flow, tab-separated, and it moves with the code. TSV over YAML because a flow is one line, the diff shows exactly which cell changed, `cut` and `awk` read it, and there is no indentation for an agent to get wrong. List cells use `;`.

| Column | Contents | Checked by |
| --- | --- | --- |
| `id` | `07` | unique |
| `flow` | `Fund cabal` | |
| `module` | `treasury` | directory exists |
| `trigger` | `POST /v1/cabals/{id}/fund` or `consumer:proposal.passed` or `poller:deposits` | route in `openapi.yaml`, subject in registry, or poller registered |
| `command` | `FundCabal` | Go type exists in `module/app` |
| `events` | `cabal.fund_submitted;cabal.funded` | each in the `events` registry |
| `consumers` | `treasury.positions;ranking;feed;notify;referrals` | each a registered durable |
| `outcomes` | `ok;InsufficientFunds;CabalPaused;PrivyUnavailable;crash:after-sign;crash:before-commit` | each `Code` in the `errs` table; each crash point a registered `faultpoint` |
| `status` | `planned`, `built`, `verified` | `built` needs every outcome test present; `verified` needs evidence (below) |
| `doc` | `docs/architecture/deposits-withdrawals.md#fund` | file and anchor exist |

`monacoctl flows check` runs in CI and fails on any column's check. It also reads `go test -json` from the run and requires, for each flow with `status` ≥ `built`, a passing test named `TestFlow<id>_<Command>_<Outcome>` per outcome. A flow row with no test is a red build, not a backlog item. The check is what makes the file a map of the system instead of a wish list.

`status = verified` means `verify-backend flow <id>` ran against real binaries and wrote `test/evidence/<id>.json` (responses, `events` rows, acks, ledger balances, and the log lines the flow must emit). The E2E job produces that file on every PR that touches the flow's module, so `verified` cannot go stale silently: the check fails a `verified` flow whose evidence file is older than the module's newest commit.

Generated from the TSV, checked fresh in CI: the table below (`monacoctl docs flows`), the `verify-backend` feature map, and the acceptance test skeletons (`just gen flow <id>` writes one failing test per outcome).

Adding a flow is one row plus the tests it names. Deleting a flow deletes the row, and the check fails until its tests go too.

| # | Flow | Command / trigger | Events | Consumers |
| --- | --- | --- | --- | --- |
| 1 | Sign in (SMS / email OTP) | `POST /v1/auth/session` with Privy token | `user.created` (first time), `user.auth_state_changed` | analytics, referrals (attribute), social (contact matches) |
| 2 | Create cabal | `CreateCabal` | `cabal.created` | feed, analytics |
| 3 | Join open cabal / request / invite / approve | `JoinCabal`, `RequestAccess`, `InviteMember`, `DecideAccess` | `cabal.member_joined`, `cabal.access_requested`, `cabal.access_decided` | notify, feed, ranking |
| 4 | Leave cabal | `LeaveCabal` (guarded: no shares, not last with money, not creator with members) | `cabal.member_left` | feed, ranking |
| 5 | Crypto deposit | Deposit poller sees USDC in member wallet | `deposit.credited` | notify, referrals (first deposit), analytics |
| 6 | Card deposit | `CreateOnrampSession`, page PATCHes status | `onramp.status_changed` then flow 5 | analytics |
| 7 | Fund cabal | `FundCabal` → Privy transfer member→treasury → confirm → mint shares at live price | `cabal.fund_submitted`, `cabal.funded` | treasury positions, ranking, feed, notify, referrals |
| 8 | Direct transfer to treasury | Treasury watcher | `cabal.external_deposit_detected`, `cabal.external_deposit_bounced` | trading (pause), notify, admin |
| 9 | Propose trade | `ProposeTrade` (advisory route + pot check via market) | `proposal.created` | feed, notify |
| 10 | Vote / tally | `CastVote`; expiry job | `proposal.passed` / `.failed` / `.expired` | trading, feed, notify |
| 11 | Execute trade | `trade-engine` consumer on `proposal.passed` | `trade.blocked` / `trade.submitted` / `trade.confirmed` / `trade.failed` | governance (executed), treasury ledger, ranking, feed, notify |
| 12 | Retry failed trade | `RetryTrade` | same as 11 | same |
| 13 | Withdraw / void proposal | `WithdrawProposal`, admin `VoidProposal` | `proposal.withdrawn`, `.voided` | feed, notify |
| 14 | Cash out | `CashOut` → burn shares → sell if short → pay USDC to member wallet | `cashout.started`, `cashout.completed` / `.partial` / `.failed` | ranking, feed, notify |
| 15 | Withdraw to address | `Withdraw` | `withdrawal.submitted`, `.confirmed`, `.failed` | notify, analytics |
| 16 | Agent lifecycle | Proposal kinds add / pause / resume / remove agent | `agent.enabled`, `.paused`, `.removed`, `agent.key_revealed` | agents, notify, feed |
| 17 | Agent trade | `SubmitAgentIntent` (key auth, budget check) | `agent.intent_created` → same engine as 11 | trading, same as 11 |
| 18 | Prices | Poller tick (fan-out over providers) | `price.updated` (core NATS only, not stored as event) | ranking, live SSE |
| 19 | Valuation + leaderboards | Every minute, and on `trade.confirmed`, `cabal.funded`, `cashout.completed` | `ranking.snapshot_written` | live SSE |
| 20 | Follow / unfollow | `Follow`, `Unfollow` | `follow.created`, `.removed` | notify, feed ranking |
| 21 | Feed + comments | `CreateComment` | `comment.created` | notify, live SSE |
| 22 | Chat | Ably for delivery; backend issues token and persists | `chat.message_posted` | notify (mentions) |
| 23 | Profile edit | `UpdateProfile` | `user.profile_updated` | ranking (names), feed |
| 24 | Notifications | Consumers write `notifications` row, then send | `notification.sent` | none |
| 25 | Referrals | Click, sign-up, first deposit | `referral.qualified` | notify, analytics |
| 26 | Admin | Any admin command | `admin.action` | notify, audit |
| 27 | Dead letters | Advisory subscriber | none | admin queue; `monacoctl deadletter retry` |

## Thin client

The iOS app renders; the server decides.

- One `GET` per screen returning everything that screen shows, already computed: pot value, share price, your stake, returns, display names, human-readable asset names, formatted-ready integer amounts with decimals. No math in Swift beyond formatting.
- Every mutating call takes an `Idempotency-Key` header; the server stores the response and replays it.
- Server-sent events (`/v1/stream`) push "this changed, re-fetch" hints from core NATS. The app never polls on a timer except as SSE-reconnect fallback. Fan-out is in-process (see [SSE hub](#sse-hub)), never one NATS subscription per phone.
- Errors carry a stable `code` and user-facing `message`. The app shows `message` in a toast; it switches on `code` only for flows that branch.
- Swift client is generated from `api/openapi.yaml`. A route change that breaks the client fails `oasdiff breaking` in CI.

## NATS hosting and budget

Hosted NATS is Synadia Cloud. Self-hosting JetStream on a PaaS means one node on one disk; Synadia runs the cluster. Start on the free Personal plan; move to Starter ($49/mo) when the `EVENTS` stream needs replication for real money. Limits are enforced by the account JWT, not billed, so going over is an error the outbox absorbs (events wait in Postgres), never a surprise invoice. The one billed item is network egress beyond the pool.

Personal plan limits, and what the design spends:

| Limit | Plan | Monaco | How it stays there |
| --- | --- | --- | --- |
| Accounts | 2 | 2 | `monaco-prod`, `monaco-staging`. Local dev and tests use embedded or compose NATS, never Synadia. |
| Connections | 10 | 2 steady, ~6 in a deploy | One `nats.Connect` per process, in `platform/bus` only (the `forbidigo` pattern in the lint config). `nats.Name("monaco-<api|worker>")` so strays show in the dashboard. Rolling deploys briefly double the count. |
| Subscriptions per connection | 100 | api: 1. worker: ~15 | SSE hub below; worker has one pull consumer per module plus `price.>` and the delivery-failure advisory. |
| Streams | 10 | 2 | `EVENTS` and `DEADLETTER`, declared in code and applied by `monacoctl bus apply` in the pre-deploy step. api and worker never create streams. No KV buckets or object stores: each is a stream underneath; that state lives in Postgres. |
| Storage | 5 GiB | 2.5 GiB cap | `MaxBytes` 2 GiB on `EVENTS`, 512 MiB on `DEADLETTER`. `MaxAge` 7 days on `EVENTS`: Postgres `events` is the source of truth, the stream is transport. |
| Network data | 10 GiB | ~4 GiB/month | Price ticks are one batched `price.tick` message per poll carrying every asset, at 10 s or slower. Per-asset messages at 5 s would be ~20 GiB/month on their own. Domain events are ~1 GiB. |
| HA streams | 0 | 0 | `Replicas: 1`. On Starter, `EVENTS` goes to `Replicas: 3`; nothing else changes. |

Stream and consumer shape:

```go
var streams = []jetstream.StreamConfig{
	{
		Name:       "EVENTS",
		Subjects:   events.Subjects(),
		Storage:    jetstream.FileStorage,
		Replicas:   1,
		MaxBytes:   2 << 30,
		MaxAge:     7 * 24 * time.Hour,
		Discard:    jetstream.DiscardNew,
		Duplicates: 2 * time.Minute,
	},
	{
		Name:     "DEADLETTER",
		Subjects: []string{"deadletter.>"},
		Storage:  jetstream.FileStorage,
		Replicas: 1,
		MaxBytes: 512 << 20,
		MaxAge:   30 * 24 * time.Hour,
	},
}
```

- `Subjects` comes from the `events` registry, so a new event can't add a stream.
- `DiscardNew`, never `DiscardOld`: a full stream rejects the relay's publish and the outbox retries; `DiscardOld` would silently drop events no consumer has read.
- `Duplicates` only works if `bus.Publish` sets `Nats-Msg-Id` to `events.id` on every publish. The relay does; the `nats-consumer` skill says so.
- One durable pull consumer per module on `EVENTS` with `FilterSubjects`, `AckExplicit`, `MaxDeliver` 10, `MaxAckPending` 64 (the cross-process concurrency bound from the concurrency rules). A module gets a consumer, never a stream.
- Boot fails if `nats.Connect` fails. No `RetryOnFailedConnect`: an instance that passes health checks with a dead bus backs the relay up silently.
- The worker exports `js.AccountInfo` every minute as OTel gauges (storage used vs limit, stream and consumer counts); alert at 80%. Connection count and egress are not in `AccountInfo`; those are read in the Synadia dashboard.

What a limit hit looks like:

| Limit hit | Symptom | Damage |
| --- | --- | --- |
| Connections | new process fails boot: `maximum account active connections exceeded` | new instance fails health check; old keeps serving |
| Storage | relay publish rejected: `maximum bytes exceeded` | events wait in the outbox; consumers catch up when space frees |
| Subscriptions | one `Subscribe` call errors | one feature breaks, loudly at boot |
| Network | billed per GiB | money, not an outage |

Not verified against the pricing page: the consumer cap (check `nats account info` after signup; the plan uses ~10), and whether the 10 GiB network figure is per month and counts both directions. Both assumed.

### SSE hub

Two things get called "channels" and they must stay separate. NATS subjects are how the backend talks to itself; an SSE connection is how the api talks to one phone. The wildcard changes only the second.

The naive shape, one NATS subscription per connected phone, hits 100 subscriptions at 100 phones and delivers the same cabal hint once per member to the same process. So the api holds one subscription, `hint.>`, and routes in memory:

```
NATS ──"hint.cabal.42.updated"──▶ api
                                    │
                                    ▼
                          hub: who is listening for cabal 42?
                                    │
                        ┌───────────┼───────────┐
                        ▼           ▼           ▼
                     phone A     phone B     phone C
```

- The hub is a map from key (`cabal:<id>`, `user:<id>`) to SSE writers. On connect, the api registers the phone under its user id and the cabals it is a member of. On a hint, the hub parses the subject, looks up the key, writes to those phones and no others.
- Correctness moved from NATS subject permissions to one function, `hub.Register`. It is an authz check and gets a test: a phone in cabal 7 does not receive cabal 42's hint. Membership changes re-register.
- NATS subscriptions scale with api processes, not phones: one api, one subscription, at any number of connected clients.
- Hints are core NATS and droppable (concurrency rule 5). A dropped hint costs one missed re-fetch until the next one; the SSE-reconnect fallback covers the rest.

## Deploy and observability

Render runs the two binaries; Synadia runs NATS; Grafana Cloud receives OTLP. Nothing is self-hosted.

| Piece | Where | Why |
| --- | --- | --- |
| `api` | Render web service, Docker runtime, `dockerCommand: dotenvx run -f .env.production -- /app/api`, `healthCheckPath: /healthz` | Long-lived SSE connections need a host without an aggressive idle timeout. |
| `worker` | Render background worker, same image, `maxShutdownDelaySeconds: 180` | Grace period longer than a 2-minute Jupiter `/execute`. |
| Migrations and stream config | Render `preDeployCommand`: `monacoctl migrate apply && monacoctl bus apply` | Runs once per deploy before the new instances start. Nothing migrates at boot. |
| Postgres | Render managed Postgres with point-in-time recovery, or Supabase Postgres (already used for photo storage) | The ledger. Either host; not a container on a volume. |
| NATS | Synadia Cloud (above) | |
| Secrets | One host env var, `DOTENV_PRIVATE_KEY_PRODUCTION`; everything else in encrypted `.env.production` | Same dotenvx flow as local. |
| Traces, metrics, logs | OTel SDK in `platform/observability`, OTLP over HTTP straight to Grafana Cloud (Tempo, Mimir, Loki), no collector. `otelslog` bridges slog. `OTEL_SERVICE_NAME` is `monaco-api` or `monaco-worker`. | One trace from HTTP request through the NATS header to the consumer (context rule 9). Switching hosts changes one endpoint. |
| Error reports | Sentry stays for panics and `KindInternal`, fed from the same boundary that logs them | Cheaper to keep than to rebuild alert routing. |
| Alerts | Grafana alert rules on `poller_errors_total`, dead-letter count, relayer SOL balance, `js.AccountInfo` usage, p95 per route | Replaces the old webhook. |

Rules the deploy exposes:

1. **Pollers run on one worker.** Consumers and the relay scale across replicas (JetStream and `SKIP LOCKED` share the work). Pollers do not: two replicas would poll deposits twice. Each poller takes a Postgres advisory lock named after the poller at tick start and skips the tick if it fails to get it. Until that lands, worker replicas stay at 1.
2. **Shutdown order on SIGTERM**: stop fetching, send `InProgress` for in-flight long work, finish or nak what is running, drain the NATS connection, close the pool, exit. Anything cut off is redelivered and deduped through `event_deliveries`.
3. **Worker health.** The worker serves `/healthz` on an internal port, and the health answer includes NATS connected, pool reachable, last poller tick age.
4. **Relayer floor.** Boot refuses to start when the relayer holds 0.001 SOL or less. The Grafana alert fires at 0.05 SOL so the deploy never hits the boot check.

Fly.io would be the choice only for self-hosted NATS or multi-region. Railway's Postgres is a container on a volume and does not fit the ledger.

## Agent-friendly codebase

The rewrite is a chance to build a codebase agents can extend without a human reading every line. Source: poteto's talk on trust ladders (transcript supplied 2026-09-26). The ladder, strongest first, and where each Monaco rule sits:

| Rung | Mechanism | Monaco examples |
| --- | --- | --- |
| 1. Codebase makes the mistake impossible | Types, package layout, generated code | Branded IDs and `Micros`; `internal/` + per-module packages; `UnitOfWork` is the only way to get a `Tx`; OpenAPI and sqlc generate the types; state machines as tables |
| 2. Static analysis | Lint, compiler, CI | `depguard` rings and module walls, `forbidigo` bans, `exhaustive`, freshness checks, changelog check |
| 3. Rules and skills | `AGENTS.md`, `.claude/skills`, review bot | Four skills below; `AGENTS.md` under 60 lines |
| 4. Style guide | Human review | Nothing should live only here. A review comment made twice becomes a rung 1 or 2 change |

Standing rules that follow:

- **One paved path per job.** One way to write a command, a consumer, a query, a provider, a migration. Generators emit it. A second way is a lint failure, not a style debate.
- **Every correction becomes structure.** When a human or reviewer corrects an agent, the fix PR also adds the lint rule, type change or generator change that stops it recurring. A rule can land before the cleanup; it stops the spread while the old sites get fixed.
- **Workarounds spread, so none land.** An agent copies whatever it sees, and a comment is where a workaround explains itself. Go code has no comments; see [No comments](#no-comments).
- **Gardener.** A scheduled agent (weekly) runs `golangci-lint` with stricter candidate settings, `deadcode`, `gremlins` and the generators' drift check, and opens small PRs that delete dead code or propose a new rule. One human owns merging them.

### No comments

Comments written by agents are low value and often wrong: they narrate the code, go stale, or justify a workaround that the next agent copies. So hand-written Go has none, tests included.

Allowed, because a tool reads them:

| Comment | Reader |
| --- | --- |
| `//go:build`, `//go:generate`, `//go:embed` and other `//go:` directives | Go toolchain |
| Anything in a file headed `// Code generated ... DO NOT EDIT.` | sqlc, oapi-codegen output; skipped via `ast.IsGenerated` |
| `-- name: GetUser :one` in `queries/*.sql` | sqlc; `.sql` is outside the check |
| `description:` in `api/openapi.yaml` | The HTTP contract and the generated Swift client; YAML is outside the check |

Banned: everything else. That includes doc comments on exported identifiers (everything is under `internal/`, so nothing reads godoc outside the module), package comments, `// TODO`, and inline `//nolint`.

Where the thought goes instead:

| Urge | Home |
| --- | --- |
| What this does | A better name or a smaller function |
| An invariant | A type (`Micros`, branded IDs) or a validating constructor |
| An edge case | A test whose name states the case |
| TODO or known gap | A GitHub issue |
| A design decision | A doc under `docs/architecture/` with a Log entry |
| Why this change | The commit message or PR body |

Enforcement:

1. **Checker.** `cmd/monacoctl lint comments` parses every non-generated `.go` file with `parser.ParseComments`, walks `file.Comments`, allows only the `//go:` prefix and prints `file:line` for the rest. Runs in the pre-commit hook and CI.
2. **Agent hook.** A Claude Code `PostToolUse` hook on `Edit|Write` for `apps/backend/**/*.go` runs the checker on the touched file and blocks with the offending lines, so the agent removes the comment in the same turn instead of in CI. Cursor gets the same check through its hooks.
3. **`AGENTS.md`.** One line: "No comments in Go. If you want one, you need a better name, a type, a test, or an issue."

Cost: IDE hover shows no docs. With `internal/`-only code and descriptive names, that is small.

### Verification skill

Agents need to prove backend work runs, not only that it compiles. `.claude/skills/verify-backend/` holds two parts:

1. **A CLI inside the skill** (`verify-backend` built from `cmd/monacoctl verify`). It boots Postgres, NATS and fake externals, starts `api` and `worker`, runs one named flow or all of them through HTTP, and prints evidence: HTTP responses, `events` rows written, consumer acks, ledger balances per asset, dead letters, p95 handler latency. Same command every session, so agents don't write throwaway scripts. Crash-point mode (`--crash-at after-execute`) restarts the worker mid-flow and checks convergence.
2. **A feature map** (`feature-map.md` in the skill) generated from the Flows table: for each flow, the route or trigger, the command, events, consumers, tables touched, and the `verify-backend flow <n>` line that exercises it. An agent handed "my fund didn't show up" looks up flow 7 and knows which events and tables to inspect. A CI check regenerates it and fails if stale, so it can't drift from the registry.

## Docs, changelog, agents

- **MkDocs Material on GitHub Pages.** `mkdocs.yml` at repo root with `docs_dir: docs`. The existing `docs/` tree (index, product, architecture decision log, how-to) is the nav. A workflow on push to `main` runs `mkdocs build --strict` (broken links fail) and deploys with `actions/deploy-pages`. PRs run `mkdocs build --strict` only. API reference renders `api/openapi.yaml` via `mkdocs-render-swagger` or a Redoc page. Event catalog page generated from the `events` registry by `go run ./cmd/monacoctl docs events > docs/reference/events.md`, checked fresh in CI.
- **CHANGELOG.md.** [Keep a Changelog](https://keepachangelog.com) format, `## [Unreleased]` at top, sections Added / Changed / Fixed / Removed / Security. One line per user- or operator-visible change, written for a person, not a commit log. CI check above enforces it. Release cuts move Unreleased under a dated version.
- **Agent rules live in structure first.** `apps/backend/AGENTS.md` stays under 60 lines: the three rings, "cross-module = event", "money = integer + UnitOfWork", "run `just test backend` and `golangci-lint run` before done", "no comments in Go", plus a pointer to the skills. Everything else is a lint.
- **Generators over instructions.** `just gen module <name>`, `just gen command <module> <Name>`, `just gen consumer <module> <name>`, `just gen provider <name>` emit the correct files, test skeletons, depguard rule, registry entry and CHANGELOG stub. Agents copy what exists; make the first copy right.
- **Skills** in `.claude/skills/` (and mirrored for Cursor):
  - `go-backend-module`: adding a command/query/consumer end to end, including test layers and flow table update.
  - `go-concurrency`: when to use `Pool`, `Stage`, `FanOut`, errgroup; the eight concurrency rules; `goleak` and `-race` required. References the Mario Carrión fan-in/fan-out article for the base pattern and the helpers for the house version.
  - `money-change`: checklist for anything touching ledgers, shares, swaps: property test, crash-point test, guarded update, event in same tx.
  - `nats-consumer`: `bus.Dispatch` contract, idempotency via `event_deliveries`, retryable vs term, `InProgress` for long work, `Nats-Msg-Id` on publish, consumer-not-stream per module.
  - `verify-backend`: the CLI and feature map above. Every backend task ends with `verify-backend flow <n>` output in the PR.

## Rollout

1. Scaffold: module, lint, comment checker and its agent hook, CI, `platform/*`, `errs`, `testkit`, generators, empty `flows.tsv` with `monacoctl flows check` green, `verify-backend` skill with an empty feature map, mkdocs, CHANGELOG. Delete `agents/momentum-bot`. No features. Prove lint fails on a planted violation of each rule, and measure the test budget table on the scaffold to set the CI gate.
2. Events + relay + `bus.Dispatch` with the bus test suite green on embedded NATS.
3. Identity, cabal, funding (deposits), treasury (fund, shares). E2E flows 1–7.
4. Governance + trading. Flows 9–13, with crash-point tests.
5. Cash out, withdraw, agents. Flows 14–17.
6. Market, ranking, social, notify, referrals, admin.
7. Cut iOS over module by module behind the generated client; delete the old backend in the same wave as the last route moves.

## Alternatives considered

| Alternative | Why not |
| --- | --- |
| Rust (axum + async-nats) | Network-bound workload; slower iteration and agent throughput; no embedded NATS server for tests. |
| Microservice per module | Distributed transactions across the state-change-plus-event write; N deploys and N outboxes for one team. |
| Full Uncle-Bob layering (entities, interactors, presenters, gateways) | One-implementation interfaces at every seam; call path spans 5+ files. |
| DI framework (fx, wire) | Hides wiring. `main.go` by hand is one readable file at this size. |
| GORM / ent | Hides SQL on money paths; sqlc keeps queries reviewable and types derived from schema. |
| `pkg/` for shared code | Nothing outside the module imports it; `internal/` is enforced by the compiler. |
| Coverage floors per layer (under 100%) | Earlier draft. Replaced by 100% merged coverage plus mutation testing, which blocks mock-assertion tests that only chase the number. |

## Decided

- **Module path** is `github.com/<org>/monaco/apps/backend`. `platform/` stays inside it. `agents/momentum-bot` is deleted in Rollout step 1, so nothing outside the module needs `platform/`.
- **Migrations use atlas**, versioned SQL files under `migrations/`, `atlas migrate lint` in CI, `atlas migrate apply` in the deploy's pre-deploy step. Nothing migrates at boot; a binary that finds a schema behind its expectation fails boot with `KindInternal`.
- **`cabal` everywhere**: Go types, tables, event subjects, and the new HTTP routes (`/v1/cabals/{id}`). The old rule "API routes and types stay `groups`" was for the backend being replaced; it ends when the iOS app cuts over to the generated client (Rollout step 7), which renames the routes on both sides in one PR per module. Legacy `/v1/groups` routes stay only as long as the old backend does.
- **NATS is Synadia Cloud**, free plan first. See [NATS hosting and budget](#nats-hosting-and-budget).

## Open questions

None at the moment.

## Log

- 2026-09-27: Logs as evidence: registered message names, join keys on every line, decisions and inaction logged, before/after on money lines, terminal line from `uow.Do`, replay and seeded scenarios.
- 2026-09-27: Errors rewritten around one `errs.Error` type and a code table; boundary-only logging; outcomes column. Testing budget made a gate with a cost table; test isolation by database with the lints that enforce it. `flows.tsv` replaces the hand table as the source of truth. Deploy on Render with OTel to Grafana. Open questions closed: module path, atlas, `cabal` everywhere.
- 2026-09-27: NATS hosting decided: Synadia Cloud, free plan first. Budget per limit, two streams, one connection per process, SSE hub with one wildcard subscription, `Nats-Msg-Id` on every publish.
- 2026-09-27: No comments in hand-written Go, only machine-read ones (`//go:` directives, generated files). Replaces the doc-comment allowance, `godox`, `nolintlint` and inline `//nolint`; adds an agent-time hook.
- 2026-09-26: Testing expanded: 100% merged coverage, mutation, model-based, fuzz, jitter via synctest, chaos dispatcher, template-DB Postgres.
- 2026-09-26: Proposed. Go, modular monolith, three-ring Clean Architecture enforced by depguard, generated contracts, lint config, flows table.
