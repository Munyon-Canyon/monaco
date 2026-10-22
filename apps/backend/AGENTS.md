# Backend agent rules

Rules live in structure first: types, generators and lints. This file names the few that a lint cannot catch, and points at the rest.

## Three rings

Each module under `internal/modules` has `domain`, `app` and `adapters`, and imports point inward only. `depguard` in `.golangci.yml` enforces the direction.

## Rules

New rules go in `docs/agents/rules.yaml`. The list below is rendered from it.

<!-- BEGIN GENERATED rules:backend-agents-md (docs/agents/rules.yaml) -->
- Always land: right after `gt submit` and `scripts/pr-body.sh`, run `monacoctl agents land-stack <top-pr>` once without asking, unless the user says in the current conversation not to land or queue, then follow it with `monacoctl agents watch` under Monitor.
- Domain is pure (no context, no I/O, no other module): enforced by `lint:apps/backend/.golangci.base.yml`.
- App holds commands and queries and defines the ports it needs: enforced by `lint:apps/backend/.golangci.base.yml`.
- Never edit `.golangci.yml`; it is generated from `.golangci.base.yml`: enforced by `hook:agent-guard.py`.
- Cross-module means an event, or the other module's query port: enforced by `lint:apps/backend/.golangci.base.yml`.
- Amounts are `money.Micros`, `money.SignedMicros` or `money.BaseUnits`; floats are banned: enforced by `lint:apps/backend/.golangci.base.yml`.
- Every balance change and its event commit together inside `db.UnitOfWork.Do`. Read the `money-change` skill first.
- No comments in Go: a better name, a type, a test or an issue instead: enforced by `hook:agent-guard-go-comments.sh`.
- Use the generators: `just gen module`, `just gen command`, `just gen query`, `just gen consumer`, `just gen provider` and `just gen flow` write the first copy. Copy what they emit, not what you remember.
- Every flow is a file: enforced by `ci:ci-jobs`.
- Before a push, run `go run ./cmd/monacoctl agents check` from `apps/backend`. It builds, vets and runs the short tests of the packages affected since the base, with each package held to 120 s. If only the budget fails on a loaded machine, rerun it later. Stage 1 CI runs the full suite and `monacoctl verify` on the real binaries. Do not run them yourself.
<!-- END GENERATED rules:backend-agents-md -->

## Where to read next

Paths below are from the repo root.

| Job | Read |
| --- | --- |
| Add a command, query, consumer, provider or module | `.claude/skills/go-backend-module/SKILL.md` |
| Goroutines, channels or fan-out | `.claude/skills/go-concurrency/SKILL.md` |
| Ledgers, shares, swaps or transfers | `.claude/skills/money-change/SKILL.md` |
| A consumer or a new event | `.claude/skills/nats-consumer/SKILL.md` |
| A failed `e2e` job or a flow moving to verified | `.claude/skills/verify-backend/SKILL.md` |
| Owning a ticket | `docs/agents/owner.md` |
| Shipping a ticket or running a milestone, step by step | `docs/how-to/ship-a-ticket.md`, `docs/how-to/run-a-milestone.md` |
| Why the platform looks like this | `docs/architecture/backend-platform.md` |
