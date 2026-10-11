# Prove an EVM trade

This page shows how to prove a cabal trade routed to an EVM listing end to end with real USDC on mainnet, through Monaco's API, and records the first run. A buy whose best route is on Base, Ethereum or Arbitrum bridges the trade amount from the cabal's Solana treasury to its EVM treasury, takes a fresh quote, swaps through Uniswap, and records the fill. A sale swaps back to the chain's cash and bridges the proceeds home.

## How a routed buy moves

1. **Pass.** The proposal passes, and execution routes again at that moment: market's router compares every listing of the stock, and an EVM listing must beat Solana by `ROUTER_MIN_EDGE_BPS` (10). If Solana wins at execution, the trade runs through Jupiter as before.
2. **Top up SOL.** The relayer gives the cabal's Solana treasury SOL if it holds less than 0.002 SOL, bringing it to 0.004 SOL. Privy's `/transfer` pays the Solana fee from the wallet itself, and gas sponsorship doesn't cover that leg.
3. **`bridge_out`.** A Privy `/transfer` sends USDC from the Solana treasury to the cabal's EVM treasury, under the key `trade:<swap>:bridge_out`. The action id is stored before polling.
4. **Requote.** A fresh quote on the winning listing at the landed amount. The slippage floor comes only from this quote.
5. **`swap`.** The Uniswap Trading API builds the swap, which goes through Privy `eth_sendTransaction` with `sponsor: true` and the key `trade:<swap>:swap`.
   - Monaco writes both approvals itself: `approve(Permit2, AmountIn)` on the input token, then `Permit2.approve(token, router, AmountIn, now+30m)`.
   - The router must be on the network's Universal Router allowlist.
   - It waits for each approval's receipt before sending the swap, and reads the fill from the swap's receipt.
6. **Record.** One transaction confirms the swap and emits `trade.confirmed`, in the network's asset strings. The treasury posts the swap, and capitalizes the bridge fee into the position's cost.

A swap that fails after the bridge leaves the USDC on the EVM treasury as cabal cash. A retry (`POST /v1/swaps/{id}/retry`) routes again at that moment:
- **The EVM route still wins.** The retry reuses the stranded cash, with no second bridge.
- **Solana wins.** It wins only if it still beats the EVM route after paying the bridge back. Then the retry bridges the cash home (`bridge_back`) and buys on Jupiter.

A dead proposal's cash is bridged back.

### Sponsored quotes and hooked pools

Privy sponsors gas through EIP-7702, so every EVM swap runs through the treasury EOA's ERC-7579 `execute`.
- **Two routes per quote.** For a buy, market's Uniswap quoter asks for two routes: the default, and one restricted to hookless pools (`hooksOptions: V4_NO_HOOKS`).
- **Each is simulated.** The quoter simulates each swap through `execute` at 3,000,000 gas, under Base's 16,777,216-gas cap per transaction, and keeps the one that delivers more.
- **The route is fixed for execution.** The chosen hook setting rides on the quote, and execution builds the swap on the same route.

## Before you run it

- Fund from the QA pot only, at most 5 USDC per transfer: `monacoctl qa fund --user <user-uuid> --usdc 5`.
- Run the backend on `.env.local`, clear of other checkouts: `MONACO_HTTP_ADDR=127.0.0.1:18080 MONACO_WORKER_HEALTH_ADDR=127.0.0.1:18081`.
- Locally, start the worker with `LISTINGS_SYNC_ENABLED=true`. The listing sync links Coinbase and Ondo listings to their stocks, and `.env.local` leaves it off. Without it, the router only ever compares Solana listings. Staging and production sync on the worker's first boot tick.
- The EVM gate turns on only when `UNISWAP_API_KEY` is set and market's quoter and the cabal treasury-wallet port are wired.
- If Helius returns 429, use the Alchemy Solana URL for the session.

## Run it

