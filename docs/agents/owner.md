# Owner

Procedure and check fixes: [Ship a ticket](../how-to/ship-a-ticket.md).

Read the [standing orders](standing-orders.md) first. The dispatch prompt has five fields: the ticket number, the worktree, the parent SHA, this brief and the standing orders. Read the GitHub issue for that ticket.

<!-- BEGIN GENERATED rules:owner-brief (docs/agents/rules.yaml) -->
- Every PR that touches `apps/mobile/**` must pass a local `just build mobile` before it is pushed, and Proof cites it.
- Every PR that touches Darwin-only code in `packages/mobile-core` must pass a local macOS `swift test` before it is pushed.
- Never ask the operator a question. Pick the option that best serves fast and correct, and record it under "What came up".
- Change files only in the worktree. It starts detached: begin with `git switch -c <n>-<slug>` and `gt track --parent staging`.
<!-- END GENERATED rules:owner-brief -->

## Done

1. `monacoctl agents check` passes.
2. Run `gt submit --stack --no-interactive --draft`.
3. For each PR, write the body to a file and run `scripts/pr-body.sh <pr> "<title>" <file>`. It checks the title, body and commits, then marks the PR ready. If it stops on a conflict with its base, report the PR and its body file and exit.
4. Run `monacoctl agents land-stack <top-pr>` once. It arms the stack, and the root's `agents watch` lands it once stage 1 passes.
5. Exit. The owner never calls `gh pr merge`.
