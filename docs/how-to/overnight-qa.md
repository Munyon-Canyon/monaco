# Overnight QA

One runner, `scripts/qa/night.sh`, runs the same way on a laptop (`just qa-night`) and in the
nightly GitHub workflow. PR and Graphite merge queue CI do not build the app. The nightly `qa` job does.

## What runs where

| When | Workflow / job | Runs |
| --- | --- | --- |
| Ready PRs touching the `backend` filter in `ci-jobs.yml` (`apps/backend/**`, the backend install, test and CI scripts, `Justfile`, `docker-compose.yml`) or a file no filter owns; manual dispatch | `ci-jobs.yml` · `lint`, `ready`, `vuln` (Linux); `backend` and `e2e` in the Graphite merge queue | backend checks, by stage in [CI](../architecture/ci.md#check-stages) |
| Every ready PR to `main`, manual dispatch | `ci.yml` · `plan` (Linux) | runner-label check (`scripts/ci/check-runners.sh`), path filters, and `npm test` for `apps/web` when it changed |
| Ready PRs to `main` touching `packages/mobile-core/**` or `ci.yml`; manual dispatch | `ci.yml` · `mobile-core` (Linux, `swift:6.3-noble`) | `swift test` in `packages/mobile-core` |
| A PR that changes `apps/mobile/**` | local, before push | `just build mobile` and `-only-testing:MonacoTests`. CI does not build the app. |
| Nightly 07:00 UTC (03:00 EDT / 02:00 EST) when `main` moved since the last green night, manual dispatch, PRs to `main` touching `nightly.yml` or `scripts/qa/**` | `nightly.yml` · `qa` (macOS) | `night.sh --screenshots`: backend, mobile-core, app build, `MonacoTests`, each sample UI test class, screenshot gallery |
| Nightly when `main` moved, and manual dispatch, on `main` only | `nightly.yml` · `warm-cache` (Linux) | builds and runs the `go` job's tests, then saves the Go module and build cache. It and `go-cache.yml` (on a push to `staging`, `vars.FEATURE_BRANCH`, or `main` that changes `go.mod` or `go.sum`) are the only jobs that save it; PRs restore it and never save. |

Drafts and PRs based on another branch run nothing. The jobs live in `ci-jobs.yml`, so each check reads `ci / <job>`. `ci / ci-ok` sums up every job above and is the one check `main` requires. When Graphite retargets a PR to `main` without a push, `ci-retarget.yml` runs the same jobs. PR and queue CI run no macOS job. The nightly app build uses the placeholder config below, so that job needs
no secret. The backend rewrite defines its checks in [CI gates](../architecture/backend-platform.md#ci-gates). When CI runs, on which runners, and the planned changes to this table are in [CI](../architecture/ci.md).

## Running it locally

```sh
just qa-night                     # one round
just qa-night --until 07:30       # loop until 07:30 local time
just qa-night --rounds 4
just qa-night --only-ui CabalsTabSampleUITests,ProposalFeedSampleUITests
just qa-night --screenshots       # also shoot every sample screen (round 1)
MONACO_QA_IOS_CONFIG=placeholder just qa-night   # what the nightly does
```

| Setting | Default | Meaning |
| --- | --- | --- |
| `MONACO_QA_IOS_CONFIG` | `generate` | `generate`: Privy config from `.env.local`. `placeholder`: compile-only config, no Privy app ID |
| `MONACO_QA_SIM` / `--sim` | "Monaco Night QA" | Simulator UDID; without either, `scripts/resolve-ios-sim.sh` picks one |
| `MONACO_QA_SCREENSHOTS` / `--screenshots` | off | Shoot the screenshot gallery |
| `MONACO_QA_CLASS_TIMEOUT` | 1500 s | Watchdog per UI class |
| `MONACO_QA_BOOT_TIMEOUT` | 180 s | Wait for the simulator to report booted |
| `MONACO_QA_MIN_FREE_SWAP_MB` | 256 | Stop starting UI classes below this free swap |

A round runs one heavy job at a time: backend, mobile-core, app build, `MonacoTests`,
then each sample-data UI class on its own with a screen recording and one retry if the test
runner itself was killed (reported as flaky, not failing).

Only sample-data classes run. They need no backend and no sign-in, and they never move money.
The live-login tests in `MonacoUITests/MonacoUITests.swift` are never run: they need a real OTP
code, so they cannot pass unattended.

## App config

The app's xcconfig includes a generated, gitignored `apps/mobile/Config/Privy.local.xcconfig`
(plus `Privy.local.Info.plist`).

| Mode | Command | Result |
| --- | --- | --- |
| `generate` | `scripts/ensure-ios-privy-config.sh generate` | Real config from `PRIVY_APP_ID` and `PRIVY_APP_CLIENT_ID` in `.env.local` |
| `placeholder` | `scripts/ensure-ios-privy-config.sh placeholder` | Empty app ID: builds, sample screens work, sign-in does not |

`placeholder` refuses to overwrite a real config; delete the file first (`generate` recreates
it). `generate` always replaces a placeholder. Without a config the round records
`ios-config: fail` and skips the app steps.

## Simulator

Locally the runner uses **Monaco Night QA**, slims it with SimSlim and keeps the Mac awake with
`caffeinate`. All three are optional: on a CI runner it logs the simulator
`resolve-ios-sim.sh` picked (newest iOS runtime) and that slimming was skipped.

```sh
udid=$(xcrun simctl create "Monaco Night QA" "iPhone 17" com.apple.CoreSimulator.SimRuntime.iOS-26-3)
simslim on "$udid" --profile scripts/simslim-profile.json
```

Create it on an iOS 18.5+ runtime: on 18.0 SimSlim cannot persist, so the runner re-applies the
profile before every round.

## Screenshot gallery

`scripts/qa/sample-screens.txt` lists every Debug sample-harness screen and its launch flags
(`-Monaco…Sample`, `-MonacoDesignGallery`). `scripts/qa/screens.sh` launches each and saves a
PNG. `screens.sh --check` (run by every capture) fails when a harness flag
or scenario in `apps/mobile/Monaco` has no manifest line: add the line when you add a harness.

## Results

| Where | What |
| --- | --- |
| `.logs/qa/<UTC stamp>/report.md` | One row per step, first failing lines, flaky steps |
| `.logs/qa/<UTC stamp>/` | `*.log`, `clips/*.mp4` per UI class, `screens/*.png`, `failures.tsv` |
| Nightly run page | `report.md` in the job summary |
| Nightly artifacts | `nightly-qa-report` (report, logs, recordings), `nightly-qa-screens` (gallery); 14 days |

`night.sh` exits 1 when a step failed without a passing retry.

## Failure alerts

| Nightly result | What happens |
| --- | --- |
| Fails, no open `nightly-failure` issue | Opens one: run link, failing steps, first failing lines of `report.md` |
| Fails, issue already open | Comments on it with the same details |
| Passes, issue open | Comments "Recovered" with the run link and closes it |

Only the scheduled run and manual runs on `main` touch issues; PR runs print what they would do.
The alert job uses `GITHUB_TOKEN` with `contents: read` and `issues: write`. GitHub disables
schedules after 60 days without repository activity; re-enable from the Actions tab.

## One build at a time

`scripts/qa/xcode-lock.sh <class> <command...>` holds a machine-wide lock around a heavy
local step. Wrap any `xcodebuild` or `swift` build or test you start by hand (or from an
agent) while the overnight run is going, so builds never compete for memory. The class says
which limit applies:

| Class | Wrap | Cost | Lock dir |
| --- | --- | --- | --- |
| `xcode` | `xcodebuild` and simulator runs | 3 to 7 GB each, so one at a time | `/private/tmp/monaco-xcodebuild.lock` (`MONACO_XCODE_LOCK_DIR`) |
| `swiftpm` | `swift build` and `swift test` in `packages/mobile-core` | about 1 to 1.5 GB each, and every core while cold | `/private/tmp/monaco-swiftpm.lock` (`MONACO_SWIFTPM_LOCK_DIR`) |

The classes are separate locks, so one `xcode` holder and one `swiftpm` holder run at the
same time. Go, lint and shell steps need no lock. A call with no class, such as
`scripts/qa/xcode-lock.sh xcodebuild ...`, runs as `xcode`.

```sh
scripts/qa/xcode-lock.sh xcode xcodebuild -project apps/mobile/Monaco.xcodeproj -scheme Monaco build
scripts/qa/xcode-lock.sh swiftpm bash -c 'cd packages/mobile-core && swift test'
```

Waiters take the lock in arrival order. Each writes a ticket in `<lock dir>.queue/` and
goes when its ticket is the oldest one whose process is still alive, so a later arrival
never overtakes it. Every 60 seconds a waiter logs where it stands:

```text
xcode-lock(xcode): queue position 2 behind pid 4121 (/Users/dev/monaco/.worktrees/1241) (120s)
```

The holder's `pid` and `cwd` are in the lock dir. A lock or ticket whose pid is gone is
taken over or dropped.

| Variable | Default | Meaning |
| --- | --- | --- |
| `MONACO_LOCK_HOLD` | 1800 | Seconds the command may run. Past it the command is stopped, the script prints `command exceeded the <n>s hold cap` and exits 124. |
| `MONACO_XCODE_LOCK_TIMEOUT` | 5400 | Seconds a waiter waits before it gives up with exit 75. |
| `MONACO_LOCK_POLL` | 2 | Seconds between checks while waiting. |

The cap uses `timeout`, or `gtimeout` from Homebrew coreutils, and a shell watchdog when a
Mac has neither. `night.sh` takes `xcode` for each `xcodebuild` step and `swiftpm` for the
mobile-core `swift test`; its 2700 s `app-build` step therefore stops at the 1800 s cap
unless `MONACO_LOCK_HOLD` is raised.

Ctrl-C or `kill` (INT or TERM) sent to the script is passed on to the command, and the lock
is released only after the command has exited, so a stopped build never runs on under a
free lock. The script then exits 130 (INT) or 143 (TERM).
