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
| `apps/mobile/MonacoUITests/Journeys/` | The XCUITest: one `<Journey>Journey.swift` with the steps, one `<Journey>JourneyUITests.swift` with `testJourney`, which runs every scenario in order |
| `apps/mobile/qa/journeys/<area>/<journey>.mutants/` | Seeded bugs for the catch rate |
| `apps/mobile/qa/journeys/<area>/<journey>.truth.sh` | The ground truth check, run after each run |
| `apps/mobile/qa/journeys/<area>/<journey>.setup.sh` | Optional. Run with the scenario id when the test reaches each scenario, to put the backend in the state the scenario starts from. It can pass a value to the test through the hand-off file |
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
actors: [A]                 # one account per actor; every actor shares one simulator
flows: [01]                 # backend flows this journey exercises
# funds: only for a journey that moves money, see below
# simslim: only for a journey that needs a category base-slim turns off, see below
# rpc: fake, only for a journey that must not read the public Solana endpoint, see below
# preconditions: only for state the run checks before it builds, see Preconditions in the run
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

`simslim` names a profile in `ci/profiles/`. `scripts/qa/journey.py` passes it as `SIMSLIM_PROFILE` to `scripts/simslim-ensure.sh` before that journey. Every other journey uses `ci/profiles/base-slim.json`, and the shared simulator is re-slimmed when the profile changes.

`rpc: fake` makes `scripts/qa/journey.py` build and start `cmd/fakes` on the slot's port (8099, or 8199 for slot 1) and start the backend with `SOLANA_RPC_URL` pointing at it, when every journey of the run declares it and none has `funds`. The public Solana endpoint answers 429 under load, and a screen that reads a portfolio value then shows a dash. A caller who exports `QA_FAKE_RPC=1` runs `bin/fakes` itself and keeps control.

The setup script and the truth check also get `MONACO_QA_HANDOFF`, the run's hand-off file: a JSON object of strings that the test reads with `JourneyHandoff.read`. A setup script writes a value there that only it can make, such as a dev token, and the truth check reads back what the run left there, such as the dev user's id.

A state the app cannot reach, such as an `auth_state` a test login never passes through, comes from the journey's setup script, never from a person editing the database. The setup and truth scripts reach Postgres through `apps/mobile/qa/journeys/psql.sh`, which uses the host `psql` or the one in the Compose `monaco-postgres` container. They call `monacoctl` only through `scripts/qa/monacoctl.sh`, which drops `MONACO_API_BASE_URL` and `MONACO_QA_*` because `monacoctl` rejects `MONACO_` variables it does not know; `journey.py check` fails on a direct `bin/monacoctl` call.

## Steps run one at a time

Every step finishes before the next one starts. There is no step that runs "in the background".

- Work the app or the backend does on its own time (a poller, a consumer, a push, a confirmation on chain) is written as a `wait` step: the thing that must show, and the longest it may take. The test polls for it. No test sleeps a fixed time.
- A journey with two actors is one list of steps. Each row names its actor, and only one actor acts at a time. "B joins while A watches" is three rows: B taps Join, A waits for B's name on the member board, B waits for the cabal screen.

That keeps a run the same every time, and a failure always has one step to point at.

## One simulator, one session

A journey runs on one simulator in one `xcodebuild` call, whatever its actors. Its scenarios run in order in one app session, and each one continues from where the one before it left off, so no scenario launches the app again to reach its starting point.

