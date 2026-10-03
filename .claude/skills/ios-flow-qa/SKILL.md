---
name: ios-flow-qa
description: Flow QA for the iOS app. Use when asked to QA or test a flow or a milestone's flows on the simulator, when a flow doc under docs/flows is added or its version changes, or when picking up a milestone's "Update the flows" ticket. For unit tests or a sample-harness screen use ios-verify.
---

# Flow QA

A **flow doc** (`docs/flows/<area>/<flow>.md`) is the source of truth for QA. An XCUITest is built from it and stamped with its version. `scripts/qa/flow.py` checks, runs and measures it; read its `--help` for commands. The doc format, versioning and file layout are in `docs/flows/README.md`. `auth/sign-in` is the worked example of every file.

When the flow doc and the app disagree, stop and report the **step id** and both sides. That disagreement is the finding: a stale doc or an app bug.

## Build or update a flow's test

1. Read the flow doc and every flow doc it `requires`. Done when you can list each scenario's step ids and each actor.
2. Resolve every target to an accessibility identifier in `apps/mobile/Monaco`. A target without one gets an identifier in the app, in this change. Done when every step's target is an identifier or a system control's label.
3. Write the XCUITest from [xcuitest.md](xcuitest.md).
4. Write one **seeded bug** per scenario under `apps/mobile/qa/flows/<area>/<flow>.mutants/`: a patch that breaks one promise the flow doc makes, with an `expect-fail: S…` line.
5. Stamp the steps enum with the doc's id and version: `static let id` and `static let version`. Done when `scripts/qa/flow.py check` exits 0.
6. `scripts/qa/flow.py run <flow> --runs 3`. Done at 3 passes of 3. A single failure is a flaky test: fix the wait that raced.
7. `scripts/qa/flow.py mutants <flow>`. Done when every seeded bug is caught.

For a version bump, change only the steps the doc changed, then run steps 5 to 7.

## QA a milestone

1. `scripts/qa/flow.py list`: take the flows whose milestone matches, and every flow they require.
2. A flow with no test yet: build it first, with the section above.
3. `scripts/qa/flow.py run <flow>` for each, required flows first.
4. Report each scenario as PASS, or as the failing step id with its assertion message and the log under `.logs/qa/flows/<run>/`. Done when every scenario of every flow has one of the two.

A flow with `funds` in its doc moves real USDC: the person or agent running it sets up the Phantom MCP and funds each actor first, then refunds the agent wallet after (`docs/flows/README.md`, Flows that move money). The test itself moves no money.

A two-actor flow needs a second simulator: `xcrun simctl clone "$SIMSLIM_UDID" "Monaco Gold B"` once per machine, then `--sim B=<udid>`.
