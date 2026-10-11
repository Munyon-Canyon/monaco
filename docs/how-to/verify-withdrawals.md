# Verify EVM withdrawals

This page shows how to prove an EVM withdrawal end to end with real USDC on mainnet, through Monaco's API, and records the first run. A member withdraws from their Solana balance to an address on Base, Arbitrum or Ethereum. The backend sends one Privy `/transfer` from the member wallet, keyed by the withdrawal id, and the withdrawal poller follows the Privy action until it ends. [Privy wallet actions](privy-wallet-actions.md) explains the transfer itself.

## How a withdrawal moves

1. `POST /v1/me/withdrawals` with `destination_kind: evm`, a `network` and an EVM `to_address`. The backend validates the network and address before anything is held, places the hold, creates the Privy transfer with idempotency key = withdrawal id, and stores the action id with the move to `submitted`.
2. Every 5 seconds the worker reads the action.
   - The first time Privy reports the Solana step `confirmed`, the worker stores that signature in `tx_signature`.
   - `succeeded` moves the withdrawal to `confirmed`, and `withdrawal.confirmed` settles it in the user ledger.
   - `rejected`, or `failed` with no step broadcast, moves it to `failed` and releases the hold.
   - `failed` after a step was broadcast keeps the hold, is never retried, and logs `funding.withdrawal.needs_support` once.
3. If the create's outcome is unknown, the row stays `created`. After `WITHDRAWAL_UNSENT_AGE` the worker asks Privy again with the same key, so the transfer is never sent twice.

## Preview what arrives

`POST /v1/me/withdrawals/preview` takes the same body as the withdrawal and holds or sends nothing. A Solana withdrawal receives the whole amount. For an EVM network, the backend asks Privy for an exact-input quote (`POST /v1/wallets/{id}/transfer/quote`) and returns its `estimated_output_amount` as `receive_micros`, with the quote's `expires_at`. Privy checks the member wallet's balance, so a preview for more than the wallet holds answers `insufficient_funds`.

A real preview on 2026-10-11, for the QA user below holding 1 USDC:

```http
POST /v1/me/withdrawals/preview

{"amount_micros":"1000000","destination_kind":"evm","network":"base","to_address":"0x670a375581f32356bd269d5e5cb66e2c55c295b2"}
```

```json
{"expires_at":"2026-10-11T10:49:29Z","receive_micros":"999250"}
```

