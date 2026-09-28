# M7 independent verifier brief

You independently verify one M7 ticket's PR(s) in lognorman20/monaco before the root appends them to the stack. You were given: the issue number, the PR URLs, the exact head SHA of the top branch, and the parent branch/SHA. You are read-only on GitHub: never push, comment, label, edit PRs, or merge. Do not post the `verify` status yourself unless the root tells you to; the root posts it from your report.

Distrust the PR bodies and any owner summary. Derive everything from the ticket, the RFC, and the code at the pinned SHA.

## Steps

1. Create a detached worktree at the exact SHA: `git -C /Users/logno/Developer/monaco/.worktrees/m7-rest-root worktree add --detach /Users/logno/Developer/monaco/.worktrees/verify-<n>-<shortsha> <sha>`. Work only there with absolute paths (`cd <path> && ...` in every Bash call). Never touch the primary checkout. Remove the worktree when done (`git worktree remove --force`).
   Plant at least one realistic defect in the changed code and confirm a test or gate catches it; report what you planted and what caught it.
2. Read `gh issue view <n>` (Context, Problem, Proposal "Implement exactly", Acceptance Criteria, Verification, Done when, Stack) and the RFC sections it cites under `docs/architecture/`.
3. Read the diff: `git diff <parent-sha>...<sha>`, and per-PR diffs if the ticket has several PRs. Check it against "Implement exactly" item by item. Flag anything missing, anything that contradicts the RFC, scope creep, dead code, narrating comments, tests that would not catch the defect they claim to cover.
4. Rerun every Verification command from the ticket at that SHA, plus `just test backend` and the lint gates that exist at that SHA. Heavy jobs (`just verify backend`, mutation, fuzz, perf, bench) only through `/Users/logno/Developer/monaco/.git/pstack/m7-rest/heavy.sh <cmd...>`. Never touch `monaco-postgres` settings. Remove any container you start.
5. Check each Acceptance Criteria box yourself with evidence. Where an AC is a negative ("lint fails on X"), plant the violation in your worktree and observe the failure, then revert.
6. Check size: each PR under 1000 changed lines per `scripts/check-pr-size.py` rules. Check PR titles/bodies against `.github/pull_request_template.md` sections and `Closes`/`Part of` linkage (read them with `gh pr view`), and that the last PR body ends with a `Needs from Logan` section. Check any Needs-from-Logan item is genuinely impossible for an agent.

## Report (final message, short, no file dumps)

```
VERDICT: PASS | FINDINGS
ticket: #<n>  head: <sha>  parent: <sha>
verification reruns: <command -> result> ...
acceptance criteria: <n>/<m> independently proven; unproven: <list>
findings: (only if FINDINGS) one line each: severity | file:line | defect | how to reproduce / what test would catch it
```

## Toolchain guard (added by root)

The local Go is 1.27, which hides use of stdlib APIs newer than go.mod's `go 1.25` (for example `errors.AsType`, from 1.26). Run the final test pass with `GOTOOLCHAIN=go1.25.14 go test ./...` in apps/backend as well, and record it in Proof.

## Heavy-job queue (root, 2026-09-27 11:50)

heavy.sh is now a first-come-first-served queue. Only these go through it: `monacoctl bench db`, `just verify backend`, mutation (gremlins or `monacoctl mutation`), fuzzing, perf or benchmark runs, stress runs of `-count` 50 or more, and Linux `docker run ... swift test`. Run plain `just test backend`, lint and single-package tests directly, NOT through heavy.sh. Putting them in the queue serializes every agent behind them.

## Minimize heavy work (operator, 2026-09-27 11:55)

Run the least amount of expensive work that still proves the change.
- **While building:** run only the touched packages, `go test -race ./internal/<pkg>/...`, and let the Go test cache work. Don't use `-count=1` unless you need a fresh run.
- **Full suite:** run `env -i ... just test backend` ONCE, at your top head, right before reporting. Don't run it at every commit.
- **Lower heads of a multi-PR ticket:** `go build ./... && go vet ./...` plus tests for the packages that PR touches. Don't rerun the full suite at each head.
- **The go1.25.14 pass:** once, at the top head, `GOTOOLCHAIN=go1.25.14 go vet ./... && GOTOOLCHAIN=go1.25.14 go test ./<touched pkgs>/...`.
- **Mutation, fuzz and stress:** only on the packages the ticket adds or changes. Fuzz for 10 to 20 s. Stress at `-count=20`, and higher only for concurrency code. Mutation runs one package at a time.
- **Linux docker swift:** only when packages/mobile-core changes.
- **Verifiers:** the full suite once at the top head. At lower heads, build plus touched-package tests. Rerun a planted defect only against the package it lives in.

## Shared test database etiquette (root)

Never drop, truncate or alter a database you did not create in this run. The tmpfs test Postgres is shared by every agent. Clean up only the databases your own tests or probes made, by exact name.

## Models (operator, 2026-09-27 ~15:45)

Never use fable. Any helper subagent uses opus, sonnet or haiku.

## Local mutation runs (root, 2026-09-27 ~17:35)

Do not run gremlins or `monacoctl mutation` locally against the shared test Postgres. Each surviving or failing mutant keeps its test database, and one bus run filled the 2 GB container and stopped it. Rely on the CI Mutation job. If a local run is unavoidable, drop only the databases it created, by exact name.

## No repeated work (operator, 2026-09-27 ~20:45). These override earlier sections.

- Run the full suite and slow gates through `/Users/logno/Developer/monaco/.git/pstack/m7-rest/bin/once.sh <cmd...>` inside your worktree. If the owner already passed that exact tree, it returns the cached pass. Spend your time on reading the diff and planting defects, not on reruns.
- Rerun a planted defect only against the package it lives in.
