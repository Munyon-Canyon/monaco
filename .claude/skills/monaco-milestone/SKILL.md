---
name: monaco-milestone
description: Names the Monaco command for each step of shipping a ticket or running a milestone. Use when writing, batching, dispatching, verifying, landing, restacking or checkpointing milestone tickets.
---

# Monaco milestone

The procedures are `docs/how-to/ship-a-ticket.md` for an owner and `docs/how-to/run-a-milestone.md` for the root. Read the one for your role first. This table names the command for each step.

Owners run stage 0 from apps/backend as `go run ./cmd/monacoctl agents check`. The root builds the tools once with `just build backend` and runs bin/monacoctl from a worktree at the feature-branch tip, rebuilding after a merge that changes cmd/monacoctl/agents.

| Step | Command |
| --- | --- |
| ticket | the write-ticket format in `.cursor/skills/write-ticket/SKILL.md`, with `Touches` in backticks and `Blocked by` on the header line |
| batch | `monacoctl agents batch <issue>...` |
| dispatch | `monacoctl agents dispatch <ticket> --model opus` (`--urgent` outside the batch, `--dry-run` to preview), then `monacoctl agents own <ticket> <agent-id>` |
| stage 0 | `monacoctl agents check` |
| submit | `gt submit --stack --no-interactive --draft`, then `scripts/pr-body.sh <pr> "<title>" <file>` for each PR |
| owner finished | `monacoctl agents done <ticket>`, then `monacoctl agents exited <ticket>` once its process stopped |
| verify | `monacoctl agents verify-plan <pr>`; the stage 2 CI job `e2e` runs `scripts/ci/e2e.sh`, which runs monacoctl verify on the real binaries. The verifier runs no tests |
| verdict | `monacoctl agents verdict pass <pr> <sha> --kind <kind> --model <model> --report <file>` (or `fail`); `monacoctl agents verdict carry <pr>` after a restack that left the diff unchanged |
| landing | `gh pr merge <n> --auto` for a single PR; `monacoctl agents land-stack <top-pr>` for a stack, and again after it merges |
| watching | `monacoctl agents watch`, `monacoctl agents forecast`, `monacoctl agents conflicts <pr>` |
| status | `monacoctl agents status --publish`, after each batch, dispatch, merge and ejection |
| restack | root only: `gt sync --no-interactive --no-restack`, `gt restack --upstack` in the stack's worktree, then stage 0 on every branch |
| checkpoint | the `CHANGELOG.md` rename PR, then one PR from the feature branch into `main` labeled `integration`; the operator merges it and `checkpoint.yml` runs `tree-matches` and `merge-back` |
| handoff | `monacoctl agents handoff`; `monacoctl agents resume <ticket>` before reusing an owner |
| timeline | `monacoctl agents timeline` |

A dispatch prompt carries only the ticket number, the worktree, the parent SHA, and the brief path (`docs/agents/owner.md` or `docs/agents/verifier.md`). `monacoctl agents dispatch` prints the owner's spawn line and prompt, and `monacoctl agents verify-plan` the verifier's; pass them as printed.

The status comment on the tracking issue carries the batch, so CI keeps it current. A handoff is a tracking-issue comment from `monacoctl agents handoff`. Nothing goes on a side branch.
