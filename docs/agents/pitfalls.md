`python3 -I scripts/agents/harvest_confessions.py --limit 400 --since 4536`

# Pitfalls

Where agents trip, counted from the "What came up" and "Reviewer focus" bullets of the last 400 closed PRs into `staging`. The script sorts each bullet into one category by the keyword table at the top of `scripts/agents/harvest_confessions.py`, and `other` takes what no keyword matches. Rerun the command above to refresh the table. A PR counts in a row once however many of its items fall there. The keyword match is crude, so read a count as a direction. The hand harvest of #4536 to #5503 that this script replaces counted 1,109 items in 377 PRs, with a different fallback and a different window, so its counts differ from the table.

| Category | Items | PRs |
| --- | ---: | ---: |
| other | 244 | 124 |
| money and price correctness | 193 | 93 |
| edits outside Touches | 69 | 61 |
| stale ticket spec | 65 | 50 |
| env and tooling | 53 | 50 |
| contract, golden and event shape | 34 | 30 |
| process | 24 | 23 |
| flaky or slow CI | 21 | 21 |
| concurrency and idempotency | 14 | 13 |
| test edited to fit | 14 | 12 |
| gate failed on first push | 11 | 11 |
| verification skipped | 11 | 10 |
| deferred to the operator | 8 | 8 |
| Total | 761 | 168 |

## The top five

**Money and price correctness.** The money-change skill (`.claude/skills/money-change/SKILL.md`) is the checklist, and the verifier reads the diff against it ([Ship a ticket](../how-to/ship-a-ticket.md)). No automated check catches a wrong price, floor or rounding rule. Tests do, when the owner writes the case.

**Edits outside Touches.** [Ship a ticket](../how-to/ship-a-ticket.md) and the [owner brief](owner.md) say to change only the paths `Touches` lists. `monacoctl agents batch` uses `Touches` to keep overlapping tickets out of one batch, but nothing compares a PR's diff to the line. No check yet.

**Stale ticket spec.** Owners record the mismatch under "What came up" (standing order 9 in [Standing orders](standing-orders.md)). The dispatcher does not re-read a ticket against `staging` before it dispatches. No check yet.

**Env and tooling.** The agent guard hooks (`scripts/agent-guard.py`, `scripts/agent-guard-dispatch.py`, `scripts/agent-guard-dev-db.sh`) block the known bad commands, and [Agent workflow setup](setup.md) lists the toolchain. A new tooling surprise has no check until someone adds a guard rule for it.

**Contract, golden and event shape.** `go generate ./...` plus the CI `ready` job fail on stale generated files, `oasdiff breaking` in the `lint` job fails a breaking OpenAPI change, and `gate-changes` warns on an edited golden file ([CI](../architecture/ci.md#what-runs-where)).
