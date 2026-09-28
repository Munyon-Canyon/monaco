# m7-rest standing orders (operator: "run it exactly", 2026-09-27 19:13 local; poteto-mode autopilot-stack; no fable)

Root worktree for git ops: /Users/logno/Developer/monaco/.worktrees/m7-rest-root. Briefs: OWNER_BRIEF.md, VERIFIER_BRIEF.md here. heavy.sh here. Prompt deviates from playbook step 5: the prompt grants merging verified tickets into backend-rewrite-3 (never main).

Autopilot-stack issue #790, then issue #789, then the last 9 tickets of milestone M7 (tracking issue #492) in lognorman20/monaco, as root orchestrator, using pstack poteto-mode and its autopilot-stack playbook. Run until #790, #789 and every ticket below are merged into `backend-rewrite-3` and the checkpoint PR is green. Then report and stop.

## Phase 0: #790

#790 adds `just migrate db` and makes schema-behind boot failures explain themselves. It has no blockers and is small, so it is one owner and one PR into `backend-rewrite-3`.

- **`just run backend` must never migrate.** It keeps failing loudly when the schema is behind, and names `just migrate db`.
- **Merge it by fast-forward**, since the merge queue doesn't exist yet. It needs an independent PASS and green dispatched CI first, and the rest of the rules below apply.
- **Then** build `just migrate db` into the proof steps for every later ticket that adds a migration.

## Phase 1: #789

Start when #790 is merged. Build #789, the M7 retro turned into CI checks, hooks and the `monacoctl agents` CLI, before any milestone ticket starts. It follows the rules below like any ticket.

