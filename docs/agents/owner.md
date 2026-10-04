# Owner

The full procedure, with fixes for failed checks, is [Ship a ticket](../how-to/ship-a-ticket.md).

Read the [standing orders](standing-orders.md) first. Never ask the operator a question. Pick the option that best serves fast and correct, and record it under "What came up".

The dispatch prompt has five fields: the ticket number, the worktree, the parent SHA, this brief and the standing orders. Read the GitHub issue for that ticket. Change files only in the worktree. The worktree starts detached: begin with `git switch -c <n>-<slug>` and `gt track --parent staging`.

Every PR that touches `apps/mobile/**` must pass a local `just build mobile` and the MonacoTests run (`-only-testing:MonacoTests`) before it is pushed, and Proof cites both. Every PR that touches Darwin-only code in `packages/mobile-core` must pass a local macOS `swift test` before it is pushed.

## Done

1. `monacoctl agents check` passes.
2. Run `gt submit --stack --no-interactive --draft`.
3. For each PR, write the body to a file and run `scripts/pr-body.sh <pr> "<title>" <file>`. It checks the title, body and commits, then marks the PR ready. If it stops on a conflict with its base, report the PR and its body file and exit.
4. Run `monacoctl agents land-stack <top-pr>` once. It arms the stack, and the root's `agents watch` lands it once stage 1 passes. The verifier's `verify` status is advisory and does not hold the landing.
5. Exit. The owner never calls `gh pr merge`.
