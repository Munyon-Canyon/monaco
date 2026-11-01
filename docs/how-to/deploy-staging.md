# Deploy staging

The staging backend runs on Render, with Postgres on Supabase and NATS JetStream on Synadia Cloud. TestFlight staging builds talk to it.

| Piece | Where | Config |
| --- | --- | --- |
| api (`monaco-api`, web service) and worker (`monaco-worker`, background worker) | Render, Virginia, Starter plan | `render.yaml`, image from `apps/backend/deployments/Dockerfile` |
| Postgres | Database `monaco_staging` in the Supabase project `monaco`, session pooler on port 5432 | `DATABASE_URL` in `.env.staging` |
| NATS | Synadia Cloud, `tls://connect.ngs.global` | `NATS_CREDS` points at the Render secret file `/etc/secrets/nats.creds`, filled from `NATS_STAGING_CREDS` in `.env.staging` |

Staging has its own accounts and keys, separate from local development. All of them live in `.env.staging`:

- A Privy app of its own (`PRIVY_APP_ID`, its secret, verification key, authorization key and the iOS client for `xyz.monacolabs.app`).
- A Helius mainnet RPC (`SOLANA_RPC_URL`).
- EVM RPCs (`EVM_RPC_URL_BASE` and the other networks). The Alchemy free tier works: it caps `eth_getLogs` at 10 blocks, which is the `EVM_LOGS_MAX_BLOCKS` default, so a bridge arrival on Base takes a tick or two longer to show than on a paid plan. On a paid plan, raise `EVM_LOGS_MAX_BLOCKS` to the plan's range.
- Its own relayer fee payer, `6zPvHbTHwVakhN3FiVmFqjUFhY5mdAAt865vUAZFFyVr`. The api and worker still boot when it holds 0.0025 SOL or less but log the `relayer.boot_underfunded` alert, and sends that need SOL fail until you top it up with SOL from any wallet. Check it with `MONACO_ENV=local` and the staging `RELAYER_PRIVATE_KEY` and `SOLANA_RPC_URL` through `monacoctl relayer balance`.

Render's GitHub app must be installed on the Munyon-Canyon org with access to this repo; without it Render never hears about pushes and nothing deploys on its own. Nothing deploys them automatically: both services set `autoDeployTrigger: "off"` in `render.yaml`, so a push to `staging` starts no deploy. Someone deploys by hand once `staging` is green (see [Deploy by hand](#deploy-by-hand)). Before the api goes live, its pre-deploy command, `predeploy` (`apps/backend/deployments/predeploy.sh`), runs `monacoctl migrate apply` then `monacoctl bus apply`. Render runs that command without a shell, so `&&` or `sh -c "..."` in `render.yaml` does not work. The worker can start before that finishes. It then stops with `db_schema_behind`, and Render restarts it until the schema catches up.

## Deploy by hand

Both services have `autoDeployTrigger: "off"` in `render.yaml`. With auto deploy on, every commit that touched `apps/backend/**` restarted the api and the worker, dozens of times on 11-01 in the middle of a fix run. The backend now reaches staging only when someone deploys it, the way the web app and the iOS app ship. The web steps are in [Deploy web](deploy-web.md); deploy both from the same green commit.

1. Pick a `staging` commit whose `ci / ci-ok` and staging runs are green.
2. Open the Render dashboard, then `monaco-api`, then **Manual Deploy** → **Deploy a specific commit**, and enter the sha. Wait until the deploy is live: its pre-deploy command has then applied the migrations and the bus.
3. Check `curl -fsS https://<api-host>/healthz`.
4. Only then do the same on `monaco-worker`. The worker stops with `db_schema_behind` while the schema is behind, so deploying it first only makes it restart.

A Blueprint sync applies `render.yaml`, including `autoDeployTrigger`. If the dashboard still shows Auto-Deploy **Yes** on either service after the sync, someone switches it off by hand under **Settings** → **Build & Deploy** → **Auto-Deploy**.

## Use the session pooler

`DATABASE_URL` must use the Supabase session pooler (`aws-0-<region>.pooler.supabase.com:5432`, user `postgres.<project-ref>`).

