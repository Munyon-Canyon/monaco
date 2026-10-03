# Flows

`apps/backend/flows.tsv` is the flow contract for the backend and the app. This package is the app's side of it. It records, for each flow, which screen carries it, how far the app has got, and where its doc lives. `monacoctl flows check` joins it to `flows.tsv` on `id` and fails on any mismatch.

## Layout

| Path | Contents |
| --- | --- |
| `app/<id>.tsv` | One registry file per flow. |
| `Package.swift` | The `MonacoFlows` Swift package, with no dependencies. |
| `Sources/MonacoFlows/Flow<id>.gen.swift` | The outcome enum for one flow. Generated; see below. |
| `Sources/MonacoFlows/Flow<id>Scenarios.gen.swift` | The harness scenario enum for one built or verified flow. Generated; see below. |

## Registry file

Each `app/<id>.tsv` holds a header and exactly one data row, tab-separated:

    id	screen	status	doc

| Column | Contents | Checked by |
| --- | --- | --- |
| `id` | `01`, or `01a` for a sub-row | equals the file name stem and an `id` in `apps/backend/flows.tsv` |
| `screen` | The screen or surface that carries the flow, such as `SignIn`. `-` when `status` is `none` | `-` exactly when `status` is `none` |
| `status` | `planned`, `built`, `verified` or `none` | one of the four values |
| `doc` | `path#anchor`, such as `docs/architecture/auth.md#login` | the file and the heading anchor exist |

The status values mean:

- `planned`: the app owes the flow a screen and has not built it.
- `built`: a screen handles every outcome of the flow. #1665 enforces it.
- `verified`: app QA drove every outcome. #1666 enforces it.
- `none`: the flow has no app surface, such as flow 18 (the prices poller), the admin flows and dead letters.

To add a flow, add its row to `apps/backend/flows.tsv`, then add `app/<id>.tsv`. To list every row, run `cat packages/flows/app/*.tsv`.

## Rules

- Keep one flow per file. No file under `packages/flows` other than this README may name two flow ids, and `monacoctl flows check` fails one that does. Tools build any list of flows at run time. A single aggregate file would conflict on every parallel stack.
- Never copy a backend column. Module, trigger, command, outcomes and codes live only in `apps/backend/flows.tsv`. The registry joins on `id`.
- Never edit the outcome enums under `Sources/MonacoFlows`. `monacoctl gen flows` generates them from `apps/backend/flows.tsv`.

## Generated Swift

`monacoctl gen flows` writes `Sources/MonacoFlows/Flow<id>.gen.swift` for each row of `apps/backend/flows.tsv`, and deletes the file of a row that is gone. `go generate ./...` in `apps/backend` runs it, and `scripts/ci/ready.sh` fails when the output is stale. Each file holds one enum, `Flow<id>Outcome`, where `<id>` is the row id verbatim (`01a` gives `Flow01aOutcome`):

- `ok` stays `ok`.
- A code outcome becomes a case named for the code with a lowercase first letter, so `InvalidInput` becomes `invalidInput`. A Swift keyword is escaped with backticks.
- Every `crash:` outcome collapses into one last case, `interrupted`, because the app resends with the same idempotency key.
- `code` returns the wire code string, such as `invalid_input`, and `nil` for `ok` and `interrupted`. `init?(code:)` is its exact inverse.
- `flowID` and `command` repeat the row's id and command. The enum carries nothing else from the row.

For each flow whose `status` here is `built` or `verified`, it also writes `Sources/MonacoFlows/Flow<id>Scenarios.gen.swift` and one line per scenario in the generated block of `scripts/qa/sample-screens.txt`:

- `Flow<id>Scenario` has one case for each case of `Flow<id>Outcome` except `ok`, with the case name as its raw value. Flow 00 gives `invalidInput`, `unauthorized` and `interrupted`.
- `matching(_:)` reads the launch arguments `-MonacoFlow <id> <case>`, such as `-MonacoFlow 00 unauthorized`, and returns that case, or `nil` for another flow or no flag.
- The manifest block sits between `# BEGIN generated flow scenarios` and `# END generated flow scenarios`. `gen flows` rewrites only that block and keeps every line around it. [Debug sample harnesses](../../docs/how-to/mobile-harness.md#flow-scenarios) says what the app adds for each scenario.

Never edit a generated file. Change the row in `apps/backend/flows.tsv` and run `cd apps/backend && go run ./cmd/monacoctl gen flows`. On a restack, `merge=ours` in `.gitattributes` keeps the local copy, and the next `gen flows` rewrites it. A `switch` over a `Flow<id>Outcome` lists every case, with no `default:`, so a new backend outcome breaks the app build until the app handles it.
