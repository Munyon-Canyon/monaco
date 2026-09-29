---
name: monaco-milestone
description: Names the Monaco command for each autopilot-stack step. Use when dispatching, verifying, or landing a milestone ticket.
---

# Monaco milestone

Run the command named for the step.

| Step | Command |
| --- | --- |
| batch | `monacoctl agents batch <issue>...` |
| dispatch | `monacoctl agents dispatch` (`--urgent` for a ticket outside the batch) |
| verify | `monacoctl agents verify-plan` and `just verify backend` |
| verdict | `monacoctl agents verdict` |
| merging | `gh pr merge <n> --auto` for a single PR; `monacoctl agents land-stack <top-pr>` for a stack |
| watching | `monacoctl agents watch` |
| status | `monacoctl agents status --publish`, after each batch and dispatch |
| handoff | `monacoctl agents handoff` |
| timeline | `monacoctl agents timeline` |
| checkpoint | the integration label |

A dispatch prompt carries only the ticket number, the worktree, the parent SHA, and the brief path (`docs/agents/owner.md` or `docs/agents/verifier.md`). `monacoctl agents dispatch` prints the owner's spawn line and prompt, and `monacoctl agents verify-plan` the verifier's; pass them as printed.

The status comment on the tracking issue carries the batch, so CI keeps it current. A handoff is a tracking-issue comment from `monacoctl agents handoff`. Nothing goes on a side branch.
