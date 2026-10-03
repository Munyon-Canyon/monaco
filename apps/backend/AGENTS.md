# Backend agent rules

Rules live in structure first: types, generators and lints. This file names the few that a lint cannot catch, and points at the rest.

## Three rings

Each module under `internal/modules` has `domain`, `app` and `adapters`, and imports point inward only.

- `domain` is pure. No context, no I/O, no other module.
- `app` holds commands and queries and defines the ports it needs.
- `adapters` holds Postgres, HTTP and consumers.

`depguard` in `.golangci.yml` enforces the direction. Never edit `.golangci.yml`, which is generated from `.golangci.base.yml`.

## Hard rules

- **Cross-module means an event.** A module never imports another module. It reacts to that module's events or reads through the query port that module exports.
- **Money means integer types plus `uow.Do`.** Amounts are `money.Micros`, `money.SignedMicros` or `money.BaseUnits`. Floats are banned. Every balance change and its event commit together inside `db.UnitOfWork.Do`. Read the `money-change` skill first.
- **No comments in Go.** If you want one, you need a better name, a type, a test or an issue. Only `//go:` directives survive. `go run ./cmd/monacoctl lint comments` fails the rest.
- **Use the generators.** `just gen module`, `just gen command`, `just gen query`, `just gen consumer`, `just gen provider` and `just gen flow` write the first copy. Copy what they emit, not what you remember.
- **Every flow is a file.** A behavior change updates its row in `packages/flows/backend/<id>.tsv` and the `TestFlow<id>_<Command>_<Outcome>` tests it names.

## Before a push

Run `go run ./cmd/monacoctl agents check` from `apps/backend`. It builds, vets and runs the short tests of the packages affected since the base, with each package held to 20 s. If only the budget fails on a loaded machine, rerun it later. The Graphite merge queue runs the full suite and `monacoctl verify` on the real binaries. Do not run them yourself.

Always land: once a stack's PRs are submitted and verified, run `monacoctl agents land-stack <top-pr>` once without asking, unless the user says in the current conversation not to, then `monacoctl agents watch` under Monitor. If stage 1 is still running, `land-stack` arms the stack, and `agents watch` lands it once stage 1 and `verify` pass, or prints `armed stack #<top> disarmed: ...` when one fails. Never `gh pr merge` into `staging` and never add `merge-queue` or `fast-track` by hand. A landed PR shows as closed, not merged. [Ship a ticket](../../docs/how-to/ship-a-ticket.md#old-flow-and-new-flow) lists the old flow and the new one.

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
