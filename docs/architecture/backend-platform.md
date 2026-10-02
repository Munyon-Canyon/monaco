# Backend platform (Go rewrite)

**Status:** Proposed 2026-09-26. Builds on [event-bus.md](event-bus.md), [trade-execution.md](trade-execution.md), [data-model.md](data-model.md). Ignored the legacy `apps/backend` code on purpose; M7 deleted it before the scaffold.

## Decision

1. **Stay on Go.** No Rust rewrite.
2. **One modular monolith, two entrypoints.** One module, one container image, `cmd/api` (HTTP, SSE, relay) and `cmd/worker` (JetStream consumers, pollers, relay). Modules talk through NATS events, never through each other's packages. Splitting a module into its own service later is a deploy change, not a code change.
3. **Clean Architecture, three rings, enforced by lint.** `domain` (pure) ← `app` (use cases, ports) ← `adapters` (Postgres, NATS, Jupiter, Privy, HTTP). Import direction is checked in CI by `depguard`, not by review.
4. **Go channels are in-process only. NATS is the only cross-module path.** "NATS channel based" means JetStream subjects between modules, and bounded Go-channel pipelines inside one handler. Never a Go channel as a substitute for the bus.
5. **Contracts are generated, not hand-written.** OpenAPI 3.1 spec is the source of truth for the iOS contract (Go server stubs via `oapi-codegen`, Swift client via `swift-openapi-generator`). SQL is the source of truth for rows (`sqlc`). Event payloads are Go types in one `events` package with a subject registry. Every payload carries a `v` field, and a breaking payload change bumps it.
6. **Harsh `golangci-lint` v2, no inline `//nolint`, no comments.** Exceptions live in `.golangci.base.yml` or in a module's own `internal/modules/<m>/lint.yml`, each with a reason. `scripts/gen-golangci` renders both into `.golangci.yml`, which is generated. Hand-written Go carries only machine-read comments. Custom `forbidigo` rules encode Monaco-specific bans (floats for money, `time.Now` in domain, `context.Background` outside `main`).

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
├── .golangci.base.yml             hand-written lint config; .golangci.yml is generated from it
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
│   │   └── money/                 Micros, SignedMicros, TokenAmount, share math (math/big)
│   ├── events/                    every event type + subject registry (one file per aggregate)
│   ├── modules/
│   │   ├── identity/              auth session, users, profile, user wallets
│   │   ├── cabal/                 cabals, members, access requests, rules, treasury wallets
│   │   ├── governance/            proposals, votes, tally, expiry, execution outcome
│   │   ├── trading/               trade engine, swaps and their state machine, sweeper
│   │   ├── treasury/              ledgers (cabal_txns, user_txns), fund, cash out, positions
│   │   ├── funding/               deposits poller, withdrawals, onramp sessions, bounce, cabal pauses
│   │   ├── market/                assets catalog, price poller, price history, provider strategies
│   │   ├── agents/                agent keys, intents, budget enforcement
│   │   ├── social/                follows, feed, comments, chat bridge
│   │   ├── ranking/               valuation snapshots, leaderboards
│   │   ├── notify/                notifications, device tokens, APNs
│   │   ├── referrals/
│   │   ├── analytics/             PostHog export; consumes events, owns no business tables
│   │   └── admin/                 admin actions, dead_letters
│   └── testkit/                   fakes, fixtures, embedded NATS, Postgres container
├── migrations/                    atlas versioned SQL, forward-only
├── flows.tsv                      every flow, its outcomes, and its test status (see Flows)
├── queries/                       sqlc .sql files, one dir per module
├── deployments/                   compose, Dockerfile, NATS stream config
└── test/
    └── e2e/                       black-box: real binary, real PG + NATS, fake externals