- The transaction pooler (port 6543) breaks the worker's session-level advisory locks (`internal/platform/db/lock.go`).
- The direct host is IPv6-only, and Render can't reach it.

## Why a separate database

The project's `postgres` database holds Supabase's own schemas (`auth`, `storage` and others) and the landing site's `waitlist` table. Atlas refuses to migrate a database that isn't clean, so staging uses its own database, `monaco_staging`, created with `create database monaco_staging`.

## Pool size

Each service opens at most this many connections:

| Process | Connections |
| --- | --- |
| api | `MONACO_DB_MAX_CONNS` |
| worker | `MONACO_DB_MAX_CONNS` + 1. The extra one holds every poller's advisory lock, however many pollers there are. |
| pre-deploy (`monacoctl migrate apply`, `bus apply`) | about 3, only while it runs |

A deploy runs the old and new copy of each service side by side for a short time, so the rule is:

    2 x (api + worker + 1) + pre-deploy <= pooler size

`render.yaml` sets `MONACO_DB_MAX_CONNS` to 4 on both: 2 x (4 + 4 + 1) + 3 = 21, against a pooler of 50. `TestDeployedPoolsFitTheSupabasePoolerThroughADeployOverlap` fails when `render.yaml` breaks the rule. Sources: the pooler size is Supabase's `default_pool_size` for the project (50, set in Database, Connection pooling, or with `PATCH /v1/projects/<ref>/config/database/pooler`), and Postgres allows 60 connections in all. When the pool is too small, the worker fails every poller and the api fails boot with `EMAXCONNSESSION max clients reached`.

