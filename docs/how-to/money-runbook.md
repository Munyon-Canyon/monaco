# Money runbook

This page is for the operator on call when money looks stuck. Each section names one state, the signal that points at it, the command that clears it, and what to check first. The log messages come from [Log messages](../reference/logs.md). A message listed there as an alert pages, and a degraded one only warns.

Nothing here moves money by hand. Every command re-drives a state machine that already exists, through its guarded update, so running one twice is safe unless a section says otherwise. When a section says "no operator command", escalate to an engineer with the row's `ref` and the log lines.

## Find every stuck row

Start here. The admin API lists every money state machine's non-final rows with their age:

    GET /v1/admin/queues                          one count and the oldest age per queue
    GET /v1/admin/queues/stuck-money              every stuck money row, oldest first
    GET /v1/admin/queues/stuck-money?kind=<kind>  one state machine
    GET /v1/admin/queues/stuck-money?older_than=1h&limit=200

Send the request with an admin token (`monacoctl admin token create --name <name>`), as [Admin panel](admin-panel.md) describes. The role is `viewer`. `older_than` defaults to five minutes and cannot exceed 24 hours. The `stuck_<kind>` queues in `GET /v1/admin/queues` use the same five minutes.

| `kind` | Non-final states listed | Section |
| --- | --- | --- |
| `withdrawal` | `created`, `submitted` | [Withdrawals](#withdrawals) |
| `fund_transfer` | `created`, `submitted`, `landed` | [Fund transfers](#fund-transfers) |
| `cash_out_job` | `started`, `selling`, `paying` | [Cash outs](#cash-outs) |
| `cash_out_payout` | `signed`, `broadcast` | [Cash outs](#cash-outs) |
| `wind_down` | `running` | [Wind-downs](#wind-downs) |
| `evm_forward` | `pending`, `failed` | [Base deposits](#deposits-from-an-evm-chain) |
| `external_deposit` | `detected`, `bouncing`, `bounce_failed` | [Direct transfers to a treasury](#direct-transfers-to-a-treasury) |
| `onramp_session` | `confirmed`, `submitted`, and `created` or `opened` past `expires_at` | [Card deposits](#card-deposits) |
| `deposit_candidate` | `pending` | [A Solana deposit that never credited](#a-solana-deposit-that-never-credited) |

Swaps have their own queue, `stuck_txns`, and are in [Swaps](#swaps).

The `ref` of a row is its id, except for a deposit candidate (`<tx signature>:<wallet>`), a cash-out payout (`<job id>:<attempt>`) and a wind-down (the cabal id).

The gauge `monaco_oldest_unresolved_money_intent_seconds` is the age of the oldest withdrawal, fund transfer or cash-out job that has not ended. It is the signal for the alert in #5849. When it climbs, list `kind=withdrawal`, `kind=fund_transfer` and `kind=cash_out_job`.

## Swaps

| Signal | Command | Check first |
| --- | --- | --- |
| `stuck_txns` count above zero | `monacoctl swaps force-resolve --signature <sig> --to confirmed\|failed [--out-amount <units>] --reason <text>` | Read the signature on an explorer. Resolve to what the chain says. `confirmed` needs the out amount. The command asks before it writes unless you pass `--yes`, and logs `trading.swap.force_resolved`. |
| `trading.routed_swap.parked` | `monacoctl swaps parked`, then `swaps unpark --swap <id> --reason <text>` or `swaps fail-parked --swap <id> --reason <text>` | A parked swap had no receipt. Check the EVM receipt first. Failing a swap that landed loses the leg. |
| `trading.bridge_back.failed` | `monacoctl swaps failed-bridge-backs`, then `swaps retry-bridge-back --swap <id> --reason <text>` | Confirm with Privy that the earlier bridge did not finish. |
| `trading.evm_cash_return.queued` | `monacoctl swaps return-evm-cash --swap <id> --reason <text>` | Compare the landed amount with the on-chain amount in the log line. |
| `trading.evm_cash_return.uncovered` | None: the return already wrote the gap off | The wallet held less than the ledger. Find where the missing cash went before the next cash out. |
| Cash-out sales bigger than the pot cash | `monacoctl swaps overstated-sells` | The command lists them. It changes nothing. |

## Events and dead letters

| Signal | Command | Check first |
| --- | --- | --- |
| `unpublished_events` count above zero | `GET /v1/admin/queues/unpublished-events` | The relay publishes them. A count that grows means the relay is down: see `bus.relay.failed` in the logs. |
| `dead_letters` count above zero, `admin.dead_letter.recorded` | `monacoctl deadletter list`, then `deadletter retry <seq>` or `deadletter retry --all --consumer <durable>` | Read the error before you retry. A retry runs the handler again, and money handlers converge, so it is safe. |
| A dead letter you want to drop | `POST /v1/admin/dead-letters/{id}/discard` (operator) | **Do not discard a letter for a consumer owned by `funding`, `treasury` or `trading`.** Nothing guards it yet (#6163), and a discarded deposit, withdrawal or fund event is never replayed. Retry it, or escalate. |

## Withdrawals

A Solana withdrawal is `created`, then `submitted` with a stored signature, then `confirmed` or `failed`. The withdrawal poller follows it. There is no operator command for a Solana withdrawal.

| Signal | What it means | What to do |
| --- | --- | --- |
| `kind=withdrawal`, status `created` older than five minutes | The poller has not sent it yet. | Check `funding.withdrawal.broadcast_failed` and the relayer: `monacoctl relayer balance` must be above 0.001 SOL. Top up the relayer if it is low. |
| `kind=withdrawal`, status `submitted` | The signature is stored and the poller is waiting for the chain. | Read the signature on an explorer. The poller settles it from the chain status, or fails it once the signature can no longer land. Do not re-send. |
| `funding.withdrawal.evm_pending` (alert) | An EVM withdrawal has been pending at Privy for more than 30 minutes. | Open the Privy action id from the log line. If Privy shows it running, wait. Never start a second transfer: the first one holds the member's hold. If Privy shows it stuck, escalate with the withdrawal id. |
| `funding.withdrawal.needs_support` (alert, repeats every 6 hours per withdrawal) | The Privy action failed after a step was broadcast. | The hold stays and the poller never retries, because a retry would send a second transfer. Escalate with the withdrawal id and the action id. `monacoctl funding withdrawal list-support` lists every withdrawal in this state (id, user, amount, age, source signature, action id); it reads the database and Privy, so it survives a restart. Once support has recovered or confirmed the money, settle the row with `monacoctl funding withdrawal resolve <id> --to failed\|confirmed --reason <text>`. `confirmed` (delivered) means support confirmed the funds reached the destination, and is refused while the chain shows the transfer failed or never landed. `failed` releases the hold and is refused while the chain shows the funds left (a Solana step that landed or is still processing, an EVM step that landed or is pending, a succeeded action); when the Solana step landed and support returned the USDC to the member's wallet, add `--refund-signature <signature>` with the finalized refund transaction (refunded). Both outcomes need the source signature on the row and refuse a withdrawal whose Privy action is still pending. `--override` books it anyway. The command logs `funding.withdrawal.support_resolved` with the operator, the reason and the chain facts. |

## Fund transfers

A fund transfer moves a member's USDC into a cabal treasury, then mints shares when it settles.

| Signal | What to do |
| --- | --- |
| `kind=fund_transfer` older than five minutes | The fund settler follows `created`, `submitted` and `landed`. Check `treasury.fund.sign_failed`, `treasury.fund.broadcast_failed` and `treasury.fund.mint_waiting` for the cause. `mint_waiting` means the settler is waiting for a price. |
| `treasury.fund.failed` | The transfer ended and the hold is released. Nothing to do. |

There is no operator command. Escalate a transfer that stays `landed` for more than an hour with its id.

## Cash outs

| Signal | What to do |
| --- | --- |
| `kind=cash_out_job`, `selling` | `treasury.cashout.sale_stuck` and `trading.cashout_sell.blocked` name the cause. A blocked sale settles on the cash on hand, partial or failed, with the share units returned. |
| `kind=cash_out_job`, `paying`, or `kind=cash_out_payout` | `treasury.cashout.broadcast_failed` shows the RPC message. The payout is re-signed and re-sent from the stored signature, never twice. |
| `trading.cashout.plan_short` | The plan could not raise the cash. Run `monacoctl swaps overstated-sells`. |

There is no operator command. Escalate a job that stays in one state for more than an hour with its id.

## Wind-downs

A banned cabal winds down by selling its holdings and paying members pro rata.

| Signal | What to do |
| --- | --- |
| `kind=wind_down`, `treasury.winddown_stuck` | The wind-down retries each member with a doubling wait past 10 attempts and logs the attempts and holders left. After fixing the cause, run `monacoctl ops winddown --cabal <id>` to reset the attempts (it also reopens a completed one). `treasury.winddown_member_failed` names the member and the code, but it logs at debug level, so production logs do not show it. |

There is no operator command. Escalate with the cabal id.

## Deposits from an EVM chain

A deposit on Base is forwarded by Privy to the member's Solana wallet. The forward row is `pending`, then `arrived` or `failed`. [Verify an EVM deposit](verify-an-evm-deposit.md) shows the whole path.

| Signal | What it means | What to do |
| --- | --- | --- |
| `funding.evm_forward.stuck` (alert) | A forward has been `pending` for more than 30 minutes. | Open the Privy execution id (`privy_execution_id`). If it is still running, wait. |
| `funding.evm_forward.failed` (alert) | The forward failed. The member's USDC sits on Base, uncredited. | List `kind=evm_forward` to see the row. Read the execution in Privy. Escalate with the forward id. No command re-sends it, and the poller never retries a failed forward. |
| `funding.evm_forward.unrouted` | Privy had no action for the execution, so the poller asked for a reindex. | Wait one tick. It alerts once per execution. |

## Direct transfers to a treasury

A transfer sent straight into a cabal treasury is recorded as an external deposit and sent back. It is kept out of pot value, so the cabal keeps trading. [Direct transfers](../architecture/deposits-withdrawals.md#direct-transfers-to-a-cabal-treasury-are-not-allowed) has the design.

    monacoctl funding bounce list [status ...]
    GET /v1/admin/queues/bounces

`bounce list` prints one tab-separated line per deposit, oldest first: id, cabal, status, amount, mint, return address (`-` when none), bounce attempts and the time detected. With no status it lists `detected`, `bouncing` and `bounce_failed`. Any external-deposit status is allowed, such as `held` or `returned`. The queue route returns the same rows, each with its `external_deposit_id`.

| Status | Signal | Command |
| --- | --- | --- |
| `bounce_failed` | `funding.bounce.failed` | Read the reason in the log. Set a new return address if the sender's account closed: `monacoctl funding bounce set-return-address <id> <address>`, then `monacoctl funding bounce retry <id>`. A lapsed bounce is re-signed at most three times on its own. |
| `detected` with no return address to use | The sender is an exchange hot wallet. | Verify the member with the exchange first, then `set-return-address`. |
| Cannot be returned | | `monacoctl funding bounce hold <id>` keeps it as unclaimed. It is final, so check the sender twice. |

The `<id>` is the external deposit id from `bounce list`, not the cabal id. Every command logs `funding.bounce.ops` with your user name.

## A Solana deposit that never credited

The deposit watch scans each member wallet, records a candidate for every signature and credits it. Two signals say a deposit slipped through:

- `funding.deposit.residual` (degraded): the wallet holds USDC that the ledger does not explain, on two checks in a row. The gauge `monaco_funding_deposit_residual_wallets` counts them.
- `funding.candidate.unresolved`, or `kind=deposit_candidate` rows: a candidate was recorded and not credited. The gauge `monaco_funding_deposit_candidates_pending_oldest_seconds` is the age of the oldest.

Re-scan the wallet:

    monacoctl funding deposit rescan <wallet>
    monacoctl funding deposit rescan <wallet> --from <signature>

The command marks the wallet's token accounts dirty and resets their cursor. Without `--from` the watch scans again from the wallet's first sight. With `--from` it scans the signatures after that one. The running worker picks it up on its next deposit-watch tick, records the candidates and credits them. Candidates and credits are unique per signature and wallet, so a rescan never credits twice. It costs RPC calls, so rescan one wallet at a time and prefer `--from` for an old wallet. The command prints `rescan <wallet> <accounts>` and fails with `not_found` for a wallet the watch does not track.

There is no manual credit. If a rescan finds the transfer and still does not credit it, read `funding.watch.ignored` and `funding.candidate.dismissed` for the reason, then escalate.

## Card deposits

An on-ramp session tracks the card flow only. It moves no money: the USDC arrives in the member's wallet and the deposit watch credits it. A session `confirmed` or `submitted` for a long time means the provider reported success. Check the member's wallet, and rescan it if the USDC arrived and no credit followed. The expiry poller closes `created` and `opened` sessions after `expires_at`.

## Pause a cabal

If money is going wrong in one cabal, stop it first and look after:

    monacoctl ops pause --cabal <id> --note <text>
    monacoctl ops pause --all --note <text>
    monacoctl ops resume --cabal <id>
    monacoctl ops resume --all

A pause stops proposals, fund sweeps and cash outs. Voting and chat continue. Resume closes only the ops pause.