```

Each module has the same four directories, and a `port` package when it exports a query port. Nothing else.

```
internal/modules/governance/
├── domain/        Proposal, Vote, Tally(), status machine. Pure. No ctx, no I/O, no errors from infra.
├── app/           commands + queries (use cases). Defines the ports it needs.
├── adapters/      postgres repo (sqlc), http handlers (oapi-codegen strict server), consumers
├── port/          exported read-only query port: interfaces and read types; imports only domain. The Postgres implementation lives in adapters
└── module.go      New(deps) → *Module; registers routes. `Consumers(config.Config)` feeds cmd/worker through generated per-module files
```

### Dependency rules (enforced by `depguard`)

| Package | May import | May not import |
| --- | --- | --- |
| `*/domain` | stdlib, `platform/money`, `events` | `app`, `adapters`, `platform/db`, `platform/bus`, `pgx`, `nats`, `net/http`, other modules |
| `*/app` | own `domain`, `events`, `platform/{money,clock}` | `adapters`, `pgx`, `nats`, `net/http`, other modules' `app` or `domain` |
| `*/adapters` | own `app` + `domain`, `platform/*`, drivers | other modules' packages |
| module A | module B | never. Cross-module effects go through an event. Cross-module reads go through a read-only query port that lives in module B's `port` package, and module B's `module.go` re-exports it. |
| `cmd/*` | everything | nothing imports `cmd` |

### Table ownership

Each table has one module that writes it. Other modules read it through that module's query port or learn of changes through its events.

`users`, `follows` and `cabal_messages` soft delete: a `deleted_at timestamptz` column, and reads filter `deleted_at IS NULL`. Every other table keeps its current delete behavior. Ledger rows are never deleted.

| Table | Owner | Rule |
| --- | --- | --- |
| `users` | identity | Soft delete through `deleted_at`. Account deletion sets it with `account_status = deleted`, scrubs PII and keeps ledger rows. `first_deposit_at` is set by identity's consumer of `deposit.credited` on the first deposit of at least $10, with a guarded update `WHERE first_deposit_at IS NULL`. Referrals reads it through identity's query port. `handle` is the unique username every user picks in onboarding (decided 2026-09-27). Referrals reads it through the same port, and after the first-deposit unlock the handle also works as a referral code ([referrals.md](referrals.md#codes)). |
| `user_wallets` | identity | One Privy wallet per user. Sign-in reuses the user's existing Privy wallet and never creates a second. |
| `treasury_wallets` | cabal | One app-owned treasury wallet per cabal. Kept apart from `user_wallets` so identity and cabal never share a table. |
| `swaps` | trading | The swap state machine. Treasury never writes it. |
| `cabal_txns` | treasury | Written by treasury's consumer of `trade.confirmed`, and by fund and cash out. |
| `user_txns` | treasury | Written by treasury's consumers of `deposit.credited` and `withdrawal.confirmed`, and by fund and cash out. |
| `cabal_pauses` | funding | The one pause record. Trading and treasury read it through funding's query port at check time. There is no `cabals.trading_paused_at`. An ops pause is a pause record with reason `ops`. |
| `price_points` | market | One table for every price sample, `price_micros bigint`. The poller writes it; price history and charts read it. Pyth is dropped. |
| `follows` | social | Soft delete through `deleted_at`. A unique partial index on `(follower_id, followee_id) WHERE deleted_at IS NULL` lets a re-follow insert a new row. Follower and following counts are an indexed `count(*)` over live rows; there is no counts table. |
| `cabal_messages` | social | Chat messages. Soft delete through `deleted_at`. |
| `chat_seen` | social | One seen watermark per member per cabal. Rows are hard-deleted when a member leaves; they are not user content. |
| `dead_letters` | admin | One row per dead-lettered message, with resolve state. |
| `events`, `event_deliveries` | `platform/bus` | Outbox and per-handler dedupe (see [NATS hosting and budget](#nats-hosting-and-budget)). |

## Patterns, and where each earns its place

| Pattern | Where | Shape |
| --- | --- | --- |
| **Unit of Work** | Every write that emits an event. Required by the events-as-outbox rule. | `uow.Do(ctx, func(ctx context.Context, tx Tx) error)`. `Tx` exposes the module's repos and `Events.Append`. Commit wakes the relay in the same process. No `Begin/Commit` anywhere else (lint: `forbidigo` on `pgx.Tx.Commit` outside `platform/db`). |
| **Command** | Every user or agent intent: `ProposeTrade`, `CastVote`, `FundCabal`, `CashOut`, `Withdraw`, `SubmitAgentIntent`, `CreateCabal`. | Typed struct with `IdempotencyKey`, one `Handle(ctx, cmd) (Result, error)` per command. HTTP handler parses → builds command → calls handler. Same command can come from HTTP, an agent key, or `monacoctl`. |
| **Query (CQRS-lite)** | Every screen read. | Reads hit projections (`cabal_positions`, `leaderboard_entries`, feed) directly via sqlc, bypassing domain. Writes go through commands. No shared "service" object doing both. |
| **Strategy** | Asset issuers (xStocks, Tessera, PreStocks) and swap venue (Jupiter today). | `type AssetProvider interface { Catalog(ctx); Quote(ctx, Asset, Micros) (Quote, error) }`. Best-price buy runs every provider that lists the asset (fan-out, below) and picks the min. `Venue` interface for execution. |
| **Registry (creational)** | Providers keyed by `Issuer`; consumers keyed by durable name; handlers keyed by handler name; events keyed by subject. | Built once in `cmd/*/main.go`. Duplicate key panics at boot. Tests assert every `events` type has at least one registered subject. |
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
| Relay publish batch | Sequential (order and ack matter); parallelism comes from multiple processes and `SKIP LOCKED`. api and worker each run a relay. | 1 |

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

How each behaves, so callers do not rediscover it:

- `Pool` returns two unbuffered channels and closes both after the last worker exits. Read `out` and `errs` in one `select` loop (or in two goroutines). A caller that ranges over `out` alone stalls at the first `fn` error until `ctx` is cancelled, because the worker holding that error is blocked on `errs`. That is backpressure, not a bug.
- `Stage` runs one goroutine, so its `Result` stream keeps input order. `buf` is its only slack.
- `FanOut` returns results in input order or nothing. The first `fn` error cancels the derived context with that error as `context.Cause`, items not yet started never run, and the returned error wraps that cause (`errors.Is` matches).
- `workers`, `buf` and `limit` must be positive; zero or negative panics at the call.

## Money and types

- USDC and token amounts are `money.Micros` / `money.BaseUnits` (unsigned 64-bit branded ints). Ledger entries and P&L are `money.SignedMicros`, a signed 64-bit branded int. A balance or amount is never negative; a delta or a return can be. Share math multiplies then divides through `math/big` and rounds down, in favour of the pot, in one function.
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

- `Code` is a string enum, declared per area in `internal/errs/codes_<area>.go`. The table `codes[Code] = {Kind, Retryable, Alert, Message}` is the single source of truth. `exhaustive` fails a switch that misses a code. A test asserts every `Code` constant has a table row and every row has a constant.
- The OpenAPI `ErrorCode` enum and the Swift test target's `ErrorCodeCases.gen.swift` are generated from that table by `monacoctl gen errors` and checked fresh in CI, so the Swift client's exhaustive `switch` and the Go table cannot drift.
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

Panics are recovered at exactly three roots: HTTP middleware, `bus.Dispatch`, and the poller tick wrapper. Each converts the panic to `CodePanic` (`KindInternal`) with the stack in `Attrs`, then follows the row above. The recovered panic is not re-raised in tests either: a test catches it through the `panic` problem or the Error line it asserts on. `http.ErrAbortHandler` is the one panic the HTTP root lets through, so `net/http` can abort the response.

### Reaching every branch

Error handling that no test reaches is decoration. The rules that make each branch reachable:

- Every port fake in `testkit` has `Fail(op string, err error)` and `FailOnce`. An `if err != nil` in `app` that a test cannot trigger through a fake is a design smell, not a coverage exclusion.
- `errcheck` runs with `check-blank: true` and `check-type-assertions: true`. `_ = f()` does not compile past lint.
- `nilerr` and `nilnil` are on: returning `nil` when `err != nil` is a lint failure.
- Mutation testing (below) flips `err != nil` to `err == nil` and removes `return err` lines. A surviving mutant is an error branch nothing checked.
- Every `KindInternal` in production is a bug. The alert links the trace, and the fix PR adds the code that should have caught it, so the `Internal` count trends to zero.

### Outcomes as a map

Each flow lists its outcomes in `flows.tsv`: the success path, every `Code` it can return, and every crash point (`after-create`, `after-sign`, `after-execute`, `before-commit`, `after-publish`). `after-create` covers a swap row committed as `created` whose process died before signing. A swap moves `created` → `submitted` in the same guarded write that stores its signed bytes, and that write commits before any send. So a `created` row was never signed or sent, and a crash after the send leaves the row in `submitted`. The trading sweeper fails any `created` row older than 2 min with no chain lookup. It resolves `submitted` rows through `getSignatureStatuses`. The crash-point test proves the flow converges. One acceptance scenario per outcome, named `TestFlow07_FundCabal_InsufficientFunds`. `monacoctl flows check` reads `go test -json` output and fails CI when an outcome in the TSV has no test that ran and passed. That is the codepath map: the file is the list, the test names are the proof, and the check is what keeps them equal.

## Logs as evidence

Three records exist, and each answers a different question. The `events` table in Postgres is what happened to state, and it is the source of truth. JetStream is how that reached the consumers. Logs are what each process did and did not do, and why, including every branch that ended with no state change. When something goes wrong, the logs must be enough to reconstruct how the current state came to be without reading code.

### Rules

1. **Structured only, stable names.** slog with `attr-only`, `static-msg`, `context: all` (`sloglint`). The message is an identifier, `treasury.fund.rejected`, never a sentence with values in it. Values are attrs. A message name is declared in a `msgs_<area>.go` file in `internal/platform/observability` next to the attrs it requires, and `go generate` registers it. A test fails on a call site that logs an unregistered name or omits a required attr. That registry is also the log catalog page in the docs.
2. **Every line carries the join keys.** `trace_id`, `span_id`, `request_id` or `event_id`, `actor`, `module`, `op`. The context logger adds them; a handler never types them. `sloglint context: all` means a call site cannot get a logger without the context.
3. **Log the decision, not the step.** One line where a branch chooses: a guard refused (the code and the numbers it compared, `have=4_000_000 need=5_000_000`), a retry was scheduled (`attempt=3 delay=30s cause=JupiterUnavailable`), a consumer skipped a duplicate (`event_id delivery=2`), a poller tick found nothing. Inaction is evidence. "Deposit poller ran at 10:04:10, scanned 212 wallets, found 0" is the line that proves a missing deposit was not the poller's fault.
4. **Money lines carry before and after.** Any line about a balance, share count or position has `before`, `after`, `delta`, `asset`, `cabal_id`. The ledger's history can be read from logs alone.
5. **Truthful means logged after commit.** A line that says "funded" before the transaction commits lies when the commit fails. `uow.Do` itself logs `tx.committed` or `tx.rolled_back` with the cause and the event ids it appended, so every write gets its terminal line without the handler doing it. Inside a transaction closure only `Debug` is allowed. That rule has no lint yet; the `money-change` skill checklist carries it until one exists.
6. **Levels mean one thing each.** `Debug` is step detail, off in production. `Info` is a decision or an outcome. `Warn` is degraded but handled (retry, fallback provider, stale price used). A module writes it with `observability.Degraded` and still returns its errors to the boundary. `Error` is a boundary only, one per failure, per the Errors section.
7. **Nothing secret, nothing personal.** The slog handler in `platform/observability` redacts by attr key (`phone`, `email`, `token`, `key`, `seed`, `signature`) and by value pattern (base58 secrets, JWTs). A golden test feeds each pattern through the handler and asserts the output. New attr keys that carry PII go on the list in the same PR.

### Replay and seeded states

The `events` table plus deterministic consumers make state replayable. `monacoctl replay --to <event_id>` rebuilds every projection (`cabal_positions`, `leaderboard_entries`, feed) into a fresh database from the event log, so "what did the leaderboard show at 14:02" is a command, not archaeology.

The ledgers (`cabal_txns`, `user_txns`) are not projections. Each ledger row commits in the same transaction as the event or delivery that caused it, and replay never rebuilds them. `monacoctl replay --verify` recomputes every balance from the event log and fails on any difference from the ledger. Replay reads events only; it never calls Jupiter, Privy or RPC, which is why consumers keep side effects behind ports.

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

`forbidigo` narrows by file only through `exclusions`, so the domain-only float and clock bans need path rules; expect to tune that on day one. Cross-module import bans (module A importing module B) need three `depguard` rules per module, generated by `scripts/gen-golangci` so a new module can't forget them. One rule covers `domain` and `adapters` and allows no other module. Another covers the module root, `app` and the remaining packages, and allows only another module's root and `port` packages, where its query port lives. A third keeps `port` to interfaces and read types that import only its own module's `domain`. The Postgres implementation lives in `adapters` and `module.go` returns it as the `port` interface, so `sqlc` types never cross a module boundary and two modules can read each other without an import cycle. Tuning numbers (`cyclop` 12, `funlen` 70) are starting points; raise per-package with a reason, never globally.

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

- One merged profile. Unit and integration tests use `-coverpkg=./...`. E2E and QA runs build the real binaries with `go build -cover` and write to `GOCOVERDIR`. `monacoctl coverage --profile <unit profile> --covdir <dir>...` merges them through `go tool covdata`, applies `coverage.exclude`, names every uncovered block as `file:start-end` and fails under 100%. `just test backend` runs it on its own profile. Until E2E exists, each `cmd/*` test re-runs its own test binary as the real `main` (`testkit.WithChild`, `testkit.StartMain`), with `GOCOVERDIR` set to the parent test's coverage directory, so the child's coverage lands in the same profile.
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
| QA | `monacoctl verify` on the real binaries; evidence is the `verify-evidence` CI artifact | `cmd/monacoctl verify` | stage 2 (Graphite merge queue) and nightly |
| Crash-point | panic at named points (after create, after sign, after `/execute`, before commit, after publish), restart, assert convergence. A `created` row was never signed or sent, so the sweeper fails it after 2 min; a crash after the send leaves the row `submitted`, which the sweeper resolves through `getSignatureStatuses`. | `faultpoint` hooks compiled in under the `faultpoints` build tag | `just test backend` in process; E2E in PR CI |
| Jitter / concurrency | pools, pipelines, relay, consumers under random delays and interleavings | `testing/synctest` + seeded delay injection + `-race` | `just test backend` (fixed seeds), nightly (seed sweep) |
| Performance (deterministic) | allocations per op on hot paths; query count per request | `testkit.AssertAllocs` (`testing.AllocsPerRun`) in `allocs_test.go`, which runs alone and without `-race`; `testkit.AssertQueries` counts the queries on the test's own `testkit.DB`. Both compare with the package's `testdata/perf/baseline.json`, and `-testkit.perf-update` rewrites it | `just test backend` |
| Performance (timing) | benchmarks compared against `main`; load on the full stack | `b.Loop` + `benchstat`; `vegeta` against the e2e stack | nightly, and on PRs labelled `perf` |
| Mutation | all non-generated packages | `gremlins` through `just test mutation` (`monacoctl mutation`): on a PR, only the lines the diff against its base changes (`gremlins --diff`); `--all` mutates every line of the whole module. A survivor fails unless `mutants.allow` lists it with a reason | PR CI (changed lines), nightly (every line) |
| Leak | every package | `goleak.VerifyTestMain`, run by `testkit.Main`, the only allowed `TestMain` body (nogo `testmain`) | always |

### Keeping it fast

Budget: `just test backend` under 90 s on a laptop, and the required PR checks under 6 min wall clock. The budget is a CI gate, not a hope: `testkit` records each package's wall time from `go test -json`, `monacoctl test-report` prints the ten slowest tests, a package over 10 s gets a warning, and a package over 20 s or a run over 90 s fails. The package limits are the same on a laptop and in CI. The run limit applies on a laptop only: in CI (`CI` set) `test-report --ci` gates packages but not the run, because a PR runner starts from a cold build cache and the job has its own 3 min budget. A test that breaks the budget gets fixed or moved to nightly, never skipped. `just test backend` runs `go test -p 4`: at the default of one test binary per core, every binary runs its parallel tests at once, and on an 8-core laptop packages that take 3.5 s alone took 10 to 13 s. With `-p 4` the same suite took 28.8 s instead of 41.1 s and every package stayed under 8 s (2026-09-27, load 8 to 10). Tests that build or run other binaries (the lint-rule fixtures and the tools they build, the `ids` type-check fixtures, the `testkit` fixture packages, the `-cover` binary merge) skip under `-short`. `scripts/test-backend.sh` runs them afterwards in a third pass that counts toward neither budget, since `test-report --budget-exempt` only prints its package times, and nightly runs them once.

Where the time went under the original 60 s budget. Every row was measured on 2026-09-27 against throwaway code shaped like the target, on an M2 under background load. Rollout step 1 re-measures on the real scaffold and in CI, and sets the gate from those numbers:

| Cost | Estimate | What keeps it there |
| --- | --- | --- |
| Compile and link the test binaries | Measured 2026-09-27 on today's backend as a stand-in (38 packages, 30 with tests, `pgx`, no NATS): cold cache 56 s throttled (`-p 2`, 4 cores, `nice 19`); nothing changed 1.4 s (31 of 31 cached); one file changed in a package most others import 10.5 s (`-p 4`, `nice 19`), which is recompile plus relink of every dependent test binary. Linking is the floor: each test package is one binary, and each NATS-importing binary adds about 0.2 s to load. | Go's build and test caches. `-count=1` disables the test cache and never writes to it, so `just test backend` never passes it. `-shuffle=on` still reseeds. Keep the test-package count near the module count: tests for a module's `app` and `adapters` live in one `_test` package per module rather than one per directory, so a change in `platform/` relinks about 15 binaries, not 50. |
| Postgres | 1 to 2 s | A separate test container, `monaco-postgres-test` on its own port, with `fsync=off`, `synchronous_commit=off`, `full_page_writes=off` and its data on tmpfs, so a crash loses nothing that matters. Migrations run once into `monaco_tmpl`. `just test backend` starts it if it is down and recreates it when its definition in `apps/backend/deployments/compose.yml` changes. The container runs with `max_connections=400` instead of the default 100. Each test clones its own database and opens a pool of up to four connections, and tests run in parallel. Six owners (`lanes` in `.monaco/agents.toml`) can run `monacoctl agents check` against this one container at once. On 2026-09-30 three owners testing at once used all 100 connections, and unrelated tests failed with `FATAL: sorry, too many clients already (SQLSTATE 53300)`. The dev container `monaco-postgres` never runs with durability off: `docker-compose.yml` pins `fsync`, `synchronous_commit` and `full_page_writes` on the command line, which overrides `ALTER SYSTEM`, and `scripts/verify-local-db.sh` fails if any is off. On 2026-09-27 a benchmark left `fsync=off` in the dev volume, the machine crashed, and the data directory came back unreadable. |
| Template clone per test | Measured 2026-09-27, Postgres 16 in a tmpfs container with durability off, today's 29 migrations (26 tables, 9 MB), `GOMAXPROCS=4`, `-parallel 4`, `nice 19`. Raw SQL: create 27 ms, drop 19 ms. Through `pgtestdb` with pgx: 64 parallel tests, each cloning, running 5 queries and dropping, took 1.0 s of test phase, 15.5 ms per test amortized and 60 ms for one test on its own (the drop runs inside `t.Cleanup`, before the slot frees, so it is on the critical path). The same 64 tests serial on one shared database with `TRUNCATE` between them took 1.9 s, 30 ms each. A single clone costs more than a `TRUNCATE`; clone-per-test wins because it runs in parallel. `-race` makes it 48 ms per test amortized. With default `fsync`, drop alone is 169 ms, which is why the test container turns durability off. Building the template from all migrations takes 1.5 s, paid once when it is missing. CI gets its own number from `monacoctl bench db` in Rollout step 1. | `github.com/peterldowns/pgtestdb`, which caches its own template keyed by the migrator's hash and clones it per test. Per-test rollback isn't an option because the Unit of Work commits and the relay reads committed rows. |
| Embedded NATS | Measured 2026-09-27 (M2, `GOMAXPROCS=4`, `nice 19`, nats-server v2.14.5): server start 27 ms, stop under 1 ms, 1.7 MiB heap. 32 parallel tests, each creating its own stream and consumer and moving 50 messages, took 114 ms of test time (3.6 ms per test amortized, 12.7 ms per test body). Plus about 0.2 s of binary load per NATS-importing package on a cold test cache. `-race` roughly triples the test phase. | One `nats-server` per package in `TestMain`, one stream per test. |
| Property, model-based, fuzz seeds, jitter | Measured 2026-09-27 on pure domain code: 3 invariants × 100 cases, a model test of 20 steps × 100 runs, 60 fuzz seeds and fixed-seed synctest jitter took 10 to 30 ms in-process together. The package costs about 0.5 s, nearly all of it the fixed per-binary start, and about 1.5 s with `-race`. A model test against the real app layer pays one `testkit.Reset` (31 ms) per run, so 100 runs is about 3 s: those run with fewer runs under `-short`. | `just test backend` runs with `-short` and sets `RAPID_CHECKS=500 RAPID_STEPS=40`, because rapid divides both by its own `-short` factor (5 and 2) to land on 100 cases and about 20 attempted steps. They are env vars, not `-rapid.*` flags, because `go test ./...` passes a flag to every test binary and a binary that does not link rapid rejects it. Corpus seeds only. Fixed jitter seeds. Nightly runs 100,000 cases and `-fuzz` per target. |
| Acceptance, one per `flows.tsv` outcome, in-process | Measured 2026-09-27 with a light stand-in scenario (own clone, 3 HTTP calls to an in-process `httptest.Server`, one pgx transaction writing two tables, rows asserted): 16.8 ms per scenario amortized at `-parallel 4`, 60 ms median and 80 ms p90 for one scenario on its own. 120 scenarios is about 2 s plus 0.4 s of binary load. Real scenarios do more work; budget 3× until the scaffold measures them. | HTTP against the in-process app, no binaries, no compose. |

Measured on the scaffold (#485, 2026-09-27): 26 test packages, no NATS and no acceptance scenarios yet. `just test backend` runs `go test -json -race -shuffle=on -short -coverpkg=./... -coverprofile`, the non-race allocation pass, `test-report`, `flows check` and `coverage`, with the test cache cleared before each run and the build cache warm. The laptop is an 8-core Apple silicon Mac with a 1-minute load average of 14 to 30 from other agents, so these are loaded numbers:

| Cost | Laptop, 8 runs | CI (`ubuntu-latest`, cold build cache) |
| --- | --- | --- |
| Whole run, first `go test` event to last | 26.0 to 47.6 s, median 35.7 s, p95 47.6 s | 64.6 s, most of it compiling with `-race` and coverage |
| Coverage instrumentation (`-coverpkg=./...`) | no difference outside the noise: 36.1 s median without, 35.7 s with | |
| Postgres, template clone per test (`platform/db`) | 6.8 to 9.2 s of package time | 2.8 s |
| Property (`platform/money`, 100 rapid cases) | 2.7 to 6.9 s | 1.3 s |
| Slowest package (`cmd/monacoctl`, fake `atlas`, `go` and `gremlins` scripts plus git) | 8.4 to 17 s | 2.3 s |
| NATS, acceptance | not measured: neither exists on the scaffold yet | |

The laptop run gate is 90 s. A test package warns at 10 s and fails at 20 s (`cmd/monacoctl/testreport.go`). Both rose on 2026-09-29 (#831; see the ci.md log). At the time of this measurement the gates were 60 s for the run (p95 times 1.5 is 71 s, which the RFC budget capped at 60 s) and 10 s for a package. `cmd/monacoctl` went over 10 s on the loaded laptop in 5 of 8 runs and never on an idle runner.

Not in `just test backend`: E2E (real binaries, compose, including their crash points), mutation, timing benchmarks. Those are PR CI or nightly.

PR CI is three required jobs, each with its own budget (when they run and on what runners is in [ci.md](ci.md)): lint plus unit plus integration (under 3 min, sharded by package), E2E (under 4 min), and mutation on changed lines (under 10 min, or the PR is too big and gets split). Nightly runs everything unbounded.

- **No sleeps.** `time.Sleep` in tests is banned by `forbidigo`, and nogo `testwait` bans its disguises outside `synctest.Test` and `testkit`: a bare `<-time.After(d)` and `time.After`, `time.Tick` or `time.NewTicker` inside a `for` loop. A test on a real socket waits on a signal, `testkit.Eventually` or `testkit.AssertNoRedelivery`. Time-dependent code takes the injected `clock.Clock`, and goroutine timing uses `testing/synctest`, where virtual time advances instantly once every goroutine is blocked. Measured: the full 1 s, 5 s, 30 s, 2 min, 10 min backoff schedule (12m36s of fake time) runs in 19 to 40 µs, and an 8-worker pool covering 12.5 s of fake time in 1 to 4 ms.
- **Bus timers are real time.** synctest and `clock.Clock` cannot speed up timers inside `nats-server`. Measured: redelivery takes exactly `AckWait` plus 1.5 ms, the max-deliveries advisory takes `MaxDeliver × AckWait`, and proving that `Term` stops redelivery costs the whole wait window. So bus-semantics tests run with `AckWait` 100 ms from `testkit.NATS`, one sample each, in parallel. That is about 0.5 s of mostly idle wall time per package. `testkit.NATS` rejects an `AckWait` over 250 ms.
- **Everything parallel.** `t.Parallel` is required (`paralleltest` and `tparallel` lint). That only works because every test owns its database and its stream, below.
- **Run what changed.** On PRs, mutation covers only the lines the diff changes, and long property runs only the packages affected by it (`go list -deps` against changed files). The nightly mutates every line, so a survivor a PR did not write still gets found.

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
| Leftover state after a failure | `pgtestdb` keeps a failed test's database for debugging. That is useful for one failure and dangerous for many: 500 failing tests would keep about 4.5 GB, and on 2026-09-27 kept clones from a failing benchmark filled a 1 GB tmpfs and killed the container. So `testkit.DB` keeps at most the first 5 failed databases per run and drops the rest, `TestMain` drops every `t_*` and `testdb_*` database older than one hour at start, and the test container's tmpfs is sized at 2 GB. |

### Keeping it deterministic

- **Every nondeterminism source is injected:** clock, UUID generator, random source and Jupiter/Privy responses. `forbidigo` already bans `time.Now`. Add `math/rand` globals and `uuid.New` outside `platform/`.
- **Seeds are printed and replayable.** `go test -shuffle=on` catches order dependence. Rapid, fuzz, jitter and the chaos dispatcher each log their seed on failure. For rapid the flag is `-rapid.seed=<n>`; measured with shrinking off, it reproduced the exact raw counterexample 3 of 3 times, also under `-short -race -shuffle=on`. Rapid saves each failure to `testdata/rapid/<Test>/*.fail` and replays it first on the next run. Those files are committed as regression cases in the fix PR. CI runs with `RAPID_NOFAILFILE=1` so a CI failure never writes to the tree.
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

Each flow is a command, the events it emits, the consumers that react, and every outcome it can end in. The table below is the target state of the rewrite, not the current code. Most of rows 1 to 17 existed in the deleted legacy backend in some form; 18 to 28 are partly new. Nothing here is "future reference": every row becomes a test before its module's rollout step closes.

### `flows.tsv` is the source of truth

The table below is a render. The file is `apps/backend/flows.tsv`, one line per flow, tab-separated, and it moves with the code. TSV over YAML because a flow is one line, the diff shows exactly which cell changed, `cut` and `awk` read it, and there is no indentation for an agent to get wrong. List cells use `;`.

| Column | Contents | Checked by |
| --- | --- | --- |
| `id` | `07`, or `01a` for a sub-row of flow 01 | unique; digits with at most one lowercase letter after them |
| `flow` | `Fund cabal` | |
| `module` | `treasury` | directory exists |
| `trigger` | `POST /v1/cabals/{id}/fund` or `consumer:proposal.passed` or `poller:deposits` | route in `openapi.yaml`, subject in registry, or poller registered |
| `command` | `FundCabal` | Go type exists in `module/app` |
| `events` | `cabal.fund_submitted;cabal.funded` | each in the `events` registry |
| `consumers` | `treasury.positions;ranking;feed;referrals` | each a registered durable |
| `outcomes` | `ok;InsufficientFunds;CabalPaused;PrivyUnavailable;crash:after-sign;crash:before-commit` | each `Code` in the `errs` table; each crash point a registered `faultpoint` |
| `status` | `planned`, `built`, `verified` | `built` needs every outcome test and a script for each non-crash outcome; `verified` needs a script per outcome (below) |
| `doc` | `docs/architecture/deposits-withdrawals.md#fund` | file and anchor exist |

`monacoctl flows check` runs in CI and fails on any column's check. It also reads `go test -json` from the run and requires, for each flow with `status` ≥ `built`, a passing test named `TestFlow<id>_<Command>_<Outcome>` per outcome. A flow row with no test is a red build, not a backlog item. The check is what makes the file a map of the system instead of a wish list.

`status = built` means every non-crash outcome has a flow script registered in `Scripts()` (`internal/testkit/flows/scripts.go`), so `monacoctl verify all` drives it against the real binaries in stage 2. Crash scripts ship with the `--crash-at` line that runs them, and the rest ship at `verified`, which means every outcome has a script. `monacoctl flows check` fails a `built` flow whose non-crash outcome has no script, and a `verified` flow with an outcome that has no script.

`monacoctl verify` checks each outcome by the kind of trigger, and every log line it asks for must be written after the script started:

| Trigger | `ok` or a crash point | A code outcome |
| --- | --- | --- |
| Route | The request answered a 2xx, and `http.request` was logged | The request answered that code's status and `code`, and `http.request` and `http.problem` were logged |
| `poller:<name>` | A `poller.tick` line for the poller | A `poller.tick.failed` line for the poller with that `code` |
| `consumer:<subject>` | A `bus.dispatched` line on the subject with outcome `ack` | A `bus.dispatched` line on the subject with that `code` |

A poller flow's script waits for the next tick with `scenario.AwaitTick(poller)` and checks its counts with `scenario.ExpectTick(poller, scanned, changed)`. The worker keeps each poller's production interval unless the flow's entry in `Env()` (`internal/testkit/flows/scripts.go`) sets a shorter one that fits the 15 s flow budget. Two selected flows that set one variable to different values fail the run before it builds anything.

Generated from the TSV, checked fresh in CI: the table below (`monacoctl docs flows`), the `verify-backend` feature map, and the acceptance test skeletons (`just gen flow <id>` writes one failing test per outcome).

Adding a flow is one row plus the tests it names. Deleting a flow deletes the row, and the check fails until its tests go too.

`notify` appears in a consumers cell only when that event sends a push ([notifications.md](notifications.md#what-notifies-mvp)). A consumer's own ticket appends its durable name to the cell when it lands.

| # | Flow | Command / trigger | Events | Consumers |
| --- | --- | --- | --- | --- |
| 1 | Sign in (SMS or email OTP in every build; Apple and Google come with #541) | `POST /v1/auth/session` with Privy token; reuses the user's existing Privy wallet | `user.created` (first time), `user.auth_state_changed` | analytics, referrals (mint code; `AttachReferral` is the only attribution path), social (contact matches) |
| 2 | Create cabal | `CreateCabal` | `cabal.created` | feed, analytics |
| 3 | Join open cabal / request / invite / approve | `JoinCabal`, `RequestAccess`, `InviteMember`, `DecideAccess` | `cabal.member_joined`, `cabal.access_requested`, `cabal.access_decided` | feed, ranking |
| 4 | Leave cabal | `LeaveCabal` (guarded: the member holds zero shares, so the app routes them to cash out first; not creator with members) | `cabal.member_left` | feed, ranking |
| 5 | Crypto deposit | Deposit poller sees USDC in member wallet | `deposit.credited` | treasury (`user_txns`), notify, identity (`users.first_deposit_at` on the first deposit of at least $10), analytics |
| 6 | Card deposit | `CreateOnrampSession`, page PATCHes status | `onramp.status_changed` then flow 5 | analytics |
| 7 | Fund cabal | `FundCabal` → Privy transfer member→treasury → confirm → mint shares at live price | `cabal.fund_submitted`, `cabal.funded` | treasury positions, ranking, feed, referrals, analytics |
| 8 | Direct transfer to treasury | Treasury watcher in funding; writes the cabal's pause record in the same transaction; a bounce writes no ledger entries | `cabal.external_deposit_detected`, `cabal.external_deposit_bounced`; `cabal.paused` when the cabal's first pause reason opens and `cabal.resumed` when its last closes, both appended by funding | `cabal.external_deposit_*`: admin. `cabal.paused`, `cabal.resumed`: notify |
| 9 | Propose trade | `ProposeTrade` (advisory route + pot check via market) | `proposal.created` | feed, notify |
| 10 | Vote / tally | `CastVote`; expiry job | `proposal.passed` / `.failed` / `.expired` | trading, feed, notify, analytics |
| 11 | Execute trade | `trade-engine` consumer on `proposal.passed`; reads the pause through funding's query port | `trade.blocked` / `trade.submitted` (appended by the swap layer on submit) / `trade.confirmed` / `trade.failed`; governance then emits `proposal.executed` / `proposal.execution_blocked` | governance (on `trade.confirmed` / `trade.blocked`), treasury (`cabal_txns`), ranking, feed, notify, analytics |
| 12 | Retry failed trade | `RetryTrade` from `POST /v1/swaps/{id}/retry` | same as 11; the swap layer appends `trade.submitted` on resubmit | same |
| 13 | Withdraw / void proposal | `WithdrawProposal`, admin `VoidProposal` | `proposal.withdrawn`, `.voided` | feed |
| 14 | Cash out | `CashOut` from `POST /v1/cabals/{id}/cashouts` → burn shares → sell if short → pay USDC to member wallet | `cashout.started`, `cashout.completed` / `.partial` / `.failed` | ranking, feed, analytics |
| 15 | Withdraw to address | `Withdraw` | `withdrawal.submitted`, `.confirmed`, `.failed` | treasury (`user_txns`), analytics |
| 16 | Agent lifecycle | Proposal kinds add / pause / resume / remove agent | `agent.enabled`, `.paused`, `.removed`, `agent.key_revealed` | governance (marks the agent proposal executed on `agent.enabled` / `.paused` / `.removed`), agents, feed |
| 17 | Agent trade | `SubmitAgentIntent` (key auth, budget check at submit and again at execution) | `agent.intent_created` → same engine as 11 | trading, same as 11 |
| 18 | Prices | One market poller, every 120 s (fan-out over providers), writes `price_points` | `price.tick` (core NATS only, one batched message per tick, not stored as event); `asset.price_moved`, appended by the same poller (there is no second poller) | `price.tick`: ranking, live SSE. `asset.price_moved`: feed |
| 19 | Valuation + leaderboards | Every 2 minutes, and on `trade.confirmed`, `cabal.funded`, `cashout.completed` | `ranking.snapshot_written` | live SSE |
| 20 | Follow / unfollow | `Follow`, `Unfollow` | `follow.created`, `.removed` | notify, analytics |
| 21 | Feed + comments | `CreateComment` | `comment.created` | notify, live SSE, analytics |
| 22 | Chat | Ably for delivery; backend issues token and persists | `chat.message_posted` | notify (mentions) |
| 23 | Profile edit | `UpdateProfile` | `user.profile_updated` | ranking (names), feed |
| 24 | Notifications | Consumers write `notifications` row, then send | `notification.sent` | none |
| 25 | Referrals | Click, sign-up, first deposit | `referral.attributed`, `referral.qualified` | `referral.attributed`: social. `referral.qualified`: analytics |
| 26 | Admin | Any admin command | `admin.action` | audit |
| 27 | Dead letters | Advisory subscriber | none | admin writes `dead_letters`; `monacoctl deadletter retry` |
| 28 | Nudges | Identity nudge job | `user.nudge_due` | notify |

## Thin client

The iOS app renders; the server decides.

- One `GET` per screen returning everything that screen shows, already computed: pot value, share price, your stake, returns, display names, human-readable asset names, formatted-ready integer amounts with decimals. No math in Swift beyond formatting.
- Every mutating call takes an `Idempotency-Key` header; the server stores the response and replays it. The one exception is `POST /v1/auth/session`, which sets `x-idempotent: false` in `api/openapi.yaml` because finding or creating the user by `privy_user_id` is already idempotent.
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
| Network data | 10 GiB | ~1.5 GiB/month | Price ticks are one batched `price.tick` message per poll carrying every asset, every 120 s (decided 2026-09-27). That is 12 times fewer ticks than the earlier 10 s cadence, so ticks drop from ~3 GiB to ~0.25 GiB a month. Per-asset messages at 5 s would be ~20 GiB/month on their own. Domain events are ~1 GiB. |
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
- A module's consumer runs one or more named handlers. `event_deliveries` is keyed by event id and handler name, so each handler dedupes on its own and a redelivery skips only the handlers that already applied it. A daily job deletes `event_deliveries` rows older than 30 days, which outlives both the 7-day `EVENTS` and the 30-day `DEADLETTER` windows.
- A message that exhausts `MaxDeliver` or is termed goes to `DEADLETTER`, and admin's consumer writes a `dead_letters` row that stays open until someone resolves it. `monacoctl deadletter retry` is the one recovery path. No safety-net poller re-scans stuck trades.
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

- The hub is a map from key (`cabal:<id>`, `user:<id>`, `global`) to SSE writers. On connect, the api registers the phone under its user id, the cabals it is a member of, and `global`. Every connection joins `global`, which carries feed and leaderboard hints. On a hint, the hub parses the subject, looks up the key, writes to those phones and no others.
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
2. **Shutdown order on SIGTERM**: stop fetching, send `InProgress` for in-flight long work, finish or nak what is running, flush the NATS connection under the shutdown deadline, then close it, close the pool, exit. The registry counts running dispatches itself, because `ConsumeContext.Closed()` does not wait for a running callback. Anything cut off is redelivered and deduplicated through `event_deliveries`.
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
| A design decision | A doc under `docs/architecture/` with an entry in `docs/architecture/log/` |
| Why this change | The commit message or PR body |

Enforcement:

1. **Checker.** `cmd/monacoctl lint comments` parses every non-generated `.go` file with `parser.ParseComments`, walks `file.Comments`, allows only the `//go:` prefix and prints `file:line` for the rest. Runs in the pre-commit hook and CI.
2. **Agent hook.** A Claude Code `PostToolUse` hook on `Edit|Write` for `apps/backend/**/*.go` runs the checker on the touched file and blocks with the offending lines, so the agent removes the comment in the same turn instead of in CI. Cursor gets the same check through its hooks.
3. **`AGENTS.md`.** One line: "No comments in Go. If you want one, you need a better name, a type, a test, or an issue."

Cost: IDE hover shows no docs. With `internal/`-only code and descriptive names, that is small.

### Verification skill

Agents need to prove backend work runs, not only that it compiles and its unit tests pass. `verify-backend` runs the real `api` and `worker` binaries against a real database and bus, drives a flow the way the app would, and writes down what the system did. It runs in the Graphite merge queue (stage 2) and nightly, after the tests.

What lives where:

| Piece | Path | Contents |
| --- | --- | --- |
| Skill instructions | `.claude/skills/verify-backend/SKILL.md` (mirrored to `.cursor/skills/`) | When to run it, the commands, how to read evidence, what counts as a pass, and what to do on a failure. Short; the CLI does the work. |
| Feature map | `.claude/skills/verify-backend/feature-map.md` | Generated from `flows.tsv` by `monacoctl docs flows`. Per flow: trigger, command, events, consumers, tables, outcomes, and the exact command that verifies it. Checked fresh in CI. |
| The CLI | `cmd/monacoctl verify` | Stack, driver, invariant checks, evidence writer. Code, tested like any other code. |
| Fakes server | `cmd/fakes`, built from `internal/testkit/fakes` | Privy, Jupiter, Solana RPC, Helius, xStocks, APNs and Ably over HTTP, replaying recorded fixtures. Scriptable per request: succeed, fail with a given error, delay, or hang. |
| Flow scripts | `internal/testkit/flows/<id>.go` | The same steps as the flow's acceptance scenario, written once and run by both `go test` (in-process) and `verify` (against binaries). |
| Seed scenarios | `internal/testkit/scenarios/` | Named event sequences replayed into the database before a flow starts (see [Replay and seeded states](#replay-and-seeded-states)). |
| Evidence | `apps/backend/.verify/<flow-id>.json`, git-ignored; CI uploads it as the `verify-evidence` artifact | One file per flow, or `<flow-id>-crash-<point>.json` for a crash run, stamped with the commit and whether the tree was dirty. |

What one run does:

1. **Stack up.** A throwaway Postgres container on its own port with data on tmpfs, never the dev container. An embedded NATS server. The fakes server. `api` and `worker` built with `-cover` and started with config that points every outside base URL at the fakes. The run fails fast if any piece is not healthy within 10 s.
2. **Seed.** Replay the flow's starting scenario, for example `cabal-with-members` for flow 7.
3. **Drive.** Run the flow script over HTTP with real auth headers, `Idempotency-Key`s and the SSE stream open.
4. **Wait for convergence.** Poll until every event the flow emitted has been acked by every consumer in `flows.tsv`, or time out at 30 s. A timeout is a failure with the stuck consumer named.
5. **Check invariants.** Ledger entries sum to zero per asset. Share units match the pot. No dead letters. No `KindInternal` in the logs. Every log line the flow must emit (registered in `msgs.go`) is present. For a route trigger, the HTTP status and `code` match the expected outcome.
6. **Write evidence** and print a readable summary.
7. **Tear down.** Stop the binaries, remove the container, merge coverage into `GOCOVERDIR`.

Modes:

| Command | Runs |
| --- | --- |
| `go run ./cmd/monacoctl verify all` | Every outcome of every `built` or `verified` flow, crash points excepted. |
| `go run ./cmd/monacoctl verify flow 00` | One flow, every outcome in its `outcomes` cell except crash points. |
| `go run ./cmd/monacoctl verify flow 00 --outcome InvalidInput` | One outcome. The fakes server is scripted to produce it. |
| `go run ./cmd/monacoctl verify all --crash-at after-publish` | The `after-publish` crash point of every flow that has one. Kills the worker there, restarts it, and checks the flow converges to the same end state. |

`scripts/ci/e2e.sh` runs `verify all`, then `verify all --crash-at after-publish`.

An evidence file holds: the commit SHA and whether the tree was dirty, the flow and outcomes run, each HTTP request and response (secrets redacted by the same slog handler), the `events` rows written, per-consumer acks and redeliveries, ledger balances per asset before and after, dead letters, the required log lines found, p50 and p95 handler latency, and pass or fail per invariant. Evidence from a dirty tree does not count.

Gates:

- **In the Graphite merge queue.** The `e2e` job runs `scripts/ci/e2e.sh` on every backend queue entry and fails on any failed invariant or budget. Owners do not run `verify`; stage 0 is `monacoctl agents check`.
- **On failure.** Fix the code and let the queue rerun it. Read the `verify-evidence` artifact to see what the system did. Never weaken an invariant or raise a budget to get green.

#### Budget: 90 s, enforced

`monacoctl verify all` finishes in under 90 s of wall time, measured from the command starting to the last container removed. That is a failure condition, not a target. The design aims for 45 s so ordinary noise never trips it.

How the budget is enforced:

1. **A hard deadline in the CLI.** `verify` runs under one `context.WithTimeoutCause` of 90 s. When it fires, the run stops, tears down, exits non-zero, and writes evidence with `"result": "over_budget"`, which fails the job.
2. **Phase budgets, so a failure says where the time went.** Stack up 10 s. Seeding 2 s per flow. Each flow, all outcomes included, 15 s. Teardown 5 s. A phase over its budget fails the run with the phase named, even when the total is under 90 s. So a slow flow is caught the day it gets slow, not the day the total finally crosses the line.
3. **Timings are evidence.** Every evidence file records each phase's duration and the machine it ran on (`GOOS`, CPU count, CI or local).
4. **Not built yet.** **Regression alarm.** CI compares each flow's duration with the median of its last 20 runs on `main` and fails the PR when a flow is more than 50% slower and more than 2 s slower. That catches creep long before the hard limit.
5. **The enforcer is tested.** `verify`'s own test suite plants a flow that sleeps past its budget and a stack that never becomes healthy, and asserts both fail with the right phase named. The budget cannot break silently.
6. **No raising the number in a PR.** The 90 s and the phase budgets live in `cmd/monacoctl/verify/budget.go`, and a change to that file needs the `budget-change` label from a human reviewer. An agent that hits the limit fixes the slow code or splits the PR.

How the design stays inside it:

- **One stack, flows in parallel.** The stack starts once. Flows run concurrently on it, 4 at a time, each in its own seeded cabal and users, so they share binaries without sharing data. Invariant checks are scoped to each flow's IDs.
- **Nothing waits on a real-world clock.** The fakes answer instantly unless scripted to delay. `verify` config sets `AckWait` to 100 ms and Jupiter `/execute` to its fake. Convergence is detected from consumer ack notifications, not by polling with sleeps. A poller flow is the exception, since it waits for the worker's next tick, so its `Env()` entry sets a short interval.
- **Build once.** Binaries come from Go's build cache; an unchanged tree relinks nothing. Seeds replay events in milliseconds instead of running commands.
- **Too many flows means too big a PR.** A branch whose touched flows cannot fit in 90 s at 4-way parallelism is split, the same rule as mutation testing's 10-minute limit.

The risk is noise: wall-clock gates are not perfectly deterministic, and a laptop at heavy load can run slow. The 45 s design target is the margin for that, and the evidence file records the load average at the start so a slow local run is easy to tell apart from slow code. `monacoctl verify all` is exempt from the 90 s total but not from the per-flow budgets.

Rollout step 1 measures a real run on the scaffold and confirms the phase budgets fit.

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
  - `verify-backend`: the instructions and feature map above. The Graphite merge queue runs it on every backend entry.

## Pull requests: small and stacked

Agents write code faster than people can review it. One large PR hides the change that matters, gets skimmed, and blocks everything behind it until it lands. So work ships as a stack of small PRs, each built on the one below, each reviewable in a few minutes and green on its own. [Graphite](https://graphite.dev) manages the stack.

### Setup

Graphite is a required tool, installed by `./scripts/install-dev.sh` (`just install`) like Go and `just`:

1. `brew install withgraphite/tap/graphite`
2. `gt auth --token <token>`, with the token from https://app.graphite.com/activate
3. `gt init --trunk staging` once per clone. Ticket stacks build on `staging`.

Agents in Claude Code on the web or CI install it with `npm install -g @withgraphite/graphite-cli` and read the token from `GRAPHITE_AUTH_TOKEN`.

### Rules

- **One PR is one verifiable unit.** It passes stage 0 (`monacoctl agents check`) and stage 1 on its own, without the PRs above it ([Verification scope](#verification-scope)). A PR that only makes sense with the next one gets merged with it.
- **Order a stack so each PR proves the next.** Delete or rename first. Then schema and migration. Then `domain` and `app` with their tests. Then adapters and HTTP. Last, the `flows.tsv` status change with its flow scripts. The Rollout steps below are each one stack, not one PR.
- **Size limit: under 1000 changed lines.** CI fails a PR at 1000 or more changed lines, counting added plus deleted lines in hand-written code, tests and docs. A pure rename counts as zero. Generated Go, `go.sum`, lockfiles, images, `testdata`, evidence files and rendered reference docs don't count; the list is `IGNORED` in `scripts/check-pr-size.py`. A human reviewer can add the `large-pr` label to let an oversized PR through, for example a mechanical change such as a rename; an agent adds it only when a human says to. Each PR of a stack is measured against its own base, the PR below it, so a stack of PRs that are each under the limit passes.
- **Split a branch that grew too big.** When work piled up on one branch or at the top of a stack, split it before submitting. The `distribute-stack-changes` skill does it by copying exact hunks onto the lowest branch that owns each behavior, restacking after each commit, and checking the top branch has zero diff from a saved reference. It never rewrites the work. `gt split --by-hunk` does the same by hand. The skill lives in each person's `~/.agents/skills`, not in the repo.
- **Title: what the PR changes.** For example `Add errs code table and problem+json mapping`: present tense, no issue number, no commit-type prefix such as `docs:` or `feat(x):`. Every stack starts from a GitHub issue in the write-ticket format, and each PR links it under Why (`Closes #212` or `Part of #212`).
- **Body: written for a human who reads nothing else.** `.github/pull_request_template.md` holds the sections, and GitHub and Graphite pre-fill it. The `pr-summary` skill in `.claude/skills` drafts it. The sections:
  - **TLDR:** what changed and its effect, in one or two sentences.
  - **Why:** the problem, and what breaks or stays slow without this PR.
  - **What changed:** grouped by behavior, not by file.
  - **Proof:** the exact commands run and what they printed. Include test counts, and measurements with their conditions. Say what was not verified.
  - **What came up:** surprises, wrong assumptions, decisions made along the way, and follow-ups filed as issues. A reviewer should learn here what the author learned.
  - **Reviewer focus:** where to look hardest.
  Open PRs as drafts with `gt submit --stack --no-interactive --draft`: without `--draft`, a new PR opens ready with the commit subject as its title and the empty template as its body, and PR format fails its first run. Then run `scripts/pr-body.sh <n> "<title>" <file>` for each PR. It runs the full `scripts/check-pr-format.py` locally (title, body, commits and stacked neighbours, with the base and head read from `gh pr view`), and only when that passes sets the title and body and runs `gh pr ready`. On a PR that is already ready it updates the title and body only.
