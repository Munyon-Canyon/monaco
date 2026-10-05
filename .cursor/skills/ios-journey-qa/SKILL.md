---
name: ios-journey-qa
description: Journey QA for the iOS app. Use when asked to QA or test a journey or a milestone's journeys on the simulator, when a journey doc under docs/journeys is added or its version changes, or when picking up a milestone's "Update the journeys" ticket. For unit tests or a sample-harness screen use ios-verify.
---

# Journey QA

A **journey doc** (`docs/journeys/<area>/<journey>.md`) is the source of truth for QA. An XCUITest is built from it and stamped with its version. `scripts/qa/journey.py` checks, runs and measures it; read its `--help` for commands. The doc format, versioning and file layout are in `docs/journeys/README.md`. `auth/sign-in` is the worked example of every file.

**Backend and the QA lock.** Every journey run needs the backend, and runs on one machine share its ports (:8080 api, :8081 worker) and the simulators. `journey.py run` and `mutants` handle both. They take `/tmp/monaco-qa.lock` (the lock `/usr/bin/lockf -k` holds) and wait for it. They refuse to go on when anything already listens on 8080 or 8081, and name its pid and worktree. Then they start `just run backend`, wait for `/healthz`, and stop the backend when the run ends. Don't start a backend for a journey yourself. Run `just migrate db` first if the backend log says `db_schema_behind`. To aim a run at a backend that is already up, set `MONACO_API_BASE_URL`. The run then only checks `/healthz`.

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

A journey with `funds` in its doc moves real USDC: the person or agent running it sets up the Phantom MCP and funds each actor first, then refunds the agent wallet after (`docs/journeys/README.md`, Journeys that move money). The test itself moves no money.

Each actor uses a dedicated `Monaco Journeys <actor>` simulator that the runner creates when needed. Use `--sim B=<udid>` only to override an actor's dedicated simulator.

## Memory

`journey.py` shuts down the simulators it boots when a run ends, whether it passes, fails or is interrupted. Pass `--keep-sims` only when a person is debugging. Never leave a simulator booted when your turn ends. Run at most 2 simulator lanes at once on a 16 GB Mac. Check with `xcrun simctl list devices booted`.
