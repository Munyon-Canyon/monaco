# M7 handoff: finish the milestone on `backend-rewrite-3`

This is for the next person to run the root orchestrator loop. You pick up after the first wave: #790, #789 PRs 1 to 4, and five fixes. You finish #789 PRs 5 to 8, then the nine tickets, then open the checkpoint PR.

## State on 2026-09-27, 22:00 EDT

- **Feature branch.** `backend-rewrite-3`, tip `edb0e1db`. It is a Graphite trunk. Every ticket PR targets it.
- **Merged into it** (oldest first):
  - #790: `just migrate db`, and a schema-behind boot that names the fix.
  - #789 PR 1: ruleset and CI triggers.
  - #789 PR 2: #796 agent guard hook and `scripts/pr-body.sh`, #799 SubagentStop reaper.
  - #789 PR 3: #792 `ready` job, PR-body and commit-subject lint.
  - #789 PR 4: #797 timestamp migrations, #798 `testkit.Main` analyzer, #804 generator rename, #800 generated `.golangci.yml`.
  - #807: bus test hang (#806) and NATS drain leak (#801).
  - #808: CI on every PR, filter base `FEATURE_BRANCH`, no up-to-date rule.
  - #809: each CI job runs only when its own inputs change.
  - #810: merges into `main` are manual.
- **Merged since:** #805, the last of #789 PR 4 (tip `ba4ea93f`).
- **In flight.** #811, which makes the verifier App optional. Its ruleset change is already applied, and auto-merge is armed.
- **Issues.** #790, #789, #801 and #806 stay open until the checkpoint PR lands on `main`, because their `Closes` lines only fire there.
- **Status comment.** https://github.com/lognorman20/monaco/issues/492#issuecomment-5854306170. Edit it in place on every merge.

## Remaining work, in order

**Land, then write.** Dispatch a PR only from a tip that already contains every PR it depends on or shares files with. Nothing is written ahead and rebased later.

1. **#789 PR 5** (items D15, D16): the `wallclock` analyzer and the testkit database guard. Can start now.
2. **#789 PR 6** (items B8 to B10), plus four additions recorded on #789:
   - `scripts/test-backend.sh` caches a pass per exact tree.
   - The agent guard blocks a raw `git rebase` in an owner worktree.
   - `scripts/pre-report.sh` picks local gates from the CI path filters.
   - CI runs the `scripts/` Go tests. Today only `just test backend` does.
   
   Can start now.
