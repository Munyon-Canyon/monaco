# CI

How Monaco runs continuous integration: which checks run, when they run, on what machines, and what it costs. CI is the last line of verification for the whole repo, so it must be deterministic, fast, and small enough that nobody is tempted to bypass it. The checks themselves are defined in [backend-platform.md, CI gates](backend-platform.md#ci-gates) and [Testing](backend-platform.md#testing); this file decides how they are scheduled and paid for.

## Decision

1. **CI confirms; it does not discover.** Every check a PR needs runs on the laptop first: `just test backend` (under 60 s), the lint pre-commit hook, and `just verify backend` behind the pre-PR hook. CI reruns the same commands on a clean machine to prove the result does not depend on the author's machine. A red CI run on a ready PR is a bug in the local gate, not a normal step.
2. **Full CI runs only on PRs that can merge.** A PR runs CI when it is not a draft and its base is `main`. Drafts run nothing. Upstack PRs in a Graphite stack run nothing until Graphite retargets them to `main`.
3. **No CI on push to `main`.** Branch protection requires the branch to be up to date before merging, so the PR run already tested the tree that lands. The nightly run covers `main`.
4. **One required check.** A final `ci-ok` job aggregates every other job with `re-actors/alls-green`. It is the only check branch protection names, so jobs can be added, split or path-filtered without touching the protection rule.
5. **Filter jobs, never workflows.** Path and draft filters go in job-level `if:`. A workflow skipped by a `paths:` filter leaves its required check pending forever; a job skipped by `if:` reports success.
6. **Linux by default, macOS only for Xcode.** `packages/mobile-core` host tests run on Linux, which it already supports. macOS runs only the iOS app build and `MonacoTests`, and only when iOS paths change.
7. **Stay on GitHub's free hosted runners while the repo is public.** They are free and unmetered for public repos. [Staying on the free tier](#staying-on-the-free-tier) lists the rules that keep it free, and [Runner options](#runner-options) says what to switch to if the repo goes private or the macOS queue gets long.
8. **Nightly runs the unbounded suites**, and skips the night when `main` has not moved since the last green nightly.

## Why

**The 2,000-minute limit does not apply today.** `lognorman20/monaco` is public. Standard GitHub-hosted runners, macOS included, are free with no minute cap in public repos ([GitHub billing](https://docs.github.com/en/billing/concepts/product-billing/github-actions)). The Free plan's 2,000 minutes and 500 MB of artifact storage apply only to private repos. What does bind a public repo is concurrency: 20 jobs at once, and only 5 of them on macOS ([limits](https://docs.github.com/en/actions/reference/limits)).

**It would apply the day the repo goes private.** Measured from the last 100 workflow runs (2026-09-23 to 2026-09-27, 3.4 days), with each job rounded up to a whole minute as GitHub bills it ([pricing](https://docs.github.com/en/billing/reference/actions-runner-pricing)):

| Job | Runner | Runs | Average | Billed minutes |
| --- | --- | --- | --- | --- |
| `go` | Linux | 89 | 0.7 min | 104 |
| `changes` | Linux | 57 | 0.1 min | 55 |
| `web` | Linux | 57 | 0.1 min | 55 |
| nightly `alert` | Linux | 11 | 0.1 min | 11 |
| `swift` (mobile-core) | macOS | 89 | 0.8 min | 126 |
| `ios` | macOS | 57 | 2.0 min | 124 |
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
| PR CI | non-draft PR, base `main` | the jobs below | 6 min wall clock for the first two jobs, 10 min for mutation |
| Nightly | 07:00 UTC, only if `main` moved | everything unbounded | 180 min |

PR CI jobs, from the [Testing](backend-platform.md#keeping-it-fast) budget:

| Job | Runner | Runs when | Contents |
| --- | --- | --- | --- |
| `plan` | Linux | always | Checks out once, runs `dorny/paths-filter`, and runs the landing page's `npm test` when `apps/web/**` changed. Folding `web` in here removes one billed minute per run. |
| `backend` | Linux | backend or CI files changed | `golangci-lint`, `govulncheck`, generated-code freshness (`sqlc diff`, `oapi-codegen` + `git diff --exit-code`), `atlas migrate lint`, `vacuum lint`, `oasdiff breaking` against `main`, `monacoctl lint comments`, changelog check, `go test -race -shuffle=on` with `goleak`, merged coverage at 100%, `monacoctl flows check` on the `go test -json` output. Under 3 min, sharded by package if it outgrows that. |
| `e2e` | Linux | backend changed | Real `api` and `worker` binaries on compose (Postgres, NATS, fake externals), crash-point tests, `verify-backend` evidence for touched flows. Under 4 min. |
| `mutation` | Linux | backend Go changed | `gremlins` on packages affected by the diff (`go list -deps`). Under 10 min, or the PR is too big and gets split. |
| `mobile-core` | Linux (`swift` container) | `packages/mobile-core/**` changed | `swift test`. |
| `ios` | macOS | `apps/mobile/**`, `packages/mobile-core/**` or the iOS scripts changed | `build-for-testing`, `MonacoTests`, sample-screen manifest check. This build also compiles `mobile-core` on Darwin, which covers the Darwin-only code paths the Linux job skips. |
| `ci-ok` | Linux | always, `if: always()` | `re-actors/alls-green` over every job above, with path-skipped jobs in `allowed-skips`. The only required check. |

The legacy backend, its migrations, its Go domain package and the reference bot were deleted in M7 before the new scaffold. Until [Rollout](backend-platform.md#rollout) step 1 adds the gates above, `backend` runs `go vet` and `go test -race` on the scaffold.

Nightly runs `scripts/qa/night.sh` as it does today ([Overnight QA](../how-to/overnight-qa.md)), plus the Linux `warm-cache` job described under [Fast and deterministic](#fast-and-deterministic), plus the unbounded backend suites the RFC assigns to it: 100,000 property cases, `-fuzz` for 10 minutes per target, the jitter seed sweep, full-module mutation, and `benchstat` against the last nightly with an alert on a regression over 10% at p < 0.05.

## Triggers

```yaml
# ci.yml
on:
  pull_request:
    branches: [main]
    types: [opened, synchronize, reopened, ready_for_review]
  workflow_dispatch:

concurrency:
  group: ci-${{ github.event.pull_request.number || github.ref }}
  cancel-in-progress: true

jobs:
  ci:
    if: ${{ !github.event.pull_request.draft }}
    uses: ./.github/workflows/ci-jobs.yml

# ci-retarget.yml
on:
  pull_request:
    branches: [main]
    types: [edited]

jobs:
  ci:
    if: github.event.changes.base != null && !github.event.pull_request.draft
    uses: ./.github/workflows/ci-jobs.yml
```

- The jobs live in `ci-jobs.yml`, a reusable workflow. Both callers name their job `ci`, so every check reads `ci / <job>` and the required check is `ci / ci-ok`.
- `branches: [main]` matches the PR's base, so only the bottom PR of a stack runs.
- A draft skips the `ci` job. That leaves one skipped check named `ci` and no `ci / ci-ok`, so the PR cannot merge until it is ready and CI passes. GitHub counts a skipped job as a passing required check, so the gate never depends on a skip.
- Inside `ci-jobs.yml`, `ci-ok` uses `always()`, not `!cancelled()`. A run cancelled by a newer push then leaves a failed `ci-ok`, not a skipped one.
- Graphite restacks an upstack PR while its base is a temporary `graphite-base/N` branch, then retargets it to `main` with no new push (seen on #452). `ci.yml` sees neither event. `ci-retarget.yml` runs on the retarget and runs the same jobs.
- GitHub counts only the newest run of each workflow on a commit. A title or body edit after a retarget, with no push in between, makes a skipped `ci-retarget.yml` run the newest one, and `ci / ci-ok` goes back to "Expected". Rerunning the retarget run does not help. Push the branch to run `ci.yml`. A `ci / ci-ok` from `ci.yml` survives later edits (probed on #648 and #718).
- `edited` is not in `ci.yml`. An edit run there shares the concurrency group, cancels the real run, and leaves a skipped `ci-ok` as the newest check (seen on #493). In `ci-retarget.yml` a title or body edit skips the caller job and creates only a skipped `ci` check.
- `workflow_dispatch` checks do not satisfy a required check (a probe ruleset on #501 stayed blocked with a green dispatched `ci-ok`). Dispatch is only for looking at results on a draft or an upstack branch. `dorny/paths-filter` then diffs against the merge base with `main`.
- `pull_request` does not run while a PR has a merge conflict. Resolve the conflict to get CI.

Ruleset on `main`:

- Require `ci / ci-ok` from GitHub Actions (integration 15368), so a commit status with the same name from another source does not count.
- Require branches to be up to date before merging.
- Block force pushes and deletion.
- Require approval before running workflows from all outside contributors (`fork-pr-contributor-approval`), not only first-time ones.

GitHub's merge queue would let expensive jobs run once per merge instead of once per push. It is not available to repos owned by a personal account ([docs](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/configuring-pull-request-merges/managing-a-merge-queue)). Revisit it if the repo moves to an organization.

## Fast and deterministic

- **Same commands as the laptop.** Every CI step calls a `just` recipe or a script the laptop also runs. CI YAML holds no test logic, so a CI failure reproduces locally with the same command.
- **No network in PR CI.** Tests use stubs for Jupiter, Privy, Helius and Pyth, as `just test backend` requires today. PR CI has no secrets, so a test that reaches for one fails instead of passing on a live service.
- **Seeds are fixed and printed.** `-shuffle=on` prints its seed. Rapid, fuzz and jitter log theirs on failure. CI sets `RAPID_NOFAILFILE=1` so it never writes to the tree.
- **No retries.** A failed job is not rerun to get green. A flaky test is fixed the same day or moved to nightly with an issue, per the [Testing](backend-platform.md#keeping-it-fast) rule "fixed or moved to nightly, never skipped".
- **Tight timeouts.** Each job's `timeout-minutes` is about twice its budget (`backend` 6, `e2e` 8, `mutation` 15, `ios` 20). The default is 360, which lets a hung Postgres burn six hours.
- **Caches.** `actions/setup-go` caches modules and `GOCACHE`. After checkout, reset `testdata` mtimes to a fixed date (`find . -path '*/testdata/*' -exec touch -t 200001010000 {} +`), because `go test` keys its result cache on file mtimes and a fresh checkout otherwise misses every time ([golang/go#58571](https://github.com/golang/go/issues/58571)). `golangci-lint-action` caches its own analysis. A PR can restore caches saved on `main` but not caches from other PRs, and with no push-to-`main` run nothing would save them. So nightly gets a small Linux `warm-cache` job on `main` that builds and runs the `go` job's tests with the same flags, and it is the only job that saves caches. Running the tests, not only compiling them (`go test -run '^$'`), is what puts test results in the cache: `-run` is part of the result cache key, so a compile-only run never makes a PR's `go test` print `(cached)`. The `go` job's `DATABASE_URL` carries an `application_name` unique to the run. `go test` keys cached results on the env vars a test reads, so a test that uses Postgres always reruns against the migrations in the PR, and only Postgres-free packages are cached. PRs restore and never save. The `ios` job caches Swift packages only; DerivedData is rebuilt from scratch after a fresh checkout anyway.
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

- 2026-09-27: Rollout steps 1 to 4 implemented in the #456 stack (#493, #497, #498, #499). The jobs moved into the reusable `ci-jobs.yml`, called by `ci.yml` and by `ci-retarget.yml` (which owns `edited`), so the required check is `ci / ci-ok` and a draft or a title edit leaves no `ci / ci-ok` at all. The ticket's `edited` in `ci.yml` let a body edit on #493 cancel CI and replace a failing `ci-ok` with a passing skip. `ios` timeout 20 minutes (12 to 16 minutes measured over the last 4 green runs). `warm-cache` runs the tests, not only compiles them.
- 2026-09-27: Decided to stay on GitHub's free hosted runners while public. Added the rules that keep it free, a runner-label check, and the steps before going private.
- 2026-09-27: Proposed. Measured a month of runs, found the repo public (free hosted runners) and the per-job rounding and macOS `swift` job as the waste. CI runs only on ready PRs based on `main`, one aggregate required check, Linux-first, nightly skips idle nights. Runner options surveyed.
