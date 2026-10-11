# Verify multichain money end to end

This runbook proves that money goes all the way through Monaco across chains, with real USDC on mainnet: out to Base, in from Base, into a cabal, a buy routed to an EVM chain at the best price, a sell, and out to Base again. [Flow 30](../architecture/backend-platform.md#flows) proves the same path against fakes. This run proves it against Privy, Uniswap, Jupiter and the chains. The design is in [Deposits from an EVM chain](../architecture/deposits-withdrawals.md#deposits-from-an-evm-chain), [Routing across chains](../architecture/trade-execution.md#routing-across-chains) and [Withdraw to an EVM wallet](../architecture/deposits-withdrawals.md#withdraw-to-an-evm-wallet).

Run it once after every M24 ticket (#4966 to #4973) has landed on `staging`, and again whenever a change touches the multichain money path. Record the run in [Results](#results).

## How the money moves

Two QA users, A and B, pass the money between them on Base, so no step needs a wallet outside Monaco, a new key or Base ETH:

```
QA pot ──qa fund──► A (Solana)
A ──EVM withdrawal to B's Base deposit address──► B's Base deposit address
B's deposit address ──Privy forwarding──► B (Solana)          one loop: withdrawal + deposit
B ──fund──► cabal ──buy on EVM, sell, cash out──► B (Solana)
B ──EVM withdrawal to A's Base deposit address──► A (Solana)  the loop again, after the trade
A and B ──withdraw──► QA pot
```

A's EVM withdrawal lands on Base exactly like a USDC transfer from any outside wallet, so B's deposit address sees a real inbound Base deposit and forwards it. Privy runs the bridge and its gas on both sides; whether Monaco sponsors that cost is open question 4 on [#4917](https://github.com/Munyon-Canyon/monaco/issues/4917), and the run records what it was charged.

## Rules

- Every step is a call to Monaco's API, or the app on top of it, by a QA user. No step moves money outside Monaco's API, and no one-off script signs a transaction.
- Fund only from the shared QA pot, with `monacoctl qa fund`, at most 5 USDC per transfer. Never fund from anywhere else. If the pot is too low, stop, and say so on [#4917](https://github.com/Munyon-Canyon/monaco/issues/4917).
- Refund what's left to the QA pot with an in-app withdrawal when the run ends, even when it fails partway.
- Record every chain hash, a screenshot of each app screen named below, the wall-clock time of each step, and the price comparison the proposal showed.

## Before you start

1. Build and start the backend with real providers, on the `.env.local` configuration (Solana mainnet, real Privy, Uniswap and Jupiter): `just build backend`, `just migrate db`, `just run backend`. `EVM_DEPOSIT_NETWORKS` must include `base`.
2. Check the pot holds at least 5 USDC and 0.01 SOL: `scripts/with-dotenv-local.sh bin/monacoctl qa pot`. Note the pot's address for the refund: `scripts/with-dotenv-local.sh bin/monacoctl qa pot --address`.
3. Sign in two QA users, A and B, in the app on the simulator with Privy test logins ([Debug login](debug-login.md)), finish onboarding for each, and note both user ids. Mint a session token for each: `scripts/with-dotenv-local.sh bin/monacoctl dev token --user <user id>`, then `export TOKEN_A=<token>`, `export TOKEN_B=<token>` and `export API=http://localhost:8080`.
4. Read both Base deposit addresses: `GET $API/v1/me/deposit-addresses` as A and as B, and keep each one's `base` item. Calling it creates the EVM deposit wallet and attaches its forwarding, so do it before any money moves.
5. Pick the stock for step 3: a launch listing whose EVM route wins against Solana. Preview a buy of the trade amount and check `price_comparison.winner_listing_id` names an EVM listing. The launch set and its quotes are in `docs/how-to/stock-listings.md`.

Each request below sends `Authorization: Bearer $TOKEN_A` or `$TOKEN_B` as the step says, and every POST also sends `Idempotency-Key: $(uuidgen)`.

## Steps

### 1. Withdraw from A to B's Base deposit address, and see it arrive for B

1. `scripts/with-dotenv-local.sh bin/monacoctl qa fund --user <A's user id> --usdc 5`. Record the Solana hash.
2. As A, `GET $API/v1/me/balance` until `available_micros` shows the 5 USDC.
3. As A, `POST $API/v1/me/withdrawals` with `{"amount_micros": "4000000", "destination_kind": "evm", "network": "base", "to_address": "<B's Base deposit address>"}`. Screenshot Withdraw with Base picked, and the confirm step with its "You'll receive about $X". Record the Privy action id and the Base hash.
4. As A, `GET $API/v1/me/withdrawals/{id}` until `confirmed`.
5. As B, `GET $API/v1/me/balance` until `arriving_micros` shows the amount (screenshot the Arriving row on Home), then until `available_micros` includes it and `arriving_micros` is gone.
6. Record B's forward: its Privy execution id and the Solana arrival signature from `evm_deposit_forwards`, and check B's `user_txns` has exactly one deposit row for it.

### 2. Fund a cabal

1. As B, create a cabal in the app, with B as its only voter.
2. As B, `POST $API/v1/cabals/{id}/fund` with `{"amount_micros": "<B's whole available_micros>"}`. Record the Solana hash.
3. `GET $API/v1/fund-transfers/{id}` until confirmed, and `GET $API/v1/cabals/{id}/pot` shows the USDC.

### 3. Propose and pass a buy where the EVM listing wins

1. As B, `POST $API/v1/cabals/{id}/proposals/preview` with the buy. Save the whole `price_comparison` in the results: every venue, its costs, its net shares, and the winner.
2. As B, `POST $API/v1/cabals/{id}/proposals` with the same buy, then `POST $API/v1/proposals/{id}/votes` with `yes`. Screenshot the proposal with its best-price line.

### 4. Confirm the bridge, swap and holdings

1. `GET $API/v1/swaps/{id}` until it is terminal. Record `route` and each of `steps`: `bridge_out` (Privy action id, Solana hash, EVM hash), the requote, and `swap` (EVM hash).
2. While `bridge_out` runs, `GET $API/v1/cabals/{id}/pot` must show the amount in transit, and the pot total must not dip.
3. After the swap, `/pot` shows one holding of the stock, priced. Screenshot the cabal's holdings and the trade detail with the price comparison.

### 5. Sell

1. Propose and pass a sell of the whole holding, as in step 3.
2. `GET $API/v1/swaps/{id}` until terminal. Record `swap` (EVM hash) and `bridge_back` (Privy action id, EVM hash, Solana hash).
3. `/pot` shows only USDC, back on Solana, and nothing in transit.

### 6. Cash out and withdraw to A's Base deposit address

1. As B, cash out the whole stake: `POST $API/v1/cabals/{id}/cashouts` with `{"all": true}`, then `GET $API/v1/cabals/{id}/cashouts/{job_id}` until done.
2. As B, `POST $API/v1/me/withdrawals` with `{"amount_micros": "1000000", "destination_kind": "evm", "network": "base", "to_address": "<A's Base deposit address>"}`. Record the Privy action id and the Base hash.
3. As A, `GET $API/v1/me/balance` until the forwarded amount is in `available_micros`.

### 7. Refund the rest to the QA pot

1. As A and as B, withdraw the whole remaining balance to the pot's Solana address in the app.
2. `bin/monacoctl qa pot` shows the pot's USDC. Record what the run cost: the pot's USDC before minus after.

## If the loop fails

If Privy won't deliver an EVM withdrawal to a Monaco deposit address, or the forwarding never sees it, record the failure on [#4917](https://github.com/Munyon-Canyon/monaco/issues/4917), then fall back to a **runner-held Base wallet**: a Base address whose key the runner holds, with a little Base ETH for gas. A withdraws to it (step 1.3 with that address), and the runner sends the USDC from it to B's Base deposit address. That wallet and its key are an operator decision, so it is used only when the loop fails.

## Results

Fill in one table per run. Times are wall clock, from the request to the state the step waits for.

**Run:** not yet run. It waits for #4966 to #4973 to land.

| Step | What | Hashes and ids | Wall-clock time | Screenshot |
| --- | --- | --- | --- | --- |
| 1 | A's EVM withdrawal arrives as B's Base deposit | | | |
| 2 | Fund the cabal | | | |
| 3 | Propose and pass the buy | | | |
| 4 | Bridge out, swap, holdings | | | |
| 5 | Sell and bridge back | | | |
| 6 | Cash out, then withdraw to A's Base address | | | |
| 7 | Refund to the QA pot | | | |

**Price comparison shown at step 3:** not yet run.

**Leftover refunded:** not yet run.