That matches the 0.99925 USDC the first run delivered. The 1 USDC funded for this preview (`2S9U9buxZo8X7GTUNg3bigPA4q6dyrdjvERGJiCKpgUafY1e9VgtkSZ3QoxziGVHBMCcFcFWEoszykYkQrQDoRes`) went back to the QA pot with an in-app Solana withdrawal, `01a12a94-ab45-7ad5-b94e-d64fb4e31120` ([`4gcGkgz24rcBongoniKTzNsdeTyzcSbtrEDGW1UcXz4VTUGMQ9GunmcihJaggKevMDLo44z3P38cBgjzLu7jSk6B`](https://solscan.io/tx/4gcGkgz24rcBongoniKTzNsdeTyzcSbtrEDGW1UcXz4VTUGMQ9GunmcihJaggKevMDLo44z3P38cBgjzLu7jSk6B)).

## Before you run it

- The member wallet pays the Solana fee for the transfer until Privy gas sponsorship is on. Fund it with a little SOL: `monacoctl qa fund --user <user-uuid> --sol 0.002`. A wallet with no SOL gets an action that ends `rejected` (`AccountNotFound`), and the withdrawal fails with `transfer_rejected`.
- Fund the USDC from the QA pot only, at most 5 USDC per transfer: `monacoctl qa fund --user <user-uuid> --usdc 1`.
- Prefix `SOLANA_RPC_URL=https://api.mainnet-beta.solana.com` on a command if the RPC in `.env.local` is out of quota.

## Run it

1. Build and start the backend on the `.env.local` configuration: `just build backend`, then `bin/api` and `bin/worker` under `scripts/with-dotenv-local.sh`. To keep clear of another checkout's backend, set `MONACO_HTTP_ADDR=127.0.0.1:18080` and `MONACO_WORKER_HEALTH_ADDR=127.0.0.1:18081`.
2. Mint a session token for the QA user: `scripts/with-dotenv-local.sh bin/monacoctl dev token --user <user-uuid>`.
3. `GET /v1/me/balance` until `available_micros` shows the funded amount.
4. `POST /v1/me/withdrawals` with an `Idempotency-Key`.
5. `GET /v1/me/withdrawals/{id}` until `confirmed`. Read the Privy action id from `withdrawals.privy_action_id`.
6. Find the arrival on the destination chain from the USDC `Transfer` logs to the destination address. Privy's bridge leg carries no hash.

## First run: 1 USDC from Solana to Base, 2026-10-11

| Item | Value |
| --- | --- |
| Commit | `4972-evm-withdrawal-wiring` on top of `b42b808b9` |
| QA user | `01a12a2b-4368-7806-b70e-20799c1cfb1e`, member wallet `hptdesauesks76u0qz6h3n32` (`4RpUTNtzuCs9xNvQnygP3dyyhH3NLh1NFhuFXeA8PDKU`), holding 0.001995 SOL |
| Funding from the QA pot | 1 USDC, [`3Syi4QXSPrRUkvz54f95zwayHVZrzrByMuY7eYrSsmqFVRiRqaBBokcLRzUiiGxwG57tmfJVDXg5WsRc74cDQrYg`](https://solscan.io/tx/3Syi4QXSPrRUkvz54f95zwayHVZrzrByMuY7eYrSsmqFVRiRqaBBokcLRzUiiGxwG57tmfJVDXg5WsRc74cDQrYg) |
| Destination | Monaco's app-owned Base wallet `vkaerr1g584yfy8ijjro6v14`, `0x670a375581f32356bd269d5e5cb66e2c55c295b2` |
| Withdrawal | `01a12a82-2822-7088-a7f3-21c4a3d924e5`, created 10:28:55Z, submitted 10:28:56Z, confirmed 10:29:10Z |
| Privy action | `0f028982-29a0-44c7-b1a7-d15a9aa7fc87`, `succeeded` |
| Solana transaction | [`2sSTiSNQzkFbG4PFWTpcrvi15hqX9EqgUhtcLmQf9JnBNDhNs1JCfMCKB2eU8zU5Jkp1DPkKefor762sJWYJYHyN`](https://solscan.io/tx/2sSTiSNQzkFbG4PFWTpcrvi15hqX9EqgUhtcLmQf9JnBNDhNs1JCfMCKB2eU8zU5Jkp1DPkKefor762sJWYJYHyN) |
| Base transaction | [`0xf1c6e07c0ecf55632a5aa782831f56debc7cbd4298a2e07194f530aeea98f022`](https://basescan.org/tx/0xf1c6e07c0ecf55632a5aa782831f56debc7cbd4298a2e07194f530aeea98f022), block 52462597, 0.99925 USDC from the bridge solver `0xf70da97812cb96acdf810712aa562db8dfa3dbef` |
| User ledger | one `withdrawal` row, `settled`, with the Solana signature |
| Received | 0.99925 USDC for 1 USDC sent (7.5 bps Privy fee) |

The request:

```http
POST /v1/me/withdrawals
Idempotency-Key: <uuid>

{"amount_micros":"1000000","destination_kind":"evm","network":"base","to_address":"0x670a375581f32356bd269d5e5cb66e2c55c295b2"}
```

The response:

```json
{"status":"submitted","tx_signature":"","withdrawal_id":"01a12a82-2822-7088-a7f3-21c4a3d924e5"}
```

`GET /v1/me/withdrawals/01a12a82-2822-7088-a7f3-21c4a3d924e5` once it confirmed:

```json
{"amount_micros":"1000000","completed_at":"2026-10-11T06:29:10.406608-04:00","created_at":"2026-10-11T06:28:55.809232-04:00","fail_code":null,"status":"confirmed","to_address":"0x670a375581f32356bd269d5e5cb66e2c55c295b2","tx_signature":"2sSTiSNQzkFbG4PFWTpcrvi15hqX9EqgUhtcLmQf9JnBNDhNs1JCfMCKB2eU8zU5Jkp1DPkKefor762sJWYJYHyN","withdrawal_id":"01a12a82-2822-7088-a7f3-21c4a3d924e5"}
```

The QA user's whole 1 USDC left in the withdrawal, so nothing was left to refund to the pot. The 0.99925 USDC sits in the app-owned Base wallet next to the run from [Privy wallet actions](privy-wallet-actions.md). It can move back once Base gas sponsorship is on.
