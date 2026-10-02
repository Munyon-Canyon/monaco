# Architecture decision log

Durable record of the design decisions behind Monaco, one file per topic. Each file holds the decision as it stands, the reasoning, the alternatives considered, and open questions. When a decision changes, edit the file and add a dated line to its log file in `log/<topic>.md` rather than writing a new file, so the history stays in one place. Logs live outside the topic files so two branches that each append a line merge without a conflict (`.gitattributes` sets `merge=union` on `log/*.md`).

`docs/architecture.md` describes how the system is built today. These files describe what we decided and why, including decisions the code has not caught up with yet. When the two disagree, the decision file states the target and `architecture.md` states the present.

[backend-platform.md](backend-platform.md) is the RFC for the Go backend rewrite. It was written after most topic files, and it wins where a topic file conflicts with it. Topic files link to its sections instead of repeating them.

## Topics

| Topic | File | Status |
| --- | --- | --- |
| Backend platform (Go rewrite RFC) | [backend-platform.md](backend-platform.md) | Proposed 2026-09-26; open points settled 2026-09-27. Lives on `docs/backend-platform-rfc` |
| Data model (tables) | [data-model.md](data-model.md) | Decided 2026-09-27 |
| Event bus (NATS) | [event-bus.md](event-bus.md) | Decided 2026-09-26; amended 2026-09-27 |
| Trade execution | [trade-execution.md](trade-execution.md) | Decided 2026-09-26; amended 2026-09-27. No pending checks; pre-IPO transfer fees deferred |
| Proposals (CRUD) | [proposals.md](proposals.md) | Decided 2026-09-26; amended 2026-09-27 |
| Feed recommendation system & comments | [feed.md](feed.md) | Decided 2026-09-26 (MVP scope); amended 2026-09-27 |
| Deposits & withdrawals | [deposits-withdrawals.md](deposits-withdrawals.md) | Decided 2026-09-26; amended 2026-09-27. No pending checks; App Store 3.1.5 deferred |
| Auth & onboarding | [auth.md](auth.md) | Decided 2026-09-26; amended 2026-09-27 |
| Analytics dashboards / admin panel | [analytics-admin.md](analytics-admin.md) | Decided 2026-09-26; amended 2026-09-27 |
| Price history & charts | [price-history.md](price-history.md) | Decided 2026-09-26; amended 2026-09-27 |
| Notifications | [notifications.md](notifications.md) | Decided 2026-09-27. Push copy is a TODO |
| Followers & following | [followers.md](followers.md) | Decided 2026-09-26; amended 2026-09-27 |
| Leaderboard | [leaderboards.md](leaderboards.md) | Decided 2026-09-26; amended 2026-09-27 |
| Chat | [chat.md](chat.md) | Decided 2026-09-27 |
| Referrals | [referrals.md](referrals.md) | Decided 2026-09-26; amended 2026-09-27 |
| Cabals | [cabals.md](cabals.md) | Decided 2026-09-27 |
| iOS app | [ios.md](ios.md) | Proposed 2026-09-30. Source of truth for the app |

Status values: **Not started**, **In discussion** (file exists, holds open questions), **Proposed** (written up in full, awaiting sign-off), **Decided** (with date; a note names any external check still pending), **Superseded** (points at the replacement).

## File template

```
# <Topic>

**Status:** Decided <date> | Proposed <date> | In discussion | Superseded by <file>

## Decision
## Why
## How it works
## Alternatives considered
## Open questions

Log: [log/<topic>.md](log/<topic>.md).
```

The log file is `log/<topic>.md`: a `# <Topic> log` title, then one `- <date>: <what changed>` line per change, newest last.
