# Verify an EVM deposit

This runbook proves with real money that USDC sent on Base reaches a member's Solana balance exactly once. It goes through Monaco's API only. Do not use a one-off script.

Privy watches each member's EVM deposit address, bridges the USDC to the member's Solana wallet, and reports each forward as an execution. The `funding.evm_deposit_forward` poller records each execution as a row in `evm_deposit_forwards`. The row counts toward `arriving_micros` until the Solana deposit poller credits the transfer. The row is then closed by whichever runs second: the `funding.evm_deposit_arrived` consumer, or the poller seeing the execution succeed.

**EVM deposits arrive net of the bridge relayer fee.** The member pays it. Gas sponsorship covers Base gas, but not the bridge's relayer fee. On the first real run, 1 USDC sent on Base arrived on Base as 0.99925 USDC, Privy charged a 0.163098 USDC relayer fee and a 0.000748 USDC Privy fee, and 0.835383 USDC was credited. Privy reports the net amount only once the execution succeeds, and reports no Solana signature. So the forward row matches its credit by that net amount.

## Before you start

- #4966 has landed, so `GET /v1/me/deposit-addresses` returns an EVM address and the poller reads real executions.
- Privy gas sponsorship is on for Base in the Privy dashboard, and the Privy gas credit balance is above zero. The member's EVM deposit wallet holds no ETH. Without sponsorship, Privy rejects the forwarding swap with `insufficient funds for gas`. Without credits, it rejects it with `Insufficient gas credits balance`.
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
6. As B, poll `GET /v1/me/balance` every 10 s. `arriving_micros` first shows the amount that reached Base. Then `available_micros` rises by the net amount and `arriving_micros` drops to `0`.
7. As B, call `GET /v1/me/txns`. It shows exactly one deposit for the net amount.
8. Return leftover USDC to the pot with an in-app withdrawal to the address from `monacoctl qa pot --address`. A withdrawal must be at least 1 USDC. For less, fund B with 0.002 SOL (`monacoctl qa fund --user <B> --sol 0.002`) and send it with `monacoctl dev privy-transfer --user <B> --network solana --to <pot> --usdc <amount> --key <key>`.

If a forward failed and the USDC is still on the deposit address, rescan it with `monacoctl dev privy-reindex --wallet <privy evm wallet id> --network base`. To move USDC out of an app-owned EVM wallet, use `monacoctl dev privy-transfer --wallet <id> --from base --network solana`.

Paste each request, response and chain hash into the PR body. Record the hashes in the table below.

## Recorded runs

