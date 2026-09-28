# M7 ticket owner brief

You own one M7 ticket (milestone tracking issue #492, feature branch `backend-rewrite-3`, repo lognorman20/monaco) end to end, up to STACK-READY. The root orchestrator gave you: the issue number, your worktree path, the parent branch, and your trail path. Read this whole file, then `gh issue view <n>`, then the RFC sections the ticket names (`docs/architecture/backend-platform.md`, `docs/architecture/ci.md`, and siblings under `docs/architecture/`). Read `AGENTS.md` at the repo root and any nested `AGENTS.md` / `CLAUDE.md` for paths you touch.

## Hard rules

- Work only inside your worktree, with absolute paths. Every Bash call starts with `cd <worktree> &&`. Never touch the primary checkout at `/Users/logno/Developer/monaco` (no checkout, edit, or git write there).
- You never spawn orchestrators and never schedule other tickets. Read-only helper subagents are fine. Do not run `/loop` or ScheduleWakeup.
- Never merge, never arm auto-merge, never close issues. Never add `large-pr` or `budget-change` labels. Never read `~/.config/monaco/verifier.pem` or post a `verify` status. If a PR needs one, stop that PR and report it as a Needs-from-Logan item.
- Graphite. The feature branch `backend-rewrite-3` is a Graphite trunk. Your worktree was created from its tip. Make your first branch with `git switch -c <branch>` then `gt track --parent backend-rewrite-3`, commit, and use `gt create -m "<msg>"` for each later PR in your stack and `gt modify` to amend. Push with `gt submit --no-interactive --publish` from your top branch. If gt refuses to create a PR, fall back to `gh pr create --base <parent-branch>` then `gt track`. Every PR's base is `backend-rewrite-3` or your own lower branch, never main. Before every push, for each branch that will be pushed, `git fetch origin && git log --oneline <branch>..origin/<branch>` must print nothing (ignore if the remote branch does not exist yet). If it prints anything, stop and report.
- Never push to `backend-rewrite-3` or main. The root merges.
- Each PR under 1000 changed lines per `scripts/check-pr-size.py` counting (run it locally if it can run offline; otherwise count with `git diff --numstat` excluding its IGNORED list). Follow the ticket's **Stack.** split into multiple PRs, one gt branch each.
- PR title: present tense, what it changes, no issue number, no type prefix. Body: the six sections of `.github/pull_request_template.md` (TLDR, Why, What changed, Proof, What came up, Reviewer focus). Write it to a file and set it with `gh pr edit <n> --body-file <file>` after submit. Why links `Part of #<n>`, and the ticket's last PR says `Closes #<n>`. Write plainly: no em dashes, short declarative sentences.
- Machine safety. Heavy jobs (`monacoctl bench db`, `just verify backend`, mutation/gremlins, fuzz, perf, benchmarks) run ONLY through `/Users/logno/Developer/monaco/.git/pstack/m7-rest/heavy.sh <cmd...>` which serializes across owners, waits on load, and applies `nice -n 19`. Never touch `monaco-postgres` durability settings. Tests use only the tmpfs test container (once #464 exists) or the existing `monaco_test` DB. Remove every throwaway container you start.
- Tools are installed via Homebrew: go (1.27; the Go version the RFC names governs go.mod, use `toolchain` / GOTOOLCHAIN to get it), golangci-lint, atlas, sqlc, vacuum, oasdiff, gremlins, nats-server. Install anything else missing yourself.

## Lifecycle

1. Start your trail at the path the root gave you (show-me-your-work TSV: ts, phase, decision, why, evidence, result). Log forks, completed units with their verification result, pivots, blockers. Keep it outside git.
2. Name the data shape before code. Use TDD where there is behavior. If you hit a real design fork the ticket and RFC do not settle, run `/pstack:interrogate` across models before committing to an answer, and log the decision in the trail and in the PR's What came up.
3. If the ticket is wrong or contradicts the RFC, pick the option closest to the RFC, comment on the issue saying what you changed and why, and continue.
4. Within about 15 minutes of real work, push your first branch snapshot and open the PR(s) (ready, not draft) with gt. Push again at least every 30 minutes of progress. The root judges liveness only by commits, pushes, PR and issue deltas.
5. Build. Then run: the ticket's Verification commands exactly, `just test backend`, the lint gates that exist at your parent (golangci-lint, the no-comments checker, flows check, etc.), and anything the ticket's Acceptance Criteria need. Stacked PRs get no PR CI, so once the relevant CI jobs exist, also run `gh workflow run ci.yml --ref <branch>` and record the run URL and result.
6. Run `/deslop` and `/no-comments` over your diff before declaring code final.
7. Proof section: every Acceptance Criteria box gets evidence (command + what it printed, or an evidence file path). Say plainly what was not verified.
8. Needs from Logan. Before recording anything as Logan's, exhaust every way to do it yourself: CLI, API, a subagent, headless `claude -p`, a workflow dispatch. What remains (secrets, account settings, human-only labels, things that only happen after the stack lands, marked "after landing") becomes a checklist item naming the exact action (command to paste, label to add, setting URL) and why an agent cannot do it.
9. Put `## Needs from Logan` at the bottom of the ticket's LAST PR body: `Nothing.` or the checklist. Post one issue comment on the ticket summarizing the PRs, head SHA, proof, and ending with `**Needs from Logan:**` then `Nothing.` or the checklist. If the list is non-empty, add the `needs-approval` label to the issue; if empty, make sure it is absent.
10. When the root sends verifier findings, fix forward: a red test first for each behavior finding (covering every site with the same defect), or a repro receipt where no test can show it. Push, then report the new head.
11. When done, report. If the tip moved and your ticket no longer fast-forwards, rebase your own branches onto `origin/backend-rewrite-3` when the root asks; you resolve your own conflicts.
12. If a migration is added, prove it with `just migrate db` once that recipe exists (#790).

## Report (your final message; keep it short, no file dumps)

```
STATUS: STACK-READY | CODE-READY | BLOCKED | PARKED
ticket: #<n>
branches (bottom->top): <branch>@<sha>, ...
parent branch: <branch>@<sha at build time>
PRs: <url>, ...
verification: <one line per command: command -> result, evidence path>
acceptance criteria: <n>/<m> proven
needs from Logan: <count> (<one-liners>)
issue comment: <url>
trail: <path>
notes: <anything the root must know, one or two lines>
```

## Toolchain guard (added by root)

The local Go is 1.27, which hides use of stdlib APIs newer than go.mod's `go 1.25` (for example `errors.AsType`, from 1.26). Run the final test pass with `GOTOOLCHAIN=go1.25.14 go test ./...` in apps/backend as well, and record it in Proof.

## Topology lock (added by root)

Only one agent rewrites the shared stack at a time. Do not run `gt move`, `gt restack`, or `gt submit` that restacks branches you do not own. If `gt submit` says a downstack branch needs a restack, stop and report to the root instead of restacking.

## CI rule (m7-rest)

Until #789's PR 1 is merged, PRs into `backend-rewrite-3` get no automatic CI. You do not dispatch CI and do not wait on it. Report STACK-READY once your local gates pass (just test backend under env -i, golangci-lint, `GOTOOLCHAIN=go1.25.14 go vet ./...`, the ticket's Verification commands). The root dispatches `gh workflow run ci.yml --ref <branch>` once per verified head. After #789's PR 1 is merged, CI runs automatically on your PRs and on merge-queue groups; you watch `ci / ci-ok` on your head and fix it red. Kill every `gh run watch` or `tail -f` you started before reporting.

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

## Commit messages (operator, 2026-09-27 ~20:05)

Every commit message is a Conventional Commit (`type(scope): subject`), generated with the `/commit` skill. PR titles keep the repo rule: present tense, no type prefix. #789 PR 2 adds a hook and PR 3 a CI check that enforce this.

## No repeated work (operator, 2026-09-27 ~20:45). These override earlier sections.

- Run the full suite and every other slow gate through `/Users/logno/Developer/monaco/.git/pstack/m7-rest/bin/once.sh <cmd...>` from inside your worktree, for example `/Users/logno/Developer/monaco/.git/pstack/m7-rest/bin/once.sh env -i HOME=$HOME PATH=$PATH just test backend`. It runs a command once per exact tree and returns the cached pass after that. Run it once, at your final head. Never run the full suite at a lower head or before a split.
- Live proofs (headless `claude -p` transcripts, end-to-end AC runs) happen once, at the final head, after any split. Not before.
- Before reporting, run every gate CI would run for your paths. If your diff touches `apps/backend/api/openapi.yaml` or error codes, run the Linux mobile-core Swift tests through heavy.sh. CI red on something you could have run locally is a wasted round.
- You never rebase onto work you didn't cause. The root dispatches you from a tip that already contains everything you depend on. If the tip moves under you anyway, update with `gt sync --no-interactive` then `gt restack`, never a raw `git rebase`, and say so in your report.
