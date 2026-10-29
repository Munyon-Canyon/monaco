# Product

What Monaco does and the rules it follows, from the user's side. For how it is built, see [architecture.md](architecture.md).

## What Monaco is

Monaco lets friends run a small hedge fund together. A group, called a **cabal**, pools USDC into one shared pot. Members propose trades in tokenized US stocks, vote on them, and passed trades execute for the whole pot. Each member owns a share of the pot, so when the pot gains, every member's stake gains with it.

The loop is: join, put money in, decide, watch the pot, decide again. Leaderboards inside each cabal and across the app are what bring people back.

## Words

| Word | Meaning |
| --- | --- |
| **Cabal** | A group with one shared pot. Go types, tables and new API routes use `cabal`. Only the old backend's `/v1/groups` routes still say group, until the cutover. |
| **Account balance** | USDC in the user's own wallet, not yet in any cabal. |
| **Deposit** | Sending USDC into your account balance from outside Monaco. |
| **Fund** | Moving USDC from your account balance into a cabal's pot. This is what buys you shares. |
| **Pot** | Everything a cabal owns: its USDC plus its stock holdings. It sits in the cabal's treasury wallet. |
| **Share units** | Your claim on a fraction of the pot. Their value moves with the pot. |
| **Proposal** | A request the cabal votes on: buy or sell. |
| **Voter set** | The members allowed to propose and vote. Either everyone or a named list. |
| **Cash out** | Selling some or all of your share units back to the cabal for USDC in your account balance. |
| **Withdraw** | Sending USDC from your account balance to an outside address on Solana, Base, Arbitrum or Ethereum. |

User-facing copy says "cabal", "pot value", "your slice" and "gain or loss". It never says "NAV", "P&L", "wallet", "gas", "mint" or "xStock", and it never shows a raw token address.

## Cabals

Anyone can create a cabal and invite people. One person can be in many cabals: a friends pot, a work pot, a pot built around one idea.

The creator sets the rules at creation:

- **Join mode.** Open (anyone can join at once) or by request (the creator approves each request). New cabals start Open; the creator can pick by request at creation or switch later. Existing cabals keep their policy. A denied requester sees that their request was rejected.
- **Voter set.** Every member, or a named list (at least one person, which may be only the creator).
- **Threshold.** Majority of the voter set, or unanimous. It is frozen on each proposal when it opens, so a later rules change does not move an open vote.
- **Whose vote counts.** The voter set is whose vote counts. There is no separate list of required approvers. Every yes from a voter counts toward passing, and people outside the voter set cannot vote.
- **Vote expiry.** How long a proposal stays open. A proposal that has not passed by then dies and nothing trades.

Rules for membership:

- Only voter-set members can propose and vote.
- The creator cannot leave while other members remain.
- A member cannot leave while they still hold share units, unless they cash out as they leave. The last member cannot leave while the pot holds money.

Each cabal also has a chat, and each proposal has a comment thread.

## Money in

Money reaches a cabal in two steps, on purpose:

1. **Deposit.** Card or Apple Pay needs no address. To send from another app or an exchange, the user picks a network and sees one deposit address for it, with a copy button:
    - **Solana:** the user's own deposit address (internally, their Solana member wallet). USDC sent there is account balance a few seconds after it lands.
    - **Base, Arbitrum or Ethereum:** one address, the same on all three. Monaco moves USDC sent there to Solana, and it shows as **Arriving** until it lands, then as account balance.

    Every balance lives on Solana; the other networks are only ways in. Only USDC counts, and only on the networks listed. There is no amount to type in the app.
2. **Fund.** The user picks a cabal and an amount up to their account balance. Monaco moves exactly that amount into the cabal's pot and credits share units at today's share price once the transfer confirms.

USDC sitting in the account balance is never swept into a cabal on its own, and never earns shares.

## Shares and pot value

You own a fraction of the pot, not a fixed number of dollars.

