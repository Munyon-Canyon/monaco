# App journeys

A journey is one thing a member does in the app from start to finish, such as signing in or creating a cabal. Each journey has one doc here. The doc is the source of truth for QA: an XCUITest is built from it, and the test says which version of the doc it was built from.

## Journeys and flows

A [flow](https://github.com/Munyon-Canyon/monaco/blob/main/packages/flows/README.md) in `packages/flows` is one backend command and the screen that handles its outcomes. `monacoctl flows check` checks flows with fixture data.

A journey is what a member does across screens in the real app. An XCUITest drives it.

Failure outcomes belong to the flow layer. A journey covers the happy path and the regressions its seeded bugs name.

The agent recipe for building the tests is the `ios-journey-qa` skill (`.claude/skills/ios-journey-qa/SKILL.md`). Its `evals/` say when the skill should and should not be picked up.

## Who writes what

| Who | Writes |
| --- | --- |
| An agent with the `ios-journey-qa` skill | The journey doc when the milestone's screens land. A person reviews it before a test is built from it. |
| The agent, with the `ios-journey-qa` skill | The XCUITest files and the seeded bugs, from the doc |
| The milestone's "Update the journeys" ticket | Any change the milestone made to a journey: the doc first, then the tests |

Every milestone carries one "Update the journeys" ticket. Its Done-when is `scripts/qa/journey.py check` passing and one passing run of each journey the milestone touched.

## Where things live

| Path | What |
| --- | --- |
| `docs/journeys/<area>/<journey>.md` | The journey doc |
| `apps/mobile/MonacoUITests/Journeys/` | The XCUITest: one `<Journey>Journey.swift` with the steps, one `<Journey>JourneyUITests.swift` with a test per scenario |
| `apps/mobile/qa/journeys/<area>/<journey>.mutants/` | Seeded bugs for the catch rate |
| `apps/mobile/qa/journeys/<area>/<journey>.truth.sh` | The ground truth check, run after each run |
| `apps/mobile/qa/journeys/<area>/<journey>.setup.sh` | Optional. Run with the scenario id before each scenario, to put the backend in the state the scenario starts from. It can pass a value to the test through the hand-off file |
| `apps/mobile/qa/journeys/accounts.tsv` | The login each actor uses |
| `scripts/qa/journey.py` | Checks, runs and measures journeys |

## The journey doc

A journey doc starts with front matter and then has five sections, in this order: Preconditions, Scenarios, Ground truth, Known failures on staging, Not covered.

```yaml
---
id: auth/sign-in            # <area>/<journey>, the same as the path under docs/journeys
title: Sign in
version: 1                  # a whole number, see Versions
milestone: M9
requires: []                # journeys that must have run first, by id
actors: [A]                 # one simulator and one account per actor
flows: [01]                 # backend flows this journey exercises
# funds: only for a journey that moves money, see below
xcuitest: [apps/mobile/MonacoUITests/Journeys/SignInJourney.swift, apps/mobile/MonacoUITests/Journeys/SignInJourneyUITests.swift]
---
```

A scenario is a `###` heading that starts with its id (`### S1 Sign in`) and a table with one row per step:

| Column | Holds |
| --- | --- |
| Step | The id, `S1.1`, `S1.2`. Ids are never reused, so a failure report names one row |
| Actor | Which actor acts, for a journey with more than one |
| Action | `launch`, `tap`, `type`, `wait`, `scroll to`, `relaunch` |
| Target | The accessibility identifier in backticks. A label in quotes only where the element has no identifier |
| Input | What is typed. Account values are written `{A.phone}`, `{A.code}`. Run values are `{QA.run}`, a short id unique to each run, and `{QA.refund_address}`, the wallet that funded the run: the QA pot by default, or the Phantom MCP agent wallet |
| Expect | What must be on screen after the step, and within how many seconds |

A step that has no accessibility identifier to target is a gap in the app. Add the identifier in the same ticket.

`scripts/qa/journey.py` passes the run values to the test, the setup script and the truth check as `MONACO_QA_RUN` and `MONACO_QA_REFUND_ADDRESS`. It makes `MONACO_QA_RUN` for each run, so a value one run writes never matches one an earlier run left on the same database. It reads `MONACO_QA_REFUND_ADDRESS` from the environment. When it is unset, it uses the QA pot's address from `monacoctl qa pot --address`, and refuses to start a journey with `funds` only when neither is available.

The setup script and the truth check also get `MONACO_QA_HANDOFF`, the run's hand-off file: a JSON object of strings that the test reads with `JourneyHandoff.read`. A setup script writes a value there that only it can make, such as a dev token, and the truth check reads back what the run left there, such as the dev user's id.

A state the app cannot reach, such as an `auth_state` a test login never passes through, comes from the journey's setup script, never from a person editing the database. The setup and truth scripts reach Postgres through `apps/mobile/qa/journeys/psql.sh`, which uses the host `psql` or the one in the Compose `monaco-postgres` container.

## Steps run one at a time

Every step finishes before the next one starts. There is no step that runs "in the background".

- Work the app or the backend does on its own time (a poller, a consumer, a push, a confirmation on chain) is written as a `wait` step: the thing that must show, and the longest it may take. The test polls for it. No test sleeps a fixed time.
- A journey with two actors is one list of steps. Each row names its actor, and only one actor acts at a time. "B joins while A watches" is three rows: B taps Join, A waits for B's name on the member board, B waits for the cabal screen.

That keeps a run the same every time, and a failure always has one step to point at.

## Two simulators

Each actor has its own simulator and its own account, so no step signs out to switch members.

- An unmapped actor uses a dedicated simulator named `Monaco Journeys <actor>`. The runner creates it with the gold simulator's device type and runtime, or the newest available iPhone and iOS runtime when no gold simulator exists.
- `scripts/qa/journey.py run <journey> --sim B=<udid>` overrides an actor's dedicated simulator.
- XCUITest drives one simulator per `xcodebuild` call, so a two-actor scenario runs as phases, each on its actor's simulator, in the doc's order. How a phase is named and how one actor hands a value to the next is in the skill's `xcuitest.md`.

## Journeys build on journeys

`requires` lists the journeys that must have run. Create cabal has `requires: [auth/sign-in]`.

The test calls the required journey's entry point (`SignInJourney.ensureSignedIn`). There is one copy of the sign-in steps, so every journey runs the current one.

## Journeys that move money

A journey that deposits, funds a cabal, trades or cashes out needs real USDC on Solana mainnet in the actor's member wallet. The test does not move that money. Whoever runs the journey does, before and after the run, from one of two wallets:

- **The QA pot** (default). One shared wallet whose key is in the encrypted `.env.local`, so every developer with `.env.keys` and every cloud agent can use it with no setup. `monacoctl qa fund` sends from it.
- **The Phantom MCP agent wallet.** One per machine, set up by hand. Cloud agents can't use it.

The journey doc says how much each actor needs, in whole USDC:

```yaml
funds:
  A: 2
  B: 1
```

1. Before the run, sign in as the actor. With the QA pot, run `monacoctl qa fund --user <actor's user id> --usdc <amount>`. It sends only to that user's member wallet, at most 5 USDC per transfer, and refuses when the pot runs low ([Agent QA: the QA pot](https://github.com/Munyon-Canyon/monaco/blob/main/README.md#agent-qa-the-qa-pot)). With Phantom, set up the MCP once per machine ([Agent QA: Phantom MCP](https://github.com/Munyon-Canyon/monaco/blob/main/README.md#agent-qa-phantom-mcp)), copy the deposit address from Add money and transfer the amount to it, checking the simulated transfer's destination before approving.
2. Keep each run under 5 USDC in total. The pot is shared.
3. Run the journey. Its first step waits for the account balance to show the amount, so an unfunded run fails there and says so.
4. After the run, cash out what is left and withdraw it in the app to `{QA.refund_address}`, the wallet that funded the run. Never move it any other way: a withdrawal keeps the ledger and the chain in agreement. Leftover test USDC belongs on the funding wallet, not in a cabal or a member wallet. For money a crashed run stranded, use `scripts/sweep-wallets.sh`.

`scripts/qa/journey.py run` prints the amounts before it starts and the refund reminder when it ends. Keep amounts small: a few dollars covers every journey.

## Versions

Git keeps the history. There are no `V2` copies of a doc or a test.

- `version` goes up by one whenever a step, a target, an input or an expectation changes. Wording fixes do not change it.
- The journey's steps file declares `static let id` and `static let version`, with the doc's id and version. `scripts/qa/journey.py check` fails when they are not the doc's, which is how a stale test is found. Swift files carry no comments in this repo, so the stamp is code.
- A journey that requires another does not pin its version. When a required journey's last screen changes (what it leaves on screen for the next journey), bump the journeys that require it too.

## Run and measure { #measure }

A run passes its API URL under a QA-only name and uses dedicated journey simulators so shared simulator settings do not point it elsewhere.
It reinstalls the app on dedicated journey simulators before every run, so every run starts signed out.
`run` and `mutants` take `/tmp/monaco-qa.lock`, so one journey run on a machine uses the backend and the simulators at a time. Holding it, they refuse to start when anything already listens on 8080 or 8081 and name its pid and worktree. Then they start the backend with `just run backend` and stop it when the run ends. With `MONACO_API_BASE_URL` set, they use that backend instead and only check its `/healthz`.

```sh
scripts/qa/journey.py check                                   # docs and tests agree
scripts/qa/journey.py run auth/sign-in                        # every scenario, once
scripts/qa/journey.py run auth/sign-in --runs 10              # for the flake rate
scripts/qa/journey.py run --all                               # every journey, one build, one backend
scripts/qa/journey.py run auth/sign-in --timeout 600          # a longer budget than 300 s
scripts/qa/journey.py run auth/sign-in --rebuild              # build even when the stamp matches
scripts/qa/journey.py mutants auth/sign-in                    # catch rate against the seeded bugs
scripts/qa/journey.py report                                  # the table across every run so far
```

Each run appends a row to `.logs/qa/journeys/results.tsv`:

| Measure | Meaning |
| --- | --- |
| Wall seconds | The whole test call, build time not counted |
| Step milliseconds | Per step, written to steps.tsv in the run's folder |
| Flake rate | Failures over all clean runs of the journey that started |
| Catch rate | Seeded bugs the test failed on, over the seeded bugs it ran |
| False passes | Seeded bugs not caught, plus clean runs that passed while the ground truth check failed |

`run` builds the app once per checkout. After a build it writes `.logs/qa/journeys/derived/build.stamp`, the sha256 of `HEAD`, the uncommitted diff of `apps/mobile` and `packages/mobile-core`, and their untracked files. The next `run` prints `reusing build <stamp>` and skips `build-for-testing` while the stamp matches and the `.xctestrun` file is still under `derived/Build/Products`. `--rebuild` builds anyway, and `--no-build` never builds. `mutants` always builds and deletes the stamp after it reverts a patch.

`run --all` runs every journey in `requires` order, ties by id, on one build and one backend, and exits with the worst result. It skips a journey with a `funds` block, printing `SKIP funds`, unless `MONACO_QA_REFUND_ADDRESS` is set.

`--timeout` (seconds, default 300) is the budget for one journey run: its setup scripts, every test call and the truth check together. The build is outside it. When the budget runs out, the run stops the running command and its children, gives the unfinished scenarios the result `TIMEOUT`, and prints `<journey> timed out after <n> s in <scenario> <phase>`. A `TIMEOUT` counts as a failure in the report and exits 1.

A setup script seeds through `scripts/qa/seed.sh`. A test never taps to create its starting state. The helper's `qa_api` calls a route as an actor, `qa_flow_seed` runs `monacoctl flows seed`, and `qa_sql` is for a state no route or flow seed can reach.

Parallel lanes each use their own checkout's `derived/` and `Monaco Journeys <lane> <actor>` simulators under `xcode-lock.sh` slots. `/tmp/monaco-qa.lock` stays one per machine while the backend ports are shared.

Known failures on staging lists each step that cannot pass yet and the ticket that blocks it, as a bullet (`- S1.2 to S1.5: <why>. Blocked by #691.`) or as a table row whose first cell names the steps and whose last cell names the tickets. `S1.2 to S1.5` covers every step of the doc from S1.2 through S1.5. `check` names a listed step that the doc does not have. A run reads the section and gives each scenario one of these results:

| Result | Meaning | Run |
| --- | --- | --- |
| `PASS` | Every step passed, and no step of the scenario is listed | Passes |
| `KNOWN` | The scenario failed at a listed step. The `expected` column reads `FAIL` | Passes |
| `FIXED` | Every step passed, including a listed one. Drop the step from Known failures | Passes |
| `FAIL` | The scenario failed at a step that is not listed: a new failure | Exits 1 |
| `TIMEOUT`, `ERROR` | The budget ran out, or the test did not run | Exits 1, 2 |

`report` ends with a second table: each journey's last clean run, with its passing scenarios, its known failures and their tickets, its fixed scenarios and its new failures.

A seeded bug is a patch under `<journey>.mutants/` that breaks one thing the doc promises. Its first lines say which scenarios must fail. `mutants` applies each patch, rebuilds, runs the test, and reverts the patch.

## Coverage and CI guard

`flows` lists the backend flow ids the journey exercises. `scripts/qa/journey.py check` requires each id to have a backend flow file and requires every seeded bug patch to apply. Regenerate a stale patch with the `ios-journey-qa` skill.

`scripts/qa/journey.py coverage` prints every backend flow with its app status and the journeys that list it. It ends with the backend flow ids that no journey covers.

Stage 0 and CI run the journey checks when mobile code, journey docs, journey QA files, or journey scripts change.
