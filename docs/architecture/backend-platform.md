# Backend platform (Go rewrite)

**Status:** Proposed 2026-09-26. Builds on [event-bus.md](event-bus.md), [trade-execution.md](trade-execution.md), [data-model.md](data-model.md). Ignores the current `apps/backend` code on purpose.

## Decision

1. **Stay on Go.** No Rust rewrite.
2. **One modular monolith, two entrypoints.** One module, one container image, `cmd/api` (HTTP + SSE) and `cmd/worker` (JetStream consumers, pollers, relay). Modules talk through NATS events, never through each other's packages. Splitting a module into its own service later is a deploy change, not a code change.
3. **Clean Architecture, three rings, enforced by lint.** `domain` (pure) ← `app` (use cases, ports) ← `adapters` (Postgres, NATS, Jupiter, Privy, HTTP). Import direction is checked in CI by `depguard`, not by review.
4. **Go channels are in-process only. NATS is the only cross-module path.** "NATS channel based" means JetStream subjects between modules, and bounded Go-channel pipelines inside one handler. Never a Go channel as a substitute for the bus.
5. **Contracts are generated, not hand-written.** OpenAPI 3.1 spec is the source of truth for the iOS contract (Go server stubs via `oapi-codegen`, Swift client via `swift-openapi-generator`). SQL is the source of truth for rows (`sqlc`). Event payloads are Go types in one `events` package with a subject registry.
6. **Harsh `golangci-lint` v2, zero `//nolint` without a reason and linter name.** Custom `forbidigo` rules encode Monaco-specific bans (floats for money, `time.Now` in domain, `context.Background` outside `main`).

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
├── migrations/                    goose SQL, forward-only
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

- Domain errors are sentinel values or typed errors (`ErrInsufficientFunds`, `*BlockedError{Code}`), declared in `domain`. `errname` enforces `Err` prefix / `Error` suffix.
- Adapters wrap infra errors with `%w` and context (`wrapcheck`). Comparisons use `errors.Is/As` (`errorlint`).
- One place maps errors to HTTP: `httpx.Problem` (RFC 9457 `application/problem+json`) with a stable `code` the iOS app switches on.
- Retryable vs permanent is a property of the error (`bus.Retryable(err)`), which `bus.Dispatch` reads to choose ack, nak-with-delay, or term.

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
    - nolintlint
    - exptostd
    - godox
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
    nolintlint: { require-explanation: true, require-specific: true }
    sloglint: { attr-only: true, context: all, static-msg: true, no-global: all }
    interfacebloat: { max: 5 }
    godox: { keywords: [TODO, FIXME, HACK, XXX, BUG] }
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
        linters: [funlen, gochecknoglobals, err113, wrapcheck]
      - path: cmd/
        linters: [forbidigo]
formatters:
  enable: [gofumpt, gci, golines]
  settings:
    gci: { sections: [standard, default, localmodule] }
    golines: { max-len: 120 }
```

`forbidigo` narrows by file only through `exclusions`, so the domain-only float and clock bans need path rules; expect to tune that on day one. Cross-module import bans (module A importing module B) need one `depguard` rule per module, generated by a `scripts/gen-depguard.go` so a new module can't forget it. Tuning numbers (`cyclop` 12, `funlen` 70) are starting points; raise per-package with a reason, never globally.

Also in CI:

| Check | Tool |
| --- | --- |
| Vulnerabilities | `govulncheck ./...` |
| Generated code is fresh | `sqlc diff`, `oapi-codegen` + `git diff --exit-code` |
| Migrations lint | `atlas migrate lint` or `squawk` on new SQL (no table rewrites, no `NOT NULL` without default on large tables) |
| OpenAPI lint and breaking changes | `vacuum lint`, `oasdiff breaking` against `main` |
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

Budget: `just test backend` under 60 s on a laptop, PR CI under 6 min wall clock. A test that breaks the budget gets fixed, not skipped.

- **Postgres without per-test containers.** One Postgres per `go test` run, tuned for tests (`fsync=off`, `synchronous_commit=off`, `full_page_writes=off`, data dir on tmpfs). Migrations run once into a template database. Each test gets `CREATE DATABASE t_<id> TEMPLATE monaco_tmpl`, which takes tens of milliseconds. `peterldowney/pgtestdb` does this out of the box. Per-test rollback isn't an option because the Unit of Work commits and the relay reads committed rows.
- **NATS in process.** One embedded `nats-server` per package, with a unique stream name per test so tests run in parallel without sharing state.
- **Everything parallel.** `t.Parallel` is required (`paralleltest` lint). CI shards packages across runners and runs e2e in its own job.
- **No sleeps.** `time.Sleep` in tests is banned by `forbidigo`. Time-dependent code takes the injected `clock.Clock`, and goroutine timing uses `testing/synctest`, where virtual time advances instantly once every goroutine is blocked. A backoff schedule of 1 s, 5 s, 30 s, 2 min and 10 min runs in microseconds.
- **Run what changed.** On PRs, mutation testing and long property runs cover only packages affected by the diff (`go list -deps` against changed files). Full sweeps run nightly.

### Keeping it deterministic

- **Every nondeterminism source is injected:** clock, UUID generator, random source and Jupiter/Privy responses. `forbidigo` already bans `time.Now`. Add `math/rand` globals and `uuid.New` outside `platform/`.
- **Seeds are printed and replayable.** `go test -shuffle=on` catches order dependence. Rapid, fuzz, jitter and the chaos dispatcher each log their seed on failure, and `-seed=<n>` reproduces the exact run.
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
| Mutation efficacy on changed packages | 100% of mutants killed, or listed in `mutants.allow` with a reason (equivalent mutants only; reviewed like `//nolint`) |
| Allocations and query counts | no increase over the recorded baseline without a baseline update in the same PR |
| `-race`, `goleak`, `-shuffle=on` | zero findings |
| Nightly benchmarks | alert on a >10% `benchstat` regression with p < 0.05 |