- **Create and push with `gt`, not `gh`.** `gt create -m "<message>"` makes a branch and commit on top of the current one. `gt modify` amends and restacks everything above. `gt submit --stack` pushes the stack and opens or updates every PR with the right base. Plain `git push` or `gh pr create` on a stacked branch sets the wrong base or breaks the stack.
- **Keep the stack current.** `gt sync --no-interactive --no-restack` pulls the trunk and deletes merged branches without restacking anyone else's work. `gt restack` rebases this stack onto it. During a batch only the root runs `gt sync`. Resolve each conflict in the branch where it appears, then `gt continue`. Never leave conflict markers staged: run `git diff --check` before `gt add`.
- **Force-push only after checking the remote.** `gt submit --force` overwrites the remote branch. First confirm the remote has no commits the local stack lacks: `git log --oneline <local>..origin/<branch>` prints nothing. On 2026-09-27 a restack found a remote branch whose hash differed from the local one; the patch was identical, and that check is what proved it safe.
- **Always land through the Graphite merge queue.** When every PR in the stack has a green `ci / ci-ok` and a `verify` success on its head, run `monacoctl agents land-stack <top-pr>` by default, without asking, unless the user said in the current conversation not to land. It adds the `merge-queue` label to each PR of the stack, bottom to top. It waits while a PR's stage 1 or PR format check is still running or red. The queue runs stage 2 once on the whole stack, then squashes each PR into one commit on `staging`: the PR title with ` (#N)`, a blank line and the PR body. A landed PR shows as closed, not merged. If a stack drops out of the queue, its labels are gone: fix it with `gt modify` and `gt submit --stack --no-interactive --draft`, then run `land-stack` again. To change a queued stack, run `monacoctl agents dequeue <top-pr>` first. The agent guard hook blocks `gt submit`, `gt modify`, `gt restack` and `git push` on a stack while any of its PRs carries `merge-queue`. Agents never run `gh pr merge` into `staging`, never add `merge-queue` or `fast-track` by hand, and never change a PR's base. [Staging and main](ci.md#feature-branches) has the ruleset, the labels and the promotion into `main`.
- **Describe each PR on its own.** A reviewer reads one PR, not the stack, so each body stands alone and links its neighbours only for context.

### Verification scope

Checks run in three stages, and each stage runs only what the stage before it skipped. The operator approved this split on 2026-09-28, so a check that runs in a later stage is not skipped.

| Stage | Where | Runs | Budget | Runs how often |
| --- | --- | --- | --- | --- |
| 0. Agent check | Owner's worktree, `monacoctl agents check` | `go build` and `go vet` on affected packages, then golangci-lint, nogo and the comment lint, then `go test -short -count=1` on affected packages, no `-race`, and the `coverage` row, which fails on an uncovered statement in a changed non-test Go file. For non-Go paths, the cheap row for that path (`bash -n` and shellcheck; `cd scripts && go test -short` on the touched test files; `python3 -m unittest …`; `swift test` for `packages/mobile-core`). Path-triggered rows mirror CI's `ready`, migration lint, OpenAPI lint and oasdiff, and `mkdocs --strict` | per row, under `[check.budget]` in `.monaco/agents.toml`; no cap on the whole run | Once before each push (hook-enforced) |
| 1. PR check | CI, `pull_request` | The repo-wide checks, including those stage 0 runs on the changed paths only: `plan`, golangci-lint, nogo, the comment lint, OpenAPI lint and oasdiff, migration lint, `ready` (tidy, generated files, sqlc, docs), PR format. **No tests.** | ≤2 min | Once per change to the PR's diff. A push with the same diff reuses the last green result. |
| 2. Queue check | CI on the Graphite merge queue's `gtmq_` draft PR | The full suite: `scripts/test-backend.sh` as three parallel legs plus a report job (race, all packages, time budget, the 100% coverage gate), the `-short`-skipped tests, `flake` on changed tests, `monacoctl verify` (`scripts/ci/e2e.sh`), `scripts` tests, and mobile-core and iOS only when their paths changed | ≤5 min backend-only (iOS adds ~12) | Once per stack in the queue. Reruns only after an ejection. |

Stage 1 runs no tests because stage 0 already ran the tests the change can affect, on exactly the pushed code, and stage 2 runs every test against the real `staging` tip with all queued changes combined before anything lands. The verifier reviews in parallel with stage 1 and runs no tests.

Stage 0 diffs `HEAD` against `origin/staging` and picks its rows from the changed paths:

- Every diff: `scripts/check-pr-size.py` and `scripts/check-gate-changes.py` with `BASE_SHA` set to the stack parent, so an upstack PR is measured against its own parent, as CI measures it. A gate-changes warning shows as a count on the passing row.
- `apps/backend/**`: the packages `monacoctl ci affected` prints, seeded per changed file. A `.go` file seeds its package. A non-Go file seeds the nearest ancestor directory that is a package, which covers `migrations/*.sql`, `api/openapi.yaml`, `testdata/` and embedded files. `queries/<m>/**` seeds `internal/modules/<m>/sqlc`. `sqlc.yaml` seeds every package whose path ends in `/sqlc`. `flows.tsv` seeds the packages that read it (`flowsReaders` in `cmd/monacoctl/ci.go`). The walk then adds their importers and the packages whose tests import it. The result is `./...` when `go.mod`, `go.sum`, `.golangci.yml` or `internal/testkit/**` changed, or when a file has no package (a new package directory, `queries/platform/**`, a root file). The lint row runs the CI lint job's golangci-lint, nogo and `monacoctl lint comments` on them, and refuses a golangci-lint that differs from `apps/backend/.golangci-lint-version`.
- A `.sh` file, or an extensionless file with a `bash`, `sh` or `zsh` shebang: `bash -n` and `shellcheck`.
- Any path: the `scripts/**/*_test.go` tests and `scripts/**/test_*.py` files that the diff touches or that name the changed file's basename in a string literal, for example `"agent-guard.py"`.
- `packages/mobile-core/**`: `swift test`.
- `apps/backend/**`, `scripts/ci/ready.sh`, `scripts/gen-docs.sh` or `scripts/install-sqlc.sh`: `scripts/ci/ready.sh`, as the CI ready job runs it.
- `apps/backend/migrations/**`, `atlas.hcl` or `.atlas-version`: `monacoctl migrate lint`.
- `apps/backend/api/openapi.yaml`, `.vacuum.yaml` or `scripts/ci/oasdiff-*`: the pinned vacuum lint in Docker, the oasdiff self-test, and oasdiff against the stack parent's spec.
- `docs/**`, `mkdocs.yml`, `requirements-docs.txt` or `openapi.yaml`: `mkdocs build --strict` from the README's `.venv`, in the worktree or the main checkout. Without one the row prints `skip` and the install hint.

A restack onto a newer `staging` changes the tree but not the PR's diff. On a pass, stage 0 also writes `checks-diff/<patch-id>`, where the patch-id is `git diff <parent> HEAD | git patch-id --verbatim`, holding the tree it passed on. When `checks/<tree>` is missing and `checks-diff/<patch-id>` exists, stage 0 writes `checks/<tree>` with `carried from <old tree>`, prints `stage 0 carried from tree <old> (same diff against <parent>)` and runs no rows. An empty diff never carries, and `check --fresh` always runs in full. Code the PR did not change stays covered by stage 1 and stage 2. A changed or regenerated file changes the patch-id, so stage 0 runs in full.

The ready and migrate rows first run `scripts/install-sqlc.sh` or `scripts/install-atlas.sh` when `.bin/` lacks the tool, as CI does. The stack parent is `gt parent`, or the `--base` ref when Graphite does not track the branch or its parent is `staging`.

A path in no row runs nothing in stage 0. The paths with no checks in any stage are `docs/**`, `**/*.md`, `.claude/**`, `.cursor/**`, `.github/**` (actionlint runs in `ci / Plan`), `scripts/cloud-setup.sh` and `.env.local`; `ci / Plan` treats them as inert ([ci.md](ci.md)). `apps/mobile/**` still needs `just build mobile` and gold-sim QA from whoever changes it.

`monacoctl agents check` prints at most 20 lines, writes the full log under `.git/pstack/<milestone>/logs/`, and exits 1 naming the slowest package when a row runs over its kind's budget, or a package in the go test -short row runs over the per-package budget in `.monaco/agents.toml` `[check.budget]`. On a pass it records `git rev-parse HEAD^{tree}` in `.git/pstack/<milestone>/checks/`. It refuses a working tree that differs from `HEAD`, since it records `HEAD`'s tree.

`scripts/agent-guard.py` holds owners and verifiers to stage 0. It applies when `.git/.monaco/agents/<ticket>.json` names the calling worktree, so the operator and the root session are unaffected. It blocks `gt submit` and `git push` until `agents check` has passed on the current tree; the heavy tests (`just test backend`, `scripts/test-backend.sh`, `go test` with `-race` or without `-short`, `go test ./...` from `apps/backend`); CI polling (`gh run watch`, `gh pr checks --watch`, `gh pr checks` in a loop, `sleep` over 10s); `gt submit` with `--publish` or without `--draft`; and a raw `gh pr ready`, since only `scripts/pr-body.sh` marks a PR ready. `gh run rerun <id> --failed` and `gh pr ready --undo` stay allowed.

The same hook blocks `gt sync` without `--no-restack` for every session, because a plain sync restacks other agents' stacks mid-build. `scripts/agent-guard-dispatch.py`, a PreToolUse hook on the Agent tool, holds the spawn itself. A prompt with a `brief: docs/agents/owner.md` or `brief: docs/agents/verifier.md` line must run as `pstack:poteto-agent` with an explicit `opus` or `sonnet` model, and an owner's `ticket: <n>` needs the record `monacoctl agents dispatch` writes after it checks that every blocker has merged. `dispatch` prints the owner's spawn line and prompt, and `verify-plan` prints the verifier's. Spawns without the brief line, such as a skill's explorers, pass untouched.

The PR's Proof section pastes the `monacoctl agents check` output and says that CI covers the rest. A PR with no checks says `No code paths affected:` and names the paths.

### Enforcement

- **Agent guard.** `scripts/agent-guard.py` runs before every agent shell command. For PRs, it blocks `gh pr edit --base` and an inline `--body` on `gh pr create` or `gh pr edit`. In an owner or verifier worktree, it also holds `gt submit` to the draft flow.
- **CI, live now.** `.github/workflows/pr-format.yml` runs `scripts/check-pr-format.py` on every non-draft PR, again whenever the title or body is edited. It fails a title that starts with an issue number or a commit-type prefix, and a body missing any of the six sections or with a section that holds only the template's comment. It also fails when Why links no ticket (`Part of #n`, or `Closes #n` on the ticket's last PR), when a PR closes a ticket that an open PR stacked directly above it is part of, when the closing PR has no `## Needs from Logan` section or a fenced command there fails `bash -n`, when the body cites a SHA that is not an ancestor of the head, and when a commit in base..head has no Conventional Commit subject (`type(scope)!: subject`; merge commits and squash merges ending in `(#n)` are exempt). Write commits with the `/commit` skill. `scripts/pr-body.sh` runs the same checker with the PR's base and head before it sets a title and body or marks a draft ready. Its unit tests run in the same job. A second job runs `scripts/check-pr-size.py` for the size limit, reruns when a label is added or removed or the body is edited, and prints the ten largest counted files. It accepts an oversized PR only with the `large-pr` label.
- **CI, still to build.** A check that fails a PR whose base branch is not `main` and has no open PR of its own, the sign of a stack pushed without Graphite.
- **Install check.** `just install --check` exits 1 when `gt` is missing, the same as for Go.

