# Flows

`apps/backend/flows.tsv` is the flow contract for the backend and the app. This package is the app's side of it. It records, for each flow, which screen carries it, how far the app has got, and where its doc lives. `monacoctl flows check` joins it to `flows.tsv` on `id` and fails on any mismatch.

## Layout

| Path | Contents |
| --- | --- |
| `app/<id>.tsv` | One registry file per flow. |
| `Package.swift` | The `MonacoFlows` Swift package. Arrives in #1664. |
| `Sources/MonacoFlows/Flow<id>.gen.swift` | The outcome enum for one flow. Generated. Arrives in #1664. |

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
