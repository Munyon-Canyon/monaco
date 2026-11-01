# Deposits & withdrawals

**Status:** Decided 2026-09-26 for deposits (crypto and card) and direct-to-treasury transfers (bounced). Withdrawals keep today's behavior, crypto only for MVP. Reconciled 2026-09-27 with [backend-platform.md](backend-platform.md), which wins where the two conflict. The `funding` module owns deposits, onramp sessions, the bounce and withdrawals to an address. The `treasury` module owns fund and cash out. Built in [Rollout](backend-platform.md#rollout) step 3 (flows 5 to 7), step 4 (flow 8) and step 5 (flows 14 and 15).

## Decision

Every deposit, however it starts, ends the same way: **USDC lands in the user's Privy member wallet on Solana, and that on-chain USDC is their platform balance.** From there it can fund a cabal. The backend never needs to know how the USDC got there. The member wallet is the user's existing Privy wallet, reused on sign-in and never recreated, even though the new backend starts on an empty database.

Two ways in:

1. **Crypto (unchanged).** The app shows the user's member-wallet address. They send Solana USDC to it from an exchange or wallet.
2. **Card / bank (new).** Tapping **Deposit → Card** opens a Monaco-hosted web page that renders Privy's fiat on-ramp. The on-ramp provider (Stripe, Meld, MoonPay or Coinbase, chosen by Privy by region) sells the user USDC and sends it straight to the same member-wallet address on `solana:mainnet`. From that moment the flow is identical to a crypto deposit.
3. **Crypto from an EVM chain (M24).** The app also shows one EVM address, the same on Base, Arbitrum and Ethereum. Privy forwards every USDC deposit there to the user's Solana member wallet, and from its arrival on Solana the flow is identical to a crypto deposit. See [Deposits from an EVM chain](#deposits-from-an-evm-chain).

The web page exists only because Privy's on-ramp is a React hook (`useFiatOnramp` from `@privy-io/react-auth`); there is no Swift equivalent. Privy's Expo SDK has `useFundSolanaWallet`, but Monaco's app is native Swift.

## Why

- **One money path.** Card deposits reuse the whole pipeline: balance read, fund-to-cabal sweep, share issuance. No new ledger logic, no provider webhooks on the money path.
- **No custody of fiat.** The provider handles KYC, card payment and compliance; Monaco only receives USDC on-chain.
- **Privy already owns the wallet and the user.** Using Privy's on-ramp keeps provider selection, regional availability and provider contracts on Privy's side.

## Card deposit flow

