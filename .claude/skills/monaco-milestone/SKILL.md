---
name: monaco-milestone
description: Names the Monaco command for each step of shipping a ticket or running a milestone. Use when writing, batching, dispatching, verifying, landing, restacking or promoting milestone tickets.
---

# Monaco milestone

The procedures are `docs/how-to/ship-a-ticket.md` for an owner and `docs/how-to/run-a-milestone.md` for the root. Read the one for your role first. This table names the command for each step.

Owners run stage 0 from apps/backend as `go run ./cmd/monacoctl agents check`. The root builds the tools once with `just build backend` and runs bin/monacoctl from a worktree at the staging tip, rebuilding after a merge that changes cmd/monacoctl/agents.

| Step | Command |
| --- | --- |
| ticket | the write-ticket format in `.cursor/skills/write-ticket/SKILL.md`, with `Touches` in backticks and `Blocked by` on the header line |
| batch | `monacoctl agents batch <issue>...` |
| dispatch | `monacoctl agents dispatch <ticket> --model opus` (`--urgent` outside the batch, `--dry-run` to preview), then `monacoctl agents own <ticket> <agent-id>` |
| stage 0 | `monacoctl agents check` |
| submit | `gt submit --stack --no-interactive --draft`, then `scripts/pr-body.sh <pr> "<title>" <file>` for each PR |
| owner finished | `monacoctl agents done <ticket>`, then `monacoctl agents exited <ticket>` once its process stopped |
| verify | `monacoctl agents verify-plan <pr>`; the stage 1 CI job `e2e` runs `scripts/ci/e2e.sh`, which runs monacoctl verify on the real binaries. The verifier runs no tests |
| verdict | `monacoctl agents verdict pass <pr> <sha> --kind <kind> --model <model> --report <file>` (or `fail`); `monacoctl agents verdict carry <pr>` after a restack that left the PR's own added and removed lines unchanged |
| landing | `monacoctl agents land-stack <top-pr>` (rules below). When a flow the stack touches changed on staging, `land-stack` arms the stack and starts `flows-verify` on just that flow, and `agents watch` lands it once that passes; a disarm that says `flows-verify failed` means restack with gt, rerun stage 1, then `land-stack` again; `flow <id> is in queued stack #M` means wait for #M to land, then `land-stack` again. The rules below cover merging, labels and queue changes. After `dequeue`, run `gt modify` and `gt submit --stack --no-interactive --draft`, then `land-stack` again. Removing the label by hand is not enough: Graphite re-adds it from its own state and may requeue unverified heads. `dequeue` removes the label from every PR, waits until it stays off and no open `gtmq_` draft lists the stack, and prints `safe to push`. |
| watching | `monacoctl agents watch` under Monitor, right after `land-stack`: it streams queue, Graphite queue draft and failure events and settles landed stacks. Never sleep or poll. `--once` for cron. Also `monacoctl agents forecast`, `monacoctl agents conflicts <pr>` |
| status | `monacoctl agents status --publish`, after each batch, dispatch, merge and ejection |
| restack | root only: `gt sync --no-interactive --no-restack`, `gt restack --upstack` in the stack's worktree, then stage 0 on every branch |
| promotion | the `CHANGELOG.md` rename PR, then one PR from staging into `main` labeled `integration`; the operator merges it with a merge commit, and staging stays the trunk |
| handoff | `monacoctl agents handoff`; `monacoctl agents resume <ticket>` before reusing an owner |
| timeline | `monacoctl agents timeline` |

A dispatch prompt carries only the ticket number, the worktree, the parent SHA, and the brief path (`docs/agents/owner.md` or `docs/agents/verifier.md`). `monacoctl agents dispatch` prints the owner's spawn line and prompt, and `monacoctl agents verify-plan` the verifier's; pass them as printed.

The status comment on the tracking issue carries the batch, so CI keeps it current. A handoff is a tracking-issue comment from `monacoctl agents handoff`. Nothing goes on a side branch.

## Rules