3. **#789 PR 7** (items F20 to F25, C14): `monacoctl agents` and `scripts/conflict-forecast.sh`. Can start now.
   - Start a fresh owner from the tip, and follow PR 4's generated-registration pattern.
   - Earlier work sits on branches `789-7-agents-core` and `789-7-verify-plan` (closed PRs #802 and #803). Uncommitted `verdict` work was left in `.worktrees/789-7` on Logan's machine and is not in the state branch. Reuse any of it, but rebuild it on the new base.
   - Design notes: `trails/789-7.tsv` in the state folder.
   - `verdict` posts `verify` with the App when its key is present, else with the gh token, and must carry a verdict to a new head when each PR's own stable patch-id is unchanged.
4. **#789 PR 8** (items G26 to G30): the short briefs and the `monaco-milestone` skill. Starts after PR 7. This PR says `Closes #789`.
5. **The nine tickets**, using the #789 tooling as it lands. Order comes from each ticket's "Blocked by" line:
   - The main chain: 479 → 480 → 481 → 483 → 484 → 486 → 491.
   - #482 runs beside #483, and #490 beside #486.
   - #491 is partial: build `monacoctl garden report` and `gardener.yml` only, with no `/schedule` routine.
   - #488 (deploy) is out of scope.
6. **Checkpoint PR**, `backend-rewrite-3` → `main`. Open it only after everything above has merged.
   - Labels: `integration` and `large-pr`.
   - Body: a `Closes` line for #790, #789, #801, #806 and the nine tickets.
   - Logan merges it by hand. No agent merges into `main`.

Open M7 tickets outside this scope: #488, #535, #536, #537 and #538.

## How to run it

1. Unpack the state folder from branch `m7-rest-state` into your clone, then rewrite its paths:

       mkdir -p .git/pstack/m7-rest
       git fetch origin m7-rest-state
       git archive origin/m7-rest-state | tar -x -C .git/pstack/m7-rest
       .git/pstack/m7-rest/bin/relocate.sh

   It holds `PROMPT.md`, `ORDERS.md`, the briefs, `decisions.tsv`, `agents.tsv`, `stack.tsv`, `trails/` and `bin/`. It stays in `.git/`, so nothing in it lands in the product tree. The large `logs/` and `cache/` folders are left out.
2. The verifier App is optional (#811). `bin/post-verify.sh` posts `verify` with your gh token when `~/.config/monaco/verifier.pem` is absent. The ruleset no longer requires `verify`. The agent guard still refuses `gh pr merge` until the head's latest `verify` is `success`.
3. Read `PROMPT.md`, then every dated section at the bottom of `ORDERS.md`. Those sections are operator decisions made after the prompt, and they override it.
4. In Claude Code at the repo root, invoke `pstack:poteto-mode` and tell it to run `.git/pstack/m7-rest/PROMPT.md` from Phase 1, PRs 5 to 8, as root orchestrator. Say "never use fable". Arm the audit loop with `/loop`.
5. Owners and verifiers are `pstack:poteto-agent` subagents.
   - Owners use opus.
   - The verifier must use a different model from its owner: sonnet by default.
   - Dispatch prompts carry only the ticket, worktree, parent SHA and brief path.

## Tools in `bin/`

These are root-only until PR 7 replaces them with `monacoctl agents`.

- `dispatch.sh <name> <dep-pr>...` creates `.worktrees/<name>` at the tip. It refuses if any dependency PR has not landed.
- `once.sh <cmd>` runs a slow command once per exact tree. Owners and verifiers run the full suite through it.
- `carry.sh <verified-sha> <new-sha>` prints CARRY when the trees or patch-ids match. Otherwise it prints REVERIFY.
- `post-verify.sh <pr> <sha> success|failure "<desc>"` posts `verify`, as the App when its key is present, else with your gh token. It refuses any SHA that is not the PR head.
- `restack-carry.sh <worktree> <pr>:<patch-id>...` runs `gt sync` and `gt restack`, checks every PR's own patch-id, then pushes and re-posts `verify`.
- `land-chain.sh <worktree> <pr>:<patch-id>...` lands a verified stack from the bottom up. It restacks automatically when a PR goes DIRTY after its parent's squash.
- `heavy.sh <cmd>` queues heavy local jobs one at a time. Never run mutation locally; CI does.

## Landing

- **Ruleset.** Ruleset 24089171 on `backend-rewrite-3` requires `ci / ci-ok` and `PR format (title, body and commits)`. `verify` is enforced by the agent guard, not the ruleset. It is squash only, with no direct pushes. It does not require branches to be up to date. No merge queue is possible, because GitHub rejects one on a personal-account repo.
- **Merge.** Get an independent PASS, then a green `ci-ok` on the exact head. Post `verify` with `post-verify.sh` and run `gh pr merge <n> --auto --squash`.
  - A PR under 50 changed lines, or one that only changes tests: the root plants one defect and watches it get caught.
  - Anything else gets a full verifier.
- **Stacks.** Each squash leaves the next PR carrying a stale copy of its parent, so it goes DIRTY. Use `land-chain.sh`. It restacks and carries the verdict when patch-ids match. A changed patch needs a new verdict.
- **Before merging**, check for another open PR that touches the same files. #807 and #800 collided on `.golangci.yml` this way and cost a CI round. Use `scripts/conflict-forecast.sh` once PR 7 lands.

## Rules the operator added during the run

- Commits are Conventional Commits, written with the `/commit` skill. PR titles have no type prefix. The hook and the PR format check enforce both.
- A clean rebase reruns neither CI nor verification.
- CI runs only the jobs whose inputs changed. A workflow-only PR runs actionlint and nothing else.
- Only the root runs `gt sync`. It rewrites every local stack in the repo, including other owners' branches.
- Owners update with `gt sync` and `gt restack`, never a raw `git rebase`.
- Owners run every gate CI would run for their paths, including the Linux mobile-core Swift tests when `openapi.yaml` or the error codes change.
- Owners run the full suite and live `claude -p` proofs once, at the final head.
- Owners never poll CI. The root stops any agent with no side effect in 20 minutes.

## Known loose ends

- **`main-merges-by-hand.yml`** takes effect only once it is on `main`, that is, after the checkpoint.
- **Nightly QA** is disabled. Re-enable it after landing with `gh workflow enable nightly.yml -R lognorman20/monaco`.
- **Kept test databases.** Seven `t_testregistry_gauges*` databases from hang runs remain in `monaco-postgres-test`. Their creators are unknown, so nobody dropped them. `just reset` clears them.
- **Laptop load** reached 20 with four owners, which pushed the backend suite past its 60 s budget. Keep heavy work on CI.
