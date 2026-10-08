# Running Monaco on a local iOS Simulator

## Signing

`scripts/ios-build` and `scripts/ios-sim` (used by `just build mobile` and
`just run mobile`) build the Debug simulator app with:

```
CODE_SIGN_IDENTITY="-" CODE_SIGNING_REQUIRED=NO CODE_SIGNING_ALLOWED=YES
```

This is **ad-hoc signing** ("sign to run locally") — it self-signs the app
without needing the project's own Apple Developer team certificate
(`DEVELOPMENT_TEAM = JSF53DFS29` in `apps/mobile/Monaco.xcodeproj`), so the
build works on any contributor's machine regardless of whether they have
that team's signing identity installed.

**Do not build the simulator app with `CODE_SIGNING_ALLOWED=NO`.** That skips
codesigning entirely and produces a fully unsigned binary. iOS ties a Keychain
item's access group to the signature of the app that created it — with no
signature, the app can't consistently reclaim its own keychain entries, so
the Privy session gets silently dropped on every relaunch or reinstall (the
user looks signed out for no reason). Ad-hoc signing (`CODE_SIGN_IDENTITY="-"`)
still produces a real, consistent signature, so the keychain entry persists
across relaunches and reinstalls, while `CODE_SIGNING_REQUIRED=NO` keeps the
build from failing if a stricter signing requirement is inherited from the
project or environment.

Some older QA docs (`docs/legacy/qa/148`, `docs/legacy/qa/160`) used to show
`CODE_SIGNING_ALLOWED=NO` in their reproduction commands — that's a copy/paste
trap for exactly this bug, and both have been updated to the ad-hoc flags
above. If you're pasting an `xcodebuild` command from an old doc or PR,
double check it doesn't carry `CODE_SIGNING_ALLOWED=NO` forward.

## Parallel agents: one simulator per worktree

Each linked git worktree is a lane, named by its directory. A lane gets its own
simulator, its own build cache and a share of the machine's build slots, so agents
in separate worktrees build and run the app at the same time without touching each
other. The primary checkout is not a lane and keeps the gold `SIMSLIM_UDID` with the
stock fallback.

- **Simulator.** `scripts/lane-sim-udid.sh` prints the lane's simulator,
  `Monaco <lane>`. The first call creates it with the gold simulator's device type
  and runtime, and slims it (see SimSlim for lanes and journeys below).
  `scripts/resolve-ios-sim.sh` and `scripts/gold-sim-udid.sh` return it in a lane,
  so `just run mobile`, `monacoctl agents check` and MobileBuildMCP's
  `--simulator-id` all use it. Journeys run on one more, `Monaco Journeys <lane>`, whatever their actors.
- **Build cache.** `scripts/ios-build` and `scripts/ios-sim` build into the
  checkout's `.build/DerivedData`, the cache `monacoctl agents check` uses, and
  `ios-sim` installs the app built there.
- **Build slots.** `scripts/qa/xcode-lock.sh` runs one xcodebuild per 16 GB of RAM
  at once and one `swift test` per 8 GB. A 16 GB Mac still builds one at a time; a
  64 GB Mac builds four. A lane holds one xcodebuild slot at a time (`MONACO_LANE`,
  else the herdr workspace, else the worktree), so several worktrees of one lane
  build one after another while other lanes use the free slots. Waiters queue in
  arrival order. Each xcodebuild run appends a row (lane, HEAD, action, wait, exit
  code, run time) to `.git/.monaco/xcode-builds.tsv` in the clone. See
  [Build slots](overnight-qa.md#build-slots). Two builds never share one
  `-derivedDataPath`, which fails with `database is locked`: the second waits for
  the first, and for any xcodebuild still building there outside the lock.
- **Stop.** `just stop mobile` in a lane stops only that lane's xcodebuild and
  uninstalls the app only on the lane's simulators. In the primary checkout it
  spares the simulators of live lanes and deletes the simulators of lanes whose
  worktree is gone. Lane simulators are recorded in `monaco-lane-sims.tsv` in the
  git common dir.
- **Taps.** Drive a lane's app with the XCUITest journeys (`scripts/qa/journey.py`)
  or with an MCP that takes a simulator id on each call. Each agent passes its own
  lane UDID from `scripts/gold-sim-udid.sh` and never another lane's.
- **Fake USDC.** Start `bin/fakes`, then export `QA_FAKE_RPC=1` and source
  `scripts/qa/seed.sh` before `journey.py` or `just run backend`, so the backend
  reads Solana RPC from the fakes server instead of mainnet. `qa_fake_usdc A 25`
  then gives actor A a platform balance of 25.00 USDC, and an actor never set
  reads the recorded fixture's 25.50.

| Variable | Default | Meaning |
| --- | --- | --- |
| `MONACO_SIM_UDID` | unset | Use this simulator in any checkout, lane or primary. Nothing is created. |
| `MONACO_XCODE_SLOTS` | RAM GB / 16, at least 1 | xcodebuild processes that may run at once. |
| `MONACO_SWIFTPM_SLOTS` | RAM GB / 8, at least 1 | `swift build` and `swift test` processes that may run at once. |

## SimSlim for lanes and journeys

When `simslim` is on PATH, `scripts/simslim-ensure.sh` slims every lane simulator and
every journey actor simulator right after it is created, with no watcher running:
`simslim on <udid> --profile "${SIMSLIM_PROFILE:-ci/profiles/base-slim.json}" --preserve-boot-state`.
Before a later run the same script calls `simslim verify`, and if the simulator is no
longer slim (recreated or erased) it re-runs `simslim on` once. A missing `simslim`
warns on stderr and the simulator stays stock; nothing fails. `MONACO_NO_SIMSLIM=1`
opts out.

`ci/profiles/base-slim.json` keeps `siri` on. Slimming it away leaves UIKit's dictation
availability handler spinning the app's main thread as soon as a text field takes focus, so
XCUITest never sees the app idle (#3224).

## SimSlim profiles

`scripts/simslim-profile.json` is Monaco's SimSlim capability profile
(see `.cursor/skills/ios-simslim-fast-qa`). `except` lists the daemon
categories that stay **on** (i.e. *not* slimmed away):

```json
{
  "except": ["icloud", "web", "messaging", "store", "telemetry", "photos"]
}
```

`photos` must stay in `except`. Slimming it away disables the photo-picker
daemons the profile-photo flow (`PHPicker`) depends on, which breaks that QA
path in a way that looks unrelated to SimSlim. If you create a narrower
profile for a specific QA pass, keep `photos` in its `except` list too unless
you're specifically testing the no-photos degraded path.

### Runtime requirement

SimSlim settings only **persist** across a simulator reboot on **iOS 18.5+**
or **iOS 26.x** runtimes. An **iOS 18.0** runtime can be slimmed for the
current boot, but the settings do not survive a reboot or `simslim erase`, so
you'll silently fall back to a stock (unslimmed, higher-RAM) simulator after a
restart. Use an 18.5+ or 26.x runtime for your gold simulator.