1. Mint a session token for the QA user: `bin/monacoctl dev token --user <uuid>`.
2. Create a one-member cabal (`POST /v1/cabals`), then fund it (`POST /v1/cabals/{id}/fund`).
3. Preview buys at the cabal's size (`GET /v1/cabals/{id}/proposals/preview?kind=buy&symbol=…&usdc_micros=…`). Pick a stock whose `price_comparison` winner is on an EVM network.
4. Propose the buy and vote it through (`POST /v1/cabals/{id}/proposals`, then `POST /v1/proposals/{id}/votes`).
5. Follow `swaps` and `trade_steps` for the proposal: `bridge_out` succeeded, then `swap` succeeded, then the swap `confirmed`.
6. For the sale, propose and pass a sell of the held units, then follow `swap` and `bridge_back`.
7. Return the leftovers to the QA pot.

## First run, 2026-10-11

| Item | Value |
| --- | --- |
| Build | `staging` plus each fix as it landed, local, ports 18080/18081 |
| QA user | `01a12a2b-4368-7806-b70e-20799c1cfb1e`, member wallet `4RpUTNtzuCs9xNvQnygP3dyyhH3NLh1NFhuFXeA8PDKU` |
| Funding from the QA pot | 5 USDC, [`58naozCw…kbKNa`](https://solscan.io/tx/58naozCwabB4bafJfYMYopdAPnckAoPafzMjcpw9WVk6ZVu2VtXMs1t5p2bd8Ph15Y1GX9Nkgn3DYmqu2xkcbKNa) |
| Cabal | `01a12bb3-db75-7933-8c28-05961e2c7ac7`. Solana treasury `6HUUQBkqjMRGWuqnp1t6wB8iHRKhY15pcTAq47ZX8ELx`, Base treasury `0xf15262a7ee9d20fa690185d58e0eb2aad475bf32` |
| Cabal funding | 5 USDC, transfer `01a12bb8-7d02-7530-913f-ebe3bd109c62`, [`5LoxsQtj…ehcyfy`](https://solscan.io/tx/5LoxsQtjN34gRLP2zyBGSevpXohRqnhBsqPrxtjHLmp9pXUG2o37zYSxfPY6xhtKjfJBEE6eD1LejGZp9Eehcyfy) |

### First buy: rejected before money moved

An AAPLx buy routed to Coinbase on Base. Privy rejected its `bridge_out` (action `dd2e79a7-5ffd-4de3-b7a0-9d5d6eb0dbd1`): "Transaction simulation failed: AccountNotFound". The Solana treasury had no SOL, and `/transfer` pays the fee from the wallet itself. The swap failed as `never_submitted`, and the ledger posted the bridge's submit and then its return. That's why the relayer now tops up SOL before every Solana-source transfer (#5233).

### Execution-time re-route: NVDA ran on Solana

At vote time NVDAx previewed Coinbase on Base, about 13 bps ahead. At execution the router picked Solana xStocks, so the buy ran through Jupiter: 5 USDC for 2,157,744 NVDAx base units, [`4LzPYuWa…jL2b`](https://solscan.io/tx/4LzPYuWaUprqkPkySxL7SHz1Q8X539YZTdec1fce9bksQ2yd5Dw4ZDTCtZvDFpRptBmoQkvFpQn1KJ3ZUH26jL2b). It was sold back for 4.995555 USDC, [`3FB3JdZe…W872r`](https://solscan.io/tx/3FB3JdZeXpr9VcLyMVPnmZStfTrYfNsgdyqSkeepU3mcskg7w5k3MSBTNgKQo47JdRs8uRLUhii2fmGsueSW872r).

A scan of the 87 stocks with a linked Coinbase listing, at 4.995 USDC, found these Base leads over the best Solana listing: GOOGLx +371 bps, MSTRx +287, NVDAx +1.6, AAPLx −6, and every other stock negative.

### GOOGLx on Base: bridged, blocked by a hooked pool, then bought on Solana

| Step | Result |
| --- | --- |
| Proposal | `01a12bd9-2e4f-7ea2-93d4-cd7e6ff38ae6`, buy 4.9 USDC, winner Coinbase GOOGL on Base (`0xb2000000000000000000002d0ba3164cc74f58b7`) |
| SOL top-up | 0.004 SOL from the relayer, [`23fEPf47…EPvB4Wd`](https://solscan.io/tx/23fEPf47G4YtcMefxFDQVrYCN59Qv9HvXXTZH6bUZ97c6jJxgjaMBpSedL8zZ6mn3HyTJconqVroXJvBHEPvB4Wd) |
| `bridge_out` | Privy action `a9f6da13-f4b6-414e-a106-1e050a5f44cd`, `succeeded`. 4.9 USDC sent, 4.896325 landed, Privy fee 0.003674. Solana [`33o6zdsn…LswHsK5`](https://solscan.io/tx/33o6zdsnBsq7qu2t4EpTF4HVvoMTiHNUAxqwN1BsnWDiUvm9tBMdNeQtZi4n8uGbLRz2UmW8nTnv9Neb1LswHsK5), Base arrival [`0x362c5487…c7ea`](https://basescan.org/tx/0x362c54877d64b681582bf8848fc6889961da2215df70581869e0e7f6e5b5c7ea) |
| Ledger | Solana USDC −4.9 into transit, then Base cash +4,896,325 and fee 3,675 recorded |
| First swap | Refused before sending, `decode_failed`. Uniswap's `check_approval` returned an unlimited approval, which the guard refuses. Fixed by building our own exact approvals |
| Retries | `POST /v1/swaps/{id}/retry` four times. Each reused the stranded Base cash (`bridge_out` marked `reused:<swap>`), with no second bridge |
| Approvals | Mined, both exactly 4,896,325: USDC→Permit2, and Permit2→Universal Router 2.1.2 with a 30-minute expiry. USDC approval tx [`0x78153e37…227c`](https://basescan.org/tx/0x78153e372d497b9d6f4e46b38709e06e82e110835b8ad5b32660b61e67b6227c) |
| Swap | Refused by Privy: `400 transaction_broadcast_failure: Execution reverted` |

**Why the swap reverted.** It wasn't the 7702 wrapper and it wasn't Privy.
- **The pool.** Uniswap's default route for GOOGLc runs through a Coinbase B20 v4 hooked pool. The hook skips a fee of about 9% only when `gasleft()` is above `1.01 × min(block.gaslimit, 2^24)` = 16,944,988.
- **Base's cap.** Base limits every transaction to 16,777,216 gas (EIP-7825, live since Azul), so no real swap gets the quoted price, sponsored or direct.
- **Why it passed before.** The direct `eth_call` that seemed to succeed ran with the node's default gas, above the threshold. At 2^24 gas it reverts the same way, with `V4TooLittleReceived(min 1,412,187, received 1,317,936)`.
- **The hookless routes.**

  | Route | Received for 4,896,325 USDC |
  | --- | --- |
  | v3 USDC/GOOGLc 0.3% | 1,388,248 |
  | v4 without hooks | 1,380,016 |

  Both succeed through `execute` at 500k gas and at 16.7M gas.

The fix quotes both routes and simulates each at real gas (#5268, #5277). The correction is on #4917.

### The retry on the fixed build

Build: `staging` plus #5268, #5269 and #5277, local. The retry routed again with 4,896,325 USDC stranded on Base. It priced every listing faithfully:

| Listing | Net GOOGL shares (base units) | Costs (micro-USDC) |
| --- | --- | --- |
| Solana, xStocks (winner) | 13,901,418 | price impact 6,707 |
| Base, Coinbase, hookless route | 13,870,252 | bridge 3,672, pool fee 14,678, gas 2,485 |
| Solana, Ondo | 13,858,636 | price impact 21,755 |

Base no longer wins once the stranded cash has to pay the bridge, so the retry bridged it back and bought on Jupiter:

| Step | Result |
| --- | --- |
| `bridge_back` | Privy action `7b7a27ea-9a09-4d56-bae7-4b50ebf955ef`, 4,896,325 USDC sent. Base [`0x4796cc66…fec0`](https://basescan.org/tx/0x4796cc66185f40c6fc968251e88ad2e1ccb53a530d01d1403c6e77af6782fec0), Solana arrival [`q9RCbfVz…nUFC`](https://solscan.io/tx/q9RCbfVzPztWo6wPUVKiPWR8FR7vcHVF5pc1sBnBhU8aCHD6iwKSbHsk4z8xuhYeXhe96vUFyqpmXYXcKRZnUFC), 4,892,653 landed |
| Buy | Swap `01a12c42-fe21-78a2-82a7-fc42111870cd`: 4,892,650 USDC for 1,385,815 GOOGLx, [`WQZTvKka…f17UL`](https://solscan.io/tx/WQZTvKkaAN2Ba9zPADr2j3o85SaAdSavSkSXMrQy5NVsrC4NoczXBeRj65VNxSKRsBvr9roaNGXEweF1b6f17UL) |
| Sale | Swap `01a12c44-ec03-7f1f-bbcf-de1538cfde54`: 1,385,815 GOOGLx for 4,880,982 USDC on chain, [`2ZbDT86o…oGSob`](https://solscan.io/tx/2ZbDT86oYj6ZPHLnSK4fjziosuDxbjh9mhbXy6wLo4nMyGB1rAei1xWAA6WdRsG8VTqaW9eaoHXxcZq6HpyoGSob) |

### Returning the money

- **The shortfall.** The full cash-out failed on chain with "insufficient funds": the cabal's ledger said 4,981,422 USDC, but the treasury held 4,971,545.
  - Both Jupiter sales recorded Jupiter's `outputAmountResult`, which is 10 bps above what reached the treasury: NVDAx 4,995,555 recorded against 4,990,560 received, GOOGLx 4,885,867 against 4,880,982.
  - #5279 fixes the recorded fill.
  - The backend logged the rejection only as `rpc_unavailable`.
- **The return.**
  - A cash-out of 4,870,000 to the member settled.
  - An in-app withdrawal sent it to the QA pot: withdrawal `01a12c4e-563c-798d-b2f5-f2a29f287713`, [`62HKVC6g…xDHSk`](https://solscan.io/tx/62HKVC6gKCTnyNYpokzT9QteUN8uBJQRRJGfU51JuEscsnU4x9j2vdPKej9xhqVFhyaTnKaQ8nVp1Yfd7QcxDHSk).
  - About 0.10 USDC stays in the QA cabal until #5279 corrects its ledger.

## Not yet proven on chain

This run proved several steps on mainnet:
- the bridge out and the SOL top-up;
- exact approvals through Privy sponsorship;
- reusing stranded cash on retry;
- execution-time routing with real-gas quotes;
- the bridge back.

It hasn't yet filled a swap on an EVM pool, or sold an EVM holding and bridged the proceeds home. At this size, no stock's EVM route beat Solana once real gas was priced. The next time a preview's winner is on an EVM network, run the buy and sale above, and add the swap and `bridge_back` hashes here.

## What the run confirmed about the Uniswap Trading API

- `aggregatedOutputs[].minAmount` for the swapper is the swap's minimum output. The recipient comes back EIP-55 mixed case.
- `slippageTolerance` as a JSON number in percent is accepted.
- `check_approval` returns `approve(Permit2, MaxUint256)`. Monaco never sends it, and writes its own exact approval instead.
- With `generatePermitAsTransaction: true`, the quote returns a Permit2 `approve(token, router, MaxUint160, ~30 days)` transaction. Monaco takes only the router from it, and writes its own approval for the swap amount, expiring in 30 minutes.