## Rollout

1. Scaffold: module, lint, comment checker and its agent hook, CI, `platform/*`, `errs`, `testkit`, generators, empty `flows.tsv` with `monacoctl flows check` green, `verify-backend` skill with an empty feature map, mkdocs, CHANGELOG. The legacy backend, its migrations, its Go domain package and the reference trading bot are deleted first, as the step's opening stack. No features. Prove lint fails on a planted violation of each rule, and measure the test budget table on the scaffold to set the CI gate.
2. Events + relay + `bus.Dispatch` with the bus test suite green on embedded NATS.
3. Identity, cabal, funding (deposits), treasury (fund, shares). E2E flows 1–7.
4. Governance + trading. Flows 8–13, with crash-point tests. Flow 8 lands here because the pause it writes is what trading checks.
5. Cash out, withdraw, agents. Flows 14–17.
6. Market, ranking, social, notify, referrals, admin.
7. Cut iOS over in one release behind the generated client. The old backend was already deleted in step 1 (M7), so the cutover deletes nothing. The new backend starts on an empty database. Nothing is backfilled from the old backend and nothing syncs between them. A returning user signs in and gets their existing Privy wallet back; every other row starts fresh. Funds left in the old treasuries are test funds only. They are wiped at cutover: swept to an ops wallet or written off. Members are not cashed out.

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

