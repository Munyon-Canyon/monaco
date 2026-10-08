# Admin panel

The business dashboards are Grafana over `GET /v1/admin/dashboards/*`. Retool keeps the lookups, queues, actions and audit. Both call the same `/v1/admin/*` API. This page covers the credential Grafana uses. Roles and actions are in [Analytics & admin panel](../architecture/analytics-admin.md#admin-panel).

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

`with-dotenv-local.sh` loads `.env.local`, which points at the local database. For another environment, export that environment's `DATABASE_URL` the way its runbook does.

Pick a lowercase name of letters, digits, `-` or `_`. A name is unique among live tokens.

### Store

Paste the token only into the Grafana Infinity data source secret, as a bearer token (`Authorization: Bearer mst_...`). Do not put it in a dashboard, a variable, a repo, a ticket or a chat message. Delete the file you wrote it to once it is in Grafana.

### Rotate

1. Create a second token under a new name, such as `grafana-2`.
2. Put it in the data source secret and check that a dashboard loads.
3. Revoke the old one.

### Revoke

    scripts/with-dotenv-local.sh bin/monacoctl admin token revoke --name grafana

The next request with that token answers 401. Revoke it the moment it leaks, and when Grafana no longer needs it. Revoking a name that has no live token fails with `no rows`.
