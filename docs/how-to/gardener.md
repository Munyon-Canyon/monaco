# Gardener

The gardener reports dead code, lint rules that could tighten and generator drift every night. It has two parts:

1. `monacoctl garden report` checks the backend module and writes `garden-report.md`.
2. The `gardener.yml` workflow runs the report every night at 05:00 UTC and keeps it in one open issue labelled `gardener`.

The gardener opens no PRs. Whoever picks up the `gardener` issue owns the follow-up PRs, and each one follows [Pull requests: small and stacked](../architecture/backend-platform.md#pull-requests-small-and-stacked). The rule behind it is in [Agent-friendly codebase](../architecture/backend-platform.md#agent-friendly-codebase): every correction becomes structure.

## The report

Run it from `apps/backend` on a clean tree:

```sh
cd apps/backend
go run ./cmd/monacoctl garden report --skip-mutation
cat garden-report.md
```

| Check | What it runs | Finding |
| --- | --- | --- |
| Candidate lint | `golangci-lint` with `.golangci.yml` plus `.golangci.candidate.yml` | a violation of a stricter setting or a linter that is not on yet, grouped by linter |
| Dead code | `deadcode -test ./...` at the version `internal/tools/garden` pins, through `go run` | a function nothing reaches from a `main` or a test |
| Generator drift | `go generate ./...`, `sqlc generate` and `scripts/gen-docs.sh`, then `git status` and `git diff` | a file whose committed copy differs from what the generators write |

The report groups findings by check, counts them and gives each one as `file:line` from the repo root. A check that cannot run shows as `failed` with its error, the other checks still run, and the command exits 1.

`.golangci.candidate.yml` holds only what is stricter than the enforced config. The report merges it over `.golangci.yml`: maps merge key by key, lists append, and the candidate's values win. Today it lowers `cyclop` to 10 and `funlen` to 60 lines, enables linters that are off, and keeps `goconst` out of test files.

The drift check changes files, so it refuses a dirty tree. `garden-report.md` is gitignored.

!!! warning
    Always pass `--skip-mutation`. Without it the report also runs `monacoctl mutation --all`, gremlins over every package, which runs every package's tests once per mutant and pegs the machine. Neither the workflow nor a laptop runs that mode. CI runs mutation through `mutation.yml`.

## The workflow

`.github/workflows/gardener.yml` runs on the nightly schedule and on `workflow_dispatch`. GitHub runs schedules from the default branch, `staging`, and the job checks out the trunk that the `FEATURE_BRANCH` repository variable names, also `staging`. The job fails and says so when the variable is unset. It installs the pinned tools the way the other jobs do (`backend-test-env` without the database, the pinned `sqlc` and `golangci-lint`), runs the report with `--skip-mutation`, uploads `garden-report.md` as the `garden-report` artifact, and writes the report into the open `gardener` issue. When no such issue is open, it opens one. It uses only `GITHUB_TOKEN`.

To run it by hand:

```sh
gh workflow run gardener.yml
```
