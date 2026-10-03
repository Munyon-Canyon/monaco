# App flows

A flow is one thing a member does in the app from start to finish, such as signing in or creating a cabal. Each flow has one doc here. The doc is the source of truth for QA: an XCUITest is built from it, and the test says which version of the doc it was built from.

The agent recipe for building the tests is the `ios-flow-qa` skill (`.claude/skills/ios-flow-qa/SKILL.md`). Its `evals/` say when the skill should and should not be picked up.

## Who writes what

| Who | Writes |
| --- | --- |
| The milestone owner | The flow doc, before the milestone's mobile tickets start |
| The agent, with the `ios-flow-qa` skill | The XCUITest files and the seeded bugs, from the doc |
| The milestone's "Update the flows" ticket | Any change the milestone made to a flow: the doc first, then the tests |

Every milestone carries one "Update the flows" ticket. Its Done-when is `scripts/qa/flow.py check` passing and one passing run of each flow the milestone touched.

## Where things live

| Path | What |
| --- | --- |
| `docs/flows/<area>/<flow>.md` | The flow doc |
| `apps/mobile/MonacoUITests/Flows/` | The XCUITest: one `<Flow>Flow.swift` with the steps, one `<Flow>FlowUITests.swift` with a test per scenario |
| `apps/mobile/qa/flows/<area>/<flow>.mutants/` | Seeded bugs for the catch rate |
| `apps/mobile/qa/flows/accounts.tsv` | The login each actor uses |
| `scripts/qa/flow.py` | Checks, runs and measures flows |

## The flow doc

A flow doc starts with front matter and then has four sections, in this order: Preconditions, Scenarios, Ground truth, Not covered.

```yaml
---
id: auth/sign-in            # <area>/<flow>, the same as the path under docs/flows
title: Sign in
version: 1                  # a whole number, see Versions
milestone: M9
requires: []                # flows that must have run first, by id
actors: [A]                 # one simulator and one account per actor
# funds: only for a flow that moves money, see below
xcuitest: [apps/mobile/MonacoUITests/Flows/SignInFlow.swift, apps/mobile/MonacoUITests/Flows/SignInFlowUITests.swift]
---
```

A scenario is a `###` heading that starts with its id (`### S1 Sign in`) and a table with one row per step:

| Column | Holds |
| --- | --- |
| Step | The id, `S1.1`, `S1.2`. Ids are never reused, so a failure report names one row |
| Actor | Which actor acts, for a flow with more than one |
| Action | `launch`, `tap`, `type`, `wait`, `scroll to`, `relaunch` |
| Target | The accessibility identifier in backticks. A label in quotes only where the element has no identifier |
| Input | What is typed. Account values are written `{A.phone}`, `{A.code}` |
| Expect | What must be on screen after the step, and within how many seconds |

A step that has no accessibility identifier to target is a gap in the app. Add the identifier in the same ticket.

## Steps run one at a time

Every step finishes before the next one starts. There is no step that runs "in the background".

- Work the app or the backend does on its own time (a poller, a consumer, a push, a confirmation on chain) is written as a `wait` step: the thing that must show, and the longest it may take. The test polls for it. No test sleeps a fixed time.
- A flow with two actors is one list of steps. Each row names its actor, and only one actor acts at a time. "B joins while A watches" is three rows: B taps Join, A waits for B's name on the member board, B waits for the cabal screen.

That keeps a run the same every time, and a failure always has one step to point at.

## Two simulators

Each actor has its own simulator and its own account, so no step signs out to switch members.

- Actor A uses the gold simulator from `scripts/gold-sim-udid.sh`. Actor B uses a clone of it: `xcrun simctl clone "$SIMSLIM_UDID" "Monaco Gold B"`, made once per machine.
- `scripts/qa/flow.py run <flow> --sim B=<udid>` maps the extra actor. With no mapping for B, the run stops and says so.
- XCUITest drives one simulator per `xcodebuild` call, so a two-actor scenario runs as phases, each on its actor's simulator, in the doc's order. How a phase is named and how one actor hands a value to the next is in the skill's `xcuitest.md`.

## Flows build on flows

`requires` lists the flows that must have run. Create cabal has `requires: [auth/sign-in]`.

The test calls the required flow's entry point (`SignInFlow.ensureSignedIn`). There is one copy of the sign-in steps, so every flow runs the current one.

## Flows that move money

A flow that deposits, funds a cabal, trades or cashes out needs real USDC on Solana mainnet in the actor's member wallet. The test does not move that money. Whoever runs the flow does, with the Phantom MCP agent wallet, before and after the run.

The flow doc says how much each actor needs, in whole USDC:

```yaml
funds:
  A: 2
  B: 1
```

1. Set up the Phantom MCP once per machine and fund its agent wallet with a little SOL and USDC: [Agent QA: Phantom MCP](https://github.com/Munyon-Canyon/monaco/blob/main/README.md#agent-qa-phantom-mcp). It is an agent wallet of its own, separate from every Privy product wallet.
2. Before the run, sign in as the actor, copy the deposit address from Add money, and transfer the actor's amount from the agent wallet to it. Check that the simulated transfer's destination is the copied address before approving.
3. Run the flow. Its first step waits for the account balance to show the amount, so an unfunded run fails there and says so.
4. After the run, cash out what is left and withdraw it to the agent wallet's address. Leftover test USDC belongs on the agent wallet, not in a cabal or a member wallet.

`scripts/qa/flow.py run` prints the amounts before it starts and the refund reminder when it ends. Keep amounts small: a few dollars covers every flow.

## Versions

Git keeps the history. There are no `V2` copies of a doc or a test.

- `version` goes up by one whenever a step, a target, an input or an expectation changes. Wording fixes do not change it.
- The flow's steps file declares `static let id` and `static let version`, with the doc's id and version. `scripts/qa/flow.py check` fails when they are not the doc's, which is how a stale test is found. Swift files carry no comments in this repo, so the stamp is code.
- A flow that requires another does not pin its version. When a required flow's last screen changes (what it leaves on screen for the next flow), bump the flows that require it too.

## Run and measure { #measure }

```sh
scripts/qa/flow.py check                                   # docs and tests agree
scripts/qa/flow.py run auth/sign-in                        # every scenario, once
scripts/qa/flow.py run auth/sign-in --runs 10              # for the flake rate
scripts/qa/flow.py mutants auth/sign-in                    # catch rate against the seeded bugs
scripts/qa/flow.py report                                  # the table across every run so far
```

Each run appends a row to `.logs/qa/flows/results.tsv`:

| Measure | Meaning |
| --- | --- |
| Wall seconds | The whole test call, build time not counted |
| Step milliseconds | Per step, from the test's own timers |
| Flake rate | Failures on an unchanged build, over all runs of that build |
| Catch rate | Seeded bugs the test failed on, over the seeded bugs it ran |
| False passes | Runs that passed while the ground truth check failed, or while a seeded bug was in |

A seeded bug is a patch under `<flow>.mutants/` that breaks one thing the doc promises. Its first lines say which scenarios must fail. `mutants` applies each patch, rebuilds, runs the test, and reverts the patch.
