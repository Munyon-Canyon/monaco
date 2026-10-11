# Privy wallet actions

This page shows how the backend moves USDC between chains with Privy wallet actions, and records the first real transfer. The client is `apps/backend/internal/platform/chain/privy/actions.go`. It implements `multichain.WalletActions`.

## How a transfer works

1. The backend sends `POST /v1/wallets/{wallet_id}/transfer` with the amount in decimal USDC, `amount_type: exact_input`, the source chain, and the destination address, chain and asset.
2. The backend signs the request with the app authorization key, the same key that signs Solana transactions (`sign.go`). The signature covers the `privy-app-id` and `privy-idempotency-key` headers.
3. Every transfer carries a `privy-idempotency-key`. A retry with the same key returns the same action and never sends twice. Store the action id before you poll.
4. Privy answers with a pending action. Poll `GET /v1/wallets/{wallet_id}/actions/{action_id}?include=steps` until the action leaves `pending`.

The client decodes the action strictly. An unknown action status, step status, step type or chain id is a `decode_failed` error, never a guess.

| Privy action status | `multichain.ActionState` |
| --- | --- |
| `pending` | `pending` |
| `succeeded` | `succeeded` |
| `rejected` | `rejected` (nothing happened on chain, safe to retry with a new key) |
| `failed` | `failed` (a transaction was broadcast; inspect the steps) |

| Privy step status | `multichain.StepState` |
| --- | --- |
| `preparing`, `queued`, `pending`, `retrying`, `custodian_reviewing` | `pending` |
| `confirmed` | `confirmed` |
| `rejected`, `reverted`, `replaced`, `abandoned`, `failed` | `failed` |

A bridge leg is an `external_transaction` step. It has no chain id and no hash: read the arrival on the destination chain.

## The source wallet pays Solana gas

Without app-wide gas sponsorship, the source wallet pays the Solana fee. A member wallet that holds no SOL gets an action that ends `rejected` with `Transaction simulation failed: "AccountNotFound"`. Fund the wallet with a little SOL first. For QA, use `monacoctl qa fund --user <user-uuid> --sol 0.002`.

## Run a transfer by hand

The `monacoctl dev` commands call the client. They refuse to run with `MONACO_ENV=production`, and they send at most 5 USDC.

```sh
# An app-owned EVM wallet to receive the transfer. The same key returns the same wallet.
scripts/with-dotenv-local.sh sh -c 'cd apps/backend && go run ./cmd/monacoctl dev privy-evm-wallet --key <key>'

# Send from a user's member wallet and poll until the action ends.
scripts/with-dotenv-local.sh sh -c 'cd apps/backend && go run ./cmd/monacoctl dev privy-transfer \
  --user <user-uuid> --to <0x address> --network base --usdc 1 --key <idempotency key>'
```

## First real transfer: 1 USDC from Solana to Base, 2026-10-11

This run settled the open question on #4917: Privy accepts our additional-signer authorization key for `/transfer` on a member wallet. No user JWT is needed.

| Item | Value |
| --- | --- |
| QA user | `01a12a2b-4368-7806-b70e-20799c1cfb1e` (`@dev_qa_4966`) |
| Member wallet | `hptdesauesks76u0qz6h3n32`, `4RpUTNtzuCs9xNvQnygP3dyyhH3NLh1NFhuFXeA8PDKU` |
| Funding from the QA pot | 1 USDC, `3WXXb8KBcUig7fKSs2auDNyWe34oAzW6JQqM19zcCPvuUY1VKGAQtwZnowP7V1CqSyGkNHXKcwCfi9T2bFd8crrf`; 0.002 SOL, `YHDVEw7BRj5E6qPeoN131KGgN5RcpbuSibc5vPsPRU2GcrCBYgm3V9McdA5rxjGx6swGns4w7LeCYUKjVzG46nV` |
| Destination | app-owned EVM wallet `vkaerr1g584yfy8ijjro6v14`, `0x670a375581f32356bd269d5e5cb66e2c55c295b2` |
| Action | `b2c90d14-283e-49ee-8fdc-a1be41981401`, `succeeded` |
| Solana transaction | [`5HH6nAAhGKPMKekeAVYod4L9zkt3caV3umKk6ihY4TNqwa1s8ZxdJAspBD8maxwcDHDbPTWMJkQ5XeHFUaV1pgTA`](https://solscan.io/tx/5HH6nAAhGKPMKekeAVYod4L9zkt3caV3umKk6ihY4TNqwa1s8ZxdJAspBD8maxwcDHDbPTWMJkQ5XeHFUaV1pgTA) |
| Base transaction | [`0x8381b91baf4617364944e0099c033be907a81748a3f354c1d08e608527fc4aad`](https://basescan.org/tx/0x8381b91baf4617364944e0099c033be907a81748a3f354c1d08e608527fc4aad), block 52459928, 0.99925 USDC from the bridge solver `0xf70da97812cb96acdf810712aa562db8dfa3dbef` |
| Cost | 0.00075 USDC Privy fee (7.5 bps), relayer fee 0, 260 lamports Solana gas |

An earlier attempt with the same wallet before it held SOL, action `03727fc4-64fe-44b6-aea5-cb5f22a8c232`, ended `rejected` with `AccountNotFound` and moved nothing.

The request the client sent, signed with our authorization key and the idempotency key `qa-4966-sol-to-base-2`:

```json
{
  "amount": "1",
  "amount_type": "exact_input",
  "source": {"asset": "usdc", "chain": "solana"},
  "destination": {"asset": "usdc", "chain": "base", "address": "0x670a375581f32356bd269d5e5cb66e2c55c295b2"}
}
```

The action after it succeeded:

```json
{
  "id": "b2c90d14-283e-49ee-8fdc-a1be41981401",
  "status": "succeeded",
  "wallet_id": "hptdesauesks76u0qz6h3n32",
  "created_at": "2026-10-11T08:59:57.325Z",
  "steps": [
    {
      "type": "svm_transaction",
      "status": "confirmed",
      "finalized": true,
      "caip2": "solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp",
      "transaction_signature": "5HH6nAAhGKPMKekeAVYod4L9zkt3caV3umKk6ihY4TNqwa1s8ZxdJAspBD8maxwcDHDbPTWMJkQ5XeHFUaV1pgTA"
    },
    {"type": "external_transaction", "status": "confirmed"}
  ],
  "type": "transfer",
  "source_asset": "usdc",
  "source_amount": "1",
  "source_chain": "solana",
  "destination_address": "0x670a375581f32356bd269d5e5cb66e2c55c295b2",
  "destination_asset": "usdc",
  "destination_chain": "base",
  "destination_amount": "0.99925",
  "estimated_fees": [{"type": "relayer", "amount": "0"}, {"type": "privy", "amount": "0.00075"}],
  "estimated_gas": {"base_amount": "260", "amount": "0.00000026", "gas_asset": "SOL"}
}
```

The 0.99925 USDC stays in the app-owned Base wallet `vkaerr1g584yfy8ijjro6v14`. It can't move back yet, because that wallet holds no ETH for Base gas. A later proof can bridge it back to the QA pot once gas sponsorship is on.
