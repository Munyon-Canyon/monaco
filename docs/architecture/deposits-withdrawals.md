# Deposits & withdrawals

**Status:** Decided 2026-09-26 for deposits (crypto and card) and direct-to-treasury transfers (bounced). Withdrawals keep today's behavior, crypto only for MVP. Reconciled 2026-09-27 with [backend-platform.md](backend-platform.md), which wins where the two conflict. The `funding` module owns deposits, onramp sessions, the bounce and withdrawals to an address. The `treasury` module owns fund and cash out. Built in [Rollout](backend-platform.md#rollout) step 3 (flows 5 to 7), step 4 (flow 8) and step 5 (flows 14 and 15).

## Decision

Every deposit, however it starts, ends the same way: **USDC lands in the user's Privy member wallet on Solana, and that on-chain USDC is their platform balance.** From there it can fund a cabal. The backend never needs to know how the USDC got there. The member wallet is the user's existing Privy wallet, reused on sign-in and never recreated, even though the new backend starts on an empty database.

Two ways in:

1. **Crypto (unchanged).** The app shows the user's member-wallet address. They send Solana USDC to it from an exchange or wallet.
2. **Card / bank (new).** Tapping **Deposit → Card** opens a Monaco-hosted web page that renders Privy's fiat on-ramp. The on-ramp provider (Stripe, Meld, MoonPay or Coinbase, chosen by Privy by region) sells the user USDC and sends it straight to the same member-wallet address on `solana:mainnet`. From that moment the flow is identical to a crypto deposit.

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
3. **Resolve.** The page exchanges the token with the API for the user's member-wallet address and suggested amount. **The destination address never travels in the URL.** A URL parameter would let a phishing link point a user's card purchase at an attacker's wallet.
4. **Authenticate with Privy.** The hook requires a Privy-authenticated user in the page, so the page runs Privy login for the same Privy account. `SFSafariViewController` keeps Safari cookies, so this happens once per device. The app's Swift SDK session does not carry into Safari, which is why the page logs in itself.
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
- `environment: 'sandbox'` in dev, `production` in prod, chosen at build time.

## Crypto deposit flow

Unchanged for the user. The deposit screen shows the copyable member-wallet address (no in-app amount entry). Any USDC sent there is platform balance. See [architecture.md](../architecture.md) (Deposit and fund a cabal).

