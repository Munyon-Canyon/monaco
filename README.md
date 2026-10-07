# Monaco

[![Staging](https://github.com/Munyon-Canyon/monaco/actions/workflows/staging.yml/badge.svg?branch=staging)](https://github.com/Munyon-Canyon/monaco/actions/workflows/staging.yml?query=branch%3Astaging)

iOS app: friends pool USDC and buy tokenized US stocks on Solana.

This README covers cloning, configuring and running the repo. To understand the system, start with the [docs index](docs/index.md): [product rules](docs/product.md), then [architecture](docs/architecture.md) (components, outside services, wallets and money flows).

Monaco lets you create a hedge fund with friends by pooling money to buy stocks together. Members propose and vote on trades, and approved trades execute for the group; as the pool profits, each member’s stake increases in value through NAV. You can even add an agent to your cabal to trade on your behalf. Built as a social trading app, Monaco turns investing into an easy group game anyone can join simply by depositing money.

<img width="339" height="677" alt="image" src="https://github.com/user-attachments/assets/59c70f26-8049-4bc2-9c39-2946bb8013e7" />


## Contents

- [Prereqs](#prereqs)
- [Clone setup](#clone-setup)
- [Privy test logins](#privy-test-logins)
- [Backend tokens](#backend-tokens)
- [Deposits](#deposits)
- [Commands](#commands)
- [Local env](#local-env)
- [Relayer (fee payer)](#relayer-fee-payer)
- [iOS API environments](#ios-api-environments)
- [Simulator](#simulator)
  - [SimSlim (optional)](#simslim-optional)
- [Agent QA: the QA pot](#agent-qa-the-qa-pot)
- [Agent QA: Phantom MCP](#agent-qa-phantom-mcp)
  - [Create a Phantom wallet](#create-a-phantom-wallet)
  - [Install the Phantom MCP (agent wallet)](#install-the-phantom-mcp-agent-wallet)
  - [Fund the agent wallet (~\$1 SOL + ~\$4 USDC on Solana)](#fund-the-agent-wallet-1-sol--4-usdc-on-solana)
  - [Send USDC into Monaco (member inbox → vault)](#send-usdc-into-monaco-member-inbox--vault)
  - [Sweep leftover back to the agent wallet (vault → Phantom)](#sweep-leftover-back-to-the-agent-wallet-vault--phantom)
- [Agent workflow setup](#agent-workflow-setup)
- [Agent skills (Cursor)](#agent-skills-cursor)
- [Tests and CI](#tests-and-ci)
- [Pull requests](#pull-requests)
- [Deploy](#deploy)
- [Layout](#layout)

## Prereqs

macOS, Xcode (iOS 18+ simulator), Docker, Go 1.25+, [just](https://github.com/casey/just), [jq](https://jqlang.org), [dotenvx CLI](https://dotenvx.com/docs/install), [Graphite CLI](https://graphite.dev/docs/install-the-cli) (`gt`). SimSlim is optional.

## Clone setup

1. Clone this repo. `cd` into the clone. Do not hard-code another machine's home path.
2. Place the encrypted `.env.local` a teammate shares in the repo root. Give dotenvx its private key in one of these ways, which `scripts/with-dotenv-local.sh` tries in this order: gitignored `.env.keys` in the checkout, `.env.keys` in the primary clone (so worktrees under `.worktrees/` need no copy), `DOTENV_PRIVATE_KEY_LOCAL` or `DOTENV_PRIVATE_KEY` in the environment, then Dotenvx Armor (`dotenvx armor`).
3. If you have no `.env.local` yet, copy `.env.example` to `.env.local` and set Privy plus relayer values with `dotenvx set KEY value -f .env.local`.
4. Run `./scripts/install-dev.sh` (or `just install`). It asks before each install (Go, golangci-lint, jq, just, dotenvx, Graphite, optional SimSlim). `just install --check` only reports. Then run `gt auth --token <token>` with the token from https://app.graphite.com/activate, and `gt init --trunk main`.
5. `just run` starts the iOS app. The backend is being rebuilt from scratch ([backend platform RFC](docs/architecture/backend-platform.md)), so until its routes return the app has no working backend; mobile UI work uses sample data. Privy is injected via `scripts/ensure-ios-privy-config.sh` and `SIMCTL_CHILD_*`. If SimSlim is missing, the scripts warn and boot a stock simulator.

Do not wrap `just` with `dotenvx run` yourself. Recipes that need secrets re-exec under `scripts/with-dotenv-local.sh`.

Local DB is Docker Compose Postgres only (`monaco`, host port `54322`), next to a Compose NATS on `4222`. Tests use a separate throwaway Postgres, `monaco-postgres-test` on `54323`, that `just test backend` starts. The services live in `apps/backend/deployments/compose.yml`. Never point `just run` / `just test backend` at hosted or production Supabase.

## Privy test logins

Fixed OTP. Dashboard Login Methods must have **Email** and **SMS** on. Product path is OTP, not a password field. iOS bundle `com.monaco.app` must be on the Privy iOS client or `sendCode` returns 403 `invalid_native_app_id`. Sign out in-app to switch users.

| Name        | Phone Number       | Login                                     | OTP      |
| ----------- | ------------ | ----------------------------------------- | -------- |
| Alfred      | `+1 555 555 7177` | `test-8081@privy.io` | `465354` |
| Bartholomez | `+1 555 555 9638` | `test-4952@privy.io` | `648588` |
| Cayman      | `+1 555 555 8215` | `test-3510@privy.io` | `115543` |

## Backend tokens

The API takes two kinds of bearer token. A Privy access token (ES256) names a Privy user, and the API answers 401 `session_required` until that user has a `users` row. A dev token (HS256) names a Monaco user id and works only outside production.

`monacoctl dev privy-token` signs a Privy token with the fakes server's fixture key, valid for 1 h, so curl checks need no Privy dashboard. It refuses `MONACO_ENV=staging` and `MONACO_ENV=production`. The api also refuses to boot in those environments when `PRIVY_VERIFICATION_KEY` is the public key that `--print-public-key` prints. Run the backend against the fakes with the matching key:

```bash
just build backend
(cd apps/backend && FAKES_ADDR=:8099 go run ./cmd/fakes) &
export PRIVY_BASE_URL=http://localhost:8099/privy
export PRIVY_VERIFICATION_KEY="$(scripts/with-dotenv-local.sh bin/monacoctl dev privy-token --print-public-key)"
just run backend &
TOKEN=$(scripts/with-dotenv-local.sh bin/monacoctl dev privy-token --sub did:privy:qa-1)
```

Send it as `Authorization: Bearer $TOKEN`. `scripts/with-dotenv-local.sh bin/monacoctl dev token --user <id>` mints a dev token.

## Deposits

You can fund a group from **personal Phantom** (iOS app or browser extension). That is your wallet, not the [agent MCP wallet](#agent-qa-phantom-mcp). No Cursor or coding agent required.

1. `just run`. Sign in (OTP above).
2. Tap **Add money** and copy your deposit address (your Privy member wallet). Not the cabal treasury.
3. In Phantom, send **USDC on Solana mainnet**. Mint must be `EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v`. USDC on Ethereum or Base is a different token and will not show up.
4. The USDC shows as your **account balance**. It is not in any cabal yet.
5. Open a cabal and **fund** it with an amount. The backend sweeps exactly that amount into the treasury, then credits share units once Solana confirms. Watch API logs or the cabal screen.
6. Member wallet and treasury do not need SOL (the relayer pays fees).

### Card and Apple Pay (local)

**Add money** → **Pay with card or Apple Pay** opens the fund page. Locally that is `http://localhost:5173/fund`, and `just run` does not serve it. Start it first, or the button opens a page that does not load.

1. In a second terminal, start the fund page with Vite on port 5173. Use `PRIVY_APP_ID` from `just show-env`:

   ```bash
   cd apps/web && npm ci && VITE_MONACO_API_URL=http://localhost:8080 VITE_PRIVY_APP_ID=<PRIVY_APP_ID> VITE_PRIVY_ENV=sandbox npx vite --port 5173
   ```

2. `just run`.
3. Tap **Add money** → **Pay with card or Apple Pay**.

`curl -s -o /dev/null -w '%{http_code}' http://localhost:5173/fund` prints `200` when the page is up. The fund page and its variables are in [apps/web/README.md](apps/web/README.md#fund-page-against-a-local-backend).

To get QA cash back out: **Cash out** of the cabal (USDC returns to the account balance), then withdraw it to your Phantom address (the **Cash out** button on the account balance card on Home or Profile). Phantom cannot spend Privy wallets. Agent-driven deposit and refund: [Agent QA: Phantom MCP](#agent-qa-phantom-mcp). The full flows are in [architecture.md](docs/architecture.md#flows).

## Commands

| Command                      | What it does                                                                                                                                                           |
| ---------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `just install`               | Ask before installing missing tools. `just install --check` reports only                                                                                               |
| `just encrypt`               | `dotenvx encrypt` on `.env.local` (and `.env.production` if present)                                                                                                   |
| `just decrypt`               | `dotenvx decrypt` on `.env.local` (and `.env.production` if present)                                                                                                   |
| `just show-env`              | Print decrypted `.env.local` keys/values via dotenvx (`export KEY='value'` lines; `.env.production` omitted). Needs `.env.local`, dotenvx, and `.env.keys` or Keychain |
| `just run`                   | `just run backend` in the background, then `just run mobile`; prints a `==>` line per step. Ctrl+C or a failed build stops both                                        |
| `just run backend`           | `bin/api` on `MONACO_HTTP_ADDR` (default `:8080`) and `bin/worker` on `MONACO_WORKER_HEALTH_ADDR` (default `:8081`); both serve `GET /healthz`                         |
| `just run mobile`            | iOS with Privy xcconfig + `SIMCTL_CHILD_*` via `./scripts/ios-sim`                                                                                                     |
| Logs                         | `just run*` tee stdout/stderr to `.logs/<timestamp>/` (`api.log`, `worker.log`, `mobile.log`, `xcodebuild.log`)                                                        |
| `just stop`                  | `just stop backend`, then `just stop mobile`                                                                                                                           |
| `just stop backend`          | SIGTERM `bin/api` and `bin/worker`, wait for both to exit                                                                                                              |
| `just stop mobile`           | Terminate Monaco on the resolved sim; stop `xcodebuild` if running                                                                                                     |
| `just reset`                 | Stop all + wipe the local Postgres and NATS volumes (dotenvx)                                                                                                          |
| `just reset backend`         | Stop backend + remove `bin/api`, `bin/worker`, `bin/monacoctl`                                                                                                         |
| `just reset mobile`          | Stop app + `xcodebuild clean` on the resolved sim                                                                                                                      |
| `just reset db`              | Wipe the local Docker Postgres volume only and start it empty; NATS data is kept (localhost only, dotenvx)                                                             |
| `just migrate db`            | Apply pending migrations to the `.env.local` database, print its revision, then apply the NATS stream config. `just run backend` does neither; a behind database stops boot with `db_schema_behind`, a stale stream with `config differs from the declared one` |
| `just killports`             | Kill listeners on the api and worker ports (default 8080 and 8081; not Postgres 54322)                                                                                 |
| `just test backend`          | `go test -race -shuffle=on -short ./...` in `apps/backend`, the slowest-ten report and 90 s budget, then the `scripts/` Go tests                                       |
| `just test mobile`           | Host `swift test` in `packages/mobile-core` — fast, no secrets                                                                                                         |
| `just build backend`         | `go build` of `bin/api`, `bin/worker`, `bin/monacoctl`                                                                                                                 |
| `just build mobile`          | Privy xcconfig, then `xcodebuild` on the resolved sim                                                                                                                  |
| `./scripts/ios-sim`          | Monaco run with Privy env. Falls back to a stock sim if slim is missing                                                                                                |
| `./scripts/ios-build`        | Monaco compile with Privy xcconfig                                                                                                                                     |

Simulator UDID is **per machine**. Never commit one. Recipes call `scripts/resolve-ios-sim.sh`. A linked worktree gets its own simulator, so agents in separate worktrees build and run at once ([parallel agents](docs/how-to/local-simulator.md#parallel-agents-one-simulator-per-worktree)).

## Local env

Secrets use [dotenvx](https://dotenvx.com). Install the CLI (not a repo dependency):

```bash
brew tap dotenvx/brew && brew trust dotenvx/brew && brew install dotenvx
```

Or `curl -sfS https://dotenvx.sh | sh`. See [install docs](https://dotenvx.com/docs/install).

1. Copy `.env.example` → `.env.local` for local dev. Optionally add `.env.production`.
2. Encrypt: `just encrypt` (or `dotenvx encrypt -f .env.local`; also encrypts `.env.production` when that file exists). Decrypt: `just decrypt`.
3. Inspect: `just show-env` prints decrypted `.env.local` as `export KEY='value'` lines via dotenvx (`.env.production` omitted). Needs `.env.local`, dotenvx, and `.env.keys` or Keychain.
4. Set values: `dotenvx set KEY value -f .env.local` (encrypts by default; `--plain` for non-secrets).

| Env | Default | Meaning |
| --- | --- | --- |
| `TESSERA_API_BASE_URL` | `https://rest-api.tessera.pe` | Public Tessera catalog used for pre-IPO tokens. |
| `TESSERA_ENABLED` | `true` | Include Tessera tokens in search, detail, and sweeps. `false` hides them from the catalog; existing holdings still value from Jupiter. |
| `PRESTOCKS_API_BASE_URL` | `https://prestocks.com` | Public PreStocks catalog used for pre-IPO tokens. |
| `PRESTOCKS_ENABLED` | `true` | Include PreStocks tokens in search, detail, and sweeps. `false` hides them from the catalog; existing holdings still value from Jupiter. |

`PUBLIC_API_BASE_URL` is the API URL agents are told to call. It is not a secret and defaults to `http://127.0.0.1:8080`. Set it to the public https URL in a deployed env.

Justfile `dotenv-load` only reads plain `.env` — not dotenvx ciphertext. Recipes that need secrets re-exec once under `dotenvx run -f .env.local` (via `scripts/with-dotenv-local.sh`). Mobile Privy uses `scripts/ensure-ios-privy-config.sh` (xcconfig) + `SIMCTL_CHILD_*` at sim launch.

Private keys: `DOTENV_PRIVATE_KEY` for `.env` / `.env.local`; `DOTENV_PRIVATE_KEY_PRODUCTION` for `.env.production`. On macOS, new keys often land in Keychain, not `.env.keys`. Export with `dotenvx native pull` or `dotenvx keypair -f .env.local`.

Encrypted `.env*` files (public key in repo) may be committed. Never commit `.env.keys`, `.env.local`, or private keys. `.gitignore` covers `.env`; keep `.env.keys` and `.env.local` out of git locally.

A pre-commit hook checks **staged** `.env*` files only (not `.worktrees` or the rest of the tree) and blocks plaintext secrets / `.env.keys`. `.env.example` is allowed. Reinstall after clone: `ln -sfn ../../scripts/githooks/pre-commit .git/hooks/pre-commit`. Do not run `dotenvx precommit --install` — that full-tree scan is slow.

## Relayer (fee payer)

The app **fee payer** is a dedicated Solana keypair from `RELAYER_PRIVATE_KEY` (base58 secret or Solana CLI JSON array in `.env.local`). Not a Privy wallet. Clones that decrypt the same shared env share the same fee payer. Never commit or log the private key.

Run `just relayer balance` to print the fee payer's pubkey and its SOL balance. Api and worker refuse to boot in staging and production when it holds 0.001 SOL or less.

## iOS API environments

The app's API base URL comes from the build, not from source: `apps/mobile/Config/Monaco.xcconfig` → Info.plist (`MONACO_ENVIRONMENT`, `MONACO_API_BASE_URL`) → `MonacoConfig.api` in `packages/mobile-core`, which both API clients use.

| Environment  | Default for | Base URL                                                                 |
| ------------ | ----------- | ------------------------------------------------------------------------ |
| `local`      | Debug       | `http://localhost:8080` (`Config/Environments/Local.xcconfig`)           |
| `staging`    | —           | `MONACO_STAGING_API_BASE_URL` — **placeholder, empty until you set it**   |
| `production` | Release     | `MONACO_PRODUCTION_API_BASE_URL` — **placeholder, empty until you set it** |

- `just run` / `just run mobile` need nothing extra: Debug is `local`.
- Set the remote URLs once (not secrets, `https://` only): `dotenvx set MONACO_STAGING_API_BASE_URL https://… -f .env.local --plain` (same for `MONACO_PRODUCTION_API_BASE_URL`). `scripts/ensure-ios-privy-config.sh` writes them to the gitignored `Config/Environment.local.xcconfig` and rejects a non-https value.
- Build for another environment: `xcodebuild … MONACO_ENVIRONMENT=staging` (or pass `MONACO_STAGING_API_BASE_URL=https://…` on the same command line).
- Point an already-built Debug sim at staging or a tunnel without rebuilding: `MONACO_API_BASE_URL=https://<tunnel-host> just run mobile` (exported as `SIMCTL_CHILD_MONACO_API_BASE_URL`; add `MONACO_ENVIRONMENT=staging` to label it). Debug builds only.
- Release builds ignore the process environment and refuse to launch (`fatalError` naming the setting to fix) when the URL is empty, malformed, not `https`, a local host, or the environment is `local`. The rules live in `MonacoAPIConfiguration` and are covered by `just test mobile`.
- ATS stays strict. Only the Debug Info.plist carries `NSAllowsLocalNetworking`; there is no `NSAllowsArbitraryLoads`, so a Debug tunnel/staging URL must be `https` too.
- The active environment is logged at launch (`API environment: …`); Debug builds also show it under the session error on the sign-in gate.
- TestFlight builds: `scripts/ios-release.sh staging` or `scripts/ios-release.sh production`. See [`apps/mobile/TestFlight.md`](apps/mobile/TestFlight.md).

## Simulator

Slim is **not** required. `just run`, `just run mobile`, and `./scripts/ios-sim` warn and use a stock Xcode simulator when SimSlim is missing or `SIMSLIM_UDID` is unset. Privy xcconfig and `SIMCTL_CHILD_*` still apply.

`just test mobile` never boots a sim (host `swift test` in `packages/mobile-core`).

Fail only if no iOS Simulator exists: Xcode → Settings → Platforms, download an iOS 18+ runtime, create an iPhone sim.

Xcode Cmd+R also works after `./scripts/ensure-ios-privy-config.sh generate`. Without that file the app shows “Privy not configured”.

Never `simctl erase` a sim you later want as gold. Never commit a UDID. Never target by device name (`iPhone 17`).

### SimSlim (optional)

**SimSlim** turns one iOS Simulator into a RAM-thin “gold” device (~0.9 GB vs ~4 GB stock). Pick **one** sim per machine, slim it, reuse it.

1. **Xcode** with an **iOS 18.5+** simulator runtime (slim does not persist across reboot below 18.5).
2. Install: `brew install mobai-app/tap/simslim`
3. Create or pick one iPhone sim, copy the UDID:

   ```bash
   xcrun simctl list devices available
   xcrun simctl list runtimes
   # Example — Apple assigns a new UDID:
   xcrun simctl create "Monaco Gold" com.apple.CoreSimulator.SimDeviceType.iPhone-16 <runtime-identifier>
   ```

4. Export `SIMSLIM_UDID` (not a secret) in shell rc **and/or** plain gitignored `.env` (Justfile `dotenv-load` reads `.env`, not dotenvx `.env.local`):

   ```bash
   export SIMSLIM_UDID="<YOUR_UDID>"
   # optional: echo "SIMSLIM_UDID=<YOUR_UDID>" >> .env
   ```

   Agent QA uses `scripts/gold-sim-udid.sh`. In a linked worktree it prints that worktree's own simulator; in the primary checkout it exits 1 unless `SIMSLIM_UDID` is set and that device exists. If slim verify fails, human recipes still use that UDID as a normal simulator.

5. Slim profile. Repo copy: [`ci/profiles/base-slim.json`](ci/profiles/base-slim.json).

   ```bash
   mkdir -p ~/.config/simslim
   cp ci/profiles/base-slim.json ~/.config/simslim/base-slim.json
   simslim on "$SIMSLIM_UDID" --profile ~/.config/simslim/base-slim.json --json
   ```

   `except` in a profile means **keep** that daemon category on. base-slim keeps `siri`: without it the keyboard's dictation handler spins the app's main thread once a text field takes focus. Lane and journey simulators use the repo copy, so `scripts/simslim-ensure.sh` ignores the one in `~/.config`.

Repo `./scripts/ios-sim` and `./scripts/ios-build` call `xcodebuild` and `simctl` after Privy injection. Optional PATH wrappers in `~/.local/bin` are **not** in git and **not** required.

Keep gold **booted** between agent sessions when you can. Clone gold after slim-once if you need a second sim.

## Agent QA: the QA pot

The QA pot is one shared Solana mainnet wallet that funds test accounts with real USDC. Its key is `QA_POT_PRIVATE_KEY` in the encrypted `.env.local`, so every developer with `.env.keys` and every cloud agent can use it with no setup. Only `monacoctl qa` reads the key, and nothing prints it. Address: `67detL1H8561xpCWPUqQoy2WKeGKqauqkXRkfRoFviW8`.

```sh
scripts/with-dotenv-local.sh bin/monacoctl qa pot                             # address, SOL and USDC balances
scripts/with-dotenv-local.sh bin/monacoctl qa fund --user <user id> --usdc 2  # send USDC to that user's member wallet
```

`qa fund` sends only to the user's member wallet from `user_wallets`, at most 5 USDC per transfer. It refuses in production, and when the pot holds less than 0.01 SOL or less USDC than the amount. Keep each agent run under $5 in total.

Send the money back with an in-app withdrawal to the pot's address (`scripts/with-dotenv-local.sh bin/monacoctl qa pot --address`), after cashing out of any cabal. Don't move it any other way: a withdrawal keeps the ledger and the chain in agreement.

**Operator setup (done once).** `monacoctl qa pot new` made the keypair and wrote the encrypted key into `.env.local`. It refuses when a key is already set. The operator funds the pot with about 0.05 SOL and 20 to 30 USDC on Solana mainnet (mint `EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v`), and tops it up when `qa fund` reports it low.

## Agent QA: Phantom MCP

A per-machine alternative to the QA pot. Cloud agents can't use it.


Use this when a coding agent (or you, in Cursor chat) must move **real Solana mainnet** USDC into a sim user’s member wallet, then pull leftover cash out of the group vault when the run is done.

Keep the agent wallet thin. Preview software. Do not park rent money here.

Three wallets people mix up:

1. **Personal Phantom** (iOS / Android / browser extension). Your money. Create it yourself (below).
2. **Agent Phantom** (MCP). New dedicated wallet the first time the agent signs in. Empty until you fund it. QA faucet and refund target.
3. **Privy product wallets.** Member inbox + group vault. Phantom MCP **cannot** spend these. The agent can only **send USDC to** the copyable member address, then **receive USDC back** when you redeem to the agent address.

Do not put `PHANTOM_APP_ID` in Monaco `.env.local`. If a Cursor plugin still wants it, put it in Cursor MCP env only. Current `@phantom/mcp-server` device-code login does not require a Portal app id.

Never insert `FAKE*` wallet rows in local Postgres. The poller will break.

### Create a Phantom wallet

Personal wallet first — that is how you buy SOL/USDC and top up the agent address.

1. Download only from [phantom.com/download](https://phantom.com/download) (iOS, Android, Chrome, Brave, Firefox, Edge). App Store: [Phantom](https://apps.apple.com/us/app/phantom-trade-markets/id1598432977). Play: [Phantom](https://play.google.com/store/apps/details?id=app.phantom).
2. Follow [How to create a new Phantom wallet](https://phantom.com/learn/guides/how-to-create-a-new-wallet): Create a New Wallet → Google or Apple, or a secret recovery phrase.
3. Write down the recovery phrase / PIN. Never paste it into git, tickets, or chat.
4. Overview: [Get started](https://phantom.com/get-started). Help: [help.phantom.com](https://help.phantom.com).

### Install the Phantom MCP (agent wallet)

This is the **wallet MCP** (`@phantom/mcp-server`): sign, transfer, swap. It is not the docs-only MCP at `https://docs.phantom.com/mcp`.

Docs: [Phantom MCP server](https://docs.phantom.com/phantom-mcp-server) · [Setup](https://docs.phantom.com/phantom-mcp-server/setup) · npm `[@phantom/mcp-server](https://www.npmjs.com/package/@phantom/mcp-server)` · [Cursor MCP](https://cursor.com/docs/context/mcp)

**Cursor plugin (easiest):** marketplace search `phantom-connect` / Add Plugin. Bundles wallet MCP + docs MCP. See [AI-assisted development](https://docs.phantom.com/developer-powertools/ai-tools).

**Manual Cursor:** add to `~/.cursor/mcp.json` (merge into existing `mcpServers`; this repo’s `.cursor/mcp.json` is MobileBuildMCP + Pyth only):

```json
{
  "mcpServers": {
    "phantom": {
      "command": "npx",
      "args": ["-y", "@phantom/mcp-server@latest"]
    }
  }
}
```

Restart Cursor. First wallet tool call opens a browser for Google/Apple device-code sign-in. Session lives in `~/.phantom-mcp/session.json`. Reset: delete that file, restart, sign in again.

**Claude Code:** `claude mcp add phantom -- npx -y @phantom/mcp-server@latest`

On auth, Phantom mints a **new agent wallet**. It is not your extension wallet. Ask the agent for Solana addresses (`wallet_addresses` / `get_wallet_addresses`). Copy the Solana pubkey. That string is the refund target for leftover QA USDC. Each developer has their own; do not hardcode someone else’s address in the repo.

### Fund the agent wallet (~$1 SOL + ~$4 USDC on Solana)

The agent cannot transact on an empty wallet.

| Asset                     | Why                                                                                                 | Ballpark            |
| ------------------------- | --------------------------------------------------------------------------------------------------- | ------------------- |
| SOL on **Solana mainnet** | Fees when the agent sends USDC to a member inbox (and ATA rent if the dest has no USDC account yet) | about **$1** of SOL |
| USDC on **Solana**        | What the app actually credits after sweep                                                           | about **$4**        |

Buy or swap inside personal Phantom, then send **SOL** and **Solana USDC** to the **agent** Solana address. Confirm mint `EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v`. Ask the agent for `wallet_balances` before the first transfer.

Product path does **not** need SOL on the member wallet or vault (relayer pays). The **agent** still needs SOL because the agent is the sender.

### Send USDC into Monaco (member inbox → vault)

Same deposit path from **personal Phantom** works without MCP; see [Deposits](#deposits).

1. `just run` (API + sim). Sign in (SMS or email OTP).
2. Tap **Add money** and copy the deposit address (the Privy member wallet). Not the cabal treasury.
3. In Cursor: transfer **small** USDC on `solana:mainnet` to that address, mint above. MCP `transfer` / `transfer_tokens` simulates first; approve only if dest matches the copied address.
4. The USDC shows as account balance. Fund a cabal from it; the backend sweeps that amount to the treasury and credits shares once confirmed.
5. Explorer: [solscan.io](https://solscan.io) on the sweep signature.

### Sweep leftover back to the agent wallet (vault → Phantom)

Phantom MCP cannot pull from Privy. The reverse of deposit is **cash out**, then **withdraw** to the agent Solana address.

1. Agent: print Solana address again. Confirm it is **your** MCP wallet.
2. Cabal screen → **Cash out** the leftover stake (the maximum empties it). USDC lands in the account balance.
3. Withdraw the account balance to that agent Solana address (the **Cash out** button on the balance card on Home or Profile).
4. Wait for confirm. Agent: `wallet_balances` — USDC should be back. Treasury USDC for that test should be ~0 (dust from swaps possible).
5. If the pot holds stock tokens, cash out sells that slice to USDC first. Tiny leftover dust can remain; keep QA notionals small.

After a funding run, leftover **agent-test USDC belongs on the agent Phantom**, not in a group vault and not in a sim user’s inbox.

## Agent workflow setup

Agent owners and verifiers ship tickets in Claude Code with the pstack plugin, a set of model roles and the repo's skills and rules. `just install` writes the model roles. Trusting the folder in Claude Code enables the plugins from `.claude/settings.json`.

- [Agent workflow setup](docs/agents/setup.md): the plugins, model roles and skills, and the one-time steps.
- [Ship a ticket](docs/how-to/ship-a-ticket.md): one ticket from its issue to a merge, for a person or an agent owner.
- [Run a milestone](docs/how-to/run-a-milestone.md): batches, dispatch, verification, landing and handoff, for the orchestrator.
- [Standing orders](docs/agents/standing-orders.md): the rules every owner, verifier and orchestrator follows.

## Agent skills (Cursor)

Cursor loads repo skills from [`.cursor/skills/`](.cursor/skills/). Attach one in chat, or let the agent pick it from the description. Humans do not need these to `just run`.

Learned prefs and durable facts live in [`AGENTS.md`](AGENTS.md). Skills are the step-by-step workflows.

| Skill | Path | When to use |
| ----- | ---- | ----------- |
| **write-ticket** | [`.cursor/skills/write-ticket/SKILL.md`](.cursor/skills/write-ticket/SKILL.md) | Draft GitHub (or Linear) issue bodies. Six-section shape: Context, Problem, Proposal (with Scope), Acceptance Criteria (≥2 checkboxes), Verification commands, Done when. Keep Context vs Problem distinct. No nested triple-backtick fences inside the ticket body. |
| **worktree-orchestrate** | [`.cursor/skills/worktree-orchestrate/SKILL.md`](.cursor/skills/worktree-orchestrate/SKILL.md) | Parallel milestone work. Parent stays on the integration branch (`milestone-N`). Implementers ship in git worktrees on `feat/*`. Default implementer model is Composer 2.5. One light review, then merge. Do not nest another orchestrator. Split mobile vs backend to separate agents. Kickoff templates: [`prompts.md`](.cursor/skills/worktree-orchestrate/prompts.md). |
| **ios-simslim-fast-qa** | [`.cursor/skills/ios-simslim-fast-qa/SKILL.md`](.cursor/skills/ios-simslim-fast-qa/SKILL.md) | Agent sim smoke / tap-through. Unit tests first (`just test mobile`, no sim). Then one gold slim sim. Never `simctl erase`. Never destination by device name. MobileBuildMCP needs `--simulator-id` from `scripts/gold-sim-udid.sh` (`SIMSLIM_UDID` required). Human `just run` uses stock-sim fallback instead. |
| **anti-ai-slop** | [`.cursor/skills/anti-ai-slop/SKILL.md`](.cursor/skills/anti-ai-slop/SKILL.md) | Any UI, SwiftUI, empty states, onboarding, or marketing copy. Banlist for purple gradients, emoji-as-icons, Inter/system-ui-as-brand, glassmorphism, generic SaaS card grids. Product copy stays social-investing language (no wallets/gas/mint in the UI). |
| **testing-expert** | [`.cursor/skills/testing-expert/SKILL.md`](.cursor/skills/testing-expert/SKILL.md) | How to write tests: small surface, deterministic, realistic data. This copy is TS/Jest-oriented; Monaco still follows the same bar in Go and Swift. `just test mobile` is host `swift test`. `just test backend` uses stubs — never hit live Jupiter. Skip property tests that run longer than ~2 minutes. |

Do not copy these skills into another machine's home path. Clone the repo; Cursor sees `.cursor/skills/` from the workspace.

## Tests and CI

| Suite | Command |
| --- | --- |
| Backend and `scripts/` Go tests | `just test backend` |
| Shared Swift logic | `just test mobile` |
| Regenerate every generated file, `docs/reference` included (CI fails when stale) | `cd apps/backend && go generate ./...` |
| Docs site, broken links fail it | `python3.13 -m venv .venv && .venv/bin/pip install -r requirements-docs.txt && .venv/bin/mkdocs build --strict` |

The docs site needs Python 3.10 or newer. `.python-version` pins 3.13, and macOS's system `python3` (3.9) cannot install `requirements-docs.txt`.

The legacy backend, its migrations, its Go domain package and the reference trading bot were deleted in M7. `apps/backend` is now the new module's scaffold: `cmd/api`, `cmd/worker` and `cmd/monacoctl` with no features yet ([backend platform RFC](docs/architecture/backend-platform.md#rollout)).

`.github/workflows/ci.yml` runs on ready pull requests based on `main`: a `backend` job (`go vet`, `go test -race` in `apps/backend`, only when it or the CI files changed), a Linux `swift test` job for `packages/mobile-core`, the `apps/web` landing page tests, an iOS app build and test job on pull requests that touch the app, and `ci / ci-ok`, the one required check. A nightly run adds UI tests and screenshots. Details: [`docs/how-to/overnight-qa.md`](docs/how-to/overnight-qa.md).

## Pull requests

Changes ship as stacks of small PRs through Graphite, not as one large PR. Each PR builds and passes tests on its own and stays under 1000 changed lines (CI counts code, tests and docs; a human can add the `large-pr` label for a mechanical change). Titles say what the PR changes, with no issue number or commit-type prefix, and the body follows `.github/pull_request_template.md`: TLDR, Why, What changed, Proof, What came up, Reviewer focus. To split a branch that grew too big, use the `distribute-stack-changes` skill or `gt split --by-hunk`.

```bash
gt sync --no-restack          # pull main, drop merged branches, leave other stacks alone
gt create -m "first step"     # new branch + commit on top of the current branch
gt create -m "next step"      # stacks on the previous one
gt modify                     # amend the current branch; restacks the branches above
gt submit --stack --draft     # push the stack; open new PRs as drafts with the right base
scripts/pr-body.sh <n> "<title>" body.md  # check title, body and commits, then mark ready
gt restack                    # rebase the stack after main moves
```

Merge bottom-up. The rules and why they exist: [Pull requests: small and stacked](docs/architecture/backend-platform.md#pull-requests-small-and-stacked).

## Deploy

There is no deploy pipeline in this repo yet. The backend is being rebuilt; how it deploys is in the RFC's [Deploy and observability](docs/architecture/backend-platform.md#deploy-and-observability) section.

**iOS.** `scripts/ios-release.sh staging` or `scripts/ios-release.sh production` archives and uploads a TestFlight build. See [`apps/mobile/TestFlight.md`](apps/mobile/TestFlight.md).

**Trading agent.** An agent needs only its key and `PUBLIC_API_BASE_URL`. A ClawPump agent connects by pasting the connect instructions. See [`docs/how-to/connect-an-agent.md`](docs/how-to/connect-an-agent.md) and [`docs/agent-trading.md`](docs/agent-trading.md).

## Layout

```
monaco/
├── Justfile
├── docker-compose.yml
├── .env.example
├── AGENTS.md
├── README.md                 this file
├── apps/backend/             Go backend, being rebuilt (docs/architecture/backend-platform.md)
├── apps/mobile/              iOS app (SwiftUI)
├── apps/web/                 waitlist landing page
├── packages/mobile-core/     Swift logic tested on the host
├── docs/                     start at docs/index.md
└── scripts/                  dev scripts behind the just recipes
```

What each part does and how they connect: [`docs/architecture.md`](docs/architecture.md#repo-map).
