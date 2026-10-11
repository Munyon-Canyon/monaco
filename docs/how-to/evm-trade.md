# EVM trades

This page says how a cabal trade reaches a stock listed on an EVM chain, which cash token each chain uses, and what the bridge costs. The design is in [Trade execution](../architecture/trade-execution.md). The listings and their verdicts are in [Stock listings](stock-listings.md).

## Cash on each chain

Every cabal keeps its idle cash as USDC in its Solana treasury. A trade on an EVM chain bridges exactly the trade amount to the cabal's EVM treasury, in that chain's cash token, and swaps there. `chain.Network.Cash()` names the token, and the Privy transfer, the router's bridge quote, the Uniswap quote and the treasury's EVM cash asset all read it.

| Network | Cash token | Address | Privy asset |
| --- | --- | --- | --- |
| Base | USDC | `0x833589fcd6edb6e08f4c7c32d4f71b54bda02913` | `usdc` |
| Arbitrum | USDC | `0xaf88d065e77c8cc2239327c5edb3a432268e5831` | `usdc` |
| Ethereum | USDC | `0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48` | `usdc` |
| Robinhood Chain | USDG | `0x5fc5360d0400a0fd4f2af552add042d716f1d168` | `usdg` |

All four tokens have 6 decimals, so amounts stay in micros on both sides of the bridge.

## A buy

1. The router quotes every enabled listing of the stock. For an EVM listing it first asks Privy what the bridge delivers: `POST /v1/deposit_accounts/crypto/quote` from `usdc` on `solana` to the listing network's cash asset. It quotes Uniswap with that amount, and the difference is the listing's `bridge` cost.
2. If an EVM listing wins, `bridge_out` sends the amount from the Solana treasury with Privy `POST /v1/wallets/{id}/transfer`, source `usdc` on `solana`, destination the network's cash asset at the cabal's EVM treasury address. On Robinhood Chain the treasury receives USDG.
3. The trade requotes on the winning chain and swaps the cash token into the stock.

## A sell

The position swaps to the chain's cash token, then `bridge_back` transfers it with the network's cash asset as the source (`usdg` on `robinhood`) and `usdc` on `solana` as the destination.

Privy refuses `usdc` on Robinhood Chain (`Asset "usdc" is not available on chain "eip155:4663"`), so a Robinhood leg must name `usdg`.

EVM deposit forwarding is separate and stays USDC only: a user's Base, Arbitrum or Ethereum deposit address forwards USDC to Solana, and Robinhood Chain has no deposit address.

## Measured costs

Privy quotes from 2026-10-11 (#4917):

| Route | In | Out | Cost |
| --- | --- | --- | --- |
| Solana USDC → Base USDC | 10 | 9.9925 | 7.5 bps |
| Base USDC → Solana USDC | 10 | 9.9925 | 7.5 bps |
| Solana USDC → Robinhood USDG | 5 | 4.968995 | 62 bps |
| Robinhood USDG → Solana USDC | 5 | 4.973276 | 53 bps |
| Solana USDC → Robinhood USDG, wallet transfer quote | 1 | 0.972604 | $0.026498 Relay fee, $0.00075 Privy fee, 0.0002 SOL gas |

A USDC to USDG bridge runs on Relay and pays Relay's fee of about $0.02 to $0.03 plus a small percentage, so it costs more than a USDC to USDC bridge at small sizes. The router counts it as the listing's `bridge` cost, and a Robinhood listing wins only when it still beats the Solana listing by `ROUTER_MIN_EDGE_BPS`.

Uniswap on Robinhood Chain (chain 4663) quoted 5 USDG into AAPL at 0.03% impact, NVDA at 0.03% and TSLA at 0.07%, each with about $0.01 of gas.
