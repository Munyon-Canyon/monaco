# CI

How Monaco runs continuous integration: which checks run, when they run, on what machines, and what it costs. CI is the last line of verification for the whole repo, so it must be deterministic, fast, and small enough that nobody is tempted to bypass it. The checks themselves are defined in [backend-platform.md, CI gates](backend-platform.md#ci-gates) and [Testing](backend-platform.md#testing); this file decides how they are scheduled and paid for.

## Decision

1. **CI confirms; it does not discover.** Every check a PR needs runs on the laptop first: `just test backend` (under 60 s), the lint pre-commit hook, and `just verify backend` behind the pre-PR hook. CI reruns the same commands on a clean machine to prove the result does not depend on the author's machine. A red CI run on a ready PR is a bug in the local gate, not a normal step.
2. **Full CI runs only on PRs that can merge.** A PR runs CI when it is not a draft and its base is `main` or a milestone feature branch (`backend-rewrite*`). Drafts run nothing. Upstack PRs in a Graphite stack run nothing until Graphite retargets them to their trunk.
3. **No CI on push to `main`.** Branch protection requires the branch to be up to date before merging, so the PR run already tested the tree that lands. The nightly run covers `main`.
4. **One required check.** A final `ci-ok` job aggregates every other job with `re-actors/alls-green`. It is the only check branch protection names, so jobs can be added, split or path-filtered without touching the protection rule.
5. **Filter jobs, never workflows.** Path and draft filters go in job-level `if:`. A workflow skipped by a `paths:` filter leaves its required check pending forever; a job skipped by `if:` reports success.
6. **Linux by default, macOS only for Xcode.** `packages/mobile-core` host tests run on Linux, which it already supports. macOS runs only the iOS app build and `MonacoTests`, and only when iOS paths change.
7. **Stay on GitHub's free hosted runners while the repo is public.** They are free and unmetered for public repos. [Staying on the free tier](#staying-on-the-free-tier) lists the rules that keep it free, and [Runner options](#runner-options) says what to switch to if the repo goes private or the macOS queue gets long.
8. **Nightly runs the unbounded suites**, and skips the night when `main` has not moved since the last green nightly.

## Why

**The 2,000-minute limit does not apply today.** `Munyon-Canyon/monaco` is public. Standard GitHub-hosted runners, macOS included, are free with no minute cap in public repos ([GitHub billing](https://docs.github.com/en/billing/concepts/product-billing/github-actions)). The Free plan's 2,000 minutes and 500 MB of artifact storage apply only to private repos. What does bind a public repo is concurrency: 20 jobs at once, and only 5 of them on macOS ([limits](https://docs.github.com/en/actions/reference/limits)).

**It would apply the day the repo goes private.** Measured from the last 100 workflow runs (2026-09-23 to 2026-09-27, 3.4 days), with each job rounded up to a whole minute as GitHub bills it ([pricing](https://docs.github.com/en/billing/reference/actions-runner-pricing)):

| Job | Runner | Runs | Average | Billed minutes |
| --- | --- | --- | --- | --- |
| `go` | Linux | 89 | 0.7 min | 104 |
| `changes` | Linux | 57 | 0.1 min | 55 |
| `web` | Linux | 57 | 0.1 min | 55 |
| nightly `alert` | Linux | 11 | 0.1 min | 11 |
| `swift` (mobile-core) | macOS | 89 | 0.8 min | 126 |
| `ios` | 2 | macOS | 57 | 2.0 min | 124 |
| nightly `qa` | macOS | 11 | 13.1 min | 152 |

At that pace a 30-day month is about 2,000 Linux minutes and 3,500 macOS minutes (extrapolated, not measured). As a private repo that exhausts the free quota on Linux alone and adds about $220 a month of macOS at $0.062 per minute. Two findings drive the decisions above:

- Half the Linux bill is rounding. `changes`, `web` and `alert` run for about 6 seconds each and bill a full minute each.
- The `swift` job pays macOS rates for tests that run on Linux, and it runs on every PR, including Go-only ones.

**Most runs were not merge candidates.** 230 of the 284 runs in the last month were `pull_request` runs, which fire on every push to every open PR, drafts and upstack PRs included. Gating on "ready and based on `main`" removes those runs without removing any check from a PR that can merge.

## What runs where

Each check runs in exactly one tier as its gate, and in the tier below it only when it is cheap enough.

| Tier | When | Runs | Budget |
| --- | --- | --- | --- |
| Local, every save or commit | pre-commit hook, `just test backend` | `golangci-lint` on changed packages, unit + integration + acceptance, fixed-seed property and jitter tests, fuzz seeds | 60 s |
| Local, before PR | pre-PR hook (`scripts/agent-guard-pr.sh`) | `just verify backend` for every flow the branch touches; fresh evidence stamped with `HEAD` | 90 s |
| Stage 1, PR check | non-draft PR (`pull_request`) | the repo-wide checks below, no tests | 2 min |
| Stage 2, queue check | merge queue entry (`merge_group`) | every job below | 6 min backend-only, iOS adds about 12 |
| Nightly | 07:00 UTC, only if `main` moved | everything unbounded | 180 min |

CI jobs, from the [Testing](backend-platform.md#keeping-it-fast) budget. The Stage column says which [check stage](#check-stages) runs the job. Stage 2 also runs every stage 1 job.

| Job | Stage | Runner | Runs when | Contents |
| --- | --- | --- | --- | --- |
| `plan` | 1 | Linux | always | Checks out once, decides [stage 1 reuse](#check-stages), runs `dorny/paths-filter` and actionlint. In stage 2 it also runs the landing page's `npm test` when `apps/web/**` changed. Folding `web` in here removes one billed minute per run. |
| `lint` | 1 | Linux | backend or CI files changed | `golangci-lint`, nogo, `monacoctl lint comments`, `vacuum lint`, `atlas migrate lint`, and `oasdiff breaking` against the PR's base (the feature branch in the queue). No Postgres. |
| `backend` | 2 | Linux | backend or CI files changed | `scripts/test-backend.sh` (`go test -race -shuffle=on -short` with `goleak`, `monacoctl test-report` with the per-package time budget, merged coverage at 100%, `monacoctl flows check` on the `go test -json` output), then the tests `-short` skips because they build or run other binaries. Under 3 min, sharded by package if it outgrows that. |
| `ready` | 1 | Linux | backend or CI files changed | `scripts/ci/ready.sh`, which the laptop runs the same way on a committed tree: `go vet` on go1.25.14 (so a newer local Go cannot hide a stdlib API that go.mod's version lacks), `go mod tidy -diff`, `go generate` and `scripts/gen-docs.sh` with no change to the tree after, `sqlc diff`, and `monacoctl flows check`. No Postgres, so it reports in about a minute. |
| `e2e` | 2 | Linux | backend changed | Real `api` and `worker` binaries on compose (Postgres, NATS, fake externals), crash-point tests, `verify-backend` evidence for touched flows. Under 4 min. |
| `flake` | 2 | Linux | backend tests or `scripts/ci/flake-tests.sh` changed | `scripts/ci/flake-tests.sh` reruns the changed test files 20 times against the base. |
| `scripts` | 2 | Linux | `scripts/` Go tests or the files they read changed: top-level `scripts/*` except `cloud-setup.sh`, `scripts/githooks/**`, `scripts/testdata/**`, `Justfile`, `.claude/settings.json`, `.claude/hooks/**`, `ci-scripts.yml` | `go test -short ./...` in the `scripts` module (the PR size and format guards, the agent guard, the staged-lint hook, the Justfile tests), called from its own reusable file `ci-scripts.yml`. |
| `mobile-core` | 2 | Linux (`swift` container) | `packages/mobile-core/**` or `ci-mobile-core.yml` changed | `swift test`, called from its own reusable file `ci-mobile-core.yml`. |
| `ios` | 2 | macOS | `apps/mobile/**`, `packages/mobile-core/**`, the iOS scripts, or `ci-ios.yml` changed | `build-for-testing`, `MonacoTests`, sample-screen manifest check, called from its own reusable file `ci-ios.yml`. This build also compiles `mobile-core` on Darwin, which covers the Darwin-only code paths the Linux job skips. |
| (any job) | | | a changed file no filter owns and no inert rule names | Every job above runs. The `plan` job's second filter step (`predicate-quantifier: every`) computes `unknown`: all changed files minus every path a job filter owns and minus the no-check rows of [Verification scope](backend-platform.md#verification-scope) (`docs/**`, `**/*.md`, `.claude/**`, `.cursor/**`, `.github/**` for actionlint, `scripts/cloud-setup.sh`, `.env.local`). A new top-level file or app therefore runs everything until someone adds it to a filter, instead of passing `ci-ok` with nothing tested. |
| `ci-ok` | 1 | Linux | always, `if: always()` | `re-actors/alls-green` over every job above, with path-skipped and stage-skipped jobs in `allowed-skips`. In stage 1 it then records the diff's patch ID in its own check run's output summary. The only CI check the rulesets require. |

The legacy backend, its migrations, its Go domain package and the reference bot were deleted in M7 before the new scaffold. Until [Rollout](backend-platform.md#rollout) step 1 adds the gates above, `backend` runs `go vet` and `go test -race` on the scaffold.

Nightly runs `scripts/qa/night.sh` as it does today ([Overnight QA](../how-to/overnight-qa.md)), plus the Linux `warm-cache` job described under [Fast and deterministic](#fast-and-deterministic), plus the unbounded backend suites the RFC assigns to it: 100,000 property cases, `-fuzz` for 10 minutes per target, the tests `-short` skips, a seed sweep (`-count=20 -short`, new `testkit.RandSeed` seeds each pass), and `benchstat` against the last nightly with an alert on a regression over 10% at p < 0.05. `scripts/ci/nightly-backend.sh` runs all of them in the nightly `backend` job, which keeps `bench.txt` as an artifact for the next night. The nightly `mutation` job calls the same `mutation.yml` with `all: true`: one job per package runs `monacoctl mutation --all --pkg <dir>` on every line, with `timeout-minutes: 60`, because `db`, `bus`, `httpx` and `httpx/sse` each take more than 10 minutes. A failed package fails the night like a failed backend suite.

## Check stages

A change passes three check stages. Each stage runs only what the stage before it skipped.

| Stage | Where | Runs |
| --- | --- | --- |
| 0. Agent check | The owner's worktree, before each push | `go build`, `go vet` and `go test -short -count=1` on the packages `monacoctl ci affected --base <feature branch>` prints. |
| 1. PR check | CI on `pull_request`, `stage: pr` | The stage 1 jobs in [What runs where](#what-runs-where): lint, `ready` and PR format. No tests. |
| 2. Queue check | CI on `merge_group`, `stage: queue` | Every job: stage 1 plus the full race suite, the tests `-short` skips, `flake`, `scripts`, and `mobile-core` and `ios` when their paths changed. |

`ci.yml` sets the `stage` input of `ci-jobs.yml` from `github.event_name`: `merge_group` is `queue`, and every other event is `pr`. `ci-retarget.yml` always passes `pr`. Mutation testing runs only in the nightly.

`monacoctl ci affected --base <ref>` prints the packages that `<ref>...HEAD` changes, plus every package that imports one of them, one per line. A package whose tests import a changed package also counts. The command reads reverse dependencies from `go list -deps -json ./...`. It prints `./...` when `go.mod`, `go.sum`, a file under `internal/testkit/`, or any file outside a Go package changed.

Stage 1 reuses a green result for an unchanged diff:

1. `ci / Plan` computes the patch ID of the PR's diff: `git diff <base>...<head> | git patch-id --stable`. A restack that leaves the diff unchanged keeps the patch ID.
2. It finds this PR's last green `ci / ci-ok` and reads the patch ID stored in that check run's output summary.
3. If the two patch IDs match, every stage 1 job skips and `ci-ok` passes.
4. `ci-ok` stores the current patch ID in its own output summary.

Stage 2 never reuses a result. A stage 2 failure removes only the failing entry from the queue. With `grouping_strategy: ALLGREEN`, GitHub then builds the group again without that entry, and the other entries still merge.

## Triggers

```yaml
# ci.yml
on:
  pull_request:
    branches: [main, 'backend-rewrite*']
    types: [opened, synchronize, reopened, ready_for_review]
  merge_group:
  workflow_dispatch:

concurrency:
  group: ci-${{ github.event.pull_request.number || github.ref }}
  cancel-in-progress: true

jobs:
  ci:
    if: ${{ !github.event.pull_request.draft }}
    uses: ./.github/workflows/ci-jobs.yml
    with:
      stage: ${{ github.event_name == 'merge_group' && 'queue' || 'pr' }}

# ci-retarget.yml
on:
  pull_request:
    branches: [main, 'backend-rewrite*']
    types: [edited]

jobs:
  ci:
    if: github.event.changes.base != null && !github.event.pull_request.draft
    uses: ./.github/workflows/ci-jobs.yml
    with:
      stage: pr
```

- The jobs live in `ci-jobs.yml`, a reusable workflow. Both callers name their job `ci`, so every check reads `ci / <job>` and the required check is `ci / ci-ok`. `mobile-core` and `ios` are themselves calls to their own reusable files (`ci-mobile-core.yml`, `ci-ios.yml`), so their checks read `ci / mobile-core / <job>` and `ci / ios / <job>`. That split lets the `plan` job's path filter key each one on its own job-definition file instead of every `ci*.yml`, so a backend PR that only edits `ci-jobs.yml` (nearly every one, since each adds its own steps there) no longer runs the macOS `ios` job or the Linux `mobile-core` job. `ci-ok` still names only job IDs (`plan`, `go`, `mobile-core`, `ios`) in `needs`, so the required check and `allowed-skips` are unaffected by the extra nesting level.
- `branches: [main, 'backend-rewrite*']` matches the PR's base, so only the bottom PR of a stack runs, whether its trunk is `main` or a feature branch.
- `merge_group` runs the jobs on the feature branch's merge queue group commit. Every workflow behind a required check must run on `merge_group`, or the queue waits out its 30-minute timeout. `pr-format.yml` also runs on it, and its `PR format (title, body and commits)` job passes without checking, since each PR passed it on its own head. `scripts/ci/workflow_triggers_test.go` fails if a workflow with a required job filters on `paths` or `paths-ignore` at the workflow level. The Plan job runs it whenever a workflow changes.
- A draft skips the `ci` job. That leaves one skipped check named `ci` and no `ci / ci-ok`, so the PR cannot merge until it is ready and CI passes. GitHub counts a skipped job as a passing required check, so the gate never depends on a skip.
- Inside `ci-jobs.yml`, `ci-ok` uses `always()`, not `!cancelled()`. A run cancelled by a newer push then leaves a failed `ci-ok`, not a skipped one.
- Graphite restacks an upstack PR while its base is a temporary `graphite-base/N` branch, then retargets it to `main` with no new push (seen on #452). `ci.yml` sees neither event. `ci-retarget.yml` runs on the retarget and runs the same jobs.
- GitHub counts only the newest run of each workflow on a commit. A title or body edit after a retarget, with no push in between, makes a skipped `ci-retarget.yml` run the newest one, and `ci / ci-ok` goes back to "Expected". Rerunning the retarget run does not help. Push the branch to run `ci.yml`. A `ci / ci-ok` from `ci.yml` survives later edits (probed on #648 and #718).
- `edited` is not in `ci.yml`. An edit run there shares the concurrency group, cancels the real run, and leaves a skipped `ci-ok` as the newest check (seen on #493). In `ci-retarget.yml` a title or body edit skips the caller job and creates only a skipped `ci` check.
- `workflow_dispatch` checks do not satisfy a required check (a probe ruleset on #501 stayed blocked with a green dispatched `ci-ok`). Dispatch is only for looking at results on a draft or an upstack branch. `dorny/paths-filter` has no PR base on a dispatch run, so with no `base:` input it falls back to its documented default for non-`pull_request` events: `git merge-base` against the repository's default branch (`main`). The `filter` step in `ci-jobs.yml` sets no `base:`, so a dispatch run already gets that default and does not run every job unfiltered; no change was needed here.
- `pull_request` does not run while a PR has a merge conflict. Resolve the conflict to get CI.

Ruleset on `main`:

- Require `ci / ci-ok` from GitHub Actions (integration 15368), so a commit status with the same name from another source does not count.
- Require branches to be up to date before merging.
- Block force pushes and deletion.
- Require approval before running workflows from all outside contributors (`fork-pr-contributor-approval`), not only first-time ones.
- Require a pull request, and allow only squash merges, so `main` gets one commit per checkpoint. `main` has no merge queue. Only the operator merges into it, by hand.

## Feature branches

A milestone lands on a feature branch (`backend-rewrite-3` today) through small ticket PRs. The operator then merges the feature branch into `main` as one checkpoint PR labeled `integration`.

`scripts/feature-branch.sh` sets a feature branch up:

- `init <name>` creates `<name>` from `origin/main`, then runs `apply`.
- `apply <name>` adds `<name>` as a Graphite trunk (`gt trunk --add`), turns on the repo's auto-merge and "Allow merge commits" settings, and creates or updates two rulesets: `feature branch <name>` and `main`.
- `ruleset <name>` and `main-ruleset` print those rulesets. `scripts/feature_branch_test.go` checks both.

The feature branch ruleset:

- Requires `ci / ci-ok` and `PR format (title, body and commits)` from GitHub Actions (integration 15368). The `verify` commit status is not a ruleset check. The agent guard hook refuses `gh pr merge` until the head's latest `verify` status is `success`, from any poster.
- Lands every PR through a merge queue (`merge_queue` rule):
  - `merge_method: MERGE`. A merge commit keeps a stack's commits unchanged, so GitHub marks the lower PRs of a stack as merged when the top one lands.
  - `grouping_strategy: ALLGREEN`. Each entry lands only when its group passes.
  - Up to 5 entries build in parallel (`max_entries_to_build: 5`), and a group merges 1 to 5 entries (`min_entries_to_merge: 1`, `max_entries_to_merge: 5`) with no wait (`min_entries_to_merge_wait_minutes: 0`), so an entry starts testing as soon as it is queued.
  - A required check that does not report within 30 minutes (`check_response_timeout_minutes: 30`) drops the entry.
- Does not require branches to be up to date. The queue tests each entry on top of the tip and the entries ahead of it, which replaces that rule.
- Requires a pull request and allows only merge commits. Nobody pushes directly, admins included.
- Has one bypass actor, org admins (`OrganizationAdmin`), for the checkpoint merge-back below. GitHub rejects GitHub Actions as a bypass actor on this repo (422, "must be part of the ruleset source or owner organization").
- Blocks force pushes and deletion.

A ticket PR lands with `gh pr merge <n> --auto` once its verdict passes. GitHub adds it to the queue when its own checks pass. The queue builds a `gh-readonly-queue/<branch>/...` commit, runs the required checks on it through `merge_group`, and merges it when they pass. A failing entry leaves the queue, and the entries behind it rebuild without it.

A Graphite stack lands as one entry through `monacoctl agents land-stack <top-pr>` ([Pull requests](backend-platform.md#pull-requests-small-and-stacked)). It changes the upper PRs' bases to the feature branch and queues only the top PR, so stage 2 runs once for the stack. The merge commit keeps every commit of the stack, and GitHub marks the lower PRs merged.

After the checkpoint PR squash-merges into `main`, `checkpoint.yml` runs two jobs:

1. `tree-matches` runs `scripts/ci/checkpoint-tree.sh` and fails unless `main`'s squash commit has the same tree as the PR's head.
2. `merge-back` merges `main` back into the feature branch with `git merge -s ours` and pushes it with the `MERGE_BACK_TOKEN` repo secret, a fine-grained PAT from an org admin with `contents: write`, so the push bypasses the ruleset. Without the secret the job fails and names it. The trees match, so the merge changes no file and only records `main` as merged. The next checkpoint PR then shows only the new work. If `tree-matches` fails, `merge-back` does not run.

## Fast and deterministic

- **Same commands as the laptop.** Every CI step calls a `just` recipe or a script the laptop also runs. CI YAML holds no test logic, so a CI failure reproduces locally with the same command.
- **No network in PR CI.** Tests use stubs for Jupiter, Privy, Helius and Pyth, as `just test backend` requires today. PR CI has no secrets, so a test that reaches for one fails instead of passing on a live service.
- **Seeds are fixed and printed.** `-shuffle=on` prints its seed. Rapid, fuzz and jitter log theirs on failure. CI sets `RAPID_NOFAILFILE=1` so it never writes to the tree.
- **No retries.** A failed job is not rerun to get green. A flaky test is fixed the same day or moved to nightly with an issue, per the [Testing](backend-platform.md#keeping-it-fast) rule "fixed or moved to nightly, never skipped".
- **Tight timeouts.** Each job's `timeout-minutes` is about twice its budget (`lint` 10 for a cold golangci-lint cache, `backend` 10, `e2e` 8, `mutation` 10 per package, `ios` 20). The default is 360, which lets a hung Postgres burn six hours.
- **Caches.** `actions/setup-go` caches modules and `GOCACHE`. After checkout, reset `testdata` mtimes to a fixed date (`find . -path '*/testdata/*' -exec touch -t 200001010000 {} +`), because `go test` keys its result cache on file mtimes and a fresh checkout otherwise misses every time ([golang/go#58571](https://github.com/golang/go/issues/58571)). `golangci-lint-action` caches its own analysis. A PR can restore caches saved on `main` but not caches from other PRs, and with no push-to-`main` run nothing would save them. So nightly gets a small Linux `warm-cache` job on `main` that builds and runs the `go` job's tests with the same flags, and it is the only job that saves caches. Running the tests, not only compiling them (`go test -run '^$'`), is what puts test results in the cache: `-run` is part of the result cache key, so a compile-only run never makes a PR's `go test` print `(cached)`. The `go` job's `DATABASE_URL` carries an `application_name` unique to the run. `go test` keys cached results on the env vars a test reads, so a test that uses Postgres always reruns against the migrations in the PR, and only Postgres-free packages are cached. PRs restore and never save. The `ios` job caches Swift packages only. A DerivedData build-product cache was tried and measured worse, not better: restoring it and validating it against a freshly checked-out tree (even with each file's mtime set back to its last commit time) took 9m38s, against 5m45s for a plain from-scratch build with the same build flags. DerivedData is rebuilt from scratch every run.
- **Few, larger jobs.** Each job pays checkout, setup and a whole-minute rounding. Split a job only when the parallelism shortens time to a result.

## Staying on the free tier

Monaco runs CI on GitHub's standard hosted runners at no cost, because the repo is public. That holds only while these rules hold:

- **Standard runner labels only.** Jobs use `ubuntu-latest`, `ubuntu-24.04`, `ubuntu-24.04-arm` or `macos-15`. Larger runners (more cores, GPU, `-xlarge` macOS) are billed even on public repos, and the plan's free minutes do not cover them. `scripts/ci/check-runners.sh` fails the `plan` job when a workflow names any other label, so this rule does not depend on review.
- **Concurrency is the limit, not minutes.** The Free plan runs at most 20 jobs at once, and only 5 of them on macOS. Busy days show up as queued `ios` jobs, not as a bill. The gating in [Decision](#decision) exists as much to keep that queue short as to save minutes. If macOS jobs regularly wait more than 5 minutes to start, move `ios` to Namespace or Xcode Cloud ([When to switch](#runner-options)).
- **Artifacts and caches stay small.** Artifacts expire after 7 days on PRs and 14 on nightly. The Actions cache is capped at 10 GB per repo, and GitHub evicts the oldest entries past that, so only the nightly `warm-cache` job saves caches.
- **Public means forks.** The [Security](#security) rules apply: no secrets in PR workflows, no `pull_request_target`, no self-hosted runner.
- **Making the repo private is a CI decision.** At the measured pace the current workflows would use the 2,000 free minutes in about two weeks. Before changing visibility, land the [Rollout](#rollout) steps and the "Repo goes private" row of the switch table in the same week. Setting a $0 Actions spending limit on the account makes an overrun stop CI instead of charging the card.

## Security

The repo is public, so anyone can open a PR from a fork.

- PR workflows use `pull_request`, never `pull_request_target`. Fork PRs then get a read-only token and no secrets.
- PR CI needs no secrets. The iOS build uses the placeholder Privy config (`scripts/ensure-ios-privy-config.sh placeholder`).
- `DOTENV_PRIVATE_KEY`, the relayer keypair and the Privy authorization key never go into a GitHub secret that a PR workflow can read.
- No self-hosted runner while the repo is public. A fork PR can run code on it and leave something behind ([GitHub hardening guide](https://docs.github.com/en/actions/security-for-github-actions/security-guides/security-hardening-for-github-actions#hardening-for-self-hosted-runners)).

## Runner options

Prices checked 2026-09-27 against vendor pages and GitHub's pricing reference. Items marked "unverified" came from third-party comparisons only.

| Option | Free tier | Linux $/min | macOS $/min | Drop-in `runs-on:` | Notes |
| --- | --- | --- | --- | --- | --- |
| **GitHub-hosted** | Unlimited on public repos; 2,000 min/month on private (Free plan) | 0.006 (x64), 0.005 (arm64) | 0.062 | baseline | 5 concurrent macOS jobs. Larger runners are billed even on public repos. |
| **Blacksmith** | 3,000 min/month | 0.004, arm 0.0025 | 0.08 | yes | Best free tier for Linux. Docker layer cache costs extra. |
| **Namespace** | 30-day trial, then pay as you go | about 0.003 for 2 vCPU (unverified) | 0.06 | yes | Cheapest hosted macOS found, slightly under GitHub. |
| **Ubicloud** | $2.50/month credit (about 1,250 min) | 0.00125 to 0.002 | none | yes | Cheapest Linux. Prices rose about 26% in mid-2026. |
| **Depot** | none; $20/month plan includes 2,000 min | 0.004 | about 0.08 | yes | Strongest at Docker image builds, which Monaco barely has. |
| **WarpBuild** | unverified | 0.004, arm 0.003 | 0.08 | yes | Linux, macOS and Windows from one vendor. |
| **RunsOn** | none | own AWS EC2 cost plus a license from €300/year | none | yes | Only pays off at high volume. No macOS. |
| **Xcode Cloud** | 25 compute hours/month with the Apple Developer Program | none | 100 h for $49.99/month | no, Apple workflows | Free fit for the `ios` job, since the team already pays for the program. Cannot run Go or Postgres. |
| **Codemagic** | 500 macOS min/month, personal accounts only | none | pay as you go | no | Free minutes do not apply to team accounts. |
| **Bitrise** | about 150 macOS min/month (Hobby) | none | credits | no | Mobile only. |
| **CircleCI** | about 3,000 Linux min/month | about 0.006 | not on the free plan | no | Separate YAML, and no free macOS. |
| **GitLab CI** | 400 min/month; self-hosted runners free | about 0.010 | 6 to 12x multiplier | no | Needs a mirror or a move off GitHub. |
| **Buildkite** | 3 concurrent jobs, 1 user, on your own agents | $0 on own agents | $0 on own Mac | no | Separate YAML. About $30 per user after the free plan (unverified). |
| **Self-hosted Mac mini** | hardware only | n/a | $0 | yes (`self-hosted` label) | Private repo only. Non-admin user, ephemeral runner, no keys on the box. |
| **Hetzner CAX11 + self-hosted runner** | €5.99/month flat | flat | none | yes | Flat price if CI runs all day. You patch it. Private repo only. |
| **`act` locally** | free | n/a | none | n/a | Linux jobs only. `just` recipes already cover this, so it adds nothing. |
| BuildJet | | | | | Shut down 2026-03-31. |
| Cirrus CI | | | | | Shut down 2026-06-01. Cirrus Runners takes no new customers. |

Sources: [GitHub runner pricing](https://docs.github.com/en/billing/reference/actions-runner-pricing), [Blacksmith](https://www.blacksmith.sh/pricing), [Namespace](https://namespace.so/pricing), [Ubicloud](https://www.ubicloud.com/docs/about/pricing), [WarpBuild](https://www.warpbuild.com/pricing), [RunsOn](https://runs-on.com/alternatives-to/github-actions-runners/), [Xcode Cloud](https://developer.apple.com/xcode-cloud/), [Codemagic](https://docs.codemagic.io/billing/pricing/), [CircleCI](https://circleci.com/pricing/), [GitLab](https://docs.gitlab.com/ci/pipelines/compute_minutes/), [Buildkite](https://buildkite.com/pricing/), [Hetzner price change](https://docs.hetzner.com/general/infrastructure-and-availability/price-adjustment/), [BuildJet shutdown](https://buildjet.com/for-github-actions/blog/we-are-shutting-down), [Cirrus CI shutdown](https://www.warpbuild.com/blog/cirrus-ci-shutting-down).

GitHub announced a $0.002 per minute fee for self-hosted runners in December 2025, then postponed it. It is not in effect as of 2026-09-27, and public repos were always exempt ([GitHub](https://github.com/resources/insights/2026-pricing-changes-for-github-actions)).

**When to switch:**

| Trigger | Move |
| --- | --- |
| Repo stays public | Stay on GitHub-hosted. It costs nothing. |
| macOS jobs queue behind the 5-job cap | Move `ios` to Namespace macOS, or to Xcode Cloud for the free 25 hours. |
| Repo goes private | Linux jobs to Blacksmith (3,000 free minutes covers the projected 2,000). `ios` to Xcode Cloud. Nightly `qa` to a self-hosted Mac mini, now allowed because there are no fork PRs. |
| Linux volume outgrows Blacksmith's free tier | Ubicloud, or a Hetzner box at a flat monthly price. |

Every drop-in option is a one-line `runs-on:` change per job, so a switch is one PR and can be reverted the same way.

## Rollout

Each step is one small PR with its own proof.

1. Gate triggers: `branches: [main]`, the `types` and `if:` above, remove the `push` trigger, add `ci-ok`, and make it the only required check. Proof: a draft PR shows no running jobs, marking it ready starts CI, and an upstack PR runs nothing.
2. Add `scripts/ci/check-runners.sh` to `plan`: it lists every `runs-on:` in `.github/workflows/` and fails on a label outside the standard set. Fold `web` into `plan`. Move `swift` to a Linux `swift` container, filtered on `packages/mobile-core/**`. Proof: a Go-only PR runs no macOS job, and a planted `runs-on: ubuntu-latest-4-cores` fails `plan`.
3. Tighten `timeout-minutes`, add the `testdata` mtime reset, add the nightly `warm-cache` job, and stop PRs saving caches. Proof: a second run of an unchanged PR shows cached test results in the `go test` output.
4. Nightly skips when `main`'s head equals the head SHA of the last successful nightly run (`gh run list --workflow nightly.yml --status success --limit 1 --json headSha`). Proof: a manual dispatch on an unchanged `main` exits in the first step.
5. At backend-platform Rollout step 1, replace the `go` job with `backend`, `e2e` and `mutation` as above, and set each budget from the times the scaffold measures in CI.

## Log

- 2026-09-29: `scripts/feature-branch.sh apply backend-rewrite-3` failed with 422: GitHub rejects GitHub Actions (integration 15368) as a ruleset bypass actor. The feature branch ruleset now lets org admins bypass, and `checkpoint.yml` pushes the merge-back with the `MERGE_BACK_TOKEN` secret, an org admin's fine-grained PAT (#831).
- 2026-09-29: Stacks land as one queue entry (#831 F). `monacoctl agents land-stack <top-pr>` retargets the upper PRs and queues the top one, `check-pr-size.py` accepts its combined size through the `Lands stack:` line, and the agent guard hook keeps base changes inside `land-stack`. The checkpoint 3 restack, carry and land-chain scripts are deleted.
- 2026-09-29: Added the feature branch merge queue (#831). The repo moved to the `Munyon-Canyon` organization, so the rulesets API accepts a `merge_queue` rule. `scripts/feature-branch.sh apply` adds it with merge commits, turns on "Allow merge commits", and makes `main` squash-only. `pr-format.yml` runs on `merge_group`. `checkpoint.yml` merges `main` back into the feature branch after a checkpoint, with GitHub Actions as the ruleset's one bypass actor (#789).
- 2026-09-27: Added the `ready` job (#789). The generated-code, reference-doc, sqlc and vet steps moved out of `backend` into `scripts/ci/ready.sh`, which adds `go mod tidy -diff`, a standalone `monacoctl flows check`, and catches new untracked generated files that `git diff --exit-code` missed. `PR format` now also checks the ticket link, `Needs from Logan`, cited SHAs and Conventional Commit subjects.
- 2026-09-27: Added feature branches (#789). `scripts/feature-branch.sh` adds the Graphite trunk and a ruleset that requires `ci / ci-ok` and the verifier App's `verify`, up to date, squash-only PRs, no bypass. `ci.yml` and `ci-retarget.yml` run on PRs into `backend-rewrite*`, and `ci.yml` also on `merge_group`. The rulesets API rejected a merge queue (422), so the up-to-date rule stands in for it. `checkpoint.yml` checks that a checkpoint squash landed the feature branch's exact tree. It does not merge `main` back: that push would need a bypass actor, and GitHub Actions cannot be one here.
- 2026-09-27: The integration PR #786 failed `mutation` (run 36354851240). The `db`, `bus`, `httpx` and `httpx/sse` jobs ran into the 10-minute timeout. The `internal/platform/lint` job failed in 1m20s: gremlins runs that package's tests without `-short`, they call `golangci-lint`, the mutation job does not install it, and with `CI` set the tests fail instead of skipping. That package has only test files, so it has nothing to mutate, and `monacoctl mutation` now leaves such packages out of its list. The `go test` readout from #485 did print the cause in the job log. Only the first line of an error reaches the job's annotation. PRs labeled `integration` now skip mutation. The nightly's full-module mutation moved out of `nightly-backend.sh`, where it ran every package in one step, into the shared `mutation.yml` matrix with a 60-minute limit per package.
- 2026-09-27: Split `mutation` into a matrix with one job per changed package. The checkpoint PR #786 touched 21 packages, and the single job mutated them one after another until its 15-minute timeout cancelled it (run 36353277744). `monacoctl mutation` gained `--list` (the changed packages as JSON) and `--pkg` (mutate one of them). The new `mutation-plan` job feeds the matrix.
- 2026-09-27: Tried and measured `ios` build changes: `ONLY_ACTIVE_ARCH=YES`, `COMPILER_INDEX_STORE_ENABLE=NO`, `-skipMacroValidation -skipPackagePluginValidation` cut a from-scratch `build-for-testing` from 8m09s (#494) to 5m45s. `-parallel-testing-enabled YES` was tried and reverted: it starved the app's own concurrency-sensitive tests of CPU on a shared runner and made two of them flake. A DerivedData build-product cache (keyed on Xcode version, `Package.resolved`, and a source hash, with each file's mtime restored from its last commit) was tried and reverted: restoring and validating it took 9m38s, worse than a plain from-scratch build with the same flags. `packages/mobile-core/**` stays in the `ios` filter: dropping it was considered to shed one macOS job on mobile-core-only PRs, but the Linux `mobile-core` job skips Darwin-only code by design, so a mobile-core change that breaks on Darwin would never be compiled there. mobile-core changes are rare in backend work, so the extra `ios` run costs little.
- 2026-09-27: `mobile-core` and `ios` were the `ios` and `mobile-core` path filters' `.github/workflows/ci*.yml` entry, which matched `ci-jobs.yml` and made both run on nearly every backend PR (each adds its own steps there). Moved both jobs into their own reusable files, `ci-mobile-core.yml` and `ci-ios.yml`, called from `ci-jobs.yml`, and pointed each filter at its own file instead of the glob. Verified `workflow_dispatch` already gets the right base: `dorny/paths-filter` defaults to `git merge-base` against `main` for non-`pull_request` events, so no change was needed there.
- 2026-09-27: Rollout steps 1 to 4 implemented in the #456 stack (#493, #497, #498, #499). The jobs moved into the reusable `ci-jobs.yml`, called by `ci.yml` and by `ci-retarget.yml` (which owns `edited`), so the required check is `ci / ci-ok` and a draft or a title edit leaves no `ci / ci-ok` at all. The ticket's `edited` in `ci.yml` let a body edit on #493 cancel CI and replace a failing `ci-ok` with a passing skip. `ios` timeout 20 minutes (12 to 16 minutes measured over the last 4 green runs). `warm-cache` runs the tests, not only compiles them.
- 2026-09-27: Decided to stay on GitHub's free hosted runners while public. Added the rules that keep it free, a runner-label check, and the steps before going private.
- 2026-09-27: Proposed. Measured a month of runs, found the repo public (free hosted runners) and the per-job rounding and macOS `swift` job as the waste. CI runs only on ready PRs based on `main`, one aggregate required check, Linux-first, nightly skips idle nights. Runner options surveyed.
- 2026-09-27: `ci.yml` runs on every PR, so a stacked PR whose base is another ticket branch gets its own filtered CI. Outside a PR, the path filter diffs against the `FEATURE_BRANCH` repo variable that `scripts/feature-branch.sh apply` sets, not `main`, so a dispatched run on a backend branch skips the iOS build. The feature-branch ruleset no longer requires up-to-date branches.
- 2026-09-27: A job runs only when its own inputs change. The backend jobs no longer run for workflow files or unrelated `scripts/ci/` scripts. A change under `.github/workflows/` or `.github/actions/` runs actionlint in the Plan job instead.
- 2026-09-27: Only the operator merges into `main`, by hand. `main-merges-by-hand.yml` turns auto-merge off on any PR into `main` as soon as someone enables it, and the agent guard blocks `gh pr merge` on a PR into `main`. Auto-merge stays on for feature branches.
- 2026-09-27: The `monaco-verifier` App is optional. The ruleset no longer requires `verify`. The agent guard still requires the latest `verify` status to be `success` before `gh pr merge`, and it accepts any poster.
- 2026-09-28: A changed file that no job filter owns now runs every job (the `unknown` filter), so editing `scripts/test-backend.sh`, `Justfile` or `docker-compose.yml` can no longer pass `ci-ok` with zero tests; those three also joined the `backend` filter. Added the `scripts` job for the `scripts/` Go module, which no PR job ran before. `docs/reference/**` left the `backend` filter: the pages are generated from backend code, whose changes already run the freshness check in `ready`. `docs.yml` deploys on a push to `main` only when its inputs changed. Corrected the `warm-cache` comment: `-shuffle=on` and `-coverpkg` are outside `go test`'s cacheable flags, so PRs reuse compiles, not test results (#814).
- 2026-09-29: Split CI into check stages (#831 B6 to B9). Pull requests run stage 1, the repo-wide checks with no tests, and reuse a green result when the diff's patch ID is unchanged. The merge queue runs stage 2, every job. The `backend` job split into `lint` (stage 1) and `backend` tests (stage 2). `mutation` left PR CI and runs only in the nightly. Added `monacoctl ci affected`.
