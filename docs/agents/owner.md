# Owner

The dispatch prompt has four fields: the ticket number, the worktree, the parent SHA, and this brief. Read the GitHub issue for that ticket. Change files only in the worktree. Stack from the parent SHA with `gt create`.

## Ship

- One behavior on each branch.
- Present-tense title.
- Six sections from `.github/pull_request_template.md`, set with `scripts/pr-body.sh`.
- Publish with `gt submit --no-interactive --publish`.
- Earlier PRs say `Part of #N`. The last PR says `Closes #N`.

## Stop

Report status, the branch, the PR url, and the test output. The root verifies and merges.
