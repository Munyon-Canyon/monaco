---
name: commit
description: Write a Conventional Commit message for the changes since HEAD that passes this repo's PR format check. Use when committing, amending with gt modify, or asked for a commit message.
---

# Commit message

Write one commit message for every staged and unstaged change since `HEAD`. Do not commit unless asked. An owner commits with `gt create` or `gt modify`, as `docs/how-to/ship-a-ticket.md` says.

## Subject

The PR format check (`scripts/check-pr-format.py`) fails a PR when any of its commit subjects does not match:

    ^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)(\([^()\s]+\))?!?: \S

- Use one of those types, in lowercase. Pick the type of the change's main intent.
- Add a scope when it names the area, such as `agents`, `backend`, `ci`, `docs` or `mobile-core`. Match the scopes in `git log --oneline -20`. A scope has no spaces or parentheses.
- Use `!` only when the diff breaks a contract. Say how in a `BREAKING CHANGE:` footer.
- Write the outcome in the imperative, lowercase after the colon, with no period. Keep it under 72 characters.
- Never put an issue number or a SHA in the subject. The PR body links the ticket.

## Body

- One or two short paragraphs, or a few bullets, that say what changed and why it matters.
- Read the diff, not only the file names. Invent no behavior, ticket or breaking change.
- End with the co-author trailer the session names, for example `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Output

Return one message in a fenced code block and nothing else. With no changes since `HEAD`, return `No changes to commit.` in the block.

```text
feat(agents): rebuild owner records from a ticket comment

A fresh clone has no local owner records, so resume, verdict and
land-stack now rebuild one from the newest record comment.

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
```
