---
name: nats-consumer
description: Writes or changes a JetStream consumer in the Go backend, following the bus.Dispatch contract, event_deliveries idempotency, and the chaos suite. Use when a module reacts to another module's event, adds a handler, or publishes a new event.
---

# NATS consumer

A module reacts to an event through a consumer that it owns. Cross-module effects always travel as an event. A consumer never calls another module's code. Flow 00's `system.echo` handler is the worked example.

## The pieces

| Piece | Path | What it does |
| --- | --- | --- |
| Outbox append | `apps/backend/internal/platform/db/events.go` | `tx.Events.Append(ctx, ev)` writes the event row inside the caller's `uow.Do`. |
| Relay | `apps/backend/internal/platform/bus/relay.go` | Drains unpublished rows and publishes each with `Nats-Msg-Id` set to the event id. Both api and worker run one. |
| Publish | `apps/backend/internal/platform/bus/publish.go` | `Conn.Publish` sets the message id through `jetstream.WithMsgID`. The `EVENTS` stream drops duplicates inside its 2 minute window. |
| Dispatch | `apps/backend/internal/platform/bus/dispatch.go` | `Registry.Dispatch` decodes, runs each handler in its own transaction, and makes one ack, nak or term call. |
| Registration | `apps/backend/internal/platform/bus/registry.go` | `bus.Consumer{Durable, Handlers}` and `bus.Handle(name, fn)`. Durable and handler names are unique. A duplicate panics at boot. |
| Dedupe table | `apps/backend/queries/platform/events.sql` | `InsertDelivery` writes `(handler, event_id)` into `event_deliveries` with `ON CONFLICT DO NOTHING`. |
| Chaos suite | `apps/backend/internal/testkit/consumer.go` | `testkit.ConsumerSuite` replays events under redelivery, reordering and crashes, and compares the end state with a clean run. |

## Worked example: flow 00

- `apps/backend/internal/events/system.go` defines `SystemPinged`.
- `apps/backend/internal/modules/system/app/record_ping.go` appends it inside `uow.Do`.
- `apps/backend/internal/modules/system/module.go` declares the consumer: durable `system_echo`, one handler `system.echo`.
- `apps/backend/internal/modules/system/adapters/echo.go` handles it with a guarded update and publishes a live hint through `tx.AfterCommit`.
- `apps/backend/internal/modules/system/echo_test.go` tests the handler, and `apps/backend/internal/modules/system/echo_chaos_test.go` runs the chaos suite.

## The Dispatch contract

A handler has this shape:

```go
func (h Echo) Handle(ctx context.Context, tx db.Tx, e events.SystemPinged, at time.Time) error
```

- `at` is the delivery time. Dispatch stamps it on `event_deliveries.handled_at`. Use it for any time the handler stores.
- Dispatch opens the transaction. It first inserts the `(handler, event_id)` row into `event_deliveries`. When the row already exists, the event was handled, so Dispatch commits and acks without calling the handler.
- The handler runs inside that same transaction. Its writes and the delivery row commit together, or neither does.
- The handler writes through `tx`. It appends follow-on events with `tx.Events.Append`. Side effects that must wait for the commit, such as live hints, go in `tx.AfterCommit`.
- The handler returns `nil` to ack.
- It returns an `errs` error to nak or term. `errs.VerdictFor` in `apps/backend/internal/errs/errs.go` decides:
  - A retryable code naks with backoff (1 s, 5 s, 30 s, 2 min, 10 min). The retryable codes are marked `Retryable: true` in `apps/backend/internal/errs/codes_*.go`: upstream, Jupiter, Privy, RPC and database unavailability.
  - Any other code terms the message and publishes a dead letter. A plain Go error counts as `internal`, and a panic counts as `panic`. Both term.
- With several handlers on one message, any nak wins, then any term, then ack.
- A message gets 10 deliveries (`MaxDeliver`) before JetStream gives up.

So a handler must be safe to run twice for the same event after a nak. Make every write converge. Use `ON CONFLICT DO NOTHING` for inserts and a guarded `UPDATE ... WHERE <expected state>` for state changes, as `EchoPing` does in `apps/backend/queries/system/pings.sql`.

## Long work

Work that can outlast the ack wait calls `stop := bus.KeepAlive(ctx)` and `defer stop()`. It sends `InProgress` every 10 s. Outside Dispatch it does nothing. Never hold a database transaction open across a slow outbound call. Split the work so the slow call happens before or after the transaction.

## One consumer per module

- A module declares its consumers in `Consumers()` in its `module.go`. `apps/backend/cmd/worker/boot.go` starts every module's consumers.
- Each consumer is a durable on the shared `EVENTS` stream, filtered to its handlers' event types. A module never creates a stream. The streams live in `apps/backend/internal/platform/bus/streams.go`.
- The durable name is `<module>_<name>` and the handler name is `<module>.<name>`. The handler name is the `event_deliveries` key and the name in the `consumers` cell of the flow's file, `packages/flows/backend/<id>.tsv`.

## Add a consumer

1. Run `just gen consumer <module> <name>`. It writes the adapter and a test, appends the consumer to `Consumers()`, and switches `main_test.go` to `testkit.Main(m, testkit.WithNATS())`.
2. Replace the placeholder event type in the adapter with the event you consume.
3. Write the handler to the contract above.
4. Replace the generated test's `t.Fatal("not implemented")` with a test that dispatches the event and asserts the end state.
5. Add a chaos test with `testkit.ConsumerSuite`, built with `//go:build faultpoints`, like `echo_chaos_test.go`.
6. Add the handler name to the `consumers` cell of every flow that emits the event.

## Add an event

1. Add the type to `apps/backend/internal/events/<aggregate>.go` with `Type`, `AggregateType` and `AggregateID` methods and a `V` field.
2. Register it in `<module>Registrations()` in `apps/backend/internal/events/<module>_registrations.go`, the file of the module that publishes it. Leave `registry.go` alone. A new module adds its file and one line to `registrations()` there.
3. Add a fixture for it to `fixtures` in `apps/backend/internal/events/contract_test.go`. The golden step needs it.
4. Write its golden payload with `go test -short ./internal/events -update`. It writes `internal/events/testdata/golden/<type>.v1.json`.
5. Update the tests that pin the registry: `TestSubjects` and `TestCatalog` in `apps/backend/internal/events/registry_test.go` (the catalog is sorted by type), and the catalog text in `TestDocsEventsPrintsTheRegistryCatalog` in `apps/backend/cmd/monacoctl/docs_test.go`.
6. Regenerate `docs/reference/events.md` with `just gen docs`.

## Checklist

- [ ] The handler is idempotent under redelivery.
- [ ] Retryable failures return a retryable `errs` code. Everything else terms.
- [ ] No outbound call runs inside the transaction.
- [ ] Post-commit side effects use `tx.AfterCommit`.
- [ ] The chaos test converges.
- [ ] The file of each flow that reaches the handler names it in `consumers`.
