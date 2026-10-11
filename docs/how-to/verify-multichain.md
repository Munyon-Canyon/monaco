# Verify multichain money end to end

This runbook proves that one person's money goes all the way through Monaco across chains, with real USDC on mainnet: in from Base, into a cabal, a buy routed to an EVM chain at the best price, a sell, and out to a Base wallet. [Flow 30](../architecture/backend-platform.md#flows) proves the same path against fakes. This run proves it against Privy, Uniswap, Jupiter and the chains. The design is in [Deposits from an EVM chain](../architecture/deposits-withdrawals.md#deposits-from-an-evm-chain), [Routing across chains](../architecture/trade-execution.md#routing-across-chains) and [Withdraw to an EVM wallet](../architecture/deposits-withdrawals.md#withdraw-to-an-evm-wallet).

Run it once after every M24 ticket (#4966 to #4973) has landed on `staging`, and again whenever a change touches the multichain money path. Record the run in [Results](#results).

## Rules

- Every step is a call to Monaco's API, or the app on top of it, by a QA user. No step moves money outside Monaco's API, and no one-off script signs a transaction.
- Fund only from the shared QA pot, with `monacoctl qa fund`, at most 5 USDC per transfer. Never fund from anywhere else. If the pot is too low, stop, and say so on [#4917](https://github.com/Munyon-Canyon/monaco/issues/4917).
- Refund what's left to the QA pot with an in-app withdrawal when the run ends, even when it fails partway.
- Record every chain hash, a screenshot of each app screen named below, the wall-clock time of each step, and the price comparison the proposal showed.

## Before you start

1. Build and start the backend with real providers, on the `.env.local` configuration (Solana mainnet, real Privy, Uniswap and Jupiter): `just build backend`, `just migrate db`, `just run backend`. `EVM_DEPOSIT_NETWORKS` must include `base`.
2. Check the pot holds at least 3 USDC and 0.01 SOL: `scripts/with-dotenv-local.sh bin/monacoctl qa pot`. Note the pot's address for the refund: `scripts/with-dotenv-local.sh bin/monacoctl qa pot --address`.
3. Sign in a QA user in the app on the simulator with a Privy test login ([Debug login](debug-login.md)), finish onboarding, and note the user id. Mint a session token for the API calls: `scripts/with-dotenv-local.sh bin/monacoctl dev token --user <user id>`, then `export TOKEN=<token>` and `export API=http://localhost:8080`.
4. Have the **QA Base wallet**: a Base address whose key the runner holds, to send the deposit from and receive the withdrawal. The QA pot is on Solana and every Monaco EVM wallet is app-owned and sends only inside a flow, so neither can send a deposit on Base. Step 0 puts USDC on the QA Base wallet through Monaco's own EVM withdrawal, so no money comes from outside the pot. The wallet also needs a little Base ETH for the deposit's gas. Which wallet this is is open on [#4917](https://github.com/Munyon-Canyon/monaco/issues/4917) until the first run records it here.
5. Pick the stock for step 3: a launch listing whose EVM route wins against Solana. Preview a buy of the trade amount and check `price_comparison.winner_listing_id` names an EVM listing. The launch set and its quotes are in `docs/how-to/stock-listings.md`.

Each request below sends `Authorization: Bearer $TOKEN`, and every POST also sends `Idempotency-Key: $(uuidgen)`.

## Steps

### 0. Get USDC onto Base through Monaco

1. `scripts/with-dotenv-local.sh bin/monacoctl qa fund --user <user id> --usdc 5`. Record the Solana hash.
2. `GET $API/v1/me/balance` until `available_micros` shows the 5 USDC.
3. `POST $API/v1/me/withdrawals` with `{"amount_micros": "3000000", "destination_kind": "evm", "network": "base", "to_address": "<the QA Base wallet>"}`. Record the preview's "you'll receive" amount, the Privy action id and the Base hash.
4. `GET $API/v1/me/withdrawals/{id}` until `confirmed`.

This step also proves an EVM withdrawal once before the measured run.

### 1. Deposit 3 USDC on Base

1. `GET $API/v1/me/deposit-addresses`. The `base` item's address is the user's EVM deposit wallet. Screenshot Add money with the Base network picked.
2. From the QA Base wallet, send the USDC it holds (about 3 USDC) to that address. Record the Base hash.
3. `GET $API/v1/me/balance` until `arriving_micros` shows the amount (screenshot the Arriving row), then until `available_micros` includes it and `arriving_micros` is gone.
4. Record the forward's Privy execution id and the Solana arrival signature from `evm_deposit_forwards`, and check the `user_txns` deposit row was written once.

### 2. Fund a cabal

1. Create a cabal in the app, or use an empty QA cabal the user is the only voter of.
2. `POST $API/v1/cabals/{id}/fund` with `{"amount_micros": "<the whole available_micros>"}`. Record the Solana hash.
3. `GET $API/v1/fund-transfers/{id}` until confirmed, and `GET $API/v1/cabals/{id}/pot` shows the USDC.

### 3. Propose and pass a buy where the EVM listing wins

1. `POST $API/v1/cabals/{id}/proposals/preview` with the buy. Save the whole `price_comparison` in the results: every venue, its costs, its net shares, and the winner.
2. `POST $API/v1/cabals/{id}/proposals` with the same buy, then `POST $API/v1/proposals/{id}/votes` with `yes`. Screenshot the proposal with its best-price line.

### 4. Confirm the bridge, swap and holdings

1. `GET $API/v1/swaps/{id}` until it is terminal. Record `route` and each of `steps`: `bridge_out` (Privy action id, Solana hash, Base hash), the requote, and `swap` (Base hash).
2. While `bridge_out` runs, `GET $API/v1/cabals/{id}/pot` must show the amount in transit and the pot total must not dip.
3. After the swap, `/pot` shows one holding of the stock, priced. Screenshot the cabal's holdings and the trade detail with the price comparison.

### 5. Sell

1. Propose and pass a sell of the whole holding, as in step 3.
2. `GET $API/v1/swaps/{id}` until terminal. Record `swap` (Base hash) and `bridge_back` (Privy action id, Base hash, Solana hash).
3. `/pot` shows only USDC, back on Solana, and nothing in transit.

### 6. Withdraw to Base

1. Cash out the whole stake: `POST $API/v1/cabals/{id}/cashouts` with `{"all": true}`, then `GET $API/v1/cabals/{id}/cashouts/{job_id}` until done.
2. `POST $API/v1/me/withdrawals` to the QA Base wallet, as in step 0, for 1 USDC. Record the Privy action id and the Base hash. Screenshot Withdraw with the Base network picked, and the confirm step.

### 7. Refund the rest to the QA pot

1. Withdraw the whole remaining balance to the pot's Solana address in the app.
2. Send the Base wallet's USDC back through Monaco: deposit it to the user's Base deposit address (as in step 1), then withdraw it to the pot.
3. `bin/monacoctl qa pot` shows the pot's USDC. Record what the run cost: the pot's USDC before minus after.

## Results

Fill in one table per run. Times are wall clock, from the request to the state the step waits for.

**Run:** not yet run. It waits for #4966 to #4973 to land.

| Step | What | Hashes and ids | Wall-clock time | Screenshot |
| --- | --- | --- | --- | --- |
| 0 | Solana → Base withdrawal | | | |
| 1 | Base deposit credited | | | |
| 2 | Fund the cabal | | | |
| 3 | Propose and pass the buy | | | |
| 4 | Bridge out, swap, holdings | | | |
| 5 | Sell and bridge back | | | |
| 6 | Cash out and withdraw to Base | | | |
| 7 | Refund to the QA pot | | | |

**Price comparison shown at step 3:** not yet run.

**Leftover refunded:** not yet run.