[Flow 6](backend-platform.md#flows): `CreateOnrampSession`, then the page reports status, which emits `onramp.status_changed`. From the USDC's arrival on, it is flow 5.

```
App                     Monaco API              monacolabs.xyz/fund         Privy / provider          Solana
 │ Deposit → Card          │                          │                          │                       │
 │── POST /v1/onramp/sessions (amount?) ─►│            │                          │                       │
 │◄── { url, session_id } ─────────────── │            │                          │                       │
 │── open url in SFSafariViewController ─────────────► │                          │                       │
 │                          │◄── GET session (token) ─ │                          │                       │
 │                          │── { wallet address, amount } ──►                    │                       │
 │                          │                          │── Privy login (if needed)►                       │
 │                          │                          │── fund({ source: USD,    │                       │
 │                          │                          │    destination: { chain: solana:mainnet,         │
 │                          │                          │    asset: USDC mint, address } }) ──►            │
 │                          │                          │                          │── pay, KYC ──►        │
 │                          │                          │◄── status confirmed ──── │                       │
 │                          │◄── PATCH session status  │                          │                       │
 │◄──────────── redirect monaco://deposit/complete?session=… ─────────────────────│                       │
 │ show "Purchase processing"                          │                          │── USDC transfer ─────►│
 │ deposit poller picks up USDC (minutes later) ───────────────────────────────────────────────────────────►│
```

1. **Start.** App calls `POST /v1/onramp/sessions` (`CreateOnrampSession`, with an `Idempotency-Key`) and an optional suggested amount. When the user arrives from a cabal, the app prefills the amount that cabal needs; otherwise the user chooses. Minimums and fees are not shown in the app: the provider shows its own. The `funding` module creates an `onramp_sessions` row and returns a one-time URL: `https://monacolabs.xyz/fund?s=<opaque token>`. The token is single-use, expires in 10 minutes, and is bound to the user.
2. **Open.** App opens the URL in `SFSafariViewController` (not `WKWebView`). Payment providers need Apple Pay and 3-D Secure, which work reliably only in Safari's engine, and the user sees real browser chrome on a payment page.
3. **Resolve.** The page exchanges the token with the API for the user's member-wallet address, suggested amount and Privy user id (`privy_user_id`). **The destination address never travels in the URL.** A URL parameter would let a phishing link point a user's card purchase at an attacker's wallet.
4. **Authenticate with Privy.** The hook requires a Privy-authenticated user in the page, so the page runs Privy login for the same Privy account. `SFSafariViewController` keeps the Privy login until the user signs out of the app. The app's Swift SDK session does not carry into Safari, which is why the page logs in itself. Before it opens the funding modal, the page compares the signed-in Privy user with `privy_user_id`. If they differ, it shows the wrong-account error, sends no status report and does not call `fund()`. After Log out and a login with the right account, the page funds. Signing out of the app clears the Safari view's website data, so the next card deposit starts from a clean login.
5. **Fund.** Page calls `fund()` with `source` USD, `destination.chain = "solana:mainnet"`, `destination.asset` = Solana USDC mint, `destination.address` = the member wallet, `defaultAmount` = suggested amount. `fund()` sends to whatever wallet address the caller specifies ([Privy fiat on-ramp docs](https://docs.privy.io/wallets/funding/fiat-onramp)), so the server-created member wallet is a valid destination (decided 2026-09-27).
6. **Return.** `fund()` resolves `confirmed` (user finished the provider flow) or `submitted` (exited before confirmation); a thrown error is a decline or a cancel. The page reports the status to the API, which applies it as a guarded update and appends `onramp.status_changed`. The page then redirects to `monaco://deposit/complete?session=…` and the app closes the browser.
7. **Arrive.** The provider's USDC transfer lands minutes later. The deposit poller credits it like a crypto deposit. Nothing on the money path depends on step 6; the status report is for UX only.

### `onramp_sessions` (UX tracking only)

```
id, user_id, token_hash, suggested_amount_micros, status, provider?, created_at, expires_at, completed_at
status: created → opened → confirmed | submitted | cancelled | failed | expired
```

The status is a state machine with a `transitions` table and guarded updates ([State machine](backend-platform.md#patterns-and-where-each-earns-its-place)). `suggested_amount_micros` is `money.Micros`. The table shows "Card purchase processing" in the app between the redirect and the USDC arriving, and feeds funnel analytics through `onramp.status_changed`. It is **not** a ledger. A session is not matched to the on-chain transfer by amount; provider fees make amounts differ.

### The web page

- Served at `monacolabs.xyz/fund`, from `apps/web`. `apps/web` is static with no build step today; the fund page needs React and a bundler (Vite), built to a static folder and deployed alongside the waitlist page.
- Holds no secrets. It talks only to the Monaco API (token exchange, status report) and Privy.
- CSP limits scripts and frames to Privy and its providers. Page is never framed (`X-Frame-Options: DENY` already set).
- Privy dashboard: enable fiat on-ramp, allow-list `monacolabs.xyz` as an app domain.
- `environment: 'sandbox'` in local dev, `production` on staging and prod (real money), chosen at build time by `VITE_PRIVY_ENV`.

## Crypto deposit flow

Unchanged for the user. The deposit screen shows the copyable member-wallet address (no in-app amount entry). Any USDC sent there is platform balance. See [architecture.md](../architecture.md) (Deposit and fund a cabal).

[Flow 5](backend-platform.md#flows) splits detection from crediting. The `funding` module's deposit watcher (`funding.deposit_watch`) watches every USDC token account a member wallet owns, not only the canonical associated token account, and records each successful signature it finds as a `deposit_candidates` row. The row and its `deposit.candidate_seen` event commit in the same transaction as the watcher's checkpoint, so a crash leaves both or neither (the `after-candidate` crash point). The watcher never reads a transaction and never credits.

Each tick spends a bounded RPC call budget (`FUNDING_DEPOSIT_TICK_BUDGET`, under the `FUNDING_DEPOSIT_RPC_RATE` limiter) on these steps, in order, and stops when the budget or the tick deadline runs out:

1. **Gate.** One `getMultipleAccounts` call per 100 accounts compares each account's state and balance with the last observation. A change, a closure or a reopen marks the account dirty.
2. **Dirty catch-up.** Each dirty account walks its signatures newest first, down to its last high signature, and commits each page with its candidates.
3. **First sight.** A wallet with no rows is seeded. It runs before rotation and discovery so a busy tick cannot starve a new wallet.
4. **Rotation.** An account whose rotation is due (`FUNDING_DEPOSIT_ROTATION`) is rescanned over the last `FUNDING_DEPOSIT_RECOVERY_SLOTS` slots, never below the wallet's first-seen slot. This catches what the gate cannot see, such as a deposit and an equal outflow between two gates.
5. **Discovery.** Once per `FUNDING_DEPOSIT_DISCOVERY`, `getTokenAccountsByOwner` finds USDC accounts the wallet opened since, and adds each as a dirty account.
6. **Reconcile.** The residual check below. It makes no RPC call and reads only stored rows.

The `funding.resolve_deposit_candidate` consumer decides what a candidate is. It reads the transaction outside the delivery transaction, sums the USDC that reached the wallet, and then applies one of three outcomes in a single write. A transaction whose signature `treasury`, `trading` or `funding` owns is `ours` and is never credited. That is the Monaco-signature rule, and it keeps payouts, bounces and sweeps from counting as deposits. A transaction with no net inbound USDC, such as an outflow, a self-transfer or a direct mint, is `not_deposit` and appends `deposit.candidate_dismissed`. Anything else appends `deposit.credited` through `CreditDeposit`. An error never marks a candidate `not_deposit`. The candidate stays `pending`, and the delivery is retried or dead-lettered.

A wallet seen for the first time starts at its creation, not at the chain tip. The watcher lists its USDC accounts and walks each account's signatures from the tip down to five minutes before the wallet's `created_at` (the skew allowance). It records every non-failed signature in that window as a candidate, so a deposit made between wallet creation and first sight is credited once, and sets the account's high signature to the newest signature at or below the observation slot. A signature before the window is never recorded, so a reused wallet's history is not replayed and `first_deposit_at` is not set by old transfers. When the first page is full and still inside the window, first sight stores the account as dirty with the page cursor and a `history_floor`, and dirty catch-up finishes the walk from that checkpoint. Discovery and rotation never reach below the first-seen slot.

Consumers of `deposit.credited` are `treasury`, which writes the `user_txns` deposit entry so the member ledger reconciles with on-chain balance, `notify`, `identity` and analytics. `identity` sets `users.first_deposit_at` on the first deposit of at least $10, with a guarded update `WHERE first_deposit_at IS NULL`; `referrals` reads it to unlock the user's handle as a referral code (decided 2026-09-27, [referrals.md](referrals.md#codes)). The watcher runs on one worker under its advisory lock ([Deploy rule 1](backend-platform.md#deploy-and-observability)), and every tick logs what it scanned and found, including nothing ([Logs as evidence](backend-platform.md#logs-as-evidence)).

The residual check compares each wallet's on-chain USDC with the ledger. Once an hour per wallet (spread, like rotation), the reconcile step computes `residual = Σ last_amount over the wallet's open accounts − (opening_micros + settled wallet ledger USDC)` from the gate's stored finalized observations. `opening_micros` is the on-chain USDC the ledger did not explain when the watcher first saw the wallet: the sum of its accounts at the first-seen slot, minus the settled ledger at that moment (zero for a new wallet). When first sight recorded candidates, or has more of the window to walk, the opening stays empty. The first conclusive check then takes `observed − settled`, which leaves out what those candidates credited, so the check never counts them twice. A wallet migrated from the old cursor table also has no opening yet, and a gap that predates the watcher stays invisible. A check is conclusive only when the wallet has no pending ledger transaction, no `pending` candidate and no dirty account. A non-zero residual on two conclusive checks in a row logs `funding.deposit.residual` at Warn and counts the wallet in the gauge `monaco_funding_deposit_residual_wallets`; an inconclusive or zero check resets the count. It never credits, never appends an event and never writes a ledger row, so a direct `mintTo` that the resolver dismissed as `not_deposit` raises an alert and nothing else. The gauge `monaco_funding_deposit_candidates_pending_oldest_seconds` is the age of the oldest `pending` candidate and alerts past one hour. The watcher credits deposits to any of the wallet's USDC accounts, but fund and withdraw spend only from the canonical associated token account, so USDC in another account of the same wallet is credited and not spendable. [Balance](#balance) shows it apart from available funds, and [Recovering funds in another account](#recovering-funds-in-another-account) says how it is moved.

The deposit screen becomes two options: **Card** (opens the fund page) and **Crypto** (shows the address).

## Deposits from an EVM chain

**Status:** Decided 2026-10-11 in M24 ([#4917](https://github.com/Munyon-Canyon/monaco/issues/4917)). Built by #4966 (Privy client and the deposit wallet) and #4967 (forwarding, arrival and `arriving_micros`). The app screen is #4968.

Every user balance stays on Solana. Base, Arbitrum and Ethereum are ways in, nothing else: USDC sent from them becomes balance only after it arrives in the Solana member wallet, and the existing deposit watcher credits it there.

### The deposit wallet

Each user gets one **EVM deposit wallet**, the same address on every EVM network. It is app-owned: `identity`'s `EnsureEVMDepositWallet` creates it through Privy with the idempotency key `evm-deposit:<user_id>`, our authorization key quorum as owner, and stores it in `user_wallets.evm_privy_wallet_id`, `evm_address` and `evm_created_at` (all three or none). A concurrent second call loses the insert and reads the stored row back. The wallet only ever forwards to the user's own Solana member wallet, so the user never signs anything for it.

### When the EVM wallets are created

Both EVM wallets are created when their owner is, so no request waits on a Privy create:

| Wallet | Created by | On | Ensure |
| --- | --- | --- | --- |
| A user's EVM deposit wallet | `identity.evm_wallet` | `user.created` | `EnsureEVMDepositWallet` |
| A cabal's EVM treasury wallet | `cabal.evm_wallet` | `cabal.created` | `EnsureTreasuryEVMWallet`, idempotency key `treasury-evm:<cabal_id>` |

Each consumer calls its ensure outside the delivery transaction, then records the delivery. A Privy failure returns a retryable code, so the bus redelivers the event, and sign-in or cabal creation neither waits on it nor fails from it. A deleted user, a user with no member wallet, or a process without Privy wired is recorded with its code and acked. The lazy ensures stay as a fallback: `GET /v1/me/deposit-addresses` and the first trade routed to an EVM chain still create a wallet that is missing. Users and cabals created before the consumers existed are filled by `monacoctl ops evm-wallets` ([Deploy staging](../how-to/deploy-staging.md#backfill-evm-wallets)).

Privy's deposit accounts would create this wallet for us, but they need a destination owned 1-of-1 by the user with no authorization key, and our member wallets carry our key as an additional signer ([#4917](https://github.com/Munyon-Canyon/monaco/issues/4917), open question 1). So forwarding uses Privy **wallet automations** instead:

1. One app automation, created once with `CreateAutomation`: trigger, a USDC deposit on Base, Arbitrum or Ethereum; action, swap to USDC on Solana.
2. Each deposit wallet is attached to it with `Attach(evmWalletID, solanaDest)`, where `params.destination_address` is the user's Solana member wallet. Our key signs the attach as owner.

`GET /v1/me/deposit-addresses` (`identity`) returns the Solana member wallet first, then one item per enabled EVM network, all with the deposit wallet's address. Enabled networks come from `EVM_DEPOSIT_NETWORKS`. Unset, it enables every EVM network that takes USDC, in the order `base,arbitrum,ethereum`; an operator sets it only to narrow that list, or to `none` to offer no EVM network. With `none`, the `user.created` consumer `identity.evm_wallet` records `deferred` and creates nothing at sign-up, and the response holds the Solana item alone. With networks offered, the wallet is created at sign-up or, failing that, on the first request; Privy is called outside any database transaction, between a short read and a short write. If creating or reading the EVM wallet fails, the response still returns the Solana item and never a 5xx.

### Forwarding and arrival

```
Sender wallet (Base)      EVM deposit wallet       Privy automation            Solana member wallet        Monaco
 │── USDC transfer ───────►│                         │                            │                          │
 │                         │── deposit trigger ─────►│                            │                          │
 │                         │                         │── swap + bridge to Solana ─►                          │
 │                         │                         │                            │◄── watcher: candidate ── │
 │                         │                         │                            │    resolver: credited     │
 │                         │◄────────────── execution poller (10 s): evm_deposit_forwards row ───────────────── │
```

- **Execution poller** `funding.evm_deposit_forward`, every 10 s, on one worker under its advisory lock. It pages through `Executions` for every enrolled wallet and upserts `evm_deposit_forwards` on `privy_execution_id`, so a repeated page changes nothing. No cursor is stored: each tick pages from the start. An execution that is `pending` or `succeeded` keeps the row `pending` (money in flight). `failed` or `rejected` sets the row `failed`. A `succeeded` execution appends `deposit.evm_forwarded`.
- **Recovery.** A `failed` execution with no wallet action calls `Reindex` once for that wallet and network, and alerts. An execution `pending` for more than 30 minutes alerts once. Privy's automation acts on the wallet's whole balance and dedupes per block, so a reindex never double-forwards.
- **Arrival.** The forwarded USDC lands in the member wallet like any deposit. The watcher records the candidate and the resolver credits it through `CreditDeposit` ([Crypto deposit flow](#crypto-deposit-flow)), so the credit, the `user_txns` row and `deposit.credited` are exactly the Solana path. A `funding` consumer of `deposit.credited` then links the credit to the forward row, by Solana signature or else by member wallet plus amount within the window, and sets it `arrived`. That consumer never credits; it only closes rows. A forward first seen already `succeeded` also matches a credit up to 24 hours before it was seen, nearest first, and never one another forward holds.
- **Arriving.** `arriving_micros` on `GET /v1/me/balance` is the sum of the user's `pending` forward rows. The app shows it as an **Arriving** row so money in flight is visible. It is never spendable. A failed read of it returns the balance without the field, never an error.
- **Polling, not webhooks.** Privy's automation webhooks need the Enterprise plan in production, so the execution list is the source of truth.

### The bridge fee

The member pays the bridge fee, and the deposit is credited net of it (operator decision of 2026-10-11 on [#4917](https://github.com/Munyon-Canyon/monaco/issues/4917)). The first real Base deposit sent 0.99925 USDC and was credited 0.835383. Privy's forward charged a relayer fee of 0.163098 USDC and a Privy fee of 0.000748 USDC. Gas sponsorship doesn't remove the relayer fee.

Monaco shows the fee before a member deposits, as `estimated_fee_micros` on each EVM item of `GET /v1/me/deposit-addresses`. The estimate is what the last forward on that network cost:

- When the execution poller sees a forward succeed, it stores `fee_micros` = Privy's `input_amount` − `output_amount` on the `evm_deposit_forwards` row, floored at 0. It stays NULL while the forward is pending or when Privy reports no input.
- `funding`'s `DepositFees` reads the fee of the most recently arrived forward per network. `identity` puts it on that network's item. A network with no arrived forward has no field, and Solana never has one. If the read fails, the response leaves every fee out, logs `identity.deposit_addresses.fee_failed`, and still answers 200.

Quotes can't give this number. Privy's walletless `POST /v1/deposit_accounts/crypto/quote` returns a flat 0.075% (0.99925 of 1 USDC on Base, Arbitrum and Ethereum), which is only Privy's own fee. A sponsored `/transfer` from Base to Solana charged no relayer fee at all. The relayer fee appears only on the deposit automation's swap. No quote wallet or extra config is needed.

USDC on any other EVM chain, or any other token, sent to the deposit wallet, does not trigger the automation and is not credited. The deposit screen says "Only send USDC on Base, Arbitrum or Ethereum to this address."

## Fund

[Flow 7](backend-platform.md#flows), owned by the `treasury` module: `FundCabal` from `POST /v1/cabals/{id}/fund` moves an exact amount of platform balance from the member wallet to the cabal treasury through a Privy transfer, waits for confirmation, and mints share units at the live price. It appends `cabal.fund_submitted`, then `cabal.funded`. Its outcomes, including `InsufficientFunds`, `CabalPaused`, `CabalBanned`, `PrivyUnavailable` and the `after-sign` and `before-commit` crash points, are the row in its [flow file](backend-platform.md#outcomes-as-a-map). The balance and share rules are in [architecture.md](../architecture.md) (Deposit and fund a cabal) and [data-model.md](data-model.md#double-entry-ledgers).

The transfer's signature is stored before broadcast, like every treasury-touching transfer, so the external-deposit classifier below never mistakes a sweep for a stray transfer.

`GET /v1/cabals/{id}/fund/preview` answers the Fund keypad before a submit: the caller's available cash, the minimum, their share now (`slice_bps_now`) and the share a fund of `amount_micros` would leave them with (`slice_bps_after`). It reads outside any write transaction and mints nothing. The share comes from `domain.SliceAfterFund`, which mints with `MintShares`, adds the units to the caller's on a copy and takes the caller's slice from `SlicesBps`, so it keeps the rounding the pot's `me.slice_bps` hands out, remainder included. `slice_bps_after` is computed for at most the available cash: an amount over it returns `over_micros` and the share for exactly the cash, and the line never promises a share the member cannot buy. It is null without an amount, below the minimum, or while a holding has no fresh price. A paused cabal answers 200 with `paused: true`; a banned one answers `cabal_banned`. `first_funded_at` is the caller's earliest confirmed fund in `cabal_activity`, null for a member who never funded. Once a transfer is `settled`, `GET /v1/fund-transfers/{id}` also returns `slice_bps`, the caller's share of the pot at read time, so the screen needs no second pot read.

## Direct transfers to a cabal treasury are not allowed

Money reaches a cabal treasury only through Monaco's flow: platform balance → **Fund this cabal** → sweep. Anything else that lands in a treasury is sent back, and until it is, its value is quarantined: it never enters pot value or mints shares, and the cabal keeps trading, funding and cashing out. This is [flow 8](backend-platform.md#flows), owned by the `funding` module.

### Warn up front

- The deposit screen only ever shows the user's **own member-wallet address**, with the line "Only send USDC on Solana to this address."
- Wherever a treasury address appears (cabal detail, explorer links), it is labelled **"Do not send funds here. Transfers are returned."** No copy button; an explorer link only.

### Detect

Two paths, same handler (the treasury watcher):

1. **Privy webhook** `wallet.funds_deposited`, subscribed for every treasury wallet, received by a `funding` HTTP adapter. Verified with Privy's Svix signature headers (`svix-id`, `svix-timestamp`, `svix-signature`). Privy delivers at least once, so the handler dedupes on the transaction signature.
2. **Surplus reconcile poller** (every minute per cabal, in the worker under its advisory lock): treasury on-chain balance per asset versus what the treasury ledger says it should be, read through the `treasury` query port. Catches anything a webhook missed.

The handler classifies each inbound transfer by its signature:

- The signature belongs to a Monaco-initiated transfer → ours, ignore. The handler asks the query ports of every module that signs from or into a treasury: `trading` (swaps), `treasury` (fund sweeps, cash outs) and `funding` (bounces). Each stores its signature **before** broadcasting, so a webhook can never arrive for our own transfer before we know about it.
- Otherwise → **external deposit**.

### Respond

One `uow.Do`:

1. Insert `external_deposits` row: `signature` (unique), `cabal_id`, `sender` (owner of the source token account), `mint`, `amount` (`money.BaseUnits`), `status = detected`. An open row is quarantined: it never counts toward pot value (below).
2. Append `cabal.external_deposit_detected`.
3. Nothing else: no pause is written ([Quarantine](#quarantine)). Consumers do the rest ([flow 8](backend-platform.md#flows)): `funding` sends the bounce, `admin` shows it, and `notify` tells every cabal member from `cabal.paused`.

### Quarantine

An unattributed deposit does not pause the cabal. Pot value is read from the treasury ledger, and the stray inflow never enters it, so share price, every mint of share units and every cash-out payout stay right while the row is open, returned, held or given up on. Opening a pause for it let an outside sender freeze a cabal for the price of a deposit, and a failed refund never cleared it.

### Pause

`funding` owns the pause. A cabal is paused while it has an ops pause, recorded in funding with reason `ops`. Pauses with reason `external_deposit` written before 2026-10-20 close when their deposit returns, is held or fails. Ops pauses go through funding too. `trading` and `treasury` read the pause through funding's query port at check time, so the pause takes effect in the same transaction that records it, with no event lag. There is no `cabals.trading_paused_at` column. The pause ends when its last reason resolves.

**Pause events.** `funding` appends `cabal.paused` in the transaction that opens a cabal's first pause reason, and `cabal.resumed` in the transaction that closes its last one. Opening a second reason or closing one of several appends nothing. Nothing reads these events to decide whether a cabal is paused; the query port stays the check. `notify` consumes both to push every member ([notifications.md](notifications.md#what-notifies-mvp)).

**While paused**, every member is notified, and: no proposal executes (the trade engine's `CabalPaused` check, [trade-execution.md](trade-execution.md#stage-2-trade-engine)), no fund-to-cabal sweeps and no cash outs (`CabalPaused` on flows 7 and 14). Voting, chat and comments continue. The pause is for ops only: a stray deposit holds value no one owns, but it is kept out of the ledger, so it cannot make pot value, share price or a payout wrong.

### Bounce

The `funding` module's consumer of `cabal.external_deposit_detected` sends the **same mint and amount** back to the sender, from the treasury, signed through Privy with the app authorization key, fee paid by the relayer. Its state lives on the `external_deposits` row: signature stored before broadcast, then the same guarded update and sweeper recovery as a swap ([trade-execution.md](trade-execution.md#stage-3-swap-layer)). Exactly one bounce per external deposit, enforced by a guarded update from `detected`. The bounce writes no ledger entries. The stray inflow never entered the treasury ledger, so inflow and return net to nothing, and `cabal_txns` holds only Monaco's own movements.

On confirm, one `uow.Do` sets `external_deposits.status = returned` and appends `cabal.external_deposit_bounced`. A pause opened for the deposit before 2026-10-20 ends in that transaction, and `cabal.resumed` is appended when it was the cabal's last reason.

### Edge cases

| Case | Handling |
| --- | --- |
| Sender is an exchange hot wallet | Bounce still goes to the sending address. The exchange may not credit it back to the user. That is why the warnings exist. Ops can override the return address on request after verifying the user. |
| Dust (below $1) | Relayer fee would exceed the value. Do not bounce, do not pause. Record as `ignored_dust`; excluded from pot value. Never swept to a platform wallet. |
| Unknown or spam token | Not in the market catalog → record, do not bounce, do not pause, exclude from pot value. |
| Stock token (xStock) sent directly | Same as USDC: bounce same mint and amount, pause meanwhile. |
| Bounce fails (e.g. sender account closed) | Status `bounce_failed` and the row is resolved, the amount stays out of pot value, ops is alerted, and the cabal keeps running. Retries are bounded: a lapsed bounce is re-signed at most 3 times. Ops decides later: retry to a new address or hold as unclaimed. |
| Several stray transfers at once | Each gets its own row and bounce. None pauses the cabal, so nothing needs clearing. |

Member wallets are **not** covered by this: sending USDC to your own member wallet from outside is the intended crypto deposit.

## Withdrawals

Behavior unchanged for the user. Both are rebuilt in Rollout step 5. A banned or suspended user can still cash out and withdraw: a ban stops them acting in cabals, never from taking their money out. Account deletion runs a cash out of every stake first, then scrubs personal data and keeps ledger rows. A banned cabal winds down by selling its holdings and returning USDC pro rata through the same cash-out path.

### Cash out

[Flow 14](backend-platform.md#flows), `treasury` module: `CashOut` burns the member's share units, sells holdings if the treasury is short on USDC, and pays USDC to the member wallet, where it is platform balance again. Events `cashout.started`, then `cashout.completed`, `cashout.partial` or `cashout.failed`. Cash out is refused with `CabalPaused` while the cabal is paused, but not because the member is banned. The route is `POST /v1/cabals/{id}/cashouts` with an `Idempotency-Key` ([`cabal` naming](backend-platform.md#decided)).

Flow 14 start errors: `invalid_input` means a malformed request (both `all` and `usdc_micros`, `all: false`, or an amount under the minimum payout). `pot_value_changed` (409, retryable) means the pot moved since the preview: in-flight cash-out reservations exceed the live pot, or the whole stake no longer clears the minimum payout. The app reloads the preview and asks the member to check the amount. `insufficient_shares`, `price_unavailable` and `cash_out_in_progress` are unchanged.

A sale that can't start (the swap venue refuses the quote, or the last retry fails) ends as `trade.blocked`. The cash out then settles on the cash on hand: partial when the pot had some cash, otherwise failed with every share unit returned.

### Withdraw

[Flow 15](backend-platform.md#flows), `funding` module: `Withdraw` sends platform balance to any Solana address the user pastes, or to an EVM address on Base, Arbitrum or Ethereum ([Withdraw to an EVM wallet](#withdraw-to-an-evm-wallet)), from `POST /v1/me/withdrawals` with an `Idempotency-Key`. Events `withdrawal.submitted`, then `withdrawal.confirmed` or `withdrawal.failed`. These replace `withdrawal.sent` in [event-bus.md](event-bus.md#who-publishes-who-subscribes). `treasury` consumes `withdrawal.confirmed` and writes the `user_txns` withdrawal entry.

A Solana withdrawal is limited two ways, both before any signature. `POST /v1/me/withdrawals` has an actor rate limit in the spec (`x-rate-limit`), and `Withdraw` refuses with `rate_limited` once the caller has sent to 5 fresh Solana destinations in the last 24 hours. A destination is fresh when no earlier withdrawal to that address was submitted or confirmed, so a repeat to a known address, and a failed attempt, do not count. The cap is `app.FreshDestinationsPerDay`; `WithdrawDeps.FreshDestinationsPerDay` overrides it, and there is no config key yet. The limiter fails open on a limiter error.

The relayer pays the network fee and, when the destination has no token account, its rent. `Withdraw` reserves that cost against three rolling 24 hour budgets before it signs: the user (5 account creations), the destination address across all users (2) and a global budget (500). The budgets are `relayer.SponsorBudgets` in `WithdrawDeps.Sponsor`, with the defaults in `relayer/sponsor.go` and no config key yet. The reservation is three rows in `relayer_sponsor_reservations`, written in the transaction that creates the withdrawal while it holds the user's wallet-outflow lock and the advisory lock of each budget, always in the order user, destination, global. Concurrent withdrawals therefore cannot jointly exceed a budget, and a refusal rolls the withdrawal back. The cost reserved is the worst case, fee plus rent, unless an earlier withdrawal to the address was confirmed. Then the token account exists and the transfer is built without the create instruction, so a recipient who closes an emptied account to recover its rent cannot make the relayer pay for it again without a confirmed withdrawal in between. When the full cost does not fit, the transfer takes the no-subsidy path: it is built without the create instruction and reserves only the fee. Before it signs a no-subsidy transfer, `Transfers.Build` reads the destination's USDC account, and a destination without one is refused with `withdraw_no_usdc_account` ("This address can't receive USDC yet") instead of sending a transfer that would fail on chain, which fails the withdrawal and releases the reservation. A confirmed withdrawal no longer proves the account still exists: before it reserves, `Withdraw` reads the account when the transfer is allowed to skip the create, and a destination that closed its account is reserved at the full cost again, so a recipient who closed an emptied account can receive USDC again while the budgets allow the create. If that read fails, the old rule applies and the check in `Build` is the backstop. When not even the fee fits, the answer is `rate_limited` and nothing is signed. If the reservation cannot be written, `Withdraw` fails with `db_unavailable` and signs nothing. A build that fails before a signature releases its reservation, and a signed withdrawal keeps it for the day. EVM withdrawals reserve nothing.

**Confirm first (step-up).** A copied access token must not be enough to send money to a new address. `POST /v1/me/withdrawals/authorizations` takes the same body as the withdrawal and stores a single-use confirmation in `withdrawal_authorizations` for that exact amount, network and address, tied to the caller's Privy session and valid for five minutes. It answers `reauth_required` unless the caller's access token was issued in the last two minutes, so the app refreshes the session first and a token copied earlier is refused. `POST /v1/me/withdrawals` then needs that `authorization_id` in the `Withdrawal-Authorization` header when the destination has no `confirmed` withdrawal from this user, or the amount is at least 1,000 USDC. `Withdraw` consumes it with one guarded update in the same transaction that inserts the row (matching user, session, amount, kind, network and address, unused and unexpired). Any other outcome is `step_up_required` (missing, unknown, used, expired or from another session) or `step_up_mismatch` (the amount or destination changed after confirming), and nothing is held or sent. A supplied confirmation is checked even when none is needed. A replay with the same `Idempotency-Key` returns the first response and checks nothing again. A banned or suspended user confirms and withdraws the same way: both routes are `x-allow-restricted`, and ending stolen sessions is the separate [session revocation](auth.md#ending-a-session). Dev tokens (outside production) carry no issue time, so they skip the freshness check and keep the rest.

**Quote.** `POST /v1/me/withdrawals/preview` answers the keypad before an amount is typed. Network, destination kind, amount and address are all optional (a missing network or kind means Solana), and it always returns `available_micros`, `fee_micros` (null with `fee_known: false` when the fee is not known yet), `fee_mode`, `max_amount_micros`, `min_micros` and `arrival_seconds`. With an amount it adds `over_micros` (0 when the amount fits) and `receive_micros` (always present, as before, and "0" whenever nothing arrives, so an old client keeps decoding), or `below_min: true` for an amount under 1 USDC, which is a 200, never an error. The two fee modes are `on_top` (the fee is added to the amount; Solana, with a fee of 0 because the relayer sponsors the network fee and no fee is charged) and `included` (the fee comes out of the amount; the EVM networks, where it is Privy's cut). `max_amount_micros` is `available - fee` on top (never below 0) and `available` when included (`domain.MaxSendable`). Below-minimum and over-the-max amounts are decided locally and never reach the bridge. Only an in-range amount on an EVM network with a valid address calls the Privy quote, and `fee_micros` is then `amount - receive_micros`; the handler remembers the last fee per network for 5 minutes and returns it for an amount-less call. An address is validated only when given. The server, never the app, decides the fee, the maximum and the arrival time (`domain.WithdrawalFee`, `domain.ArrivalSeconds`), so the screen draws what it is told and hard-codes none of it.

Each move of a withdrawal (created, submitted, failed) publishes the user's `balance_changed` hint after commit, so Home drops the amount the moment the withdrawal is submitted. Fund-to-cabal ([flow 7](backend-platform.md#flows), `treasury` module) does the same at submit, from the transaction that appends `cabal.fund_submitted`, and again when the transfer settles.

### Withdraw to an EVM wallet

**Status:** Decided 2026-10-11 in M24 ([#4917](https://github.com/Munyon-Canyon/monaco/issues/4917)). Built by #4972. The network picker in the app is #4973.

The member picks a network (Solana, Base, Arbitrum or Ethereum) and pastes an address. A Solana withdrawal is unchanged: `destination_kind: solana` is the default. An EVM withdrawal sends `destination_kind: evm`, a `network` and the EVM address as `to_address`:

1. **Validate.** `ParseEVMAddress` accepts any case and checks the EIP-55 checksum when the case is mixed. The network must be enabled. The row stores the lowercase address in both `evm_address` and `to_address`.
2. **Transfer.** `WalletActions.Transfer` from the member's Solana wallet to the address on that network, with the withdrawal id as the Privy idempotency key. Privy bridges the USDC. From Solana the amount is exact input only ("Exact output bridging is not yet supported when Solana is the source chain"), so the preview shows "you'll receive ~$X" from Privy's quote, not an exact amount.
3. **Store, then poll.** The Privy action id is stored on the row (`privy_action_id`) before the first poll, and the row moves to `submitted` in the same write. The withdrawal poller reads `Action` until it is terminal.
4. **Settle.** `succeeded` confirms the row and appends `withdrawal.confirmed`, so `treasury` writes the `user_txns` entry as for Solana. `rejected`, or `failed` with no confirmed on-chain step, fails the row and releases the hold. A transfer whose on-chain step confirmed is never retried, because a retry would send a second transfer; it goes to ops.

The balance hold works as for Solana: the in-flight amount is subtracted from `GET /v1/me/balance` until the row is terminal.

Rows take turns. The withdrawal poller reads at most 100 unsent and 100 submitted EVM rows a tick, and the fund settler reads at most 100 landed or submitted fund transfers. `last_attempt_at` on `withdrawals` and `fund_transfers` is set to the poll time just before each row is worked, and the lists order by `last_attempt_at NULLS FIRST, created_at, id`. A row that cannot move (a fund waiting on a price, an EVM action that failed after a step broadcast) goes to the back of the line, so it cannot lock out newer rows. Each row also has its own time limit, so one slow call cannot use the tick, and a tick that runs out of time stops without an error, leaving the rows it did not reach first in line. The schedule lives in the database and survives a restart. Rows that cannot move keep their hold and still log for support; a durable support queue is a separate change.

No fiat off-ramp in MVP: withdrawals are crypto only. Privy's on-ramp docs cover funding only, and a provider off-ramp is a separate decision after MVP.

### Balance

`GET /v1/me/balance` is owned by `funding`. It returns the member wallet's on-chain USDC minus the amounts of the caller's in-flight fund transfers and withdrawals, so money already on its way out is never shown as spendable. In-flight withdrawals come from `funding`'s own `withdrawals` rows; in-flight fund transfers come from `treasury`'s query port, which owns `fund_transfers` ([data-model.md](data-model.md#decision)). The wallet is read at `finalized`, and the read returns its slot. A submitted fund transfer or withdrawal whose signature finalized at or before that slot is already in the reading, so it is not subtracted again while its row waits for the withdrawal poller or the fund settler. That check costs one `getSignatureStatuses` call, made only when the caller has a submitted outflow. If the call fails, every in-flight amount is subtracted. A deposit shows about 13 seconds after it confirms.

The read splits the wallet's USDC accounts. The canonical associated token account is the only one that fund and withdraw spend from (`relayer.Transfers.Build` takes its source from it), so only its balance, less the in-flight amounts, is `available_micros`. `on_chain_micros` stays the total the wallet holds, and `unspendable_micros` is the part held in any other account of the same wallet. The field is absent when that part is zero. So `available_micros` never promises more than a fund or withdraw can send, and admission, which reads `available_micros`, refuses what the canonical account cannot cover. Account deletion still counts `on_chain_micros`, so a wallet holding USDC in another account cannot be deleted.

When the RPC is rate-limited, the display read serves the user's last on-chain reading if it is under 10 minutes old and none of the user's fund transfers or withdrawals was created or moved since (in-flight amounts still fresh, `as_of` is the reading's time); fund and withdraw never use it, because a command must not act on a stale balance.

### Recovering funds in another account

USDC lands in a non-associated account when a sender used a custom token account or an older wallet opened one. The money stays in the member's wallet. Until the member moves it, the app shows it as `unspendable_micros` and support recovers it.

`relayer.Transfers.BuildConsolidation(ConsolidationSpec{Wallet, Source, Mint, Amount})` builds the recovery. One transaction creates the wallet's associated account if it is missing (`CreateIdempotent`) and moves `Amount` from `Source` into it with `TransferChecked`. The member wallet signs through Privy and the relayer pays the fee, as in every other transfer. A spec whose source is the associated account itself, whose amount is zero, or whose wallet is the relayer is refused before anything is signed. `solana.Client.TokenBalancesAt` lists the accounts to move in `Stranded` (non-zero, not the associated account); an account that is not `initialized` is listed but `Movable` is false.

The path is idempotent in three ways. The caller stores `SignedTx.Signature` before it broadcasts, as for every transfer, and broadcasts the stored bytes again after a crash; the network applies one signature once. A rebuild against the same blockhash returns the same signature. A rebuild after the first transfer landed finds the source empty in `TokenBalancesAt`, so nothing is built. If two different transactions for the same source race, the second fails on chain for lack of funds and moves nothing.

No operator command or scheduled job calls it yet. Support asks an engineer to consolidate a wallet that shows `unspendable_micros`, using the primitive above. A command and an automatic consolidation after the watcher credits such a deposit are follow-up work.

## Alternatives considered

| Alternative | Why not (for now) |
| --- | --- |
| Call providers directly (Coinbase Onramp session token, MoonPay signed URL) from the backend and open the provider URL in Safari | No React page and no web-side Privy auth. But Monaco then holds provider contracts, keys and regional routing itself. Rejected: `fund()` takes the member-wallet address we specify, so no fallback is needed (decided 2026-09-27). |
| Rewrite the app in React Native / Expo to use `useFundSolanaWallet` | Rewrite of the whole app for one button. Also Expo gets only MoonPay and Coinbase, not Stripe or Meld. |
| `WKWebView` inside the app | Apple Pay and 3-D Secure redirects are unreliable, and users cannot see the domain on a payment page. |
| Destination address as a URL parameter | Phishing risk: a crafted link funds someone else's wallet. |
| Treat the `confirmed` callback as a credit | Callback is client-reported and arrives before the USDC does. Only on-chain USDC counts. |
| Pause kept as a record in `trading` (and a copy in `treasury`) fed by the detection event | A window between detection and the consumer running, and two copies to keep in step. Funding owns it and both read it at check time. |
| Funding sets the cabal's pause flag in the detection transaction | The cabal row belongs to the `cabal` module. Cross-module writes go through events ([dependency rules](backend-platform.md#dependency-rules-enforced-by-depguard)). |

## Gap between this and the code

The rewrite replaces the old backend rather than refactoring it ([backend-platform.md](backend-platform.md)). What the old backend does today (checked 2026-09-27):

- `CreditUncreditedTreasuryUSDC` (`apps/backend/internal/app/deposit_reconcile.go`), called from the sweep poller, **credits** unexplained treasury USDC as a deposit, minting shares across current holders. This decision reverses that: detect, pause, bounce. The rewrite keeps a reconcile poller only as the detection fallback, with no credit path.
- No Privy webhook endpoint, no `external_deposits` table, no cabal pause, no bounce.
- No onramp sessions or fund page.
- Nothing migrates at cutover: the new backend starts on an empty database. Privy wallets carry over because they live on-chain and in Privy.
- Old treasury funds are test funds only (decided 2026-09-27). At cutover they are wiped: swept to an ops wallet or written off. Members are not cashed out ([Rollout](backend-platform.md#rollout) step 7, [data-model.md](data-model.md#cutover)).
- Routes: `GET /v1/me/balance`, `POST /v1/me/withdrawals`, `POST /v1/groups/{id}/withdraw-to-balance`.

## Open questions

None.

**Deferred.** App Store Guideline 3.1.5 review of card deposits is not a concern now (decided 2026-09-27).

Log: [log/deposits-withdrawals.md](log/deposits-withdrawals.md).
