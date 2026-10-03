---
name: go-backend-module
description: Adds a command, query, consumer, provider or module to the Go backend end to end, from the generator to the flow file. Use when a backend ticket adds or changes behavior under apps/backend/internal/modules.
---

# Go backend module

Every job has one paved path, and a generator writes its first copy. Start from the generator, then shape the output to look like flow 00 (the `system` module). Run every command from `apps/backend` unless a step says otherwise.

## The worked example: flow 00

| Piece | Path |
| --- | --- |
| Flow file | `packages/flows/backend/00.tsv` |
| Module wiring | `apps/backend/internal/modules/system/module.go` |
| Domain type with a parse constructor | `apps/backend/internal/modules/system/domain/note.go` |
| Command | `apps/backend/internal/modules/system/app/record_ping.go` |
| Query | `apps/backend/internal/modules/system/app/get_ping_query.go` |
| HTTP adapter | `apps/backend/internal/modules/system/adapters/http.go` |
| Consumer | `apps/backend/internal/modules/system/adapters/echo.go` |
| SQL | `apps/backend/queries/system/pings.sql`, generated into `apps/backend/internal/modules/system/sqlc/` |
| Migration | `apps/backend/migrations/20260929120000_system.sql` |
| Event | `apps/backend/internal/events/system.go`, registered in `apps/backend/internal/events/system_registrations.go` |
| Contract | `apps/backend/api/spec/system.yaml`, bundled into `apps/backend/api/openapi.yaml` and generated into `apps/backend/internal/platform/httpx/api/api.gen.go` |
| Flow scripts | `apps/backend/internal/testkit/flows/f00.go`, listed in `apps/backend/internal/testkit/flows/scripts.go` |
| Flow tests | `apps/backend/internal/modules/system/flow00_test.go`, `apps/backend/internal/modules/system/flow00_crash_test.go` |

## Generators

`just gen <kind> <args>` runs `go run ./cmd/monacoctl gen <kind> <args>` in `apps/backend`. The generator table is `apps/backend/internal/tools/gen/gen.go` and the templates are in `apps/backend/internal/tools/gen/templates/`. A generator never overwrites a file. `just gen help` prints the list.

| Command | Writes |
| --- | --- |
| `just gen module <name>` | `module.go`, `main_test.go`, the `domain`, `app` and `adapters` packages, `queries/<name>/`, an `api/spec/<name>.yaml` stub with `paths: {}`, a CHANGELOG line, the first migration `migrations/<timestamp>_<name>_init.sql` with the rewritten `atlas.sum`, and the regenerated `sqlc.yaml`, `.golangci.yml` and `cmd/*/module_<name>.gen.go` |
| `just gen command <module> <Name>` | `app/<name>.go` and `<name>_test.go` |
| `just gen query <module> <Name>` | `queries/<module>/<name>.sql` and `app/<name>_query.go`, then runs sqlc |
| `just gen migration <module> <name>` | `migrations/<timestamp>_<module>_<name>.sql`, empty, and the rewritten `atlas.sum`. `just gen migration --rebase` moves the branch's own migrations above the newest on origin/staging |
| `just gen consumer <module> <name>` | `adapters/<name>.go` and `<name>_test.go`, and appends the consumer to `Consumers()` in `module.go` |
| `just gen provider <name>` | a client, its fake and fixtures under `internal/providers/<name>/` and `internal/testkit/fakes/testdata/fakes/<name>/` |
| `just gen flow <id>` | `flow<id>_test.go` with one failing test per outcome of the row |

Generated stubs are placeholders. The command and consumer templates append or take `events.SystemPinged`, and every generated test ends in `t.Fatal("not implemented")`. Replace both before you push.

## Add a command

1. Run `just gen command <module> <Name>`.
2. Give the command struct the fields the caller supplies. The template adds an `IdempotencyKey` field. Drop it when the only caller is HTTP: the `Idempotency-Key` header is deduped by the middleware in `apps/backend/internal/platform/httpx/idempotency.go`, which is why `RecordPing` has no such field. Parse raw input into domain types at the adapter, as `domain.ParseNote` does. The app layer takes typed values.
3. Do the write inside `h.uow.Do`. Use the module's sqlc queries through `sqlc.New(tx.Queries())`. Append the event with `tx.Events.Append` in the same function, so the row and the event commit together. The event's actor comes from the context, and a poller tick runs as `system:poller.<name>`, so a poller never sets its own actor.
4. Add the event if the command needs a new one. Follow "Add an event" in the `nats-consumer` skill; it touches five files besides the event type.
5. Add the SQL. Put the statement in `queries/<module>/*.sql` and regenerate with `../../.bin/sqlc generate` (installed by `scripts/install-sqlc.sh`). A state change is a guarded update: `UPDATE ... WHERE id = $1 AND <expected state>` with `:execrows`, and zero rows means the state moved.
6. A schema change is a new atlas migration. Run `just gen migration <module> <name>`: it picks the timestamp prefix (now, or one second past the newest prefix when the clock is behind it) and reruns `atlas migrate hash`. Never choose a prefix by hand and never edit an applied migration. After a restack conflict in `migrations/`, run `just gen migration --rebase`.
7. Expose it. Add the route and the schemas only it uses to `apps/backend/api/spec/<module>.yaml`, never to the generated `openapi.yaml`, and run `go generate ./...` in `apps/backend`, which bundles the spec files and then regenerates `api.gen.go`. A schema that more than one module uses goes in `api/spec/base.yaml`. Add the method to the module's routes interface in `apps/backend/internal/platform/httpx/server.go`, and implement it in the module's HTTP adapter, as `apps/backend/internal/modules/system/adapters/http.go` does. Errors are `errs` codes, never ad hoc HTTP statuses. A new code goes in `apps/backend/internal/errs/codes_<module>.go` (the constant and its table row) and in that area's sorted `apps/backend/internal/errs/testdata/codes_<module>.golden`, then `go generate ./...` rewrites the generated `api/spec/error_codes.yaml` and the Swift case list. Never edit `api/spec/base.yaml` for a new code.
8. Wire the handler in `module.go`, as `Routes` does for `RecordPing`.

