# Verify an EVM deposit

This runbook proves with real money that USDC sent on Base reaches a member's Solana balance exactly once. It goes through Monaco's API only. Do not use a one-off script.

Privy watches each member's EVM deposit address, bridges the USDC to the member's Solana wallet, and reports each forward as an execution. The `funding.evm_deposit_forward` poller records each execution as a row in `evm_deposit_forwards`. The row counts toward `arriving_micros` until the Solana deposit poller credits the transfer. The `funding.evm_deposit_arrived` consumer then closes the row.

## Before you start

- #4966 has landed, so `GET /v1/me/deposit-addresses` returns an EVM address and the poller reads real executions.
- The shared QA pot holds at least 2 USDC. Check it with `monacoctl qa pot`.
- `EVM_DEPOSIT_NETWORKS` includes `base`. It is the default.

## Run it

Run every command from the repo root.

1. Start the backend with real providers: `just run backend`. It reads `.env.local`.
2. Sign in QA users A and B with Privy test logins. Keep each user's id and session token.
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
| pending | base | 1 USDC | pending: needs #4966 and QA pot funds | pending | pending | pending |

## If it goes wrong

- `GET /v1/me/deposit-addresses` returns only the Solana item. The EVM wallet call failed. Look for the `identity.deposit_addresses.evm_failed` log line and its `code`.
- `arriving_micros` never appears. Check the `funding.evm_deposit_forward` tick lines for failures. A `funding.evm_forward.unrouted` alert means Privy failed the execution before it created an action. The poller has already asked Privy to reindex the wallet.
- `arriving_micros` stays above zero. A `funding.evm_forward.stuck` alert fires once when a forward stays pending for more than 30 minutes. If the deposit was credited but the row stayed pending, the credit matched neither the execution's Solana signature nor the amount within 24 hours.