[Flow 5](backend-platform.md#flows): the `funding` module's deposit poller scans member wallets with a bounded worker pool ([Concurrency rules](backend-platform.md#concurrency-rules)) and appends `deposit.credited` for each new inbound transfer. Consumers are `treasury`, which writes the `user_txns` deposit entry so the member ledger reconciles with on-chain balance, `notify`, `identity` and analytics. `identity` sets `users.first_deposit_at` on the first deposit of at least $10, with a guarded update `WHERE first_deposit_at IS NULL`; `referrals` reads it to unlock the user's handle as a referral code (decided 2026-09-27, [referrals.md](referrals.md#codes)). The poller runs on one worker under its advisory lock ([Deploy rule 1](backend-platform.md#deploy-and-observability)), and every tick logs what it scanned and found, including nothing ([Logs as evidence](backend-platform.md#logs-as-evidence)).

The deposit screen becomes two options: **Card** (opens the fund page) and **Crypto** (shows the address).

## Fund

[Flow 7](backend-platform.md#flows), owned by the `treasury` module: `FundCabal` from `POST /v1/cabals/{id}/fund` moves an exact amount of platform balance from the member wallet to the cabal treasury through a Privy transfer, waits for confirmation, and mints share units at the live price. It appends `cabal.fund_submitted`, then `cabal.funded`. Its outcomes, including `InsufficientFunds`, `CabalPaused`, `PrivyUnavailable` and the `after-sign` and `before-commit` crash points, are the row in its [flow file](backend-platform.md#outcomes-as-a-map). The balance and share rules are in [architecture.md](../architecture.md) (Deposit and fund a cabal) and [data-model.md](data-model.md#double-entry-ledgers).

The transfer's signature is stored before broadcast, like every treasury-touching transfer, so the external-deposit classifier below never mistakes a sweep for a stray transfer.

## Direct transfers to a cabal treasury are not allowed

Money reaches a cabal treasury only through Monaco's flow: platform balance → **Fund this cabal** → sweep. Anything else that lands in a treasury is sent back, and the cabal's trading pauses until it is. This is [flow 8](backend-platform.md#flows), owned by the `funding` module.

### Warn up front

- The deposit screen only ever shows the user's **own member-wallet address**, with the line "Only send USDC on Solana to this address."
- Wherever a treasury address appears (cabal detail, explorer links), it is labelled **"Cabal treasury. Do not send funds here. Transfers are returned."** No copy button; an explorer link only.
- The fund-this-cabal sheet repeats it: "To add money to this cabal, use Fund. Sending USDC straight to the treasury will be returned and pauses the cabal's trading."

### Detect

Two paths, same handler (the treasury watcher):

1. **Privy webhook** `wallet.funds_deposited`, subscribed for every treasury wallet, received by a `funding` HTTP adapter. Verified with Privy's Svix signature headers (`svix-id`, `svix-timestamp`, `svix-signature`). Privy delivers at least once, so the handler dedupes on the transaction signature.
2. **Surplus reconcile poller** (every minute per cabal, in the worker under its advisory lock): treasury on-chain balance per asset versus what the treasury ledger says it should be, read through the `treasury` query port. Catches anything a webhook missed.

The handler classifies each inbound transfer by its signature:

- The signature belongs to a Monaco-initiated transfer → ours, ignore. The handler asks the query ports of every module that signs from or into a treasury: `trading` (swaps), `treasury` (fund sweeps, cash outs) and `funding` (bounces). Each stores its signature **before** broadcasting, so a webhook can never arrive for our own transfer before we know about it.
- Otherwise → **external deposit**.

### Respond

One `uow.Do`:

1. Insert `external_deposits` row: `signature` (unique), `cabal_id`, `sender` (owner of the source token account), `mint`, `amount` (`money.BaseUnits`), `status = detected`. An unresolved row is itself the pause (below).
2. Append `cabal.external_deposit_detected`.
3. Write the pause reason and, when it is the cabal's first open reason, append `cabal.paused` ([Pause](#pause)).
4. Nothing else. Consumers do the rest ([flow 8](backend-platform.md#flows)): `funding` sends the bounce, `admin` shows it, and `notify` tells every cabal member from `cabal.paused`.

### Pause

`funding` owns the pause. A cabal is paused while it has an unresolved external deposit or an ops pause, both recorded in funding with a reason (`external_deposit` or `ops`). Ops pauses go through funding too. `trading` and `treasury` read the pause through funding's query port at check time, so the pause takes effect in the same transaction that records it, with no event lag. There is no `cabals.trading_paused_at` column. The pause ends when its last reason resolves.

**Pause events.** `funding` appends `cabal.paused` in the transaction that opens a cabal's first pause reason, and `cabal.resumed` in the transaction that closes its last one. Opening a second reason or closing one of several appends nothing. Nothing reads these events to decide whether a cabal is paused; the query port stays the check. `notify` consumes both to push every member ([notifications.md](notifications.md#what-notifies-mvp)).

**While paused**, every member is notified, and: no proposal executes (the trade engine's `CabalPaused` check, [trade-execution.md](trade-execution.md#stage-2-trade-engine)), no fund-to-cabal sweeps and no cash outs (`CabalPaused` on flows 7 and 14). Voting, chat and comments continue. The reason: until the stray money is gone, the treasury holds value no one owns, so pot value, share price, every mint of share units and every cash-out payout would be wrong.

### Bounce

The `funding` module's consumer of `cabal.external_deposit_detected` sends the **same mint and amount** back to the sender, from the treasury, signed through Privy with the app authorization key, fee paid by the relayer. Its state lives on the `external_deposits` row: signature stored before broadcast, then the same guarded update and sweeper recovery as a swap ([trade-execution.md](trade-execution.md#stage-3-swap-layer)). Exactly one bounce per external deposit, enforced by a guarded update from `detected`. The bounce writes no ledger entries. The stray inflow never entered the treasury ledger, so inflow and return net to nothing, and `cabal_txns` holds only Monaco's own movements.

On confirm, one `uow.Do` sets `external_deposits.status = returned` and appends `cabal.external_deposit_bounced`. When the cabal has no other unresolved external deposits and no ops pause, the pause ends in that transaction, which also appends `cabal.resumed`; `notify` tells every member that trading resumed.

### Edge cases

| Case | Handling |
| --- | --- |
| Sender is an exchange hot wallet | Bounce still goes to the sending address. The exchange may not credit it back to the user. That is why the warnings exist. Ops can override the return address on request after verifying the user. |
| Dust (below $1) | Relayer fee would exceed the value. Do not bounce, do not pause. Record as `ignored_dust`; excluded from pot value. Never swept to a platform wallet. |
| Unknown or spam token | Not in the market catalog → record, do not bounce, do not pause, exclude from pot value. |
| Stock token (xStock) sent directly | Same as USDC: bounce same mint and amount, pause meanwhile. |
| Bounce fails (e.g. sender account closed) | Status `bounce_failed`, alert ops, cabal stays paused. Ops decides: retry to a new address or hold as unclaimed. |
| Several stray transfers at once | Each gets its own row and bounce. Trading resumes only when all are resolved. |

Member wallets are **not** covered by this: sending USDC to your own member wallet from outside is the intended crypto deposit.

## Withdrawals

Behavior unchanged for the user. Both are rebuilt in Rollout step 5. A banned or suspended user can still cash out and withdraw: a ban stops them acting in cabals, never from taking their money out. Account deletion runs a cash out of every stake first, then scrubs personal data and keeps ledger rows. A banned cabal winds down by selling its holdings and returning USDC pro rata through the same cash-out path.

### Cash out

[Flow 14](backend-platform.md#flows), `treasury` module: `CashOut` burns the member's share units, sells holdings if the treasury is short on USDC, and pays USDC to the member wallet, where it is platform balance again. Events `cashout.started`, then `cashout.completed`, `cashout.partial` or `cashout.failed`. Cash out is refused with `CabalPaused` while the cabal is paused, but not because the member is banned. The route is `POST /v1/cabals/{id}/cashouts` with an `Idempotency-Key` ([`cabal` naming](backend-platform.md#decided)).

Flow 14 start errors: `invalid_input` means a malformed request (both `all` and `usdc_micros`, `all: false`, or an amount under the minimum payout). `pot_value_changed` (409, retryable) means the pot moved since the preview: in-flight cash-out reservations exceed the live pot, or the whole stake no longer clears the minimum payout. The app reloads the preview and asks the member to check the amount. `insufficient_shares`, `price_unavailable` and `cash_out_in_progress` are unchanged.

### Withdraw

[Flow 15](backend-platform.md#flows), `funding` module: `Withdraw` sends platform balance to any Solana address the user pastes, from `POST /v1/me/withdrawals` with an `Idempotency-Key`. Events `withdrawal.submitted`, then `withdrawal.confirmed` or `withdrawal.failed`. These replace `withdrawal.sent` in [event-bus.md](event-bus.md#who-publishes-who-subscribes). `treasury` consumes `withdrawal.confirmed` and writes the `user_txns` withdrawal entry.

No fiat off-ramp in MVP: withdrawals are crypto only. Privy's on-ramp docs cover funding only, and a provider off-ramp is a separate decision after MVP.

### Balance

`GET /v1/me/balance` is owned by `funding`. It returns the member wallet's on-chain USDC minus the amounts of the caller's in-flight fund transfers and withdrawals, so money already on its way out is never shown as spendable. In-flight withdrawals come from `funding`'s own `withdrawals` rows; in-flight fund transfers come from `treasury`'s query port, which owns `fund_transfers` ([data-model.md](data-model.md#decision)).

When the RPC is rate-limited, the display read serves the user's last on-chain reading if it is under 10 minutes old (in-flight amounts still fresh, `as_of` is the reading's time); fund and withdraw never use it, because a command must not act on a stale balance.

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