## Add a query

Reads bypass the domain. `just gen query <module> <Name>` writes the SQL and a function that takes a `sqlc.DBTX`. The HTTP adapter calls it with the pool, as `GetSystemPing` does with `Reads`. A query never appends an event and never runs inside `uow.Do`.

## Add a consumer

Run `just gen consumer <module> <name>`, then follow the `nats-consumer` skill.

## Add a provider

Run `just gen provider <name>`. Wire types stay in the provider package. The fake and its JSON fixtures are what tests and `monacoctl verify` talk to. Tests never call the real service.

## Test layers

| Layer | Where | Runs against |
| --- | --- | --- |
| Domain and app unit tests | `<module>/*_test.go` | `testkit.DB` on the `postgres-test` container |
| Consumer tests and the chaos suite | `echo_test.go`, `echo_chaos_test.go` | embedded NATS through `testkit.Main(m, testkit.WithNATS())` |
| Flow acceptance tests | `flow<id>_test.go`, calling a script in `internal/testkit/flows` | api and worker in one process through `scenario.New` |
| Crash tests | `flow<id>_crash_test.go`, `//go:build faultpoints` | the same scenario with a fault point armed |
| End to end | `monacoctl verify` in the Graphite queue | real binaries, see the `verify-backend` skill |

Every `TestMain` lives in `main_test.go` and is exactly `testkit.Main(m, ...)`. It runs goleak.

## The flow file

A flow is one file, `packages/flows/backend/<id>.tsv`: the tab-separated header line and one row whose `id` equals the file name stem. List cells use `;`. `monacoctl flows check` reads every file and fails one with a second row, a wrong header or an id that differs from its name. The command cell may list several commands. Then each outcome needs a test from at least one of them, each command needs at least one test, and only the pairs a command can return get a test. The trigger cell then holds one trigger shared by every command, or one per command in the same order, and `monacoctl verify` checks each command's outcomes against its own trigger.

| Status | Needs |
| --- | --- |
| `planned` | Valid shape. The module directory, trigger, events, consumers, outcome codes, crash points and doc anchor all resolve. |
| `built` | A command, a passing test per outcome named `TestFlow<id>_<Command>_<Outcome>` (`OK` for `ok`, `Crash<Point>` for `crash:<point>`), and a script for every non-crash outcome registered in `internal/testkit/flows/scripts.go`. |
| `verified` | A script for every remaining outcome, including each crash outcome not already registered, in `Scripts()`. |

1. Add `packages/flows/backend/<id>.tsv` with the row as `planned` when the ticket starts.
2. Run `just gen flow <id>`. Write the non-crash scripts in `internal/testkit/flows/f<id>.go`, register them in `Scripts()`, and make each test call its script. Then set `built`. Scripts for the non-crash outcomes ship with the `built` row.
3. Crash scripts ship with the `verify all --crash-at <point>` line in `scripts/ci/e2e.sh` that runs them. Register the rest and set `verified`.
4. Regenerate the docs with `just gen docs` and the feature map with `go run ./cmd/monacoctl docs flows --feature-map > ../../.claude/skills/verify-backend/feature-map.md`. `scripts/ci/ready.sh` fails when either is stale.

A test named `TestFlow<id>_...` with no row fails the check. Deleting a flow deletes its file and its tests together.

## Before you push

Lines stay under 120 characters. `golines` in the pre-commit hook rejects longer ones.

- `go run ./cmd/monacoctl lint comments` passes. Go has no comments except `//go:` directives.
- `go run ./cmd/monacoctl flows check --structure-only` passes.
- `go run ./cmd/monacoctl agents check` passes. It builds, vets and runs the short tests of the packages affected since the base. A change to a shared package such as `internal/events` or a module's sqlc output affects most of the module, so the test row runs nearly everything. Each package in that row has a 20 s budget. The row as a whole has none. A package over 20 s is slow code: fix it. Never raise the budget.