- **Pot value** = the pot's USDC plus each stock holding at its current price. Example: $40 USDC + 0.1 AAPLx worth $60 = $100.
- **Share price** = pot value ÷ total share units. In an empty cabal the first dollar buys one share at $1.
- **Funding** buys shares at today's price: `shares = USDC in ÷ share price`.
- **Your stake** = `your shares ÷ total shares × pot value`.
- **Cashing out** returns shares for that fraction of the pot in USDC.

**Why shares and not dollars.** Alex puts in $100 and the cabal buys Apple. Apple rises 10%, so the pot is $110. If Blair then put in $110 and was simply "owed $110", she would own half of a pot that includes Alex's gain. With shares, her $110 buys at $1.10 a share, so she gets the same 100 shares Alex has and does not take his gain.

Worked through (ignoring swap fees):

1. Empty cabal. Share price $1.
2. Alex funds $100 and gets 100 shares. Pot $100.
3. The cabal buys AAPLx with the $100.
4. AAPLx rises 10%. Pot $110. Alex's 100 shares are worth $110. Share price $1.10.
5. Blair funds $110 and gets `110 ÷ 1.10 = 100` shares. Pot $220, 200 shares. Each owns half.
6. Blair cashes out 50 shares: `50 ÷ 200` of the pot = $55. She keeps 50 shares.

Fine print, all enforced in code:

- Shares and payouts round down, in favour of the pot.
- Minting shares or paying out needs a live price for every holding. Without one, a fund waits and retries, and a cash out fails with the shares returned. Screens may show the purchase price instead so they still render.
- USDC that lands in a treasury without a matching fund is not anyone's gain. It is returned to the sender, and the cabal's trading pauses until it is.
- If the stock token still trades after the US market closes, the app shows an after-hours label.

## Trading

1. A voter picks an asset from the catalog and an amount. Monaco checks every version of that stock it can trade, on every chain, and shows the best all-in price, labelled as the best across the venues checked. A buy can't be bigger than the whole pot. If no venue can route it, the app refuses the proposal.
2. The voter set votes yes or no before expiry.
3. When the proposal passes, Monaco swaps from the pot at the best price at that moment, which can be on another chain than the one quoted at the vote. Bought tokens land in the cabal's treasury.
4. If the swap fails, the trade is marked failed and any member can retry it.

Sells work the same way and keep the issuer and chain already held.

**Best price across chains.** The same stock can exist as several tokens from several issuers. Monaco compares them by underlying shares received for the dollars spent, after every cost of the route: price impact, fees, moving USDC between chains, and network fees (internally, gas). Solana wins ties, and another chain must beat it by a small margin before Monaco moves money for it. Holdings are shown by stock, never split by chain or issuer, and the pot counts money on its way between chains so it never dips during a trade.

