# Run a milestone

This page is for the orchestrator of a milestone, a person or an agent session called the root. The root turns the milestone's tickets into merged PRs on `staging`, then promotes `staging` into `main`. It writes no ticket code itself. Each ticket's owner follows [Ship a ticket](ship-a-ticket.md).

Set up the clone with [Agent workflow setup](../agents/setup.md). The [standing orders](../agents/standing-orders.md) bind the root as well as every owner and verifier. The rules behind these steps are in [Pull requests: small and stacked](../architecture/backend-platform.md#pull-requests-small-and-stacked) and [CI](../architecture/ci.md). This page says what to run and when.

## Roles

| Role | Who | Does |
| --- | --- | --- |
| Operator | a person | Merges each promotion into `main` by hand. Adds the `large-pr` and `gate-change-approved` labels. Owns accounts, secrets and paid services. |
| Root | a person or an agent | Writes and batches tickets, dispatches owners and verifiers, lands PRs, restacks, keeps the tracking issue current. The only session that runs `gt sync` or restacks. |
| Owner | an agent on `opus`, one per ticket | Builds the ticket's stack, runs stage 0, submits drafts, sets bodies, exits. |
| Verifier | an agent on another model (`sonnet` for an `opus` owner) | Reviews each PR against its ticket and posts `verify`, which is advisory. Runs no tests. |

## Start a milestone

Do these once per milestone.

1. Check the trunk. Every milestone lands on `staging`, so there is no branch to create. `gh variable get FEATURE_BRANCH` prints `staging`. If the `staging` or `main` ruleset is missing or differs from what `scripts/branch-rulesets.sh` prints, an org admin applies it with that script. [Staging and main](../architecture/ci.md#feature-branches) describes both rulesets.

2. Run `gt init --trunk staging` once per clone, so Graphite builds stacks on `staging`.

3. Set `.monaco/agents.toml`: `repo`, `feature_branch` (`"auto"` reads the `MONACO_FEATURE_BRANCH` env, then the `FEATURE_BRANCH` repo variable; name a branch to pin it), `tracking` (the tracking issue number), `lanes` (the most owners running at once), `milestone` (the name of the local state directory under `.git/pstack/`), `queue_concurrency` (Graphite's merge queue concurrency; with that many `gtmq_` drafts open, `land-stack` waits for a slot instead of advising a rerun), and the verifier App's `verifier_app` and `verifier_installation`. The `[batch]` table holds `size` (the most tickets per batch) and `shared`, the globs two tickets may both touch and still share a batch. `[check]` holds `slots`, the most `agents check` runs that execute rows at once on a machine. `[check.budget]` holds the stage 0 budget of each row. To change capacity on one machine, set `lanes`, `[check] slots` or `[dispatch] max_load` in `.git/.monaco/agents.local.toml`. It applies to every worktree of that clone, is never committed and accepts no other key.

4. Open the tracking issue. Its body holds the wave table: one row per ticket with its wave, issue, title and blockers. `monacoctl agents status --publish` adds the status comment. `monacoctl agents` reads only status, batch and handoff comments written by an owner, member or collaborator of the repository or by `github-actions[bot]`, and edits only its own. Anyone else's comment with the same marker is ignored, and a new comment is posted instead.

5. Make a worktree at the `staging` tip for the root's own commands, and build the tools there:

        git worktree add --detach .worktrees/root origin/staging
        cd .worktrees/root
        just build backend

    `just build backend` writes `bin/monacoctl`. The commands below run as `bin/monacoctl agents <command>` from that worktree. Rebuild after any merge that changes `apps/backend/cmd/monacoctl/agents`. Every worktree of the clone shares the records under `.git/.monaco/agents/` and `.git/pstack/<milestone>/`, so any worktree works. Each owner record is also a comment on its ticket, marked `<!-- monacoctl agents record -->`. `dispatch` posts it, and `done`, `exited` and `verdict` update it. A command that needs a record this clone lacks rebuilds it from the newest such comment, with the worktree path set to this clone's `.worktrees/<n>`.

6. The verifier's statuses post as the verifier App when `~/.config/monaco/verifier.pem` exists, and as your `gh` user otherwise.

7. Start the milestone's decision log at `docs/milestones/<milestone>.md`. Every milestone orchestrator keeps one, like the [M7 closeout log](../milestones/m7-closeout.md). Write one line per decision as it happens: the time, what was decided and why, and what broke and how it was fixed. Keep the log on its own branch with a draft PR, commit each batch of entries with `gt modify`, push with `gt submit --stack --no-interactive --draft`, and land it at each handoff and promotion. A log that exists only in one session is lost when that session ends. This log is the milestone's own. A change to an architecture decision goes in `docs/architecture/log/<topic>.md`, one dated line appended by the ticket that made the change, never in the milestone log or in a `## Log` section of the topic page.

## Write tickets

1. Write each ticket in the write-ticket format (`.cursor/skills/write-ticket/SKILL.md`): Context, Problem, Proposal with an "Implement exactly" list, Acceptance Criteria, Verification and Done when. Leave the owner as few choices as you can.

2. Start the body with the header line. `batch` and `dispatch` parse it:

        **Milestone:** M7 Backend platform · **Blocked by:** #483, #536 · **Tracking:** #492 · **Base branch:** `staging` · **Touches:** `apps/backend/cmd/monacoctl/agents/**`, `docs/architecture/ci.md`

    - Put every `Touches` glob in backticks. A bare `**` breaks GitHub Markdown. `batch` defers a ticket with no `Touches` and keeps two tickets whose globs overlap out of one batch.
    - `Blocked by` lists issue or PR numbers, or `none`. An issue blocker counts as merged when it is closed as completed, or when a PR landed on `staging` says `Closes #<n>`. A PR blocker counts when it is merged, or closed with its head commit on `staging`.

3. Set the ticket's milestone and add its row to the tracking issue's wave table.

4. Mirror the header in GitHub's native "Blocked by" links. The header is the source of truth. The links only drive GitHub's UI. `scripts/sync-blocked-by.py` compares the two for every open ticket in the milestone, skipping the tracking issue. It prints the plan by default and changes links only with `--apply`:

        scripts/sync-blocked-by.py --milestone "<milestone title>" --tracking <n>
        scripts/sync-blocked-by.py --milestone "<milestone title>" --tracking <n> --apply

    Run it after you add a ticket or change a `Blocked by` line.

5. When a ticket's scope changes, edit its body and add a comment that says why. A ticket must never describe work that no longer matches the plan.

## Run a batch

1. `agents dispatch` enforces the load gate (`[dispatch] max_load`) and the queue breaker (two `gtmq_` drafts closed in the last 60 minutes that failed on the same job), so there is nothing to check by hand. Send a pipeline fix with `--urgent`, which bypasses both.

2. Pick the tickets:

        bin/monacoctl agents batch <n>...

    It admits up to `[batch] size` tickets and prints `deferred #<n>: <reason>` for each one it holds back: no `Touches`, an unmerged blocker, a blocker in the same batch, a `Touches` overlap or a full batch. Two tickets whose overlapping globs both lie inside `[batch] shared` still share a batch, and it prints `shared: #<a> and #<b> both touch <globs>` for each such pair. It writes `.git/pstack/<milestone>/batch.json` and replaces the last batch. Copy the old `batch.json` first if you still want its timeline.

3. Publish the board:

        bin/monacoctl agents status --publish

    Run it again after every dispatch, merge and ejection. `agents-status.yml` also refreshes it on every push to `staging` or `main` and every 10 minutes while a batch is active.

4. Dispatch each admitted ticket:

        bin/monacoctl agents dispatch <n> --model opus

    It fetches `staging` and refuses a ticket outside the batch (unless you pass `--urgent`), a ticket with an unmerged blocker, and a dispatch past the `lanes` cap. It then creates `.worktrees/<n>` detached at the tip, writes the owner record, starts `caffeinate` for the calling Claude Code process, and prints the spawn line, the prompt and a conflict forecast. `--dry-run` prints the same without changing anything. `--urgent` also logs the dispatch on the tracking issue. `dispatch` must run inside a Claude Code session, because it ties `caffeinate` to that process.

5. Spawn the owner exactly as printed: `subagent_type` `pstack:poteto-agent`, the printed model, in the background, with the five-line prompt. The `scripts/agent-guard-dispatch.py` hook refuses a spawn that carries a `brief:` line with another agent type, a model other than `opus` or `sonnet`, or no dispatch record. Then record the agent ID:

        bin/monacoctl agents own <n> <agent-id>

6. When the owner reports its PRs, run `bin/monacoctl agents done <n>`. Once its process has stopped, run `bin/monacoctl agents exited <n>`. The lane cap counts the owners not marked exited whose worktree exists on this machine and whose ticket is open; `dispatch` prints the others as `not counted`, and `agents status` lists owners whose worktree is on another machine under their own heading. `agents check` splits the CPUs among the counted owners.

## Verify and land

1. Plan the review of each PR:

        bin/monacoctl agents verify-plan <pr>

    It prints the kind (`light` or `full`), the verifier model, the reason and the spawn line. A diff with 50 or more non-test lines, a change under `platform/db`, `platform/bus`, `platform/money`, `platform/concurrency` or the `trading`, `treasury` and `funding` modules, or a patch that adds goroutines, channels or locks is `full`. The verifier model never equals the owner's model. One verifier may review a whole stack.

2. Spawn the verifier as printed. It reviews the diff against the ticket's Done when and acceptance criteria, reads `gh pr checks <pr>` once, runs no tests, writes a report file, and posts:

        bin/monacoctl agents verdict pass <pr> <head sha> --kind <kind> --model <model> --report <file>

    `verdict` refuses the owner's model, `fable`, a SHA that is not the PR head, and a kind weaker than the plan's. On `fail` the report says exactly what to fix.

3. Land the stack.

    - The owner runs `bin/monacoctl agents land-stack <top-pr>` right after `gt submit` and `scripts/pr-body.sh`, by default, without asking. `verify` is advisory: landing does not wait on it, so review runs alongside stage 1. A single PR is a stack of one. It adds the `merge-queue` label to each PR, bottom to top. The Graphite merge queue runs stage 2 on the whole stack and squashes each PR into one commit on `staging`. If a PR still lacks stage 1, or its PR format check is running or red, it arms the stack, prints `armed #<top>; agents watch lands it once stage 1 passes (waiting on ...)` and exits 0. `agents watch` lands an armed stack once nothing is waiting, and disarms it with `armed stack #<top> disarmed: #<n> <check> failed` when stage 1 fails. If a check has already failed, it prints `not landing #<n>; waiting on ...` and arms nothing. `dequeue <top-pr>` also disarms a stack. Before it labels anything, it reruns every cancelled or failed workflow run on each PR's head and waits for them to finish, printing one line per rerun, because Graphite keeps a PR with such a run on its head out of the stack's queue draft (a restack followed by a second push leaves them). If a rerun fails again it stops with the run's name and URL and labels nothing. After labeling it waits for the Graphite draft and prints `queued together: #a #b #c` or `Graphite queued only #a; held back: #b (reason)`. Follow a queued stack with `bin/monacoctl agents watch` under Claude Code's Monitor tool, not a sleep loop.
    - Never run `gh pr merge`, and never add `merge-queue` or `fast-track` by hand. The hook blocks all three. Only the operator adds `fast-track`, a PR under 100 counted lines that touches nothing under `apps/backend/`, `.github/` or `docker-compose.yml`.
    - A landed PR shows as closed, not merged. It counts as landed when `staging` has its squash commit, whose first line ends with ` (#<pr>)`.
    - To change a queued stack, run `bin/monacoctl agents dequeue <top-pr>` first, then `gt modify`, `gt submit` and `land-stack` again. The agent guard hook blocks `gt submit`, `gt modify`, `gt restack` and `git push` on a stack while any of its PRs carries `merge-queue`.

4. The ticket closes itself when its last PR's squash commit reaches `staging`, because that commit carries the PR body and its `Closes #<n>`. If the issue is still open after the last PR lands, run `gh issue close <n>`.

5. On a `fail` verdict, spawn a fresh owner on the same worktree with the report. It amends with `gt modify` and submits again, and a new verifier reviews the new head. Give one problem at most three `opus` attempts. The last resort after that is one `fable` attempt at medium effort, and then you park the ticket with a comment. The dispatch hook accepts only `opus` and `sonnet` for a spawn with a `brief:` line, so the `fable` attempt is a plain spawn that carries the failure evidence and no brief.

## Watch the run

Keep `bin/monacoctl agents watch` running under Claude Code's Monitor tool for the whole run. Never poll CI with `gh run watch` or a sleep loop.

- `bin/monacoctl agents watch` streams one line per change, every 30 seconds (`--every <duration>`, at least 10s). It covers idle owners and owners finished but still running, each queued PR (`queued`, `landed` or `ejected`), the Graphite queue's `gtmq_` draft PRs and their finished checks, and each ejection or red stage 1 with its failing job and a fresh-owner prompt. When every PR of a stack landed, it runs `gt sync` and prints `stack #<top> landed (...)`. A stack that stays out of the queue for two rounds is unmarked and printed as `stack #<top> ejected: ...`. `--once` prints one pass for cron or `/loop` and exits nonzero when it flagged anything.
- `bin/monacoctl agents forecast` lists files that more than one open stack touches. Land those stacks one after the other, not together.
- `bin/monacoctl agents conflicts <pr>` says whether a PR is behind its base and which files conflict.
- `bin/monacoctl agents timeline` prints dispatch, push, stage 1, verdict, queue and merge times for each ticket of the batch. `--batch <file>` reads a saved batch.

Act on an ejection by its cause, as the table in [When a check fails](ship-a-ticket.md#when-a-check-fails) says. Two more rules keep the queue moving:

- **Land a fix ahead of what it unblocks.** When a defect on the tip ejects PRs, stop requeueing them. Land the fix first, then requeue the rest behind it. A queued stack tested without the fix fails again. To pull a queued PR out, remove its label:

        gh pr edit <pr> --remove-label merge-queue

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

    A conflict in `apps/backend/api/spec/<module>.yaml` is a real conflict, because only that module's tickets edit the file. Merge both sides by hand, keep every path and schema, and run `go generate ./...`. The bundler fails on a path or schema defined in two spec files and names both. A conflict in `apps/backend/api/openapi.yaml` or `api.gen.go` needs no hand merge: take either side and run `go generate ./...`, which rewrites both from the spec sources.

    A conflict in any other file that `[batch] shared` lists in `.monaco/agents.toml` needs no hand merge. Take either side, then regenerate from `apps/backend` and commit the result:

        go generate ./...
        go run ./cmd/monacoctl gen migration --rebase
        ../../scripts/gen-docs.sh
        go run ./cmd/monacoctl docs flows --feature-map > ../../.claude/skills/verify-backend/feature-map.md

    `go generate` bundles `api/spec/*.yaml` into `openapi.yaml`, then rebuilds `api.gen.go` and the other generated Go from the bundle and the module list. `gen migration --rebase` renames the branch's own migrations (files not on `origin/staging`) to fresh prefixes above the newest one there, in their original order, then runs `atlas migrate hash` (the pinned build `scripts/install-atlas.sh` puts in `.bin/`) to rewrite `migrations/atlas.sum`. It renames nothing when the branch's files already sort above staging's, and still rehashes. A new migration comes from `just gen migration <module> <name>`, never a hand-picked prefix. `gen-docs.sh` rewrites `docs/reference/`. The last line is the feature-map step of `scripts/ci/ready.sh`. `flows.tsv` and `CHANGELOG.md` merge with `merge=union` and should not conflict. If they do, keep both sides' lines. `agents check` then sorts `flows.tsv` by id. [Shared files](../architecture/ci.md#shared-files) says why each glob is safe.

3. Check every branch of the stack, not only the top. A clean textual restack can still break compilation.

        gt checkout <branch>
        cd apps/backend && go run ./cmd/monacoctl agents check

4. Confirm the remote has no commit the local stack lacks. `git log --oneline <branch>..origin/<branch>` prints nothing for each branch. Then push:

        gt submit --stack --no-interactive --draft

5. Where a PR's diff did not change, repost its verdict on the new head: `bin/monacoctl agents verdict carry <pr>`. Where it changed, verify it again. Then land the stack.

Dequeue a stack before you restack it: remove the `merge-queue` label from each of its PRs.

## Promote staging to main

A promotion merges `staging` into `main` with a merge commit. Only the operator merges it. `staging` stays in place afterwards, and open stacks need no move.

1. Make sure `main` has nothing `staging` lacks: `git log --oneline origin/staging..origin/main` prints nothing. If it prints commits, land a PR on `staging` that merges `origin/main` first.

2. Land a PR on `staging` that renames `## [Unreleased]` in `apps/backend/CHANGELOG.md` to `## [checkpoint N] - <date>` and opens a new `## [Unreleased]` with only an empty `### Added` heading. The `Changelog (checkpoint into main)` check requires it.

3. Check for an open promotion PR: `gh pr list --base main --head staging`. GitHub allows one open PR per head and base. An open one already shows every later merge, so update its body instead of opening another.

4. Open the PR with a body file that follows the template and lists the tickets merged since the last promotion:

        gh pr create --base main --head staging --label integration --title "<what the promotion ships>" --body-file <file>

    `gh pr create` is right here, since the PR is not a Graphite stack. `scripts/pr-body.sh` refuses a promotion, because the PR format check reads every commit since `main`, including old ones without Conventional subjects. That check is not required on `main`: only `ci / ci-ok` and the changelog check are. Under "Needs from Logan", list the `large-pr` label and the merge, with a merge commit and not a squash, for the operator.

Never push to `main` or `staging` yourself.

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
3. Starts `bin/monacoctl agents watch` under Monitor to see ejections, idle owners and queued stacks.
4. Carries on. Owner records rebuild from their ticket comments the first time a command needs them, so `resume`, `verdict` and `land-stack` work on in-flight tickets. A record is trusted only from a comment whose author is a repo owner, member or collaborator, because anyone can comment on a public repo. Each person's commands edit only their own record comment and post a new one otherwise. `bin/monacoctl agents resume <n> --transcript <file>` says whether a transcript is small enough to resume (250k tokens or fewer). A transcript from another machine is usually unavailable, so give the ticket a fresh owner on its pushed branch. The fresh owner's worktree starts from the branch, not from the parent SHA.

One ticket changes hands the same way. [Hand off a ticket](ship-a-ticket.md#hand-off-a-ticket) has the owner's side.

## Lessons from M7

Each rule below came from a failure in the M7 run.

- **A PR format failure caused by the body is fixed by editing the body.** `gh run rerun --failed` replays the original event with the old body, and it failed again on #895 and #962.
- **A checker fix reaches a PR only through its base.** `PR format` and `PR size` run `scripts/check-pr-format.py` and `scripts/check-pr-size.py` from the PR's base commit, so a PR's own edit to a checker does not change that PR's verdict, and an upper PR of a stack picks up a checker fix only after a restack. #1090 failed `PR size` on a merge ref that still had the old checker.
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

## Cloud root

A Claude Code cloud session runs as root in a Linux container. It can own a ticket the same way a laptop does once the environment below is in place.

Egress must allow these hosts:

- `api.github.com`, for REST (`gh api`, `monacoctl agents`, `scripts/pr-body.sh`).
- `api.graphite.com`, for `gt submit`.
- `proxy.golang.org`, for Go modules and toolchains.
- `mirror.gcr.io` or Docker Hub, for `mirror.gcr.io/library/swift:6.3-noble` and for Postgres.

The Claude GitHub App must be installed on `Munyon-Canyon/monaco` with write on contents, pull requests, issues and commit statuses. Without that write, pushes, comments and `verdict` fail with `Resource not accessible by integration`.

The session needs `DOTENV_PRIVATE_KEY` and `GRAPHITE_TOKEN`. `DOTENV_PRIVATE_KEY` decrypts `.env.local` when `.env.keys` is absent. `GRAPHITE_TOKEN` is what `gt submit` uses.

These steps still need a Mac: `just build mobile`, the MonacoTests run (`-only-testing:MonacoTests`), anything that uses a simulator, and a Darwin-only `swift test`. On the cloud host, `just test mobile` runs `swift test` in `packages/mobile-core` through the Swift shims when the Swift tarball cannot be downloaded.
