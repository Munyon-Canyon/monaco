# Stock listings

A stock such as Apple can trade as several tokens: xStocks on Solana, Ethereum and Arbitrum, Ondo Global Markets on Solana and Ethereum, Coinbase B20 on Base, and Robinhood stock tokens on Robinhood Chain. Monaco calls each token a *listing*. The `asset_listings` table holds every listing, and the router quotes the enabled ones of a stock on every trade (#4969).

This page records the launch listings, the real quotes behind each verdict, and how to check a listing again.

## Where listings come from

| Issuer | Source | Networks kept | Share conversion |
| --- | --- | --- | --- |
| xStocks | `GET https://api.xstocks.fi/api/v2/public/assets?page=N`, every `deployments[]` entry | Solana, Ethereum, Arbitrum | `assets.ui_multiplier_num / ui_multiplier_den` |
| Ondo | `GET https://app.ondo.finance/api/v2/assets`, then `supportedNetworks[]` and `sharesMultiplier` from `https://app.ondo.finance/assets/<symbol>` | Solana, Ethereum | `sharesMultiplier` |
| Coinbase | `GET https://api.coinbase.com/v1/tokenized-stocks` | Base | `multiplier` |
| Robinhood | `GET https://api.robinhood.com/rhj/assets` | Robinhood Chain | `currentMultiplier`, then `pendingMultiplier` at its effective time |

The sync runs inside the hourly catalog run when `LISTINGS_SYNC_ENABLED` is on (the default in staging and production, off locally and in tests, so no test calls an issuer). Each run reads every issuer, upserts its listings into `asset_listings` and refreshes `shares_per_token`. Robinhood's `pendingMultiplier` replaces `currentMultiplier` only after its effective time. The Robinhood adapter doesn't read an effective time yet, so `currentMultiplier` applies. One issuer failing doesn't stop the others. When Ondo's asset page has no `supportedNetworks` block, the run disables that asset's listings and logs `market.listings.source_missing` once, on the run that disables them.

A listing is enabled only after on-chain verification passes:

- An EVM token answers `symbol()` with the issuer's symbol (an xStocks wrapper answers `w<symbol>`) and `decimals()`. The run stores the on-chain decimals.
- For a Coinbase B20 token on Base, the check reads the policy ID of the transfer sender, receiver and executor scopes with `policyId(bytes32)`. The policy registry at `0x8453000000000000000000000000000000000002` must answer `isAuthorized(policyId, 0x1111111111111111111111111111111111111111)` with true for each one. The policy is global, so a fresh address stands in for any app-owned treasury, and an allowlist fails verification.
- A Solana mint exists and is owned by SPL Token or Token-2022. The run stores the mint's decimals.

Each run verifies the listings that are not verified yet. A listing that fails stays disabled and logs `market.listings.verify_failed` with the reason, and the next run tries it again.

Run the check on demand from `apps/backend`. Without `--all` it checks the unverified listings. With `--all` it checks every listing and disables any listing that no longer passes:

    go run ./cmd/monacoctl listings verify [--all]

Turn a listing on or off by hand, or return it to the issuer's flag, with its `asset_listings.id`. A listing that is not verified stays disabled whatever the override says:

    go run ./cmd/monacoctl listings override <listing-id> on|off|auto

### Listings that start off

Two kinds of listing start off, whatever the issuer says (#5186):

- xStocks listings on any network other than Solana. None of them has a usable route (see [Venues and pools](#venues-and-pools)).
- Every listing on Robinhood Chain. The chain's cash is USDG, and Privy doesn't bridge USDC to it.

The sync inserts these rows with `tradable_override = false`, so they stay disabled after they verify. A later sync never changes an override. To turn one on, find its `asset_listings.id` and set the override:

    go run ./cmd/monacoctl listings override <listing-id> on

To turn it off again, use `off`, not `auto`: `auto` follows the issuer's flag, which is usually on.

## Launch listings

Quotes were taken on Sunday 2026-10-11 at about 08:50 UTC, with US markets closed. Solana quotes come from Jupiter (`/swap/v2/order`). EVM quotes come from the Uniswap Trading API (`POST /v1/quote`, `routingPreference: BEST_PRICE`, protocols V2, V3 and V4). Shares are tokens times the share conversion. The price per share is the dollars spent divided by the shares received.

### Apple

| Listing | Address | Shares per token | $100 buys | $1,000 buys | Verdict |
| --- | --- | --- | --- | --- | --- |
| AAPLx, Solana | `XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp` | 1.003269 | 0.297076 sh ($336.61) | 2.970498 sh ($336.64) | Enabled |
| AAPLon, Solana | `123mYEnRLM2LLYsJW3K6oyYh8uP1fngj732iG638ondo` | 1.003376 | 0.296447 sh ($337.33) | 2.964444 sh ($337.33) | Enabled |
| AAPLon, Ethereum | `0x14c3abf95cb9c93a8b82c1cdcb76d72cb87b2d4c` | 1.003376 | 0.295516 sh ($338.39) | 2.951654 sh ($338.79) | Enabled |
| AAPLc, Base | `0xb200000000000000000000c2e324d24d7eecd1fb` | 1.000000 | 0.306906 sh ($325.83) | 3.066911 sh ($326.06) | Enabled |
| AAPLx, Ethereum | `0x9d275685dc284c8eb1c79f6aba7a63dc75ec890a` (wrapper `0x943bf64d566c32a2bcd41ac92fb63c111cc9de8f`) | 1.003269 | no route | no route | Disabled |
| AAPLx, Arbitrum | same token and wrapper | 1.003269 | no route | no route | Disabled |
| AAPL, Robinhood Chain | `0xaf3d76f1834a1d425780943c99ea8a608f8a93f9` | 1.000566 | not quoted | not quoted | Disabled |

### Tesla

| Listing | Address | Shares per token | $100 buys | $1,000 buys | Verdict |
| --- | --- | --- | --- | --- | --- |
| TSLAx, Solana | `XsDoVfqeBukxuZHWhdvWHBhgEHjGNst4MLodqsJHzoB` | 1 | 0.261182 sh ($382.87) | 2.611195 sh ($382.97) | Enabled |
| TSLAon, Solana | `KeGv7bsfR4MheC1CkmnAVceoApjrkvBhHYjWb67ondo` | 1 | 0.260506 sh ($383.87) | 2.605031 sh ($383.87) | Enabled |
| TSLAon, Ethereum | `0xf6b1117ec07684d3958cad8beb1b302bfd21103f` | 1 | 0.259253 sh ($385.72) | 2.580715 sh ($387.49) | Enabled |
| TSLAc, Base | `0xb2000000000000000000001e800a7f5189430cd0` | 1 | 0.256319 sh ($390.14) | 1.068954 sh ($935.49) | Enabled; the impact ceiling drops it above about $100 |
| TSLAx, Ethereum | `0x8ad3c73f833d3f9a523ab01476625f269aeb7cf0` | 1 | 0.165542 sh ($604.08) | 0.220309 sh ($4,539.08) | Disabled: thin pool |
| TSLAx wrapper, Ethereum, and both on Arbitrum | wrapper `0xc3fdbe3a68ee5de461d30415a8165cf9aefe1171` | 1 | no route | no route | Disabled |
| TSLA, Robinhood Chain | `0x322f0929c4625ed5bad873c95208d54e1c003b2d` | 1 | not quoted | not quoted | Disabled |

### Nvidia

| Listing | Address | Shares per token | $100 buys | $1,000 buys | Verdict |
| --- | --- | --- | --- | --- | --- |
| NVDAx, Solana | `Xsc9qvGR1efVDFGLrVsmkzv3qi45LTBjeUKSPmx9qEh` | 1.001701 | 0.432766 sh ($231.07) | 4.327642 sh ($231.07) | Enabled |
| NVDAon, Solana | `gEGtLTPNQ7jcg25zTetkbmF7teoDLcrfTnQfmn2ondo` | 1.001715 | 0.432274 sh ($231.33) | 4.322715 sh ($231.34) | Enabled |
| NVDAon, Ethereum | `0x2d1f7226bd1f780af6b9a49dcc0ae00e8df4bdee` | 1.001715 | 0.433577 sh ($230.64) | 4.335343 sh ($230.66) | Enabled |
| NVDAc, Base | `0xb20000000000000000000078ee7ce2fe4908108c` | 1.000538 | 0.446809 sh ($223.81) | 4.465429 sh ($223.94) | Enabled |
| NVDAx, Ethereum | `0xc845b2894dbddd03858fd2d643b4ef725fe0849d` | 1.001701 | 0.000297 sh | 0.000298 sh | Disabled: no liquidity |
| NVDAx wrapper, Ethereum, and both on Arbitrum | wrapper `0xa8ddb5cd96b5222afe198316e9a57caa642850d5` | 1.001701 | no route | no route | Disabled |
| NVDA, Robinhood Chain | `0xd0601ce157db5bdc3162bbac2a2c8af5320d9eec` | 1.000776 | not quoted | not quoted | Disabled |

## Venues and pools

- **Ondo on Ethereum:** single Uniswap v3 pools against USDC. AAPLon uses the 1% tier (`0xad82c9eb065a5cfed71db087e4a52c8a09c69921`), TSLAon the 1% and 0.3% tiers, and NVDAon the 0.3% tier (`0xc9b4bc7c15423fcfe6b75ab5118ba773945a3a46`). Uniswap reports about 1% price impact for Apple and Tesla and 0.3% for Nvidia, mostly the pool fee.
- **Coinbase B20 on Base:** Uniswap routes these tokens itself, so no aggregator such as Kyber is needed. Apple and Nvidia route USDC to WETH (v3, 0.01%) to `mmETH` to the stock through v4 pools whose hooks quote the price. Tesla routes through a v4 USDC pool with a hook (0.2% fee). Uniswap reports `priceImpact: 100` for the hooked routes, which is not a real impact, so the router measures impact against Monaco's own reference price instead.
- **Coinbase quotes below the reference price:** the Base quotes for Apple and Nvidia come in about 3% under the last Chainlink total-return feed price ($336.84 for Apple, $230.26 for Nvidia, both from Friday 2026-10-09). The hooks may quote stale or indicative prices while the market is closed. #4970's first EVM execution confirms whether these quotes fill.
- **xStocks on EVM:** neither the token nor its wrapper has a Uniswap route on Arbitrum. On Ethereum only the raw TSLAx and NVDAx tokens have pools, and those are too thin to use. Every xStocks EVM listing stays disabled at launch, by override (see [Listings that start off](#listings-that-start-off)).
- **Robinhood Chain:** the dataset's addresses are unverified (no `decimals`), and the chain's cash asset is USDG, which Privy does not bridge as `usdc`. These listings stay disabled by override, even once they verify.

## Hold and swap eligibility

An app-owned treasury wallet must be able to hold and swap a token with no allowlist.

- **Coinbase B20:** each token stores a policy ID per scope, and the B20 policy registry at `0x8453000000000000000000000000000000000002` answers `isAuthorized(uint64,address)`. For AAPLc, TSLAc and NVDAc, the transfer sender, receiver and executor scopes all use policy 5, which authorizes a fresh address. Catalog verification asks the registry about the fixed probe address `0x1111111111111111111111111111111111111111` for each scope, and a B20 listing whose policy is an allowlist stays disabled.
- **Ondo:** a simulated `transfer` of each token from its Uniswap pool to a fresh address succeeds on Ethereum. Ondo pauses trading outside market hours through `isTradingPaused`, which the catalog reads.
- **xStocks:** not checked, because no EVM listing is enabled.

`monacoctl listings override` applies per listing.
