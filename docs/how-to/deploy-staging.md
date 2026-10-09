# Deploy staging

The staging backend runs on Render, with Postgres on Supabase and NATS JetStream on Synadia Cloud. TestFlight staging builds talk to it.

| Piece | Where | Config |
| --- | --- | --- |
| api (`monaco-api`, web service) and worker (`monaco-worker`, background worker) | Render, Virginia, Starter plan | `render.yaml`, image from `apps/backend/deployments/Dockerfile` |
| Postgres | Supabase project `monaco`, session pooler on port 5432 | `DATABASE_URL` in `.env.staging` |
| NATS | Synadia Cloud, `tls://connect.ngs.global` | `NATS_CREDS` points at the Render secret file `/etc/secrets/nats.creds` |

Every push to `staging` that touches `apps/backend/**` or `render.yaml` redeploys both services. Before the api goes live, its pre-deploy command runs `monacoctl migrate apply && monacoctl bus apply`. The worker can start before that finishes. It then stops with `db_schema_behind`, and Render restarts it until the schema catches up.

## Use the session pooler

`DATABASE_URL` must use the Supabase session pooler (`aws-0-<region>.pooler.supabase.com:5432`, user `postgres.<project-ref>`).

- The transaction pooler (port 6543) breaks the worker's session-level advisory locks (`internal/platform/db/lock.go`).
- The direct host is IPv6-only, and Render can't reach it.

## Set up from scratch

1. Add a payment method to the Render workspace. `render blueprints validate render.yaml` fails with `need_payment_info` until you do.
2. In the Render dashboard, choose **New** → **Blueprint**, pick this repo, and point it at `render.yaml` on `staging`. This creates the two services and the `monaco-staging` env group.
3. Fill `.env.staging` with the secrets (`dotenvx set KEY value -f .env.staging`), then run `just encrypt`. `render.yaml` sets the non-secret keys.
4. Push the secrets and the NATS creds file into the env group:

   ```bash
   scripts/render-staging-env.sh ~/.config/monaco/staging.creds
   ```

5. Trigger a deploy (`render deploys create <service-id>`), then check `curl -fsS https://<api-host>/healthz`.

## Change a secret

Edit `.env.staging` with `dotenvx set`, rerun `scripts/render-staging-env.sh`, then redeploy both services. Env group changes reach a service only on its next deploy.

## Synadia limits

The free plan allows 2.5 GiB of JetStream file storage and requires `MaxBytes` on every stream. EVENTS (2 GiB) and DEADLETTER (512 MiB) reserve all of it. Raising either cap needs a larger plan.
