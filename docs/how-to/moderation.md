# Moderate reported content

Monaco meets App Store Guideline 1.2 with five parts: members can report and block from every kind of user content, a word filter refuses objectionable text, a report pages the team, a moderator removes the content, and Settings carries a way to reach the team. This page is the runbook for the third and fourth part and states the response commitment.

## Response commitment

The team reviews every report within 24 hours of the member sending it. Objectionable content is removed, and the user who posted it is suspended when the abuse is serious or repeated. The member who reported it is not told who acted. Settings lists the support address, so a member who wants to follow up writes to it.

## How a report reaches the team

1. A member taps Report on a feed post, a chat message, a comment, a profile or a cabal. `POST /v1/reports` stores an open report and appends `report.created`.
2. The `social.report_alert` consumer logs `social.report_received` with the report id, kind, target, reason and a link to the admin panel.
3. Two gauges feed Grafana: `monaco_reports_open` and `monaco_reports_oldest_open_seconds`. The rule `monaco-reports-open` pages the team one minute after any report is open. The rule `monaco-reports-overdue` pages again as critical when the oldest open report passes 12 hours, which is half of the commitment.

## Act on a report

Open the Reports page of the [admin panel](admin-panel.md), or call `GET /v1/admin/reports`, which lists open reports oldest first. Each row names the kind and the target id. Read the content, then pick the action. All of them need the `moderator` role except the cabal ban request, which needs an `operator`. Each sends an `Idempotency-Key` and needs a reason of 3 to 500 characters. Each appends `admin.action`, which never holds the content.

| Report kind | Remove the content | Suspend the user |
| --- | --- | --- |
| `message` | `DELETE /v1/admin/chat-messages/{id}` | `POST /v1/admin/users/{id}/ban` |
| `comment`, which is also a post in the feed | `DELETE /v1/admin/comments/{id}`, which also deletes its feed item | `POST /v1/admin/users/{id}/ban` |
| `user` | Clear a bio or name with `POST /v1/admin/users/{id}/handle/revoke` when the handle is the problem | `POST /v1/admin/users/{id}/ban` |
| `cabal` | `DELETE /v1/admin/cabals/{id}/picture` for the picture. A cabal that must go is a ban request: `POST /v1/admin/cabals/{id}/ban-requests`, approved by a second operator | `POST /v1/admin/users/{id}/ban` for its creator |

A proposal in a cabal feed is a post too. `POST /v1/admin/proposals/{id}/void` removes it.

Then resolve the report with `POST /v1/admin/reports/{id}/resolve` and a reason. Resolving an already resolved report changes nothing. A report whose content is fine is resolved the same way, with the reason `no action needed`.

## Word filter

`internal/platform/textfilter` holds the list, `blocklist.txt`. It lower-cases the text, strips accents, maps leetspeak (`0 1 3 4 5 7 8 @ $ ! +`) to letters, splits on anything that is not a letter and compares each word with repeated letters collapsed. It matches whole words only, so a listed word inside a longer word passes. Chat messages, comments, display names, bios and cabal names go through it, and a refused text answers `content_not_allowed` (400).

To add a word, append a line to `blocklist.txt`. A word with a double letter collapses with it, so `faggot` also blocks `fagot`. The list is a floor, not a promise: the report flow above catches the rest.
