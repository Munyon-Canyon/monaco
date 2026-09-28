---
name: monaco-milestone
description: Names the Monaco command for each autopilot-stack step. Use when dispatching, verifying, or landing a milestone ticket.
---

# Monaco milestone

Run the command named for the step.

| Step | Command |
| --- | --- |
| dispatch | `monacoctl agents dispatch` |
| verify | `monacoctl agents verify-plan` and `just verify backend` |
| verdict | `monacoctl agents verdict` |
| merging | `gh pr merge --auto --squash` into the feature branch |
| watching | `monacoctl agents watch` |
| status | `monacoctl agents status` |
| checkpoint | the integration label |

A dispatch prompt carries only the ticket number, the worktree, the parent SHA, and the brief path (`docs/agents/owner.md` or `docs/agents/verifier.md`).
