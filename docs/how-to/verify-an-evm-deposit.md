# Verify an EVM deposit

This runbook proves with real money that USDC sent on Base reaches a member's Solana balance exactly once. It goes through Monaco's API only. Do not use a one-off script.

Privy watches each member's EVM deposit address, bridges the USDC to the member's Solana wallet, and reports each forward as an execution. The `funding.evm_deposit_forward` poller records each execution as a row in `evm_deposit_forwards`. The row counts toward `arriving_micros` until the Solana deposit poller credits the transfer. The `funding.evm_deposit_arrived` consumer then closes the row.

## Before you start

- #4966 has landed, so `GET /v1/me/deposit-addresses` returns an EVM address and the poller reads real executions.
- Privy gas sponsorship is on for Base in the Privy dashboard. The member's EVM deposit wallet holds no ETH, so without sponsorship Privy rejects the forwarding swap (see the first recorded run).
- The shared QA pot holds at least 2 USDC. Check it with `monacoctl qa pot`.
- `EVM_DEPOSIT_NETWORKS` includes `base`. It is the default.

## Run it

Run every command from the repo root.

1. Start the backend with real providers: `just run backend`. It reads `.env.local`.
2. Create QA users A and B with real Privy member wallets, and mint a session token for each: `scripts/with-dotenv-local.sh sh -c 'cd apps/backend && go run ./cmd/monacoctl dev token --user new --pool <name> --ttl 6h'`. The same `--pool` name reuses the same user. `--user <id>` mints a token for an existing user.
3. Fund A from the QA pot: `monacoctl qa fund --user <A's user id> --usdc 2`. It sends at most 5 USDC per transfer.
4. As B, call `GET /v1/me/deposit-addresses`. The first item is B's Solana wallet. Copy the address of the `base` item.
5. Send 1 USDC on Base from A to that address:
    - Once #4972 has landed, call `POST /v1/me/withdrawals` as A with `destination_kind: evm`, `network: base`, the copied address and `amount_micros: 1000000`.
    - Until then, use a Privy `/transfer` from A's member wallet through the #4966 client.
6. As B, poll `GET /v1/me/balance` every 10 s. Wait for `arriving_micros` to show the forwarded amount, then for it to drop to `0` as `available_micros` rises.
7. As B, call `GET /v1/me/txns`. It shows exactly one deposit for the forwarded amount.
8. Return leftover USDC to the pot with an in-app withdrawal to the address from `monacoctl qa pot --address`.

Paste each request, response and chain hash into the PR body. Record the hashes in the table below.

## Recorded runs

| Date | Network | Amount | Base send hash | Privy execution id | Solana credit signature | Credited once |
| --- | --- | --- | --- | --- | --- | --- |
| 2026-10-11 | base | 1 USDC sent, 0.99925 arrived | [`0xf60796ba…439871`](https://basescan.org/tx/0xf60796baf1c0d16e2dcc9f7864bd491b467aca5bbef06778f285009812439871) | `ucemiieac4nz8uyq08oq1ldm`, failed | none: Privy rejected the swap for gas | not reached |

### 2026-10-11: the forward fails without Base gas sponsorship

| Step | Result |
| --- | --- |
| QA users | A `01a12a2b-4368-7806-b70e-20799c1cfb1e` (`@dev_qa_4966`, Solana `4RpUTNtzuCs9xNvQnygP3dyyhH3NLh1NFhuFXeA8PDKU`). B `01a12ad0-4e7a-71f0-a45a-8033338e9d04` (`@dev_qa_4967_b`, Solana `7QqVy7ZAW528eEN4pTTZfFioNUHkwvZYd8hfY2n4Y51s`) |
| `GET /v1/me/deposit-addresses` as B | Solana first, then `base` `0x88e114ffa14f6c8c4cfb9a87671f60623c607ffc`. This created Privy EVM wallet `bi1x0oa1thtml29bp8oojev6` and attached the forwarding automation to B's Solana wallet |
| Fund A from the QA pot | 1 USDC [`21L3vsjG…78AWEg`](https://solscan.io/tx/21L3vsjG1wW3a2XxYkpgnJ41iwNbos2vgG2TCjny4EiP5z8AkQk3CMgdzdYcSchZ1HsoCxnm1M2Jfaq16t78AWEg), 0.002 SOL [`5yZYf8S1…ZJZvfsGQ`](https://solscan.io/tx/5yZYf8S1wCKDewqy7hsfwyBXc8WbvZ5Cn23V9uJrC9VVLzpStLq1MvYLQvjyKwpiMtRrrq1a8ay8RaPzZJZvfsGQ) |
| Send 1 USDC from A to B's Base address | `POST /v1/me/withdrawals` with `destination_kind: evm` still answered `not_implemented` (#4972's wiring, #5091, hadn't landed), so `monacoctl dev privy-transfer` sent it. Action `cd2f08a4-8daf-4025-95ae-06c30b250a46` succeeded: Solana [`NyPe12UL…ChMo5zVn`](https://solscan.io/tx/NyPe12ULpBPzoc7rm67q8uhK5ZhuoRKXaeL1DaESrEMo9G6XRsrqjQz9uoTwG4wfQK4Y7Z3Hm1sG42iChMo5zVn), Base arrival `0xf60796ba…439871`, 0.99925 USDC |
| Privy forwarding | Execution `ucemiieac4nz8uyq08oq1ldm` started 9 s after the Base arrival. Its swap action `6ede3d64-e56c-453e-9e6f-896661554c34` was `rejected`: `insufficient funds for gas * price + value: have 0 want 1883811600000`. Estimated fees were a 0.163078 USDC relayer fee and a 0.000749 USDC Privy fee |
| Monaco | The poller stored forward `01a12ad3-620a-743a-a81b-01dd5100e930` as `pending` at 11:57:38 UTC, and B's `arriving_micros` read `999250`. At 11:57:48 the rejected action moved the row to `failed`, and `arriving_micros` returned to `0`. No `deposit.evm_forwarded` was published and nothing was credited |

The 0.99925 USDC stays on B's Base deposit address. Once gas sponsorship is on for Base, ask Privy to rescan the wallet with `POST /v1/wallet_automations/reindex` for wallet `bi1x0oa1thtml29bp8oojev6` on `base`. The automation then forwards the full balance to B's Solana wallet, and the run continues from step 6.

## If it goes wrong

- `GET /v1/me/deposit-addresses` returns only the Solana item. The EVM wallet call failed. Look for the `identity.deposit_addresses.evm_failed` log line and its `code`.
- `arriving_micros` appears, then drops to `0` with no credit. Privy failed the execution, and the row is `failed`. Read the execution's action with `GET /v1/wallets/{wallet_id}/actions/{action_id}?include=steps`. A rejected `evm_transaction` step with `insufficient funds for gas` means gas sponsorship is off for that network.
- `arriving_micros` never appears. Check the `funding.evm_deposit_forward` tick lines for failures. A `funding.evm_forward.unrouted` alert means Privy failed the execution before it created an action. The poller has already asked Privy to reindex the wallet.
- `arriving_micros` stays above zero. A `funding.evm_forward.stuck` alert fires once when a forward stays pending for more than 30 minutes. If the deposit was credited but the row stayed pending, the credit matched neither the execution's Solana signature nor the amount within 24 hours.