- The test switches members by signing out through Profile and signing the next actor in with the `auth/sign-in` steps (`session.act(as:)` in the skill's `xcuitest.md`). The runner passes every actor's login to the test.
- A scenario passes or fails on its own steps. A scenario after a failed one did not run, and its result is `SKIP`.
- The runner uses one dedicated simulator for all of a run's actors, named `Monaco Journeys <lane> <logins>` after the logins the run holds (`Monaco Journeys <logins>` in the primary checkout), so a journey remapped from A onto C never shares A's simulator with a run on A. It creates it with the gold simulator's device type and runtime, or the newest available iPhone and iOS runtime when no gold simulator exists. `--sim <udid>` overrides it.

## Journeys build on journeys

`requires` lists the journeys that must have run. Create cabal has `requires: [auth/sign-in]`.

The test signs in with `session.act(as:)`, which runs the `auth/sign-in` steps. There is one copy of the sign-in steps, so every journey runs the current one.

## Preconditions the run checks

A journey that needs state the run cannot make itself lists it under `preconditions`. `journey.py` checks each before the build and before the first scenario:

```yaml
preconditions:
  balance.A: 1        # actor A holds at least $1 of cash (`funds` implies the same amounts)
  photos: 1           # the photo fixture exists; the setup script adds it with qa_add_photo <udid>
  fresh_handle: true  # the setup script's --check-handle finds no foreign holder of the handle
```

`balance.<actor>` reads the actor's available cash from `GET /v1/me/balance` on the run's backend. `photos` checks that `apps/mobile/qa/journeys/fixtures/photo.jpg` exists (Photos has no cheap count), and the setup script adds it to the simulator with `qa_add_photo` from `scripts/qa/seed.sh`. `fresh_handle` runs `<journey>.setup.sh --check-handle`.

A precondition that fails ends the run as `SETUP`, exit code 2, before any build, and prints the command that fixes it, such as `A's cash is $0.00, under the $1 money/deposit needs: run scripts/qa/monacoctl.sh qa fund --user <id> --usdc 1`. `report` counts `SETUP` runs in their own column, apart from `FAIL` and `ERROR`.

## Pruning old QA cabals

Every run, once it holds `/tmp/monaco-qa-run-<run id>.lock` (taken with `flock` for the whole run), runs `apps/mobile/qa/journeys/prune.sh`. A `QA %` cabal whose name ends in a run id (six characters of the `{QA.run}` alphabet, such as `QA chat K7M2QX`) is closed once that run's lock is no longer held, whatever its age, so two runs on one machine never prune each other's cabals. A `QA %` name without a run id keeps the one-hour rule. A cabal that holds money is never pruned.

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

## Dev users and the Privy cap

Every dev user is a real user on the dev Privy app, which caps its users. A setup that needs a throwaway counterpart (a host, an inviter) must not mint one per run: it passes `monacoctl dev token --user new --pool <name>`, a stable name for that seed role of at most 16 characters (`browse-host`, `vote-host`). The first run looks `dev-<name>@example.com` up in Privy and adopts it, or creates it when Privy has none, and keeps one local row under the handle `dev_<name>`. Every later run, on any machine that shares the Privy app and after a database reset, gets the same user back, so repeated runs create nothing new. A pool name is lowercase letters, digits and single hyphens, and it is never eight hex characters, the shape of a throwaway. `--pool` works only against the local dev database. The `dev_` handle prefix is reserved, so no member can claim a pool's handle.

Two journeys need a user the app has never seen: `onboarding/first-run` (S5) and `settings/delete-account`. They mint with plain `--user new`, and each run leaves one throwaway `dev-<8 hex>@example.com` user on the Privy app. Nothing deletes it yet.

When a setup fails with `privy_user_limit`, the dev Privy app has hit its user cap. Nothing in the repo clears it. Someone with Privy dashboard access deletes the `dev-<8 hex>@example.com` users there, and only those.

## Versions

Git keeps the history. There are no `V2` copies of a doc or a test.

- `version` goes up by one whenever a step, a target, an input or an expectation changes. Wording fixes do not change it.
- The stamp lives in the generated steps file, `apps/mobile/MonacoUITests/Journeys/Generated/<X>JourneySteps.swift`, which `scripts/qa/journey.py gen <journey>` writes from the doc (`static let id`, `static let version` and one value per step row). A journey not yet migrated declares them in its own steps file. `scripts/qa/journey.py check` fails when they are not the doc's, and when a generated file differs from what `gen` would write, which is how a stale test is found. It also fails when a doc's step tables changed against `origin/staging` and `version` did not go up, so the bump rule above is enforced. Swift files carry no comments in this repo, so the stamp is code.
- A journey that requires another does not pin its version. When a required journey's last screen changes (what it leaves on screen for the next journey), bump the journeys that require it too.

## Run and measure { #measure }

A run passes its API URL under a QA-only name and uses dedicated journey simulators so shared simulator settings do not point it elsewhere.
It reinstalls the app on dedicated journey simulators before every run, so every run starts signed out.
`run` and `mutants` take a backend slot, so two journey runs can share a Mac. Slot 0 is api :8080 and worker :8081, slot 1 is :8180 and :8181. A run takes the first free slot with `/tmp/monaco-qa-slot<N>.lock`, waits when both are taken, and `--slot N` forces one. Holding its slot, a run refuses to start when anything already listens on its two ports and names its pid and worktree. Then it runs `monacoctl bus apply` on the local dev NATS (so a new event subject cannot stop boot) and starts the backend with `just run backend` on those ports (with `TRADE_ENGINE=stub`, so a passed vote reaches a stub of the trade engine and moves no money, unless `TRADE_ENGINE` is set or the run moves real USDC) and stops it when the run ends. It stops only the backend it started, through that backend's process group (SIGTERM to api and worker, SIGKILL for the rest), never `just stop backend`, which kills by name and would take the other slot's backend down too. The slot's api URL goes to the app (`MONACO_QA_API_BASE_URL`) and to the setup and truth scripts (`MONACO_API_BASE_URL` and `MONACO_QA_API_BASE_URL`), so a script never assumes :8080. At the start of every run, a local run also runs `apps/mobile/qa/journeys/prune.sh`, which closes the `QA %` cabals that A, B and C created and leaves the ones they only joined, when their run is no longer live (see Pruning old QA cabals) or, for a name without a run id, they are more than an hour older than the run, and never touches a cabal that holds money (any fund transfer, or any position above zero). A database that is not on localhost is never pruned. A run also holds `/tmp/monaco-qa-actor-<login>.lock` on the `accounts.tsv` login of each of its actors: its own login when free, otherwise the first free one among A, B and C (L, the link number, only goes to an actor L). When too few logins are free it waits. Setup and truth scripts read the run's accounts through `QA_ACCOUNTS_FILE` (`.logs/qa/journeys/accounts-<hash>.tsv`), where each journey actor's row is the login it holds, so an actor remapped from A onto C seeds C's user. Before the first scenario, `scripts/qa/ready-login.sh <actor>` gives each held login a handle (`qa_<name>`, or `qa_<name>_<first 6 characters of its Privy id>` when another user holds that one) and an `auth_state` past `CREATED` through the API (`PUT /v1/me/handle`, then skipping the phone step), so the first-run gate opens the tab bar (sign-in P4). A login with no `users` row waits for its first sign-in. Backend logs are `.logs/qa/journeys/backend-slot<N>.log` (boot) and `.logs/qa/journeys/backend-slot<N>-<time>/` (api.log, worker.log) and build logs `build-<logins>.log`, one per slot and per set of logins. A two-actor journey on A and B in slot 0 leaves C for a one-actor journey in slot 1. With `MONACO_API_BASE_URL` set, they use that backend instead, take no slot and only check its `/healthz`.

```sh
scripts/qa/journey.py check                                   # docs and tests agree
scripts/qa/journey.py run auth/sign-in                        # every scenario, once
scripts/qa/journey.py run auth/sign-in --runs 10              # for the flake rate
scripts/qa/journey.py run auth/sign-in --scenario S2          # S1, then S2: a scenario starts where the last one ended
scripts/qa/journey.py run --all                               # every journey, one build per actor set, one backend
scripts/qa/journey.py run cabals/join profile/overview       # rerun several on one build per actor set and one backend
scripts/qa/journey.py run auth/sign-in --timeout 600          # a longer budget than 300 s
scripts/qa/journey.py run auth/sign-in --rebuild              # build even when the stamp matches
scripts/qa/journey.py mutants auth/sign-in                    # catch rate against the seeded bugs
scripts/qa/journey.py report                                  # the table across every run so far
```

Rerun several failing journeys with one `run` call, not one call each: every call starts its own backend. A test of an existing build launches from its `.xctestrun`, which skips the project load (about 6 s to the first test instead of 10 to 29 s) and holds no Xcode build slot.

Each run appends a row to `.logs/qa/journeys/results.tsv`:

| Measure | Meaning |
| --- | --- |
| Wall seconds | The whole test call, build time not counted |
| Step milliseconds | Per step, written to steps.tsv in the run's folder |
| Flake rate | Failures over all clean runs of the journey that started |
| Catch rate | Seeded bugs the test failed on, over the seeded bugs it ran |
| False passes | Seeded bugs not caught, plus clean runs that passed while the ground truth check failed |
| Build seconds | Time in `build-for-testing`, not counting the wait for a build slot; 0 when the run reused a build |
| Build queue seconds | Time spent waiting for an Xcode build slot; 0 when the run reused a build |
| Boot seconds | Time to boot the simulator, not counting the wait for a simulator slot |
| Boot queue seconds | Time spent waiting for a simulator slot |
| Backend seconds | Time from starting the backend until `/healthz` answers |
| Setup seconds | Time in the journey's setup script, summed over its scenarios |
| Switch seconds | Time in the P1 steps that switch the signed-in actor, summed; also per scenario |
| Truth seconds | Time in the ground truth check |
| Uninstall seconds | Time to uninstall the app before the run |

The new columns sit at the end of `results.tsv`, so older rows still parse; `report` skips them in the phase medians. `report` adds a table of the median of each phase and a total (median wall plus the other phase medians; actor switches are left out because they happen inside the wall time), and one line per recent run saying `built in <n> s after <n> s in the queue` or `reused build`. `run` and `mutants` print the same phases when they end. The backend, simulator boot and build times belong to the first run of a call that takes them.

`run` builds the app once per checkout. After a build it writes `.logs/qa/journeys/derived-<logins>/build.stamp` (one derived folder per set of logins the run holds, such as `derived-A-B`, so two runs never share one), the sha256 of `HEAD`, the uncommitted diff of `apps/mobile` and `packages/mobile-core`, and their untracked files. The next `run` prints `reusing build <stamp>` and skips `build-for-testing` while the stamp matches and the `.xctestrun` file is still under `derived-<logins>/Build/Products`. `--rebuild` builds anyway, and `--no-build` never builds. `mutants` always builds and deletes the stamp after it reverts a patch.

`run --all` runs every journey in `requires` order, ties by id, on one build per actor set and one backend, and exits with the worst result. It skips a journey with a `funds` block, printing `SKIP funds`, unless `MONACO_QA_REFUND_ADDRESS` was exported when the run started. The QA pot fallback does not count, and naming a journey (`run money/deposit`) is the opt-in for that one.

`--timeout` (seconds, default 300) is the budget for one journey run: its setup scripts, every test call and the truth check together. The build is outside it. When the budget runs out, the run stops the running command and its children, gives the open scenario the result `TIMEOUT` and the ones after it `SKIP`, and prints `<journey> timed out after <n> s in <scenario> test` (or `<scenario> setup`). A `TIMEOUT` counts as a failure in the report and exits 1.

A setup script seeds through `scripts/qa/seed.sh`. A test never taps to create its starting state. The helper's `qa_api` calls a route as an actor, `qa_flow_seed` runs `monacoctl flows seed`, and `qa_sql` is for a state no route or flow seed can reach.

Parallel lanes each use their own checkout's `derived-<logins>/` and `Monaco Journeys <lane> <logins>` simulator under `xcode-lock.sh` slots. A run takes one backend slot and the locks of its actors' logins, so runs on disjoint logins can run at once.

Known failures on staging lists each step that cannot pass yet and the ticket that blocks it, as a bullet (`- S1.2 to S1.5: <why>. Blocked by #691.`) or as a table row whose first cell names the steps and whose last cell names the tickets. `S1.2 to S1.5` covers every step of the doc from S1.2 through S1.5. `check` names a listed step that the doc does not have. A run reads the section and gives each scenario one of these results:

| Result | Meaning | Run |
| --- | --- | --- |
| `PASS` | Every step passed, and no step of the scenario is listed | Passes |
| `KNOWN` | The scenario failed at a listed step. The `expected` column reads `FAIL` | Passes |
| `FIXED` | Every step passed, including a listed one. Drop the step from Known failures | Passes |
| `FAIL` | The scenario failed at a step that is not listed: a new failure | Exits 1 |
| `SKIP` | An earlier scenario did not pass, so this one never ran | Takes the earlier scenario's result |
| `TIMEOUT`, `ERROR` | The budget ran out, or the test did not run | Exits 1, 2 |
| `SETUP` | A `preconditions` check failed before the build | Exits 2 |

`report` ends with a second table: each journey's last clean run, with its passing scenarios, its known failures and their tickets, its fixed scenarios and its new failures.

A seeded bug is a patch under `<journey>.mutants/` that breaks one thing the doc promises. Its first lines say which scenarios must fail. `mutants` applies each patch, rebuilds, runs the whole journey, and reverts the patch. The bug is caught when each scenario it names fails, or is skipped after one of them failed.

## Coverage and CI guard

`flows` lists the backend flow ids the journey exercises. `scripts/qa/journey.py check` requires each id to have a backend flow file and requires every seeded bug patch to apply. Regenerate a stale patch with the `ios-journey-qa` skill.

`check` also reads the identifiers. It lists every id the app sets in `apps/mobile/Monaco` and `packages/mobile-core/Sources`: a literal `accessibilityIdentifier("x")` is an exact id, and an interpolated one (`"x-\(id)"`) is the prefix `x-`. Ids built from a parameter at run time are rows of `apps/mobile/qa/journeys/ids-composed.tsv` (pattern, the file that builds it, why); a row whose file builds no id at run time, or whose fixed text is in no app file, fails `check`, so the file cannot rot. Then `check` reads every backticked id in the Target, Input and Expect columns of the step tables (`<…>` and `{…}` placeholders turn the id into a prefix) and every id an `*Journey.swift` passes to `element(_:)`, `buttons[…]`, `staticTexts[…]` or an `identifier BEGINSWITH` predicate. A target the app sets nowhere fails with the doc or test file, the step, the target and the three nearest ids, before any simulator run. A step under "Known failures on staging" with a ticket gives a warning instead. Quoted labels, system controls and ids the text says must be absent ("no `x`", "`x` does not exist") are skipped.

`check` also reads the copy. It lists every string literal in `apps/mobile/Monaco` and `packages/mobile-core/Sources` (an interpolated literal becomes its ordered fixed parts), every double-quoted span in the Expect column, and every literal an `*Journey.swift` passes to `waitForText`, `waitForLabel`, `waitForToast`, `expectText`, `expectLabel`, `expectToast`, `staticTexts[…]` or a `label CONTAINS` / `BEGINSWITH` predicate. A test literal must appear as a quote in the same step of the doc, and a doc quote must equal or sit inside an app string, match an interpolated one by its fixed parts in order, or be a row of `apps/mobile/qa/journeys/copy-runtime.tsv` (copy, source, why: text only the backend or a setup makes, such as `QA vote {QA.run}` or a seeded name). Digits, amounts, `…` and `{…}` in a quote match anything. A miss names the doc or test file, the step, the text and the nearest app string, and a step under "Known failures on staging" warns instead.

`scripts/qa/journey.py coverage` prints every backend flow with its app status and the journeys that list it. It ends with the backend flow ids that no journey covers.

Stage 0 and CI run the journey checks when mobile code, journey docs, journey QA files, or journey scripts change.