- **Module path** is `github.com/<org>/monaco/apps/backend`. `platform/` stays inside it. The reference trading bot was deleted with the legacy backend in Rollout step 1, so nothing outside the module needs `platform/`.
- **Migrations use atlas**, versioned SQL files under `migrations/` named `YYYYMMDDHHMMSS_name.sql`, `atlas migrate lint` in CI (a rebase that brings in another branch's migration regenerates `atlas.sum` with `atlas migrate hash`), `atlas migrate apply` in the deploy's pre-deploy step. The pinned build is atlas community (`apps/backend/.atlas-version`), because from v0.38 the official build requires a login for `migrate lint`. `scripts/install-atlas.sh` (run by `just install`) puts it at the repo's gitignored `.bin/atlas`. `monacoctl migrate apply|status|lint` finds the `apps/backend` module from the working directory or its own executable, runs that exact path there, never the `atlas` first on `PATH`, and refuses to run when `atlas version` is not the pinned community build. `just migrate db` runs it against `.env.local`. Nothing migrates at boot; a binary that finds a schema behind its expectation fails boot with `db_schema_behind` and logs `have`, `want` and the hint `run: just migrate db`.
- **`cabal` everywhere**: Go types, tables, event subjects, and the new HTTP routes (`/v1/cabals/{id}`). The old rule "API routes and types stay `groups`" was for the backend being replaced; it ends when the iOS app cuts over to the generated client (Rollout step 7), which renames the routes on both sides in the same release. The legacy `/v1/groups` routes went with the old backend in M7.
- **The legacy backend is deleted in M7**, at the start of Rollout step 1, not at the iOS cutover. Every later ticket builds on an empty module instead of working around the old code. Accepted consequence: the iOS app has no working backend, local or deployed, until the domain milestone rebuilds its routes; mobile UI work uses sample data or the fakes server.
- **NATS is Synadia Cloud**, free plan first. See [NATS hosting and budget](#nats-hosting-and-budget).
- **JSON is snake_case.** API request and response fields and event payload fields are snake_case. Go and Swift identifiers follow their own language's conventions; the generated clients map between them.

## Open questions

None at the moment.

Log: [log/backend-platform.md](log/backend-platform.md).
