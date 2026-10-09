# Deploy staging

The staging backend runs on Render, with Postgres on Supabase and NATS JetStream on Synadia Cloud. TestFlight staging builds talk to it.

| Piece | Where | Config |
| --- | --- | --- |
| api (`monaco-api`, web service) and worker (`monaco-worker`, background worker) | Render, Virginia, Starter plan | `render.yaml`, image from `apps/backend/deployments/Dockerfile` |
| Postgres | Database `monaco_staging` in the Supabase project `monaco`, session pooler on port 5432 | `DATABASE_URL` in `.env.staging` |
| NATS | Synadia Cloud, `tls://connect.ngs.global` | `NATS_CREDS` points at the Render secret file `/etc/secrets/nats.creds`, filled from `NATS_STAGING_CREDS` in `.env.staging` |

Every push to `staging` that touches `apps/backend/**` or `render.yaml` redeploys both services. Before the api goes live, its pre-deploy command, `predeploy` (`apps/backend/deployments/predeploy.sh`), runs `monacoctl migrate apply` then `monacoctl bus apply`. Render runs that command without a shell, so `&&` or `sh -c "..."` in `render.yaml` does not work. The worker can start before that finishes. It then stops with `db_schema_behind`, and Render restarts it until the schema catches up.

## Use the session pooler

`DATABASE_URL` must use the Supabase session pooler (`aws-0-<region>.pooler.supabase.com:5432`, user `postgres.<project-ref>`).

- The transaction pooler (port 6543) breaks the worker's session-level advisory locks (`internal/platform/db/lock.go`).
- The direct host is IPv6-only, and Render can't reach it.

## Why a separate database

The project's `postgres` database holds Supabase's own schemas (`auth`, `storage` and others) and the landing site's `waitlist` table. Atlas refuses to migrate a database that isn't clean, so staging uses its own database, `monaco_staging`, created with `create database monaco_staging`.

## Pooler size

The api and worker each open up to `MONACO_DB_MAX_CONNS` (default 11) connections, and the pre-deploy step opens a few more. The pooler's plan default of 15 server connections per database is too few, and the worker then fails every poller with `EMAXCONNSESSION max clients reached`. The project's pool size is set to 40 (Supabase dashboard, Database, Connection pooling, or `PATCH /v1/projects/<ref>/config/database/pooler` with `default_pool_size`). Postgres allows 60 connections in all.

## Set up from scratch

1. Add a payment method to the Render workspace. `render blueprints validate render.yaml` fails with `need_payment_info` until you do.
2. In the Render dashboard, choose **New** → **Blueprint**, pick this repo, and point it at `render.yaml` on `staging`. This creates the two services and the `monaco-staging` env group.
3. Fill `.env.staging` with the secrets (`dotenvx set KEY value -f .env.staging` encrypts as it writes). `render.yaml` sets the non-secret keys. The file's private key, `DOTENV_PRIVATE_KEY_STAGING`, is not in the repo; share it like the `.env.local` key.
4. Push the secrets and the NATS creds file into the env group:

   ```bash
   scripts/render-staging-env.sh
   ```

5. Trigger a deploy (`render deploys create <service-id>`), then check `curl -fsS https://<api-host>/healthz`.

## Change a secret

Edit `.env.staging` with `dotenvx set`, rerun `scripts/render-staging-env.sh`, then redeploy both services. Env group changes reach a service only on its next deploy.

## Connect to staging NATS

`scripts/nats-staging.sh` decrypts `NATS_STAGING_CREDS` to `~/.config/monaco/staging.creds`, saves the nats CLI context `monaco-staging`, and selects it for the `nats-channel` Claude Code plugin. It needs the `.env.staging` private key. Then `nats --context monaco-staging stream ls` lists `EVENTS` and `DEADLETTER`, and `/reload-plugins` connects the plugin.

## Synadia limits

The free plan allows 2.5 GiB of JetStream file storage and requires `MaxBytes` on every stream. EVENTS (2 GiB) and DEADLETTER (512 MiB) reserve all of it. Raising either cap needs a larger plan.
