---
name: verify-backend
description: Explains what monacoctl verify checks, how to read a failed ci / e2e job and its verify-evidence artifact, and what counts as a pass. Use when the E2E job fails in the merge queue, when a flow moves to verified, or when reproducing an end-to-end failure.
---

# Verify backend

`monacoctl verify` runs the real `api` and `worker` binaries against a throwaway Postgres, an embedded NATS server and the fakes server. It drives each flow the way the app would and writes down what the system did. Unit and flow tests prove the code. `verify` proves the built binaries.

`feature-map.md` in this directory lists every flow, each outcome's test, and the command that verifies it. It is generated from `apps/backend/flows.tsv`.

## Who runs it

- **The merge queue.** Stage 2 runs the `e2e` job in `.github/workflows/ci-jobs.yml`. It shows as `ci / E2E (monacoctl verify on the real binaries)`. The job runs `scripts/ci/e2e.sh`, which runs `verify all` and then `verify all --crash-at after-publish`. It uploads the `.verify` directory under `apps/backend` as the `verify-evidence` artifact, kept for 7 days.
- **Nightly.** `scripts/ci/nightly-backend.sh` runs `verify all` and uploads `nightly-verify-evidence`.
- **Owners do not run it.** Stage 0 is `go run ./cmd/monacoctl agents check`. The agent guard in `scripts/agent-guard.py` blocks heavy test runs in owner worktrees. The operator or the root agent may reproduce a failure locally.

## What one run does

The code is in `apps/backend/cmd/monacoctl/verify/`.

1. Builds `./cmd/api`, `./cmd/worker` and `./cmd/fakes` with `-cover`, adding `-tags faultpoints` for a crash run.
2. Starts a Postgres container named `monaco-verify-<run id>` on a random port with data on tmpfs. It never touches `monaco-postgres`.
3. Starts NATS, applies the atlas migrations, and starts the binaries.
4. Runs each selected outcome's flow script from `apps/backend/internal/testkit/flows/scripts.go`, up to 4 at a time. A crash run goes one at a time.
5. Waits for every emitted event to be handled by every consumer that watches it.
6. Checks the invariants below, writes evidence, and tears down.

Only flows at `built` or `verified` run. Without `--crash-at`, crash outcomes are skipped.

## What it checks

Per outcome, in `apps/backend/cmd/monacoctl/verify/invariants.go` and `apps/backend/cmd/monacoctl/verify/converge.go`:

- **Convergence.** Every handler that watches an emitted event handled it, and no consumer has pending or unacked messages.
- **Response.** `ok` gets a 2xx. A code outcome such as `InvalidInput` gets that code's HTTP status and `code`.
- **Log lines.** `http.request` with method and route. `http.problem` with the code for a code outcome. Otherwise a relay tick and one dispatched line per watching handler. Each line carries its required attrs.

Per run:

- **No dead letters.** The dead-letter stream is empty.
- **No internal errors.** No log line carries an internal-kind `errs` code.
- **Ledger checks.** `verify.LedgerChecks` holds the treasury ledger check. It asserts that every header sums to zero per asset, that both headers of a transfer share a status, and that the cabal and user positions, cost basis included, equal their entries. The balance comparison with the money events joins it when the first money event registers a balance rule.

## Budgets

`DefaultBudget` in `apps/backend/cmd/monacoctl/verify/budget.go` caps the whole run at 90 s. The phase caps are stack 10 s, seed 2 s, each flow 15 s, convergence 30 s and teardown 5 s. A phase over its cap fails the run with that phase named, even when the total is under 90 s. A change to `budget.go` is a gate change for a human to review. Fix the slow code instead.

## Read a failed e2e job

1. Find the failing run. Open the `ci / E2E ...` check from the merge queue failure, or run `gh run list --workflow ci.yml --event merge_group`.
2. Read the log with `gh run view <run-id> --log-failed`. Each unit prints `PASS flow <id> <outcome>` or `FAIL flow <id> <outcome>` with the failure. A usage error exits 2, and any failure exits 1.
3. Download the evidence with `gh run download <run-id> -n verify-evidence`.
4. Open `<id>.json` for the flow, or `<id>-crash-<point>.json` for a crash run.

| Field | Read it for |
| --- | --- |
| `result` | `pass`, `fail` or `over_budget`. `over_budget` wins over `fail`. |
| `phase`, `error` | Which phase ran out of budget, or the run error. |
| `commit`, `dirty` | The tree it ran against. Evidence from a dirty tree does not count. |
| `outcomes[].failure` | The first failed invariant for that outcome. |
| `outcomes[].exchanges` | Each HTTP request and response, with secrets redacted. |
| `outcomes[].events` | Each event row, whether it was published, and its deliveries. |
| `outcomes[].log_lines` | The required log lines that were found. |
| `consumers` | Per durable: delivered, ack floor, ack pending, redelivered. A stuck consumer shows pending or unacked messages here. |
| `dead_letters` | Messages a handler termed. |
| `latency_ms`, `phases_ms`, `host` | Where the time went, and the machine's load when it ran. |

A run error marks every flow's file, so read `error` before blaming one flow.

## Reproduce locally (operator or root only)

From `apps/backend`, with Docker running and the pinned atlas installed by `scripts/install-atlas.sh`:

```
go run ./cmd/monacoctl verify flow 00
go run ./cmd/monacoctl verify flow 00 --outcome InvalidInput
go run ./cmd/monacoctl verify flow 00 --crash-at after-publish
go run ./cmd/monacoctl verify all
```

`--outcome crash:<point>` implies `--crash-at <point>`, and the point must be a registered fault point. Evidence lands in the `.verify` directory under `apps/backend`, which git ignores.

## What counts as a pass

- Every unit prints `PASS`, and the process exits 0.
- Every evidence file has `"result": "pass"` and `"dirty": false`.
- The run covered every outcome of every `built` or `verified` flow, including its crash points. `scripts/ci/e2e.sh` runs only `after-publish`. A flow that adds another crash point adds its `verify all --crash-at <point>` line there.

## On failure

Fix the code. Never weaken an invariant, raise a budget, skip an outcome, drop a consumer from `flows.tsv` or edit an evidence file to get green. If the invariant itself is wrong, that change is its own PR with its own reason.

## Moving a flow to verified

1. Write one script per outcome in `apps/backend/internal/testkit/flows/f<id>.go`. Make the flow tests call the same scripts.
2. Register them in `Scripts()` in `apps/backend/internal/testkit/flows/scripts.go`.
3. Set the row's status to `verified` and regenerate the feature map:

```
go run ./cmd/monacoctl docs flows --feature-map > ../../.claude/skills/verify-backend/feature-map.md
```

`scripts/ci/ready.sh` fails when the feature map is stale.
