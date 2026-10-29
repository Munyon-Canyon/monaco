# Owner

Procedure: [Ship a ticket](../how-to/ship-a-ticket.md).

Read the [standing orders](standing-orders.md) first. The dispatch prompt names the ticket, worktree, parent SHA, this brief and the standing orders. Read the ticket's issue.

<!-- BEGIN GENERATED rules:owner-brief (docs/agents/rules.yaml) -->
- Every PR that touches `apps/mobile/**` must pass a local `just build mobile` before it is pushed, and Proof cites it.
- Every PR that touches Darwin-only code in `packages/mobile-core` must pass a local macOS `swift test` before it is pushed.
- Never ask the operator a question. Pick the option that best serves fast and correct, and record it under "What came up".
- Change files only in the worktree. It starts detached: begin with `git switch -c <n>-<slug>` and `gt track --parent staging`.
<!-- END GENERATED rules:owner-brief -->

Test changed packages with go test -short ./<pkg>/... first, then run agents check once; after its third failure on one row, hand back unpushed.

For `apps/mobile`, run `just build mobile` once; report a failed rerun.

## Done

1. `monacoctl agents check` passes.
2. Run `gt submit --stack --no-interactive --draft`.
3. For each PR, write the body to a file and run `scripts/pr-body.sh <pr> "<title>" <file>`. It marks the PR ready. On a base conflict, report the PR and body file and exit.
4. Run `monacoctl agents land-stack <top-pr>` once. It arms the stack for the root's `agents watch`.
5. Exit. Never call `gh pr merge`.