| Issuer | Chains it lists on | At launch |
| --- | --- | --- |
| xStocks | Solana, Ethereum, Arbitrum | Solana only. Ethereum and Arbitrum are deposit and withdrawal networks only, not trading venues ([#5491](https://github.com/Munyon-Canyon/monaco/issues/5491)); real quotes found no Uniswap route on Arbitrum and 31–99% price impact on Ethereum ([#4969](https://github.com/Munyon-Canyon/monaco/issues/4969)). |
| Ondo | Solana, Ethereum | Solana, per listing, once verified. Ethereum is a deposit and withdrawal network only ([#5491](https://github.com/Munyon-Canyon/monaco/issues/5491)). |
| Coinbase | Base | Per listing, once verified. Routes through Uniswap v4 pools. |
| Robinhood | Robinhood Chain | Per listing, once verified |

Which chain words a member sees:

- **Network names** (Solana, Base, Arbitrum, Ethereum, Robinhood Chain) appear only in the Deposit and Withdraw network pickers, the Review sheet's venue rows, the chain picker, and the stock page's best-price line. Nowhere else. Arbitrum and Ethereum are deposit and withdrawal networks only, so only the Deposit and Withdraw pickers name them; trades happen on Solana, Base and Robinhood Chain ([#5491](https://github.com/Munyon-Canyon/monaco/issues/5491)).
- **Venue names** appear only on a trade's detail, which shows what was checked, for example "Checked 3 venues for Apple. Best: Ondo. You got 0.002 more shares than the next best." Issuer names follow the copy rule above: never "xStock".
- Nowhere else: proposals, holdings, the pot and activity name the stock, not the chain.

### What can be bought

- **Tokenized US stocks** from xStocks (`AAPLx`, `TSLAx`, …). The whole public catalog is searchable, not a fixed list.
- **Pre-IPO tokens** from Tessera and PreStocks (SpaceX, OpenAI, Kalshi, Anthropic, …). They use the same propose, buy, hold and sell path.

| Pre-IPO rule | Detail |
| --- | --- |
| Units | 9 decimals. Quantities are tokens, not shares. PreStocks amounts are stored raw and shown times the token's UI multiplier. |
| Best price | SpaceX, OpenAI and Kalshi exist from both issuers. A buy compares both and keeps the cheaper. Anthropic, Anduril, Neuralink, Figure and Polymarket are PreStocks only. |
| Price | The on-chain price values the pot. A private-market reference is shown beside it when fresh. |
| Hours | They trade around the clock and never show an after-hours label. |
| Names | Display names drop Tessera's `T-` prefix and PreStocks' suffix ("SpaceX"). Holdings say which issuer they came from. |
| Fees | Tessera withholds 0.2% on transfer, PreStocks 1%. |
| Charts | Empty until price samples exist; the screen says so. |

Reference addresses (Solana mainnet):

- USDC: `EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v`
- xStocks: `GET https://api.xstocks.fi/api/v2/public/assets/{symbol}`, then the `deployments` entry where `network == Solana`. `AAPLx` is `XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp`, `TSLAx` is `XsDoVfqeBukxuZHWhdvWHBhgEHjGNst4MLodqsJHzoB`.
- Tessera: `tSpaceX` `TSPXcLV76s6V2zDiZQ18kBfcbnjaE2ZzNT3ga2Pd99v`, `tKalshi` `TKLSidmLVt3cqGaaodG8tyRzoANfQwoh67AccjmubeZ`, `tOpenAI` `oPAiAikWTaFj9RYoRFD35ccfwhnMcB3ThgBZRHSkjTZ`.
- PreStocks: `SPACEX` `PreANxuXjsy2pvisWWMNB6YaJNzr7681wJJr2rHsfTh`, `OPENAI` `PreweJYECqtQwBtpxHL171nL2K6umo692gTm7Q3rpgF`, `KALSHI` `PreLWGkkeqG1s4HEfFZSy9moCrJ7btsHuUtfcCeoRua`, `ANTHROPIC` `Pren1FvFX6J3E4kXhJuCiAD5aDmGEb7qJRncwA8Lkhw`, `ANDURIL` `PresTj4Yc2bAR197Er7wz4UUKSfqt6FryBEdAriBoQB`, `NEURALINK` `PrekqLJvJ3qVdXmBGDiexvwUTF4rLFDa6HWS4HJbw9S`, `FIGUREAI` `PreZad18qfPtbxNpMtMuAuX2zVpvkEU8DnJx56faCWd`, `POLYMARKET` `Pre8AREmFPtoJFT8mQSXQLh56cwJmM7CFDRuoGBZiUP`.

## Performance

Performance is the product, and the launch leaderboard ranks **right calls**, never dollars. People and cabals are ranked on the share of their resolved votes that were right, over Week, Month and All time. A row shows the rate with its record ("91%" over "10 of 11 right"), five resolved votes in the period are needed to rank, and turnout ("Voted 11 of 11") is the sub-line. A vote is judged on the asset's price move after the proposal is decided: [Right calls](architecture/right-calls.md) has the rule. The app reads one screen per board and period from `GET /v1/leaderboards/calls`, which also serves the caller's row with the row above it, the gap to pass it, the rows around it and the not-ranked-yet card.

The return boards (`percent return = current value ÷ net USDC in - 1`, where net USDC in is money funded minus money cashed out) are kept for the P&L views, such as a cabal's value history and a member's return on their profile. They no longer rank anyone. Dollar gain or loss sits next to a name only where a screen shows the pot or a stake.

A member can share a gain card, an image of their own all-time gain and return with their invite link, from the cabal screen (for a cabal they are in), from Profile and from Home. It carries only the sharer's own figures and own cabals, never another member's.

**Inside a cabal** (the cabal screen):

- **Pot.** USDC plus each holding with units, price, value and purchase price.
- **You.** Your stake in dollars and as a percent of the pot, with your gain or loss.
- **Members.** Every member, ranked by their return in this cabal. This is a P&L view, not the launch leaderboard.

**Across the app** (Home and the Cabals tab):

- **Cabal board.** One row per cabal with at least five resolved calls in the period, ranked on the cabal's right-call rate. Cabals compete on this. Join-by-request hides entry, not the score.
- **People board.** One row per person across all their cabals, ranked on their right-call rate. Alex in three cabals is one row. Friends is the same board over the people you follow and you.

Pot value is saved after every fund, trade and cash out, so charts and boards can be replayed without recomputing from wallets.

## Money out

Also two steps, mirroring money in:

1. **Cash out** (the **Cash out** button on the cabal screen). Pick an amount from a small minimum up to your whole stake; cashing out everything is the same flow at the maximum. Your shares are debited, your slice of stock is sold if the pot is short of USDC, and USDC equal to your share of the pot lands in your account balance. You get your fraction of the pot now, not the dollars you put in. If the sale raises less than your slice, you are paid what it raised and keep the shares that didn't cover.
2. **Withdraw** (the **Withdraw** button on the account balance row, on Home or Profile). Pick a network (Solana, Base, Arbitrum or Ethereum) and paste an address on it. Monaco sends USDC from your account balance there. On Solana the exact amount arrives. On another network Monaco moves it across chains first, so the screen shows "You'll receive about $X". Transfers can't be undone, so the address is checked for its network before you can continue. This never touches cabal holdings.

Payouts are always USDC, never stock.

## Profile

The Profile tab shows the user's photo, display name, `@handle`, join date, account balance, deposit address, and each cabal they are in with its pot, their stake and their return.

- **Handle.** Every user picks a unique `@handle` during onboarding, before anything else. 3–20 letters, digits or underscores, case-insensitive. It can change once every 30 days, and a deleted account's handle is never reused. After the user's first deposit of $10 or more, the handle also works as their referral link. Rules: [auth.md](architecture/auth.md#handle).
- **Display name.** A label, separate from the handle and not unique. 1–32 characters, at least one letter or digit, no invisible or control characters, except a joiner between two emoji. Edits show immediately and roll back with a toast if the server rejects them.
- **Photo.** Picked on Profile or Settings. Storage: [ops-profile-photos.md](ops-profile-photos.md).
- Both are rate limited, and a change updates every board that shows the user.

## Out of scope

- A custom on-chain vault, share token or on-chain voting
- Android and web apps, copy trading
- Buying or redeeming directly with a stock issuer (trades go through Jupiter on Solana and Uniswap on other chains)
- Holding a balance on any chain other than Solana
- App Store listing, KYC and AML, securities licensing

## Before a public launch

Tokenized stocks like `AAPLx` track a stock on-chain; they are not shares held at a broker. Pooling money and trading for a group raises broker-dealer, investment adviser and money-transmitter questions in the US. Get securities and fintech counsel before a consumer launch. Some issuers may require geo-fencing or a licensed partner for US users.