<!-- New rules go in docs/agents/rules.yaml; `go generate ./...` in apps/backend renders them below. -->
<!-- BEGIN GENERATED rules:monaco-milestone (docs/agents/rules.yaml) -->
- Always land: right after `gt submit` and `scripts/pr-body.sh`, run `monacoctl agents land-stack <top-pr>` once without asking, unless the user says in the current conversation not to land or queue, then follow it with `monacoctl agents watch` under Monitor.
- If stage 1 is still running, `land-stack` arms the stack, and `agents watch` lands it once stage 1 passes, or prints `armed stack #<top> disarmed: ...` when it fails. The verifier's `verify` status is advisory: landing does not wait on it. It adds the `merge-queue` label to every PR of the stack, and the queue squashes each PR into one commit on `staging` (title, ` (#N)`, then the body), so a landed PR shows as closed, not merged (merged when it landed on stage 0), a `revert-<top>` PR from `agents watch` is landed, not fought, and `Closes #N` in its body closes the ticket.
- no direct merge: enforced by `hook:rule_merge_needs_verify`.
- no base edits: enforced by `hook:rule_edit_base`.
- queue labels: enforced by `hook:rule_queue_label`.
- If a stack drops out of the queue, fix it with `gt modify` and `gt submit --stack`, then rerun `land-stack`.
- To change a queued stack, run `monacoctl agents dequeue <top-pr>` first.
- queued stack guard: enforced by `hook:rule_queued_stack`.
- Run every agent that writes code (ticket owners, bug fixers, runner changes) on `sonnet`, and keep planning, review and landing in the root. Pass `--model sonnet` to `monacoctl agents dispatch` and `model: sonnet` on `Agent` calls. Why: the operator asked for this during the 2026-10-06 overnight run, after Opus owners had been spawned. It overrides the Opus default in the pstack model sheet.
- For a milestone run, give each ticket or ticket group its own herdr workspace with an Opus session that orchestrates Sonnet coders and submits and lands its own stack. The main session supervises the lanes and the CI manager, and does not run owners as in-session subagents. Dispatch one ticket at a time (`agents batch <n>`, then `dispatch <n> --model opus`) so Touches overlaps inside a lane do not defer tickets. Why: the operator wants each lane visible in the herdr sidebar, with Opus orchestrating and Sonnet coding, and the same holds for the CI manager's tickets.
- After `monacoctl agents dispatch` creates a worktree, open it with `herdr worktree open --cwd <repo root> --path <worktree> --label "<n> <short name>"`, and close that workspace when the worktree is removed. Why: dispatch and `git worktree add` do not open a herdr workspace, so the worktree stays out of the operator's sidebar.
- For a rewrite-era ticket (pot, cash out, fund, withdraw, holdings, mobile money screens), name the files to read with `git show c838bd24:<path>` in the owner's brief and say to port that behavior (rules, states, edge cases, UI flow, copy) into the current module architecture. Do not redesign, and do not copy its code patterns. Why: commit c838bd24 (2026-09-25) is the last working pre-rewrite build, and the operator wants its behavior integrated, not rewritten.
- Immediately before `monacoctl agents dispatch <n>`, run `gh pr list --state open --search "head:<n>-"` and `git ls-remote origin "refs/heads/<n>-*"`. A PR or branch by someone else under 24 hours old means a teammate owns the ticket, so skip it. Before you file a ticket for a staging failure, search open PRs for the failing test's name and its file. Why: an hours-old probe missed a teammate's four-PR stack, and a ticket was filed for a red test that a teammate's PR had fixed ten minutes earlier.
- Treat simulator QA that needs a signed-in app as agent work. `apps/mobile/qa/journeys/accounts.tsv` holds Privy test logins with fixed codes, and XCUITest can drive both the app and the iOS Settings app. Only a real third-party OAuth login or real mainnet money is the user's, so check an owner's hand-back against those two before it goes on the "Needs from Logan" list. Why: owners listed OTP sign-in and missing tap tools as operator items on an overnight run, and the operator asked why real QA was not being done.
- Run parallel work at the machine's real capacity: about 6 `agents check` slots and 4 Xcode slots. When load spikes, find the bug behind it (a broad `pkill`, a hung `xcodebuild`, Docker down) before you cut slots, restore capacity when load falls, and measure `uptime` and `memory_pressure` before and after any throttle. Cap the synthetic load of a load-test fix at 9 busy-loop processes. Why: cutting slots to 1 to 3 during a spike left 27 checks queued with 90% of memory free, and the spike's causes were bugs, not capacity.
<!-- END GENERATED rules:monaco-milestone -->
