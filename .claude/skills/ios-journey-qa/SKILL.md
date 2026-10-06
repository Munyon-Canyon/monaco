---
name: ios-journey-qa
description: Journey QA for the iOS app. Use when asked to QA or test a journey or a milestone's journeys on the simulator, when a journey doc under docs/journeys is added or its version changes, or when picking up a milestone's "Update the journeys" ticket. For unit tests or a sample-harness screen use ios-verify.
---

# Journey QA

A **journey doc** (`docs/journeys/<area>/<journey>.md`) is the source of truth for QA. An XCUITest is built from it and stamped with its version. `scripts/qa/journey.py` checks, runs and measures it; read its `--help` for commands. The doc format, versioning and file layout are in `docs/journeys/README.md`. `auth/sign-in` is the worked example of every file.

**Backend slots and actor locks.** Every journey run needs the backend. Two runs can share a Mac: slot 0 is api :8080 and worker :8081, slot 1 is :8180 and :8181 (never more, 16 GB). `journey.py run` and `mutants` take the first free slot with `/tmp/monaco-qa-slot<N>.lock` (the lock `/usr/bin/lockf -k` holds) and wait when both are taken. `--slot N` forces a slot. They refuse to go on when anything already listens on the slot's ports, and name its pid and worktree. Then they start `just run backend` on those ports, wait for `/healthz`, and stop the backend when the run ends. The slot's api URL goes to the app and to the setup and truth scripts, so nothing hard-codes :8080. Each run also locks the logins of its actors (`/tmp/monaco-qa-actor-<login>.lock`): an actor takes its own login when free, otherwise a free one of A, B and C, and the run waits when too few are free. A journey on A and B in slot 0 leaves C for a one-actor journey in slot 1. Each run still needs its own lane simulators. Don't start a backend for a journey yourself. Run `just migrate db` first if the backend log says `db_schema_behind`. To aim a run at a backend that is already up, set `MONACO_API_BASE_URL`. The run then takes no slot and only checks `/healthz`.

When the journey doc and the app disagree, stop and report the **step id** and both sides. That disagreement is the finding: a stale doc or an app bug.

## Draft a journey doc

1. Read the mobile ticket, its Done when, its linked design doc, and the screens on staging. Done when the journey's user outcome is clear.
2. List the backend flow ids it exercises from `packages/flows`. Done when every id has a backend TSV file.
3. Write `docs/journeys/<area>/<journey>.md` in the README format. Done when every target exists in `apps/mobile/Monaco` or is a ticket gap to add.
4. Run `scripts/qa/journey.py check`. Done when it exits 0.
5. Open the doc for a person to review before building a test. Done when the reviewer has the doc.

## Build or update a journey's test

1. Read the journey doc and every journey doc it `requires`. Done when you can list each scenario's step ids and each actor.
2. Resolve every target to an accessibility identifier in `apps/mobile/Monaco`. A target without one gets an identifier in the app, in this change. Done when every step's target is an identifier or a system control's label.
3. Write the XCUITest from [xcuitest.md](xcuitest.md).
4. Write one **seeded bug** per scenario under `apps/mobile/qa/journeys/<area>/<journey>.mutants/`: a patch that breaks one promise the journey doc makes, with an `expect-fail: S…` line.
5. Stamp the steps enum with the doc's id and version: `static let id` and `static let version`. Done when `scripts/qa/journey.py check` exits 0.
6. `scripts/qa/journey.py run <journey> --runs 3`. Done at 3 passes of 3. A single failure is a flaky test: fix the wait that raced.
7. `scripts/qa/journey.py mutants <journey>`. Done when every seeded bug is caught.

For a version bump, change only the steps the doc changed, then run steps 5 to 7.

## QA a milestone

1. `scripts/qa/journey.py list`: take the journeys whose milestone matches, and every journey they require.
2. A journey with no test yet: build it first, with the section above.
3. `scripts/qa/journey.py run <journey>` for each, required journeys first.
4. Report each scenario as PASS, or as the failing step id with its assertion message and the log under `.logs/qa/journeys/<run>/`. Done when every scenario of every journey has one of the two.

A journey with `funds` in its doc moves real USDC: the person or agent running it funds each actor first, from the QA pot with `monacoctl qa fund` (the default, and the only option in cloud sessions) or from the Phantom MCP agent wallet, then withdraws what is left back to that wallet after (`docs/journeys/README.md`, Journeys that move money). The test itself moves no money.

Each actor uses a dedicated `Monaco Journeys <actor>` simulator that the runner creates when needed. Use `--sim B=<udid>` only to override an actor's dedicated simulator.

**SimSlim for lanes and journeys.** When `simslim` is on PATH, the runner slims each actor simulator right after creating it (`scripts/simslim-ensure.sh`: `simslim on <udid> --profile "${SIMSLIM_PROFILE:-$HOME/.config/simslim/base-slim.json}" --preserve-boot-state`), and before a run it runs `simslim verify` and re-runs `simslim on` once if the simulator is no longer slim. A missing `simslim` warns and continues stock. `MONACO_NO_SIMSLIM=1` opts out. No `simslim watch` is needed.

## Memory

`journey.py` shuts down the simulators it boots when a run ends, whether it passes, fails or is interrupted. Pass `--keep-sims` only when a person is debugging. Never leave a simulator booted when your turn ends. Run at most 2 simulator lanes at once on a 16 GB Mac. Check with `xcrun simctl list devices booted`.