- **PR order.** Its 8 PRs go in the order of the ticket's Stack section, all into `backend-rewrite-3`. Items that don't touch the same files may build in parallel, up to 4 owners.
- **PR 1 comes first.** It covers landing: the Graphite trunk, the ruleset and merge queue on `backend-rewrite-3`, the `verify` status, and CI triggers for feature-branch PRs and `merge_group`. Merge it before the others.
- **Merging after PR 1.** Once PR 1 is merged, direct pushes to `backend-rewrite-3` are blocked, so every later PR lands through the merge queue. After `ci / ci-ok` is green and `monacoctl agents verdict` (or, before that PR exists, a `verify` status posted with the App's installation token) is on the head, run `gh pr merge <n> --auto`. The queue retests against the tip and squashes. Then run `gt sync` so the stacks above it restack.
- **Switching to the new tooling.** Once each piece of #789 lands, use it instead of the manual steps below:
  - `monacoctl agents dispatch`, `watch`, `verify-plan`, `verdict` and `conflicts`
  - the hooks
  - the `ready` job
  - the short briefs in `docs/agents/`
  - the `monaco-milestone` skill as a thin layer over pstack's autopilot-stack

  The manual rules below are the fallback until then.
- **The verifier App exists.** It is `monaco-verifier`: App ID 5101392, installation 165592710, commit-status write only.
  - The key is `~/.config/monaco/verifier.pem`. Never print it or the tokens it mints.
  - `app-jwt.sh`, in this directory, signs the JWT.
  - PR 1 makes the ruleset require `verify` from integration_id 5101392.
  - Until `monacoctl agents verdict` lands, the verifier (never an owner) posts `verify` by minting an installation token and calling `POST /repos/lognorman20/monaco/statuses/<sha>`.

## Phase 2: the 9 tickets

Start when #789 is fully merged. Run them with the #789 tooling.

## Scope

- **Tickets:** #479, #480, #481, #482, #483, #484, #486, #490, #491. Nothing else. #488 (deploy) stays out.
- **#491 is partial.** Build `monacoctl garden report` and `gardener.yml` only. Do not create the /schedule routine.
- **Dependency order** comes from each ticket's "Blocked by" line. Every other blocker is already on main. The longest chain is 479 → 480 → 481 → 483 → 484 → 486 → 491. #482 runs alongside #483. #490 runs alongside #486.
- **Start #483 and #484 (verify-backend) as soon as their blockers land.** Every later ticket uses `just verify backend` as proof.

## Branch model

- **Feature branch.** `backend-rewrite-3` already exists, created from main at 6a13e756. Every ticket PR targets it. Do not open the checkpoint PR (`backend-rewrite-3` → main) until all 9 are merged into it, because Graphite refuses to stack on a branch whose own PR has already merged.
- **Graphite.** Each ticket is a Graphite stack on `backend-rewrite-3`: `gt create`, `gt modify`, `gt submit --no-interactive --publish`.
  - Try `gt trunk --add backend-rewrite-3` first.
  - If gt refuses, use `gh pr create --base <parent>`, then `gt track`.
  - Keep each PR under 1000 changed lines.
  - PR bodies use `.github/pull_request_template.md`, set with `gh pr edit --body-file`.
  - A ticket's last PR says `Closes #n`, and the others say `Part of #n`.
- **Always start from the tip.** Create each owner worktree under `.worktrees/` from the current tip of `backend-rewrite-3`, never from another ticket's branch. Never git-write in the primary checkout at `/Users/logno/Developer/monaco`.
- **Merge as soon as a ticket passes.** Once it has an independent PASS and green CI on that exact commit, merge it within minutes. After #789's PR 1 is merged, use the merge queue (`gh pr merge <n> --auto`, then `gt sync`). Before that, fast-forward `backend-rewrite-3`:
  1. Retarget the ticket's PRs to `backend-rewrite-3`.
  2. Push with `git push origin "${SHA}:refs/heads/backend-rewrite-3"`, which marks the PRs merged.
  3. Delete the merged local branches.

  If the ticket isn't a fast-forward, the owner rebases it onto the tip. Only if the owner's branch SHAs must stay fixed, merge it into the tip with a merge commit whose tree you have checked. Never hold passed work.
- **Owners resolve their own conflicts.** You resolve only mechanical union conflicts: sorted lists in `msgs.go`, `.golangci.yml`, `go.mod` via tidy, and generated docs.

## Agents

- **Models.** Owners and verifiers are `pstack:poteto-agent` subagents. Use opus, sonnet or haiku only, never fable. Owners use opus. The verifier must use a different model from its owner: sonnet by default, and opus for db, bus, money or concurrency code.
- **Parallelism.** At most 6 owners at once. Heavy work runs on CI, not this laptop.
- **Keep the laptop awake.** Run `caffeinate -dimsu -w $$` for the whole run, because the Mac slept 49 minutes during M7.
- **Briefs.** Copy `/Users/logno/Developer/monaco/.git/pstack/m7/OWNER_BRIEF.md` and `VERIFIER_BRIEF.md` into `/Users/logno/Developer/monaco/.git/pstack/m7-rest/`. Fix the stale paths in the copies: the heavy.sh path and the `m7-root` worktree. Replace "no CI dispatch" with the CI rule below. Dispatch prompts carry only the ticket number, the worktree path, the parent SHA and the brief path. Never paste the brief itself.
- **Right-size verification.**
  - Test-only changes, or under 50 changed lines: you plant one defect yourself and watch it get caught. That is your verdict.
  - Anything else: a fresh verifier, briefed to plant defects and rerun the ticket's Verification commands.
  - Findings go to the owner as a fix round. Each finding gets a red test first.
- **Keep token use low.**
  - If an owner's transcript is huge (over roughly 250k tokens), do not resume it for a small fix. Dispatch a fresh owner with the trail, the branch and the findings instead.
  - Brace shell variables (`"${T}:refs/..."`), because zsh eats `$T:r`.
  - Keep command output short: `tail`, `grep`, and `head -N`.
- **Watch every 10 minutes by side effects**: commits, pushes, PRs, issue comments, worktree changes.
  - An agent with no side effect in 20 minutes gets asked once, then stopped and replaced.
  - An agent that reported STACK-READY or done gets stopped with TaskStop, and you confirm it stopped with ListAgents.
  - Edit the #492 status comment (id 5854306170) in place on every merge into `backend-rewrite-3`.

## CI

- **Dispatch CI yourself until #789's PR 1 lands.** Before it, CI runs on ready PRs to main only, so PRs into `backend-rewrite-3` get no automatic CI. After it, CI runs automatically on those PRs and on merge-queue groups. Dispatch `gh workflow run ci.yml --ref <branch>` once per verified head, and merge only when its `ci / ci-ok` is green on that exact commit.
- **Mutation gate.** It mutates only changed lines, one job per package, with a 10-minute limit each. Never run `gremlins` or `monacoctl mutation` locally: it filled the shared test Postgres once. A surviving mutant must be killed with a test, or listed in `mutants.allow` with a reason.
- **Checkpoint.** The checkpoint PR (`backend-rewrite-3` → main) gets the `integration` label, which skips the mutation matrix, and the `large-pr` label. Every `Closes #n` for #790, #789 and the 9 tickets goes in its body. Merge it only when the operator says to.

## Machine safety and verification

- **Test databases.** Tests use only the tmpfs test container. Never touch `monaco-postgres` or its durability settings. Drop only the databases you created, by exact name.
- **Verification is never skipped.** For every ticket, run all of these and prove every acceptance-criteria box with evidence in the PR's Proof section:
  - its Verification commands
  - `env -i HOME=$HOME PATH=$PATH just test backend` (100% coverage, 60 s run, 10 s per package)
  - golangci-lint
  - `GOTOOLCHAIN=go1.25.14 go vet ./...`
  - CI
- **#484:** prove each hook live with a headless `timeout 180 claude -p` inside the worktree, and paste the blocked-then-allowed transcript.
- **#490:** evaluate it with a fresh agent that is given only `apps/backend/AGENTS.md` and the skills.
- **#486:** it replaces CI jobs. Reconcile it with the current `ci-jobs.yml`, which already has the mutation matrix, the `backend-test-env` action and the `integration` skip. Do not regress those.

## Needs from Logan

- Every ticket's issue comment and last PR body ends with `**Needs from Logan:**` followed by `Nothing.` or a checklist. Before listing an item, exhaust the CLI, API and headless routes.
- Add the `needs-approval` label to a ticket only when its list is non-empty.
- Never merge to main, arm auto-merge or close issues yourself.

## State and report

- **State:** `/Users/logno/Developer/monaco/.git/pstack/m7-rest/`. That's the ORDERS.md you write from this prompt, plus `decisions.tsv` (the show-me-your-work trail), `agents.tsv`, `stack.tsv` and `trails/<n>.tsv`.
- **Background reading:** `.git/pstack/m7/retro/evidence.md` and issue #789 explain why each rule above exists.
- **Final report:** first a #790 line (PR, verdict and the loud-failure proof), then a #789 section with its PRs, the before and after for each enforced rule, and which fallbacks you still needed. Then:
  - links to `backend-rewrite-3` and the checkpoint PR
  - one line per ticket: PRs, verdict, head SHA, rounds, wall time
  - anything parked, with the reason
  - the #492 Needs comment link

## Operator addition 2026-09-27 ~20:05

"also i want to enforce conventional commits using the /commit skill." Root default: commits follow Conventional Commits, written with the `/commit` skill; enforced by a PreToolUse hook (#789 PR 2) and a PR-format CI check on base..head commits (#789 PR 3, merge-queue squash excepted). PR titles keep the AGENTS.md no-prefix rule unless the operator says otherwise.

## Operator correction 2026-09-27 ~20:35

Land, then write. A PR is dispatched only from a tip that already contains PR 1 and every earlier PR it shares files with, so no owner ever rebases old work. The root broke this by starting #789 PRs 2 to 4 before PR 1 landed and PR 7 before PR 2 and PR 4 landed. PR 7 is on hold until they land.

## Operator 2026-09-27 ~20:45: no repeated work

Root tools in /Users/logno/Developer/monaco/.git/pstack/m7-rest/bin: dispatch.sh (refuses unless every dependency PR has landed, then makes the worktree at the tip), once.sh (a slow command runs once per exact tree, and later calls return the cached pass), carry.sh (a verdict carries to a new head when the patch-id is unchanged, so no second verifier). The root uses dispatch.sh for every dispatch and carry.sh before any re-verify. Briefs carry the owner and verifier rules. Updates use gt sync + gt restack only. Repo-level enforcement goes into #789 PR 6: test-backend.sh caches by tree hash, the agent guard blocks raw git rebase in owner worktrees, and scripts/pre-report.sh picks gates from the CI path filters.

## Operator 2026-09-27 ~21:10: stop before the next wave

Land what is in flight: #789 PR 3 (#792), PR 4 (#797, #798, #804, #800, #805) and the bus-hang fix. Dispatch nothing new (no PR 5, 6, 7, 8, no milestone tickets). Then report what remains and stop.

## Operator 2026-09-27 ~21:20: a pure rebase reruns nothing

A PR that passed its checks and verdict merges without being brought up to date; a clean rebase does not rerun CI or verification. Ruleset 24089171 now has strict_required_status_checks_policy=false (applied by root via gh api). scripts/feature-branch.sh still writes strict=true, so re-running `apply` would undo this: fix the script and its test in the next #789 PR that touches it. Stacked children get CI by workflow_dispatch on their own heads; a dispatch run's ci-ok counts for the PR head.

## Operator 2026-09-27 ~21:45: main merges by hand

Auto-merge only into non-main branches. Merges into main are the operator's, by hand. Enforced by #810: a workflow disables auto-merge on any PR into main, and the agent guard blocks `gh pr merge` on a PR into main.