## Flows

Each flow is a command, the events it emits, and the consumers that react. E2E suite has one test per row.

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
- Server-sent events (`/v1/stream`) push "this changed, re-fetch" hints from core NATS. The app never polls on a timer except as SSE-reconnect fallback.
- Errors carry a stable `code` and user-facing `message`. The app shows `message` in a toast; it switches on `code` only for flows that branch.
- Swift client is generated from `api/openapi.yaml`. A route change that breaks the client fails `oasdiff breaking` in CI.

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
- **Workarounds spread, so none land.** An agent copies whatever it sees. No `// TODO`, `// HACK`, `// workaround` (`godox` blocks them), and no explanatory comments inside function bodies. A `scripts/lint-comments.go` check allows only doc comments on exported identifiers, `//go:` directives and `//nolint:<linter> // <reason>`. If code needs a comment to explain a workaround, fix the cause or open an issue.
- **Gardener.** A scheduled agent (weekly) runs `golangci-lint` with stricter candidate settings, `deadcode`, `gremlins` and the generators' drift check, and opens small PRs that delete dead code or propose a new rule. One human owns merging them.

### Verification skill

Agents need to prove backend work runs, not only that it compiles. `.claude/skills/verify-backend/` holds two parts:

1. **A CLI inside the skill** (`verify-backend` built from `cmd/monacoctl verify`). It boots Postgres, NATS and fake externals, starts `api` and `worker`, runs one named flow or all of them through HTTP, and prints evidence: HTTP responses, `events` rows written, consumer acks, ledger balances per asset, dead letters, p95 handler latency. Same command every session, so agents don't write throwaway scripts. Crash-point mode (`--crash-at after-execute`) restarts the worker mid-flow and checks convergence.
2. **A feature map** (`feature-map.md` in the skill) generated from the Flows table: for each flow, the route or trigger, the command, events, consumers, tables touched, and the `verify-backend flow <n>` line that exercises it. An agent handed "my fund didn't show up" looks up flow 7 and knows which events and tables to inspect. A CI check regenerates it and fails if stale, so it can't drift from the registry.

## Docs, changelog, agents

- **MkDocs Material on GitHub Pages.** `mkdocs.yml` at repo root with `docs_dir: docs`. The existing `docs/` tree (index, product, architecture decision log, how-to) is the nav. A workflow on push to `main` runs `mkdocs build --strict` (broken links fail) and deploys with `actions/deploy-pages`. PRs run `mkdocs build --strict` only. API reference renders `api/openapi.yaml` via `mkdocs-render-swagger` or a Redoc page. Event catalog page generated from the `events` registry by `go run ./cmd/monacoctl docs events > docs/reference/events.md`, checked fresh in CI.
- **CHANGELOG.md.** [Keep a Changelog](https://keepachangelog.com) format, `## [Unreleased]` at top, sections Added / Changed / Fixed / Removed / Security. One line per user- or operator-visible change, written for a person, not a commit log. CI check above enforces it. Release cuts move Unreleased under a dated version.
- **Agent rules live in structure first.** `apps/backend/AGENTS.md` stays under 60 lines: the three rings, "cross-module = event", "money = integer + UnitOfWork", "run `just test backend` and `golangci-lint run` before done", plus a pointer to the skills. Everything else is a lint.
- **Generators over instructions.** `just gen module <name>`, `just gen command <module> <Name>`, `just gen consumer <module> <name>`, `just gen provider <name>` emit the correct files, test skeletons, depguard rule, registry entry and CHANGELOG stub. Agents copy what exists; make the first copy right.
- **Skills** in `.claude/skills/` (and mirrored for Cursor):
  - `go-backend-module`: adding a command/query/consumer end to end, including test layers and flow table update.
  - `go-concurrency`: when to use `Pool`, `Stage`, `FanOut`, errgroup; the eight concurrency rules; `goleak` and `-race` required. References the Mario Carrión fan-in/fan-out article for the base pattern and the helpers for the house version.
  - `money-change`: checklist for anything touching ledgers, shares, swaps: property test, crash-point test, guarded update, event in same tx.
  - `nats-consumer`: `bus.Dispatch` contract, idempotency via `event_deliveries`, retryable vs term, `InProgress` for long work.
  - `verify-backend`: the CLI and feature map above. Every backend task ends with `verify-backend flow <n>` output in the PR.

## Rollout

1. Scaffold: module, lint, comment check, CI, `platform/*`, `testkit`, generators, `verify-backend` skill with an empty feature map, mkdocs, CHANGELOG. No features. Prove lint fails on a planted violation of each rule.
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

## Open questions

- Module name path (`github.com/<org>/monaco/apps/backend`) and whether `platform/` moves to its own module if `agents/momentum-bot` wants it.
- `goose` vs `atlas` for migrations (atlas lint is stronger; goose is simpler).
- Synadia Cloud vs self-hosted NATS (carried from [event-bus.md](event-bus.md)).
- Cabal vs group naming in Go types (carried from [data-model.md](data-model.md#naming)). Recommendation: `cabal` everywhere in the rewrite, since there is no legacy code to match.

## Log

- 2026-09-26: Testing expanded: 100% merged coverage, mutation, model-based, fuzz, jitter via synctest, chaos dispatcher, template-DB Postgres.
- 2026-09-26: Proposed. Go, modular monolith, three-ring Clean Architecture enforced by depguard, generated contracts, lint config, flows table.
