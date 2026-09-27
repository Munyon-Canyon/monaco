---
name: pr-summary
description: Use when drafting PR titles, PR summaries, diff overviews, or PR descriptions. Trigger phrases include "write a PR summary", "PR overview", "diff overview", "PR description", "PR title", or "copyable PR body".
disable-model-invocation: true
---

# PR Summary Drafting

## TLDR

Write a PR title and body a human can read once and understand everything about the change: what changed, why, the proof it works, and what came up along the way. The body format matches `.github/pull_request_template.md`. Rules and reasons: `docs/architecture/backend-platform.md#pull-requests-small-and-stacked`.

## Inputs

Determine the base branch:
1. If a PR exists: `gh pr view --json baseRefName -q .baseRefName`
2. Otherwise the Graphite parent (`gt parent --no-interactive`), else `main`

Then run:
- `git diff $(git merge-base HEAD <base>)...HEAD` for the full diff
- `git log --oneline $(git merge-base HEAD <base>)..HEAD` for commit context
- `gh issue view <n>` for the issue the stack belongs to

Also collect proof from the session or CI: commands run and their output, `just verify backend` evidence summaries, test counts, measurements and their conditions, screenshots.

## Title

What this PR changes, present tense. No issue number and no commit-type prefix; the issue is linked under Why.

- Good: `Add errs code table and problem+json mapping`
- Bad: `#212 Add errs code table`, `docs: stuff`, `feat(treasury): fund`, `Fix bug`

Link the issue the stack belongs to in the Why section ("Closes #212" or "Part of #212").

## Output Format

Return the title on its own line, then one copyable markdown code block for the body. The body's first line MUST be `## TLDR`. Keep all six sections; write "None." rather than deleting one.

```markdown
## TLDR

[1-2 sentences: what changed and what that does for the product or codebase]

## Why

[The problem. What breaks, stays slow, or stays unsafe without this PR. Link the issue.]

## What changed

[Grouped by behavior, not file-by-file. What a caller, a user, or the next engineer will notice.]

## Proof

[Exact commands and what they printed. Evidence summaries, test counts, measurements with conditions. Say plainly what was not verified.]

## What came up

[Surprises, assumptions that turned out wrong, decisions made along the way and why, follow-ups filed as issues with links. What the author learned that the reviewer should know.]

## Reviewer focus

[Where to look hardest. Anything risky: migrations, money paths, force pushes, deleted data.]
```

## Style Rules

- Concrete behavior over file inventory
- Group by outcome, not implementation order
- Every claim in Proof has its command or its source; label guesses as guesses
- "What came up" is honest: include mistakes made and corrected during the work
- No emojis, no long code snippets
- Clear product and engineering language, no filler
- A PR in a stack stands alone: a reviewer reads this PR, not the stack

## Applying it

After `gt submit --stack`, set title and body:

```bash
gh pr edit <n> --title "<what changes>" --body-file <file>
```

## Example

Title: `Replace hand-written test mocks with factories`

```markdown
## TLDR

Tests now build fixtures from factories, so fixtures stay aligned with real database shapes.

## Why

Hand-written mocks duplicated schema fields and drifted from production rows. Closes #212.

## What changed

Tests use `CabalFactory`, `UserFactory` and `TransactionFactory` instead of literal structs. Poller helpers seed through `TransactionFactory` instead of raw SQL.

## Proof

`just test backend`: 31 packages ok, 0 failures. `just verify backend`: flows 05 and 07 pass, evidence in `test/evidence/`.

## What came up

Two tests relied on a hard-coded user ID and started failing once IDs came from the generator; they now read the ID from the factory. Filed #219 for a flaky timing test found along the way.

## Reviewer focus

`internal/testkit/factories.go`: the defaults for money fields.
```
