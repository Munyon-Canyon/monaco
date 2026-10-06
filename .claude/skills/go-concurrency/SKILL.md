---
name: go-concurrency
description: Chooses between Pool, Stage, FanOut and errgroup for concurrent Go backend work, and applies the eight concurrency rules. Use when backend code starts goroutines, uses channels, or fans work out over providers, wallets or cabals.
---

# Go concurrency

Business code never starts a goroutine by hand. It calls one of three helpers in `apps/backend/internal/platform/concurrency/concurrency.go`, or `errgroup`. The rules come from [Concurrency rules](../../../docs/architecture/backend-platform.md#concurrency-rules).

## Pick the helper

| Job shape | Use | Behavior |
| --- | --- | --- |
| A known slice of items, every result needed, first error stops the rest | `concurrency.FanOut(ctx, limit, items, fn)` | Results come back in input order, or not at all. The first error cancels the derived context with that error as `context.Cause`. Items not yet started never run. The returned error wraps the cause, so `errors.Is` matches. |
| A stream of work from a channel, handled by N workers | `concurrency.Pool(ctx, workers, in, fn)` | Returns `out` and `errs`, both unbuffered, both closed after the last worker exits. Read them in one `select` loop. Ranging over `out` alone stalls at the first error, because that worker blocks on `errs`. |
| One step of a pipeline that must keep input order | `concurrency.Stage(ctx, in, buf, fn)` | One goroutine, so its `Result` stream keeps input order. `buf` is its only slack. |
| A few different calls that run side by side and share a lifetime | `errgroup.Group` from `golang.org/x/sync/errgroup` | `Wait` returns the first error. Use `errgroup.WithContext` when one failure must cancel the others. |

`workers`, `buf` and `limit` must be positive. Zero or negative panics at the call.

Real call sites:

- `FanOut` batches price calls in `apps/backend/internal/platform/chain/jupiter/prices.go`.
- `errgroup.Group` runs the request and the event stream side by side in `apps/backend/cmd/monacoctl/verify/drive.go` and `apps/backend/internal/testkit/scenario/stream.go`.

`Pool` and `Stage` have no production callers yet. Their tests in `apps/backend/internal/platform/concurrency/` show the calling pattern.

## The eight rules

1. No bare `go f()` in business code. The `nogo` analyzer in `apps/backend/internal/platform/lint/nogo/nogo.go` fails any `go` statement outside `cmd/` and `internal/platform/`. CI runs it with `go run ./internal/platform/lint/nogo/cmd/nogo ./...`.
2. Every goroutine has an owner that waits for it. Every package's `TestMain` is `testkit.Main(m, ...)` in `main_test.go`, and `testkit.Main` runs goleak. A leaked goroutine fails the package.
3. The sender closes the channel. Receivers never close.
4. Every channel is bounded. Use unbuffered or an explicit, justified size. No "big enough" buffers.
5. Backpressure over dropping. Drop only live SSE hints, and count the drops in a metric.
6. Shared mutable state lives behind one goroutine or a mutex in the same struct, never both. Prefer ownership over locks.
7. The race detector runs in CI. Stage 1's `backend` job runs `go test -race` over the backend through `scripts/test-backend.sh`.
8. JetStream `MaxAckPending` is the cross-process bound. It is `maxAckPending` (64) in `apps/backend/internal/platform/bus/registry.go`. An in-process pool inside a consumer must not exceed it.

## Context in concurrent code

- Pass `ctx` first. Every `select` that sends or receives also selects `<-ctx.Done()`.
- Cancel with `context.WithCancelCause` so logs say why the work stopped. `FanOut` already does this.
- Never start work on `context.Background()` outside `main`, tests and the bus consumer roots.

## Tests

- Time-dependent code takes the injected `clock.Clock` from `apps/backend/internal/platform/clock`. Tests use `testing/synctest` or `testkit.Eventually`. `time.Sleep` is banned in tests, and the `testwait` analyzer in `apps/backend/internal/platform/lint/nogo/testwait.go` bans its disguises.
- Test the failure path. Assert that the first error cancels the rest, that results keep their order, and that a cancelled parent context ends the work.
- Stage 0 (`go run ./cmd/monacoctl agents check`) runs the short tests without `-race`. Stage 1 runs them with `-race`. Owners do not run `-race` locally.

## Checklist

- [ ] No `go` statement outside `cmd/` or `internal/platform/`.
- [ ] Each goroutine's owner waits for it, and the package's `TestMain` uses `testkit.Main`.
- [ ] Each channel has a sender that closes it and a bounded size.
- [ ] Each blocking `select` includes `<-ctx.Done()`.
- [ ] Any in-process pool in a consumer stays at or under `MaxAckPending`.
- [ ] A test covers the first-error path and the cancellation path.
