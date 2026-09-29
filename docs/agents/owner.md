# Owner

The full procedure, with fixes for failed checks, is [Ship a ticket](../how-to/ship-a-ticket.md).

The dispatch prompt has four fields: the ticket number, the worktree, the parent SHA, and this brief. Read the GitHub issue for that ticket. Change files only in the worktree. Stack from the parent SHA with `gt create`.

## Done

1. `monacoctl agents check` passes.
2. Run `gt submit --stack --no-interactive --draft`.
3. For each PR, write the body to a file and run `scripts/pr-body.sh <pr> "<title>" <file>`. It checks the title, body and commits, then marks the PR ready.
4. Exit. The owner never calls `gh pr merge`.
