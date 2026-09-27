# Legacy

Older versions of the codebase and the documents written for them. Nothing here is maintained. Read it as an archive of how Monaco was built, not as a source of truth for how it works or should work.

The backend is being rewritten. The target design is [architecture/backend-platform.md](../architecture/backend-platform.md), and the live docs under [docs/](../index.md) describe what stays true through that rewrite. When a file here disagrees with either, the file here is the one that is out of date.

Some of it is still accurate. Product rules, the Privy wallet model, the Solana and USDC mechanics, the sweep procedure and most of the mobile behavior have not changed. Those facts live in the live docs too, so cite the live doc, not the copy here.

## Docs for the deleted legacy backend

| File | What it described | What replaces it |
| --- | --- | --- |
| [architecture.md](architecture.md) | The current `apps/backend` code: packages, pollers inside the API process, `supabase/migrations` applied at boot, hand-written HTTP structs, the old table names | [architecture/backend-platform.md](../architecture/backend-platform.md) for the target; [docs/architecture.md](../architecture.md) for what carries over |
| [api.md](api.md) | The legacy backend's hand-maintained route table, kept in sync by a route test until M7 deleted that backend. The route table, `{ "error", "requestId" }` error shape, `idempotencyKey` body field, `/v1/groups` routes | `api/openapi.yaml` in the rewrite, rendered in the docs site; problem+json errors and `Idempotency-Key` header per the RFC |
| [ops-sweep-usdc.md](ops-sweep-usdc.md) | The `sweep-wallets.sh` runbook for moving stuck USDC out of Privy wallets; the script and its Go command were deleted with the legacy backend in M7 | Nothing yet. Funds move only through the ops runbooks the funding module brings back |
| [ops-observability.md](ops-observability.md) | Sentry, Prometheus `/metrics`, the alert webhook, `LOG_FILE`, the old metric and alert names | The RFC's Deploy and observability and Logs as evidence sections; the iOS part moved to [how-to/read-ios-logs.md](../how-to/read-ios-logs.md) |

## Build history

| Folder or file | What it was |
| --- | --- |
| `milestones/` | The build plan: M0 scaffold through M5 mobile UI, plus the Tessera and PreStocks pre-IPO work. Each file lists tickets, tests and manual checks. `scripts/create_milestone_issues.py` (deleted in M7) created GitHub issues from these. |
| `architect/` | Early architecture sketches from three candidate designs, and the synthesis that picked from each |
| `superpowers/` | Plans and specs for individual features (propose-sell, the home dashboard) |
| `qa/` | Screenshots and notes from QA passes, by issue number |
| `submission/` | Hackathon submission notes |
| `m3-agent-handoff.md`, `m4-agent-handoff.md`, `m5-agent-handoff.md`, `m4-overnight-prompt.md` | Handoff notes and prompts for coding agents between milestones |
