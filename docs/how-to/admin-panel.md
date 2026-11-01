# Admin panel

The admin panel is Retool over `/v1/admin/*`, plus Grafana for the business dashboards. Every action is already a backend endpoint, so Retool is only a UI and nothing is hosted on Render. The backend is the only enforcement: Retool and Grafana log in to nothing themselves, and the `admins` table decides every request. Roles and actions are in [Analytics & admin panel](../architecture/analytics-admin.md#admin-panel). This page lets another operator rebuild the panel. It holds no secrets, tokens or personal emails.

Routes and roles below are checked against `apps/backend/api/openapi.yaml`. When a route and this page disagree, the spec wins.

## Retool organization

- Retool Cloud organization `monacolabs` (US) on the Free plan, at `https://monacolabs.retool.com`. The founder's Google account owns it. Add each teammate as a Retool user.
- Retool's own login only gates the UI. Google SSO restricted to the team is Enterprise-only, so it is not on. Retool access never grants API access: every call needs a Privy token for a user with an `admins` row.
- One app, `Monaco Admin`, with one folder-level resource per environment (see [REST resources](#rest-resources)).

## Custom domain and Privy origin

- `admin.monacolabs.xyz` needs a Retool Business plan, so it is dropped for now and the panel lives on the `retool.com` address. If the plan changes, add the CNAME that Retool's custom-domain settings show at the DNS provider.
- In the Privy dashboard, the app's allowed origins are empty (open) while the app is in development mode. Before production, set the full list in one change: `https://monacolabs.retool.com` (or the custom domain), the fund page's domain, and local dev.

## Privy login component

A Retool custom component signs the operator in with the Privy JS SDK (`@privy-io/react-auth`):

- Google is the only login method.
- After login the component stores the Privy access token in the Retool state variable `privyToken` and refreshes it before it expires (read the expiry from the token and call the SDK's `getAccessToken()` a minute early).
- It stores nothing about the admin role and checks nothing client-side. The role comes from `GET /me`.
- A Google account with no `admins` row signs in fine and then gets 403 `admin_forbidden` on every query. Every page shows that `problem+json` `message` and loads no data.

## REST resources

Create two REST API resources:

| Resource | Base URL |
| --- | --- |
| `Monaco Admin (staging)` | `https://<staging api host>/v1/admin` |
| `Monaco Admin (production)` | `https://<production api host>/v1/admin` |

Both send `Authorization: Bearer {{ privyToken.value }}`. Every mutating query also sends `Idempotency-Key: {{ uuid.v4() }}`, generated once per form submit and reused if the same submit retries, so a replay returns the first response instead of acting twice. Point each page's queries at staging first and switch to production only after the staging checks pass.

## Pages and queries

| Page | Queries (`/v1/admin`) |
| --- | --- |
| Home | `GET /queues` for the queue counts, `GET /me` for the role badge |
| Dashboards | A link to the Grafana dashboards (see [Grafana](#grafana)). Retool does not draw them. |
| Lookups: user | `GET /users/{id}` by id, `GET /users?handle=` by handle |
| Lookups: cabal | `GET /cabals/{id}`, `GET /cabals/{id}/proposals` |
| Lookups: proposal | `GET /proposals/{id}` |
| Lookups: transaction | `GET /txns/{id}` by id, `GET /txns?signature=` by signature, with a Solscan link |
| Queues | `GET /queues/stuck-txns`, `GET /queues/stuck-money` (see the [money runbook](money-runbook.md)), `GET /queues/unpublished-events`, `GET /queues/bounces`, `GET /dead-letters` (filters `status`, default `open`, and `consumer`) |
| Reports | `GET /reports` (`status`, default `open`, oldest first), the page the `monaco-reports-open` alert links to; see [Moderate reported content](moderation.md) |
| Approvals | `GET /approvals` (`status`, default `pending`) |
| Audit | `GET /actions` with filters `admin_id`, `target_type`, `target_id`, `action`, paged with `cursor` and `limit` |

Each lookup shows its recent admin actions: the user, cabal and proposal lookups return the 20 newest on the target, and a transaction's actions come from `GET /actions` filtered by `target_type` and `target_id`. After a cabal ban is approved, the cabal lookup shows status `banned`.

## Actions, reasons and roles

Every action is a form with a required multi-line `reason`. The submit button stays disabled until the reason has 3 characters. The server trims it and accepts 3 to 500 characters, otherwise it answers `reason_required`. Submitting opens a confirm modal that names the target (the cabal name, the `@handle` or the proposal title, never a raw id alone). On success the form shows a success toast and refreshes the lookup. On failure it shows the `message` of the backend's `problem+json` in an error toast.

Each button is hidden when the role from `GET /me` is below the route's `x-admin-role`. That is for convenience only. The backend check is the enforcement. Roles rank `viewer`, `moderator`, `operator`.

| Action (on page) | Route | `x-admin-role` |
| --- | --- | --- |
| Void proposal (proposal) | `POST /proposals/{id}/void` | moderator |
| Ban user (user) | `POST /users/{id}/ban` | moderator |
| Unban user (user) | `POST /users/{id}/unban` | moderator |
| Revoke handle (user) | `POST /users/{id}/handle/revoke` | moderator |
| Reassign handle (user) | `POST /handles/{handle}/reassign`, body `to_user_id` and `reason` | moderator |
| Remove comment (user or cabal) | `DELETE /comments/{id}` | moderator |
| Remove chat message (cabal) | `DELETE /chat-messages/{id}` | moderator |
| Remove cabal picture (cabal) | `DELETE /cabals/{id}/picture` | moderator |
| Resolve report (Reports) | `POST /reports/{id}/resolve` | moderator |
| Pause cabal (cabal) | `POST /cabals/{id}/pause` | operator |
| Resume cabal (cabal) | `POST /cabals/{id}/resume` | operator |
| Pause all cabals (Home) | `POST /pause` | operator |
| Resume all cabals (Home) | `POST /resume` | operator |
| Request cabal ban (cabal) | `POST /cabals/{id}/ban-requests` | operator |
| Approve (Approvals) | `POST /approvals/{id}/approve` | operator |
| Reject (Approvals) | `POST /approvals/{id}/reject` | operator |
| Redrive dead letter (Queues) | `POST /dead-letters/{id}/redrive` | operator |
| Discard dead letter (Queues) | `POST /dead-letters/{id}/discard` | operator |

The read pages (`GET`) are `viewer`, except `GET /admins` (operator) and `GET /reports` (moderator). A `viewer` sees dashboards, lookups, queues and audit and no action button.

Notes:

- A cabal ban needs two operators. Requesting it changes nothing. A different operator must approve it within 24 hours; approving your own request answers `same_approver`.
- Pausing an already paused cabal answers `already_paused`. Resuming a cabal with no `ops` pause answers `no_ops_pause`, and one that still has an external-deposit pause answers `cabal_still_paused`. Show that message in the toast.
- A banned user can still withdraw and cash out.
- Copy says "cabal", never "group" or "club". The panel shows symbols and never raw mint addresses, and shows wallet addresses truncated and without hyphens.

## Access seeding

Rights come only from the `admins` table. Grant them with `monacoctl` against the environment's database, after the person has signed in to the Monaco app once so their handle exists:

    just build backend
    DATABASE_URL=<staging database url> bin/monacoctl admin grant --handle <handle> --role operator
    DATABASE_URL=<production database url> bin/monacoctl admin grant --handle <handle> --role operator

`--role` is `viewer`, `moderator` or `operator`. The command prints the user id. Run it against staging first, then production. Take each environment's `DATABASE_URL` from its own runbook ([Deploy staging](deploy-staging.md)), never from this page. Granting an existing admin changes the role. Remove access with `bin/monacoctl admin revoke --handle <handle>`.

Today the founder is the only admin, with role `operator`. When others join, the default is `moderator`, and the two-person cabal ban check needs a second `operator`.

## Grafana

The business dashboards (money, governance, social and safety) are Grafana Cloud dashboards in the folder `Monaco Business`, on the stack `monacolabs`. They read `GET /v1/admin/dashboards/{money,governance,social,safety}` of **staging** through the Infinity data source `Monaco Admin API (staging)`, with the time picker mapped to `from` and `to` and the `bucket` variable (`day` or `week`) mapped to `bucket`. Money is shown in USDC from micros strings. The data source authenticates with the viewer service token below, held in a Grafana secure field.

The dashboards live in the repo, so Grafana can be rebuilt from it:

| File | What it holds |
|---|---|
| `ops/grafana/folder.json` | The `Monaco Business` folder. |
| `ops/grafana/datasource.json` | The Infinity data source, with no secret. `__MONACO_API_BASE_URL__` becomes the staging `PUBLIC_API_BASE_URL`, and the token is not in it. |
| `ops/grafana/dashboards/*.json` | The four dashboards, one panel per line. |

Each panel is one request to the route, and a stat panel draws several tiles from one request, because a service token is limited to 60 requests a minute with a burst of 20. A dashboard opens in 4 to 8 requests. The `from`, `to` and `bucket` fields the routes echo back are not drawn.

### Sync

    scripts/grafana-sync.sh

It reads `GRAFANA_URL` and `GRAFANA_SERVICE_ACCOUNT_TOKEN` from the decrypted `.env.local`, and the staging API URL from `PUBLIC_API_BASE_URL` in `.env.staging` (it needs `DOTENV_PRIVATE_KEY_STAGING` in `.env.keys`; set `MONACO_API_BASE_URL` to skip that). It upserts the folder, the data source and every dashboard by uid, so running it twice changes nothing. Run it after editing a file under `ops/grafana/`. A data source update without `--set-token` keeps the secure header Grafana already holds.

The Grafana service account needs `datasources:create` and `datasources:write` on top of dashboard and folder write, or the sync answers 403 on the data source.

To edit a dashboard, change it in Grafana, export it with the share menu's JSON model, and replace its file. Keep the uid.

## Service tokens for Grafana

Grafana cannot do an interactive Privy login, and a person's Privy token expires within hours and carries that person's role. Grafana uses a service token instead. A service token reads the dashboards and nothing else:

- It works only on `GET` operations under `/v1/admin/dashboards/` whose `x-admin-role` is `viewer`. Every other route answers 403 `admin_forbidden`, including the user, transaction and cabal lookups, `/v1/admin/actions`, `/v1/admin/me`, every `POST`, `PATCH` and `DELETE` and every route that needs `moderator` or `operator`. A leaked token therefore exposes the dashboard aggregates, not user or transaction rows.
- An unknown or revoked token answers 401.
- It acts as `service:<name>`. Requests log the name in `admin.request` and as `actor` on the access line, never the token. A service token cannot change anything, so it never appears in `admin.action`.

Privy tokens are unchanged.

### Create

Run it against the database of the environment the token is for. The token prints once, on its own line, and only its SHA-256 is stored, so nobody can read it back:

    just build backend
    (umask 077; scripts/with-dotenv-local.sh bin/monacoctl admin token create --name grafana > ~/grafana-token.txt)

`with-dotenv-local.sh` loads `.env.local`, which points at the local database. For staging, decrypt `.env.staging` over it (from the clone that holds `.env.keys`, or add `-fk <path to .env.keys>`) and pipe the token straight into Grafana, so it is never shown or written to a file:

    scripts/with-dotenv-local.sh bash -c 'MONACO_ENV=staging NATS_URL=tls://connect.ngs.global \
      dotenvx run -f .env.staging --overload --quiet -- bin/monacoctl admin token create --name grafana' \
      | scripts/grafana-sync.sh --set-token

`--overload` makes `.env.staging` win over `.env.local`, and the command only reads `MONACO_ENV` and `NATS_URL` to pass config validation. Build `bin/monacoctl` from a commit whose latest migration the staging database has, or the command stops with `db_schema_behind`. `--set-token` reads the token from stdin and stores it in the data source's secure header.

Pick a lowercase name of letters, digits, `-` or `_`. A name is unique among live tokens.

### Store

The token goes only into the Grafana Infinity data source secret, as a bearer token (`Authorization: Bearer mst_...`), which `scripts/grafana-sync.sh --set-token` sets. Do not put it in a dashboard, a variable, a repo, a ticket or a chat message. Delete the file you wrote it to once it is in Grafana.

### Rotate

1. Create a second token under a new name, such as `grafana-2`, and pipe it into `scripts/grafana-sync.sh --set-token` as in Create.
2. Open Monaco Business → Money and check that it loads.
3. Revoke the old one with `admin token revoke --name grafana`, run against the staging database the same way.

### Revoke

    scripts/with-dotenv-local.sh bin/monacoctl admin token revoke --name grafana

The next request with that token answers 401. Revoke it the moment it leaks, and when Grafana no longer needs it. Revoking a name that has no live token fails with `no rows`.
