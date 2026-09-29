# Owner

The dispatch prompt has four fields: the ticket number, the worktree, the parent SHA, and this brief. Read the GitHub issue for that ticket. Change files only in the worktree. Stack from the parent SHA with `gt create`.

## Done

1. `monacoctl agents check` passes.
2. Run `gt submit --stack --no-interactive --publish`.
3. Set each PR body from a file with `scripts/pr-body.sh <pr> <file>`.
4. Exit. The owner never calls `gh pr merge`.