| Date | Network | Amount | Base send hash | Privy execution id | Solana credit signature | Credited once |
| --- | --- | --- | --- | --- | --- | --- |
| 2026-10-11 | base | 1 USDC sent, 0.99925 arrived | [`0xf60796ba…439871`](https://basescan.org/tx/0xf60796baf1c0d16e2dcc9f7864bd491b467aca5bbef06778f285009812439871) | `ucemiieac4nz8uyq08oq1ldm`, failed | none: Privy rejected the swap for gas | not reached |
| 2026-10-11 | base | 0.99925 USDC reindexed, 0.835383 credited | [`0x11229470…f4acc8`](https://basescan.org/tx/0x1122947090ae3745a526c1aa48411e82601cc4899be2d8057b017d4c16f4acc8) (forward) | `jh0rpn9be4m37nviomijc2h1`, completed | [`3qS4waw6…JqBnb`](https://solscan.io/tx/3qS4waw65QtqzwciFhxpWB8m8Q9zK3i9CRVdi5A5hwj8ucDNnCdvDufiEKddRfEttbxv6zUpaq21AVBSaiqJqBnb) | yes |

### 2026-10-11: the forward fails without Base gas sponsorship

| Step | Result |
| --- | --- |
| QA users | A `01a12a2b-4368-7806-b70e-20799c1cfb1e` (`@dev_qa_4966`, Solana `4RpUTNtzuCs9xNvQnygP3dyyhH3NLh1NFhuFXeA8PDKU`). B `01a12ad0-4e7a-71f0-a45a-8033338e9d04` (`@dev_qa_4967_b`, Solana `7QqVy7ZAW528eEN4pTTZfFioNUHkwvZYd8hfY2n4Y51s`) |
| `GET /v1/me/deposit-addresses` as B | Solana first, then `base` `0x88e114ffa14f6c8c4cfb9a87671f60623c607ffc`. This created Privy EVM wallet `bi1x0oa1thtml29bp8oojev6` and attached the forwarding automation to B's Solana wallet |
| Fund A from the QA pot | 1 USDC [`21L3vsjG…78AWEg`](https://solscan.io/tx/21L3vsjG1wW3a2XxYkpgnJ41iwNbos2vgG2TCjny4EiP5z8AkQk3CMgdzdYcSchZ1HsoCxnm1M2Jfaq16t78AWEg), 0.002 SOL [`5yZYf8S1…ZJZvfsGQ`](https://solscan.io/tx/5yZYf8S1wCKDewqy7hsfwyBXc8WbvZ5Cn23V9uJrC9VVLzpStLq1MvYLQvjyKwpiMtRrrq1a8ay8RaPzZJZvfsGQ) |
| Send 1 USDC from A to B's Base address | `POST /v1/me/withdrawals` with `destination_kind: evm` still answered `not_implemented` (#4972's wiring, #5091, hadn't landed), so `monacoctl dev privy-transfer` sent it. Action `cd2f08a4-8daf-4025-95ae-06c30b250a46` succeeded: Solana [`NyPe12UL…ChMo5zVn`](https://solscan.io/tx/NyPe12ULpBPzoc7rm67q8uhK5ZhuoRKXaeL1DaESrEMo9G6XRsrqjQz9uoTwG4wfQK4Y7Z3Hm1sG42iChMo5zVn), Base arrival `0xf60796ba…439871`, 0.99925 USDC |
| Privy forwarding | Execution `ucemiieac4nz8uyq08oq1ldm` started 9 s after the Base arrival. Its swap action `6ede3d64-e56c-453e-9e6f-896661554c34` was `rejected`: `insufficient funds for gas * price + value: have 0 want 1883811600000`. Estimated fees were a 0.163078 USDC relayer fee and a 0.000749 USDC Privy fee |
| Monaco | The poller stored forward `01a12ad3-620a-743a-a81b-01dd5100e930` as `pending` at 11:57:38 UTC, and B's `arriving_micros` read `999250`. At 11:57:48 the rejected action moved the row to `failed`, and `arriving_micros` returned to `0`. No `deposit.evm_forwarded` was published and nothing was credited |

The 0.99925 USDC stayed on B's Base deposit address until the second run below.

### 2026-10-11: credited once, net of the relayer fee

Base gas sponsorship was on, and the operator had added Privy gas credits.

| Step | Result |
| --- | --- |
| Sponsorship check | `monacoctl dev privy-transfer --wallet vkaerr1g584yfy8ijjro6v14 --from base --network solana --usdc 1.9985` moved the USDC stranded on Monaco's app-owned Base wallet back to the QA pot. Action `cff0078b-a71b-44c3-adff-7d0e93e666f9`, sponsored user operation `0x40ff4e5a…be7eb`, Base tx [`0x541e45dd…bc13f5`](https://basescan.org/tx/0x541e45ddac84037e7e08cf9c5b1f090aeeff361047e6ed45749ea33547bc13f5). 1.997002 USDC reached the pot |
| Reindex | `monacoctl dev privy-reindex --wallet bi1x0oa1thtml29bp8oojev6 --network base` at 14:49:27 UTC. Privy started execution `jh0rpn9be4m37nviomijc2h1` |
| Forward | Swap action `5ae2dee7-9b06-4491-bea5-f79481138698` succeeded: 999250 micros in, 835383 out. Fees: relayer 0.163098 USDC, Privy 0.000748 USDC. User operation `0xd5dfb6e8…6efedb`, Base tx [`0x11229470…f4acc8`](https://basescan.org/tx/0x1122947090ae3745a526c1aa48411e82601cc4899be2d8057b017d4c16f4acc8) |
| Monaco | At 14:49:38 the poller stored forward `01a12b70-d806-7e50-ba0e-0e54b633c15f` as `pending` for 999250, and published one `deposit.evm_forwarded` for 835383. At 14:50:07 the deposit poller credited [`3qS4waw6…JqBnb`](https://solscan.io/tx/3qS4waw65QtqzwciFhxpWB8m8Q9zK3i9CRVdi5A5hwj8ucDNnCdvDufiEKddRfEttbxv6zUpaq21AVBSaiqJqBnb) for 835383: one `deposit.credited`, deposit `01a12b71-4bc8-761a-a048-1047984cb7e2`. The forward closed at 14:53:49 as `arrived` on that signature, once the poller refreshed its amount to the net 835383 and matched the credit |
| B's balance | `{"arriving_micros":"0","available_micros":"835383","on_chain_micros":"835383",...}`. `GET /v1/me/txns` shows one settled deposit of 835383 |
| Refund | 0.002 SOL from the pot to B, [`2UfSp5L9…jw6L979`](https://solscan.io/tx/2UfSp5L94akW5d6AKp8t5pmH7CBNLGeDsZRFyhC6t3hNLwTwnuGUiKdQTtQXBeuYRWjHAsBdVXNGNHdt1jw6L979). Then B's 0.835383 USDC to the pot, action `3f9c89cf-b2fa-4dcb-843f-c74f80e1dddf`, [`3dddBVpt…GBXoxSL`](https://solscan.io/tx/3dddBVptQW1Dp3fJy6CMa3UMN6gFHzLAdLfPqHZbovTcyzR1VjqW1aqFHsyQZ4s5FTUF9ttx4p8zDhdHBGFXoxSL) |

## If it goes wrong

- `GET /v1/me/deposit-addresses` returns only the Solana item. The EVM wallet call failed. Look for the `identity.deposit_addresses.evm_failed` log line and its `code`.
- `arriving_micros` appears, then drops to `0` with no credit. Privy failed the execution, and the row is `failed`. Read the execution's action with `GET /v1/wallets/{wallet_id}/actions/{action_id}?include=steps`. A rejected `evm_transaction` step with `insufficient funds for gas` means gas sponsorship is off for that network.
- `arriving_micros` never appears. Check the `funding.evm_deposit_forward` tick lines for failures. A `funding.evm_forward.unrouted` alert means Privy failed the execution before it created an action. The poller has already asked Privy to reindex the wallet.
- `arriving_micros` stays above zero. A `funding.evm_forward.stuck` alert fires once when a forward stays pending for more than 30 minutes. If the deposit was credited but the row stayed pending, the credit matched neither the execution's Solana signature nor its net amount within 24 hours of the row's creation.
- Privy rejects the forward with `Insufficient gas credits balance`. Sponsorship is on, but the Privy gas credit balance is empty. Add credits in the Privy dashboard, then reindex the wallet.
