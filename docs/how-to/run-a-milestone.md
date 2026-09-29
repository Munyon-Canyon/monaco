# Run a milestone

This page is for the orchestrator of a milestone, a person or an agent session called the root. The root turns the milestone's tickets into merged PRs on the feature branch, then into a checkpoint on `main`. It writes no ticket code itself. Each ticket's owner follows [Ship a ticket](ship-a-ticket.md).

Set up the clone with [Agent workflow setup](../agents/setup.md). The [standing orders](../agents/standing-orders.md) bind the root as well as every owner and verifier. The rules behind these steps are in [Pull requests: small and stacked](../architecture/backend-platform.md#pull-requests-small-and-stacked) and [CI](../architecture/ci.md). This page says what to run and when.

## Roles

| Role | Who | Does |
| --- | --- | --- |
| Operator | a person | Merges each checkpoint into `main` by hand. Adds the `large-pr` and `gate-change-approved` labels. Owns accounts, secrets and paid services. |
| Root | a person or an agent | Writes and batches tickets, dispatches owners and verifiers, lands PRs, restacks, keeps the tracking issue current. The only session that runs `gt sync` or restacks. |
| Owner | an agent on `opus`, one per ticket | Builds the ticket's stack, runs stage 0, submits drafts, sets bodies, exits. |
| Verifier | an agent on another model (`sonnet` for an `opus` owner) | Reviews each PR against its ticket, posts `verify`, lands a passing PR. Runs no tests. |

## Start a milestone

Do these once per milestone.

1. Pick the `<feature>` name: a lowercase slug such as `leaderboards`. Its feature branches are `<feature>-checkpoint-<N>`, such as `leaderboards-checkpoint-1`, and must match `^[a-z0-9]+(-[a-z0-9]+)*-checkpoint-[0-9]+$`. The first is `<feature>-checkpoint-1`, and each checkpoint cuts the next. Several milestones run at once, each on its own feature branch, and each checkpoints on its own.

2. Create `<feature>-checkpoint-1` from `main` and protect it. This needs an org admin token.

        scripts/feature-branch.sh init <feature>-checkpoint-1

    `init` creates `<feature>-checkpoint-1` at `origin/main`, then runs `scripts/feature-branch.sh apply <feature>-checkpoint-1`. `apply` adds the Graphite trunk, turns on auto-merge and merge commits, and adds the exact `refs/heads/<feature>-checkpoint-1` to the include list of the one feature branch ruleset. A ruleset with a merge queue takes exact ref names only, so each checkpoint's `next-branch` job adds the branch it cuts. If `<feature>-checkpoint-1` already exists, run `apply <feature>-checkpoint-1` alone. [Feature branches](../architecture/ci.md#feature-branches) describes both rulesets.

3. Add the milestone's tracking issue to `.monaco/agents.toml` as `[features.<feature>]` with `tracking = <issue>`. Give each ticket a `**Base branch:** `<feature>-checkpoint-<N>`` header: `dispatch`, `batch` and `check` read it, and `--branch <feature>-checkpoint-<N>` overrides it. The file also holds `repo`, `lanes` (the most owners running at once), `batch` (the most tickets per batch), `milestone` (the name of the local state directory under `.git/pstack/`), and the verifier App's `verifier_app` and `verifier_installation`. `[check.budget]` holds the stage 0 budget of each row.

4. Open the tracking issue. Its body holds the wave table: one row per ticket with its wave, issue, title and blockers. `monacoctl agents status --publish` adds the status comment. `monacoctl agents` reads only status, batch and handoff comments written by an owner, member or collaborator of the repository or by `github-actions[bot]`, and edits only its own. Anyone else's comment with the same marker is ignored, and a new comment is posted instead.

5. Make a worktree at the feature-branch tip for the root's own commands, and build the tools there:

        git worktree add --detach .worktrees/root origin/<feature branch>
        cd .worktrees/root
        just build backend

    `just build backend` writes `bin/monacoctl`. The commands below run as `bin/monacoctl agents <command>` from that worktree. Rebuild after any merge that changes `apps/backend/cmd/monacoctl/agents`. Every worktree of the clone shares the records under `.git/.monaco/agents/` and `.git/pstack/<milestone>/`, so any worktree works. Each owner record is also a comment on its ticket, marked `<!-- monacoctl agents record -->`. `dispatch` posts it, and `done`, `exited` and `verdict` update it. A command that needs a record this clone lacks rebuilds it from the newest such comment, with the worktree path set to this clone's `.worktrees/<n>`.

6. Confirm the `MERGE_BACK_TOKEN` repo secret exists (`gh secret list`). `checkpoint.yml` needs it to cut the next feature branch, its `Variables: write` permission to move the `FEATURE_BRANCH` fallback variable, and its `Administration: write` permission to add the new branch to the feature branch ruleset. The verifier's statuses post as the verifier App when `~/.config/monaco/verifier.pem` exists, and as your `gh` user otherwise.

7. Start the milestone's decision log at `docs/milestones/<milestone>.md`. Every milestone orchestrator keeps one, like the [M7 closeout log](../milestones/m7-closeout.md). Write one line per decision as it happens: the time, what was decided and why, and what broke and how it was fixed. Keep the log on its own branch with a draft PR, commit each batch of entries with `gt modify`, push with `gt submit --stack --no-interactive --draft`, and land it at each handoff and checkpoint. A log that exists only in one session is lost when that session ends.

## Write tickets

1. Write each ticket in the write-ticket format (`.cursor/skills/write-ticket/SKILL.md`): Context, Problem, Proposal with an "Implement exactly" list, Acceptance Criteria, Verification and Done when. Leave the owner as few choices as you can.

2. Start the body with the header line. `batch` and `dispatch` parse it:

        **Milestone:** M7 Backend platform · **Blocked by:** #483, #536 · **Tracking:** #492 · **Base branch:** `<feature>-checkpoint-<N>` · **Touches:** `apps/backend/cmd/monacoctl/agents/**`, `docs/architecture/ci.md`

    - Put every `Touches` glob in backticks. A bare `**` breaks GitHub Markdown. `batch` defers a ticket with no `Touches` and keeps two tickets whose globs overlap out of one batch.
    - `Blocked by` lists issue or PR numbers, or `none`. An issue blocker counts as merged when it is closed as completed, or when a PR merged into the feature branch says `Closes #<n>`. A PR blocker counts when its merge commit is on the feature branch.

3. Set the ticket's milestone and add its row to the tracking issue's wave table.

4. Mirror the header in GitHub's native "Blocked by" links. The header is the source of truth. The links only drive GitHub's UI. `scripts/sync-blocked-by.py` compares the two for every open ticket in the milestone, skipping the tracking issue. It prints the plan by default and changes links only with `--apply`:

        scripts/sync-blocked-by.py --milestone "<milestone title>" --tracking <n>
        scripts/sync-blocked-by.py --milestone "<milestone title>" --tracking <n> --apply

    Run it after you add a ticket or change a `Blocked by` line.

5. When a ticket's scope changes, edit its body and add a comment that says why. A ticket must never describe work that no longer matches the plan.

## Run a batch

1. Check the machine. Run `uptime` and hold new dispatches while the 1-minute load is over about 12. Each owner runs `agents check`, and five at once pushed the load past 40 in M7.

2. Pick the tickets:

        bin/monacoctl agents batch <n>...

    It admits up to `batch` tickets and prints `deferred #<n>: <reason>` for each one it holds back: no `Touches`, an unmerged blocker, a blocker in the same batch, a `Touches` overlap or a full batch. It writes `.git/pstack/<milestone>/batch.json` and replaces the last batch. Copy the old `batch.json` first if you still want its timeline.

3. Publish the board:

        bin/monacoctl agents status --publish

    Run it again after every dispatch, merge and ejection. `agents-status.yml` also refreshes it on PR events and every 10 minutes while a batch is active.

4. Dispatch each admitted ticket:

        bin/monacoctl agents dispatch <n> --model opus

    It fetches the feature branch and refuses a ticket outside the batch (unless you pass `--urgent`), a ticket with an unmerged blocker, and a dispatch past the `lanes` cap. It then creates `.worktrees/<n>` detached at the tip, writes the owner record, starts `caffeinate` for the calling Claude Code process, and prints the spawn line, the prompt and a conflict forecast. `--dry-run` prints the same without changing anything. `--urgent` also logs the dispatch on the tracking issue. `dispatch` must run inside a Claude Code session, because it ties `caffeinate` to that process.

5. Spawn the owner exactly as printed: `subagent_type` `pstack:poteto-agent`, the printed model, in the background, with the five-line prompt. The `scripts/agent-guard-dispatch.py` hook refuses a spawn that carries a `brief:` line with another agent type, a model other than `opus` or `sonnet`, or no dispatch record. Then record the agent ID:

        bin/monacoctl agents own <n> <agent-id>

6. When the owner reports its PRs, run `bin/monacoctl agents done <n>`. Once its process has stopped, run `bin/monacoctl agents exited <n>`. The lane cap counts every owner not marked exited, and `agents check` splits the CPUs among them.

7. If an owner stops because stage 0 failed only on a budget under load, run the failing package on its own to confirm it passes. Then submit the stack yourself from the owner's worktree and set its bodies with `scripts/pr-body.sh`. The owner hook does not apply to the root.

## Verify and land

1. Plan the review of each PR:

        bin/monacoctl agents verify-plan <pr>

    It prints the kind (`light` or `full`), the verifier model, the reason and the spawn line. A diff with 50 or more non-test lines, a change under `platform/db`, `platform/bus`, `platform/money`, `platform/concurrency` or the `trading`, `treasury` and `funding` modules, or a patch that adds goroutines, channels or locks is `full`. The verifier model never equals the owner's model. One verifier may review a whole stack.

2. Spawn the verifier as printed. It reviews the diff against the ticket's Done when and acceptance criteria, reads `gh pr checks <pr>` once, runs no tests, writes a report file, and posts:

        bin/monacoctl agents verdict pass <pr> <head sha> --kind <kind> --model <model> --report <file>

    `verdict` refuses the owner's model, `fable`, a SHA that is not the PR head, and a kind weaker than the plan's. On `fail` the report says exactly what to fix.

3. Land a passing PR.

    - A single PR: `gh pr merge <pr> --auto`. The hook refuses it without a `verify` success on the head. Auto-merge queues the PR once stage 1 passes.
    - A stack: post a verdict on every PR, then run `bin/monacoctl agents land-stack <top-pr>`. It points the upper PRs at the feature branch, writes `Lands stack: #a #b #c` as the top body's first line, and queues only the top PR, so stage 2 runs once. If a PR still lacks stage 1 or `verify`, it prints `not landing #<n>; waiting on ...` and exits 0. Run it again later.

4. After a stack merges, run `land-stack <top-pr>` once more. It closes any lower PR GitHub did not mark merged, runs `gt sync` in the stack's worktree and clears the queued mark.

5. Close the ticket when its last PR merges: `gh issue close <n>`. A merge into a branch other than the default branch does not close issues.

6. On a `fail` verdict, spawn a fresh owner on the same worktree with the report. It amends with `gt modify` and submits again, and a new verifier reviews the new head. Give one problem at most three `opus` attempts. The last resort after that is one `fable` attempt at medium effort, and then you park the ticket with a comment. The dispatch hook accepts only `opus` and `sonnet` for a spawn with a `brief:` line, so the `fable` attempt is a plain spawn that carries the failure evidence and no brief.

## Watch the run

Run these on each pass through the queue. Never poll CI with `gh run watch` or a sleep loop.

- `bin/monacoctl agents watch` clears the queued mark of an ejected stack, lists owners idle for 20 minutes or finished but still running, and reports each queue ejection or red stage 1 with its failing job and a fresh-owner prompt. It exits nonzero when it flagged anything.
- `bin/monacoctl agents forecast` lists files that more than one open stack touches. Land those stacks one after the other, not together.
- `bin/monacoctl agents conflicts <pr>` says whether a PR is behind its base and which files conflict.
- `bin/monacoctl agents timeline` prints dispatch, push, stage 1, verdict, queue and merge times for each ticket of the batch. `--batch <file>` reads a saved batch.

Act on an ejection by its cause, as the table in [When a check fails](ship-a-ticket.md#when-a-check-fails) says. Two more rules keep the queue moving:

- **Land a fix ahead of what it unblocks.** When a defect on the tip ejects PRs, stop requeueing them. Land the fix first, then requeue the rest behind it. A queue group built without the fix fails again. To pull a queued PR out, dequeue it by its node ID:

        gh api graphql -f query='mutation { dequeuePullRequest(input: {id: "<pr node id>"}) { clientMutationId } }'

- **Land overlapping stacks in order.** When two stacks edit the same file, land one, then restack the other onto the new tip.

## Restack a stack

Only the root restacks, one stack at a time.

1. Update the trunk without restacking anyone's branches:

        gt sync --no-interactive --no-restack

    A plain `gt sync` restacks every tracked branch, including owners' branches mid-build. The hook blocks it.

2. In the stack's worktree, check out its bottom branch and restack it and everything above:

        gt checkout <bottom branch>
        gt restack --upstack

    Resolve each conflict in the branch where it appears. Run `git diff --check`, then `git add` the files and `gt continue`.

3. Check every branch of the stack, not only the top. A clean textual restack can still break compilation.

        gt checkout <branch>
        cd apps/backend && go run ./cmd/monacoctl agents check

4. Confirm the remote has no commit the local stack lacks. `git log --oneline <branch>..origin/<branch>` prints nothing for each branch. Then push:

        gt submit --stack --no-interactive --draft

5. Where a PR's diff did not change, repost its verdict on the new head: `bin/monacoctl agents verdict carry <pr>`. Where it changed, verify it again. Then land the stack.

The hook refuses `gt submit`, `gt modify` and `gt restack` while the stack is queued. Run `land-stack <top-pr>` to clear a stale mark first.

## Open a checkpoint into main

A checkpoint squash-merges the feature branch into `main`. Only the operator merges it.

1. Make sure `main` has nothing the feature branch lacks: `git log --oneline origin/<feature branch>..origin/main` prints nothing. If it prints commits, land a PR on the feature branch that merges `origin/main` first, or `tree-matches` fails after the squash.

2. Land a PR on the feature branch that renames `## [Unreleased]` in `apps/backend/CHANGELOG.md` to `## [checkpoint N] - <date>` and opens a new `## [Unreleased]` with only an empty `### Added` heading. The `Changelog (checkpoint into main)` check requires it.

3. Drain the feature branch. No ticket PR or stack may still be open on it: `gh pr list --base <feature branch> --state open` prints nothing. Land each open PR, or park it by closing it with a comment. GitHub deletes the feature branch with the checkpoint merge and retargets any PR still open on it to `main`, where its diff is wrong. `next-branch` warns about each ticket PR it finds on `main` after the cut, but that warning is only the backstop.

4. Check for an open checkpoint PR: `gh pr list --base main --head <feature branch>`. GitHub allows one open PR per head and base. An open one already shows every later merge, so update its body instead of opening another.

5. Open the PR with a body file that follows the template and lists the tickets merged since the last checkpoint:

        gh pr create --base main --head <feature branch> --label integration --title "<what the checkpoint ships>" --body-file <file>

    `gh pr create` is right here, since the PR is not a Graphite stack. `scripts/pr-body.sh` refuses a checkpoint, because the PR format check reads every commit since `main`, including old ones without Conventional subjects. That check is not required on `main`: only `ci / ci-ok` and the changelog check are. List the `large-pr` label and the merge under "Needs from Logan" for the operator.

6. After the operator merges, check `checkpoint.yml`: `gh run list --workflow checkpoint.yml --limit 1`. `tree-matches` proves `main` got the feature branch's exact tree, and `next-branch` creates `<feature>-checkpoint-<N+1>` from the squash commit on `main`. `next-branch` also puts `refs/heads/<feature>-checkpoint-<N+1>` in place of the old ref in the feature branch ruleset. If it warns that it could not, run `scripts/feature-branch.sh apply <feature>-checkpoint-<N+1>` with an org admin token. If either job fails, or `next-branch` warns about the variable or about a PR now based on `main`, report it and move that PR onto the new branch as in the next step. Never push to `main` or a feature branch yourself.

7. Continue on the new branch. GitHub deletes `<feature>-checkpoint-<N>` with the merge and leaves it deleted. Run `gt trunk --add <feature>-checkpoint-<N+1>`, move each open stack onto it with `gt track --parent <feature>-checkpoint-<N+1>`, `gt restack --upstack` and `gt submit --stack --no-interactive --draft`, and base new tickets on it. [Feature branches](../architecture/ci.md#feature-branches) has the details.

8. GitHub runs `schedule` and `workflow_dispatch` workflows only from the default branch. A workflow added on the feature branch cannot run until its checkpoint lands. Then start it with `gh workflow run <file>`.

## Hand off

A milestone or a ticket can change hands at any point. Everything the next person needs lives on GitHub and in the repo, so the incoming orchestrator can work from a fresh clone.

The outgoing orchestrator:

1. Commits the milestone decision log (`docs/milestones/<milestone>.md`) and pushes its branch.
2. Makes sure every in-flight owner's branch is pushed. A draft PR is fine. Its "What came up" section records where the owner stopped and what comes next. For an owner that is still running, stop it first and push what it has with `gt submit --stack --no-interactive --draft` from its worktree.
3. Refreshes the board and writes the handoff comment on the tracking issue. The handoff names the batch board, the running agents and the next command to run.

        bin/monacoctl agents status --publish
        bin/monacoctl agents handoff

The incoming orchestrator:

1. Sets up the clone ([Agent workflow setup](../agents/setup.md)), then makes the root worktree and builds `bin/monacoctl` as [Start a milestone](#start-a-milestone) step 5 says.
2. Reads the handoff comment, the status board on the tracking issue and the decision log.
3. Runs `bin/monacoctl agents watch` to see ejections and idle owners.
4. Carries on. Owner records rebuild from their ticket comments the first time a command needs them, so `resume`, `verdict` and `land-stack` work on in-flight tickets. A record is trusted only from a comment whose author is a repo owner, member or collaborator, because anyone can comment on a public repo. Each person's commands edit only their own record comment and post a new one otherwise. `bin/monacoctl agents resume <n> --transcript <file>` says whether a transcript is small enough to resume (250k tokens or fewer). A transcript from another machine is usually unavailable, so give the ticket a fresh owner on its pushed branch. The fresh owner's worktree starts from the branch, not from the parent SHA.

One ticket changes hands the same way. [Hand off a ticket](ship-a-ticket.md#hand-off-a-ticket) has the owner's side.

## Lessons from M7

Each rule below came from a failure in the M7 run.

- **A PR format failure caused by the body is fixed by editing the body.** `gh run rerun --failed` replays the original event with the old body, and it failed again on #895 and #962.
- **Order the queue so a fix lands ahead of the PRs it unblocks.** Three PRs queued ahead of the teardown fix #952 would have failed on the same defect. They were dequeued and requeued behind it.
- **One ejection from a proxy or runner error is an infra flake, not a code defect.** Docker Hub, Go proxy and `curl` 500 errors ejected #843, #933 and #934. Each landed on requeue with no code change.
- **Build every branch after a restack.** The #482 stack restacked cleanly and still failed to compile, because the tip had renamed a helper that #903 called.
- **Only one checkpoint PR can be open per head and base.** Checkpoint 4 could not open while checkpoint 3 (#894) was open. Its live head carried the later batches instead.
- **GitHub dispatches and schedules workflows only from the default branch.** Dispatching the gardener from the feature branch returned 404 until its checkpoint reached `main`.
- **Never run plain `gt sync` during a batch.** It restacked owners' in-progress branches twice.
- **Cite no SHA that a restack can change.** Amended SHAs and `agents check` tree hashes in bodies failed PR format on #838 and #848. Write tree hashes as `<tree>`.
- **Never commit build output.** Two 35 MB binaries were committed (#837, #869). The size check now fails any binary outside `testdata/` and media files.
- **Never run mutation testing locally.** One owner's local run pushed the load to 47 and slowed every other owner.
- **Queue runs start with a cold Go cache.** Tests that invoke the Go toolchain passed on the PR and went over the package budget in the queue. They now skip under `-short`, and `go-cache.yml` warms the cache on the feature branch.
- **Diagnose the cause, not the symptom.** The #897 ejection looked like a missing changelog heading. The cause was a matcher that also matched the heading's text inside the preamble.
- **Check units against a real response on money paths.** The #537 verifier caught Jupiter's `priceImpactPct` read as a fraction instead of a percent, which would have refused every real trade.
- **Watch the machine, not only the queue.** Five owners running stage 0 at once pushed the load to 60, and every `agents check` then went over budget. Dispatch in a rolling window instead.
- **Save `batch.json` before the next batch.** `batch` replaces it, and `timeline` needs it.