Recommended for launch day, not yet applied (needs the founder's OK): api 12 and worker 8, which is 2 x (12 + 8 + 1) + 3 = 45, under 50. The api spends several queries per request (rate limit, idempotency claim and complete, auth, handler); the worker's connections are shared by about 20 consumers, 30 pollers and the relay.

Every pooled connection can also carry `statement_timeout` (`MONACO_DB_STATEMENT_TIMEOUT`) and `idle_in_transaction_session_timeout` (`MONACO_DB_IDLE_TX_TIMEOUT`). Both are off when unset. Recommended: 15s and 30s, so a hung call made while a transaction is open frees its connection instead of pinning one of the pool. The poller lock connection sets neither: it is idle between ticks by design.

## Jupiter limit

The api and the worker each hold their own limiter on one shared Jupiter key (`MONACO_JUPITER_RATE_LIMIT`, calls per 10 s, default 8), so the key's budget is split between two processes that do not see each other. Their sum times 6 (windows a minute) must stay at or under the key's per-minute limit. At the default 8 each, the two can send 96 a minute.

| Key | Limit | Recommended api | Recommended worker |
| --- | --- | --- | --- |
| Free (the only plan documented at [developers.jup.ag/docs/portal/rate-limits](https://developers.jup.ag/docs/portal/rate-limits), read 2026-09-27) | 60 a minute, Price, Swap and Token calls sharing one bucket | 4 | 6 |

The worker needs 6 because its price lane gets the limit minus 2 and the ~1,500 listed mints take 30 calls of 50 within the 105 s fetch window (lane 4: 9 windows x 4 x 50 = 1,800 mints). Trades may use the whole limit and prices only the limit minus 2, so a price tick never blocks a trade. A trade is two calls, the order and the execute. `TestPaceLoad_fiftyTradesAndThreePriceTicksFitTheFreeKeyWithNoQuoteTimeout` replays 50 trades and three price ticks in 5 minutes against those numbers: the busiest minute sends 38 of the key's 60 calls and no trade call waits for the limiter.

Whether the production key is on the free plan is not confirmed (check the portal at https://portal.jup.ag). On a paid plan, raise both numbers in the same ratio and keep the worker at 6 or more. Render env changes take effect on the next deploy and need the founder's OK.

## Sign-ups

The `user.created` consumer `identity.evm_wallet` creates the user's EVM deposit wallet through Privy, one event at a time. It calls Privy between two short transactions, never inside one. A burst of sign-ups against a slow Privy still queues behind that one consumer, so set `EVM_DEPOSIT_NETWORKS=none` while EVM deposits are not offered: sign-up then records `deferred` and calls Privy not at all.

## Set up from scratch

1. Add a payment method to the Render workspace. `render blueprints validate render.yaml` fails with `need_payment_info` until you do.
2. In the Render dashboard, choose **New** → **Blueprint**, pick this repo, and point it at `render.yaml` on `staging`. This creates the two services and the `monaco-staging` env group.
3. Fill `.env.staging` with the secrets (`dotenvx set KEY value -f .env.staging` encrypts as it writes). `render.yaml` sets the non-secret keys. Leave `EVM_DEPOSIT_NETWORKS` unset so Deposit offers Base, Arbitrum and Ethereum after Solana; set it, as in `EVM_DEPOSIT_NETWORKS=base`, only to narrow that list. The file's private key, `DOTENV_PRIVATE_KEY_STAGING`, is not in the repo; share it like the `.env.local` key.
4. Push the secrets and the NATS creds file into the env group:

   ```bash
   scripts/render-staging-env.sh
   ```

5. Trigger a deploy (`render deploys create <service-id>`), then check `curl -fsS https://<api-host>/healthz`.

## Telemetry and analytics keys

`.env.staging` holds the same PostHog project and Grafana Cloud stack as `.env.local`, so staging data lands beside local data and is told apart by one field each:

| Key | Staging value | Tells staging apart by |
| --- | --- | --- |
| `POSTHOG_API_KEY` | the project key from `.env.local` | the `env` property (`staging`) on every server and iOS event |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | the Grafana Cloud OTLP gateway from `.env.local` | `service_name` |
| `OTEL_EXPORTER_OTLP_HEADERS` | the Grafana Cloud auth header from `.env.local` | `service_name` |
| `OTEL_SERVICE_NAME` | `monaco-staging` | `service_name` in Loki, Tempo and Prometheus |

After a key change, push and redeploy as in the next section. To check, query `{service_name="monaco-staging"}` in Grafana Explore (Loki), and filter PostHog events on `env = staging`. When Grafana Cloud rotates the token, update both `.env.local` and `.env.staging`.

## Backfill EVM wallets

Every user with a member wallet gets an EVM deposit wallet, and every cabal with a treasury wallet gets an EVM treasury wallet. `monacoctl ops evm-wallets` creates the ones that are missing. It calls the same ensures as the app, one at a time and at most 5 a second, so a rerun creates nothing twice. `--users` or `--cabals` limits the run to one kind, and `--dry-run` prints the counts without calling Privy.

From the repo root, build `bin/monacoctl` with `just build backend`, then run:

```bash
dotenvx run -f .env.local -f .env.staging --overload -- bin/monacoctl ops evm-wallets --dry-run
dotenvx run -f .env.local -f .env.staging --overload -- bin/monacoctl ops evm-wallets
```

It prints one line per created wallet (kind, id, address), then `evm-wallets: users created=N existing=N failed=N; cabals created=N existing=N failed=N`. It exits 1 when any ensure failed, and stderr names each failure. Rerun it until `failed=0`.

## Change a secret

Edit `.env.staging` with `dotenvx set`, rerun `scripts/render-staging-env.sh`, then redeploy both services. Env group changes reach a service only on its next deploy.

## Connect to staging NATS

`scripts/nats-staging.sh` decrypts `NATS_STAGING_CREDS` to `~/.config/monaco/staging.creds`, saves the nats CLI context `monaco-staging`, and selects it for the `nats-channel` Claude Code plugin. It needs the `.env.staging` private key. Then `nats --context monaco-staging stream ls` lists `EVENTS` and `DEADLETTER`, and `/reload-plugins` connects the plugin.

## Synadia limits

The free plan allows 2.5 GiB of JetStream file storage and requires `MaxBytes` on every stream. EVENTS (2 GiB) and DEADLETTER (512 MiB) reserve all of it. Raising either cap needs a larger plan.
