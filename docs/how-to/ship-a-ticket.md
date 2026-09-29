# Ship a ticket

This page takes one ticket from its GitHub issue to a merge on the milestone's feature branch. It is for the ticket's owner, a person or an agent. The orchestrator's side (batching, dispatch, verification and landing across many tickets) is in [Run a milestone](run-a-milestone.md).

The design behind each step lives elsewhere. [Pull requests: small and stacked](../architecture/backend-platform.md#pull-requests-small-and-stacked) holds the PR rules, [Verification scope](../architecture/backend-platform.md#verification-scope) holds the three check stages, and [CI](../architecture/ci.md) holds the jobs and the merge queue.

## Before you start

- Set up the clone once with [Agent workflow setup](../agents/setup.md), and read the [standing orders](../agents/standing-orders.md). They apply to a person as well as to an agent.
- Install the tools with `just install`: Go, `just`, `gh` and Graphite (`gt`). Authenticate `gh` and run `gt auth --token <token>` once.
- `.monaco/agents.toml` names the feature branch (`feature_branch`), the tracking issue (`tracking`) and the per-row check budgets. The examples below use `backend-rewrite-3`, the feature branch today.
- An agent owner gets a dispatch prompt with five fields: `ticket`, `worktree`, `parent`, `brief` and `orders`. The brief is [`docs/agents/owner.md`](../agents/owner.md), and `orders` names [`docs/agents/standing-orders.md`](../agents/standing-orders.md). The worktree already exists at the parent SHA, and `.git/.monaco/agents/<ticket>.json` registers it, so the agent guard hooks apply to it.
- A person owning a ticket makes the worktree by hand (step 2). The Claude Code hooks do not run in a plain terminal, so a person follows the same rules without the guard.

## Steps

1. Read the ticket in full.

        gh issue view <n>

    The header line names the milestone, `Blocked by`, the base branch and `Touches`. Change only the paths `Touches` lists. When the Proposal says "Implement exactly", do that and nothing more. An unrelated defect you find goes under "What came up" in the PR body, or into a new ticket.

2. Work in a worktree under `.worktrees/`, never in the primary checkout. A dispatched agent `cd`s into the `worktree` path from its prompt. A person makes one at the feature-branch tip:

        git fetch origin backend-rewrite-3
        git worktree add --detach .worktrees/<n> origin/backend-rewrite-3
        cd .worktrees/<n>

    Use absolute paths inside the worktree. An agent's shell may reset its working directory between commands.

3. Start the first branch and track it on the feature branch:

        git switch -c <n>-<slug>
        gt track --parent backend-rewrite-3

    Each further PR of the stack starts with `gt create <branch> -m "<subject>"`. Amend the current branch with `gt modify`, which also restacks the branches above it. Order the stack so each PR proves the next: deletions and renames, then schema, then `domain` and `app`, then adapters and HTTP, then the `flows.tsv` status change.

4. Write the change. Read `apps/backend/AGENTS.md` first for backend code, and the skill it names for the job:

    | Change | Read first |
    | --- | --- |
    | A command, query, consumer, provider or module | `.claude/skills/go-backend-module/SKILL.md` |
    | Goroutines, channels or fan-out | `.claude/skills/go-concurrency/SKILL.md` |
    | Ledgers, shares, swaps, transfers, the relayer or Privy signing | `.claude/skills/money-change/SKILL.md` |
    | A consumer or a new event | `.claude/skills/nats-consumer/SKILL.md` |
    | A failed `e2e` job or a flow moving to verified | `.claude/skills/verify-backend/SKILL.md` |

5. Commit with a Conventional Commit subject: `type(scope): subject`, where the type is one of `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore` or `revert`. PR format fails any other subject in the PR's commits.

6. Run stage 0 from `apps/backend` on the committed tree:

        cd apps/backend
        go run ./cmd/monacoctl agents check

    It diffs `HEAD` against `origin/backend-rewrite-3` and runs one row per kind of changed path: PR size and gate changes always, then `go build`, `go vet`, the lint row and `go test -short -count=1` on the affected packages, and the shell, `scripts`, Python, Swift, `ready`, migration, OpenAPI and `mkdocs` rows when their paths changed. [Verification scope](../architecture/backend-platform.md#verification-scope) lists every row and its trigger.

    - Each row has its own budget under `[check.budget]` in `.monaco/agents.toml`. The `go test -short` row has none as a whole. Instead each package gets the `package` budget (20 s), the same limit that fails a package in CI.
    - It prints at most 20 lines. The full log is the `log:` path it prints, under `.git/pstack/<milestone>/logs/`.
    - It refuses a dirty tree, because it records `HEAD`'s tree. On a pass it writes `.git/pstack/<milestone>/checks/<tree>`, and the hook lets an owner's push through only when that record matches the current tree.
    - Run it again after every `gt modify`. A tree that already passed prints `stage 0 already passed`.
    - If only a budget fails and the machine is loaded (check `uptime`), rerun it once the load drops. Run the named package on its own to prove the code is fine: `go test -short -count=1 ./<pkg>`.

    - Before stage 0, run what the change must regenerate, and commit the output. CI's `ready` job (`scripts/ci/ready.sh`) fails on a stale `go generate ./...`, `go mod tidy`, sqlc output, `scripts/gen-docs.sh` output or the `verify-backend` feature map. `scripts/ci/ready.sh` runs the same steps locally on a committed tree.

    Never run `just test backend`, `go test -race` or `go test ./...` from `apps/backend` as an owner. Never run mutation testing locally. The merge queue runs the full suite, and the nightly runs mutation.

7. Push the stack as drafts:

        gt submit --stack --no-interactive --draft

    A draft runs no CI. Without `--draft` a new PR opens ready, with the commit subject as its title and an empty body, and PR format fails at once.

8. For each PR, write its body to a file and set it:

        scripts/pr-body.sh <pr> "<title>" <file>

    The script runs the full PR format check locally, sets the title and body, and marks a draft ready, which starts stage 1. Fix whatever it reports and run it again. Never set a body with `gh pr edit --body` and never run `gh pr ready` yourself.

    - **Title:** what the PR changes, in the present tense, with no issue number and no `feat:` style prefix. For example, "Name the child that spent the verify teardown budget".
    - **Body:** the six sections of `.github/pull_request_template.md`: TLDR, Why, What changed, Proof, What came up, Reviewer focus. The `pr-summary` skill in `.claude/skills` drafts it.
    - **Why** says `Part of #<n>`. The ticket's last PR says `Closes #<n>` instead and adds a `## Needs from Logan` section, holding "Nothing." or a checklist of what only the operator can do.
    - **Proof** pastes the `agents check` output and says that CI runs the rest. Write the tree hash it prints as `<tree>`.
    - Cite only commit SHAs that are already on the feature branch. The check treats any 7 to 40 character hex string as a commit, and a restack changes the PR's own SHAs.
    - A docs-only PR says `No code paths affected:` in Proof and names the paths.

9. Stop. An owner never merges. The verifier reviews the diff against the ticket, posts a `verify` status and lands the PR (see [Run a milestone](run-a-milestone.md#verify-and-land)). A dispatched agent exits with its PR URLs, head SHAs, the stage 0 summary and every decision it made.

## Hand off a ticket

When you stop before the ticket is done, leave it so another person or agent can pick it up from a fresh clone.

1. Commit what you have and push the stack as drafts: `gt submit --stack --no-interactive --draft`. Unfinished work still goes up. A draft runs no CI.
2. Keep or open the draft PR, and write the state into its body: what is done, what is not, and what you decided. Put where you stopped and the next step under "What came up".
3. Post a comment on the ticket that names the branch, the PR and the next step:

        gh issue comment <n> --body "Handing off: branch <branch>, draft PR #<pr>. Next: <step>."

The next owner reads the ticket, its comments and the draft PR. It makes a worktree at the feature-branch tip, pulls the stack into it by its top PR, and carries on from step 4 of [Steps](#steps):

    git worktree add --detach .worktrees/<n> origin/backend-rewrite-3
    cd .worktrees/<n>
    gt get <top pr> --no-interactive

## What CI runs after you push

| Stage | Trigger | Runs |
| --- | --- | --- |
| 1. PR check | the PR is ready and based on the feature branch | `plan`, `lint`, `ready`, `vuln`, PR format, PR size and `gate-changes`. No tests. A push with an unchanged diff reuses the last green result. |
| 2. Queue check | the PR entered the merge queue | Stage 1, plus `backend` (the race suite with the per-package budget and 100% coverage), the tests `-short` skips, `e2e` (`scripts/ci/e2e.sh`), `flake` on changed test files, `scripts`, and `mobile-core` and `ios` when their paths changed |

Only the bottom PR of a stack runs stage 1, since CI runs on PRs whose base is the feature branch. The only required check is `ci / ci-ok`. `gate-changes` and the `status` check from `agents-status.yml` never block. [What runs where](../architecture/ci.md#what-runs-where) has every job.

## Read the checks

Read the checks once, and never watch them:

    gh pr checks <pr>

The result says which of these holds:

- Pending: nothing to do. The owner has already exited.
- `ci / ci-ok` failed: open the failing job's log from the link and fix the cause with `gt modify`, `agents check` and `gt submit --stack --no-interactive --draft`.
- `PR format (title, body and commits)` failed: see the first entry under [When a check fails](#when-a-check-fails).

## When a check fails

**PR format failed because of the body.** Fix the body file and run `scripts/pr-body.sh` again. The body edit runs a fresh check. Never use `gh run rerun --failed` here. A rerun replays the original event with the old body and fails again.

**PR format failed on a commit subject.** Reword the commit with `gt modify` (Conventional Commit subject), rerun `agents check` and submit again.

**PR size failed.** Split the branch below 1000 changed lines per PR with `gt split --by-hunk`, or with the `distribute-stack-changes` skill. Only a person adds the `large-pr` label. A committed binary also fails this check. Build into `bin/` or `/dev/null`, never into the tree.

**Stage 1 shows a stale red `ci / ci-ok` from a cancelled run.** A newer push cancelled it. The newest run is the one that counts.

**The queue ejected the PR.** `monacoctl agents watch` reports it with the failing job and a prompt for a fresh owner. Read the failing job's log, then act on the cause:

| Cause | Fix |
| --- | --- |
| Infra flake: a Go proxy or Docker Hub error, a `curl` 500 while installing a tool, a runner timeout with every package `ok`, a run cancelled by a newer one | Requeue with no code change: `gh pr merge <pr> --auto` for a single PR, `monacoctl agents land-stack <top-pr>` for a stack. For a failed PR-stage job, `gh run rerun <run-id> --failed` once. |
| A real test failure, including a package over the 20 s budget | A fresh owner fixes it on the same branch. The verifier reviews the new head, then the PR lands again. |
| A failure that already exists on the feature branch tip | Not this PR's defect. Fix the tip in its own PR, land that first, then requeue this one. |
| A merge conflict with a stack that landed ahead | Wait for that stack to land, restack onto the tip ([Restack a stack](run-a-milestone.md#restack-a-stack)), build every branch, and land again. |
| `land-stack` reports a stack still marked queued | Run `monacoctl agents land-stack <top-pr>` again. It sees the ejection, clears the mark and relands. |

**A test flakes.** Fix it the same day, or move it to the nightly with an issue. Never skip it, never raise its budget, and never retry CI until it passes. The rule is in [Keeping it fast](../architecture/backend-platform.md#keeping-it-fast). The `flake` job reruns every changed test file 20 times, so a fix proves itself in stage 2.

**The `e2e` job failed.** Follow `.claude/skills/verify-backend/SKILL.md`. It explains the `verify-evidence` artifact and how to reproduce one flow with `monacoctl verify flow <id>`.

## Money paths

A PR that moves or records money meets every item in `.claude/skills/money-change/SKILL.md`, and its body says how. The verifier checks at least these points by reading the diff:

- Amounts use `money.Micros`, `money.SignedMicros` or `money.BaseUnits`, never floats.
- Each balance change and its event commit together inside `uow.Do`, with a guarded update.
- A transfer stores its signature before broadcast.
- The relayer floor holds and was not lowered to pass a test.
- No key, token or signed transaction reaches a log.
- An external API's units match a recorded real response. In M7 a Jupiter `priceImpactPct` read as a fraction instead of a percent would have refused every real trade.
