---
name: money-change
description: Checklist for any backend change that moves or records money, including ledgers, shares, swaps, transfers and balances. Use before writing or reviewing code that touches USDC, token amounts, share math, the relayer or Privy signing.
---

# Money change

Money bugs are silent and permanent. Every item below is required. A PR that touches money says in its body how it meets each one.

## Types

- USDC amounts are `money.Micros`. Signed deltas and P&L are `money.SignedMicros`. Token amounts are `money.BaseUnits`, which carry their decimals. All three live in `apps/backend/internal/platform/money/money.go`.
- Build them with `money.ParseMicros`, `money.MicrosFromUint64` or `money.NewBaseUnits` at the boundary. Arithmetic goes through `Add`, `Sub` and `Delta`, which return an error on overflow and underflow. Handle the error. Never unwrap to `uint64` to do math.
- Share math is `money.MulDiv(a, b, c)`. It multiplies before it divides through `math/big` and rounds down, so rounding favors the pot. Never divide first.
- Floats are banned in money code by `forbidigo` in `apps/backend/.golangci.yml`. Display formatting is the client's job.
- Chain values use the types in `apps/backend/internal/platform/chain/chain.go`: `chain.SolanaAddress`, `chain.Signature` and `chain.Mint`.

## The write

1. **One transaction.** Every balance change happens inside `uow.Do` (`db.UnitOfWork.Do` in `apps/backend/internal/platform/db/uow.go`). Nothing else opens or commits a transaction, and `forbidigo` fails `Begin` or `Commit` outside `apps/backend/internal/platform/db`.
2. **Event in the same transaction.** Append the event with `tx.Events.Append` inside the same `uow.Do` as the ledger write, as `apps/backend/internal/modules/system/app/record_ping.go` does. A consumer that reacts to money reads the event, never the other module's tables.
3. **Guarded update.** Every state or balance change is an `UPDATE ... WHERE id = $1 AND <expected state>` returning the row count (`:execrows` in sqlc). Zero rows means someone else moved it first. Return a conflict or treat it as already done. Never read, check in Go, then write. `db.GuardedUpdate` in `apps/backend/internal/platform/db/guarded.go` enforces one row. `EchoPing` in `apps/backend/queries/system/pings.sql` is the guarded pattern.
4. **Idempotent.** Commands carry an idempotency key. Consumers are deduped by `event_deliveries` (see the `nats-consumer` skill), but a handler must still converge when it runs twice.
5. **No outbound call inside the transaction.** Sign, quote and broadcast happen outside `uow.Do`. Record intent before the call and the result after it, each in its own transaction.

## On-chain transfers

The relayer package is `apps/backend/internal/platform/chain/relayer`.

1. **Build and sign.** `relayer.NewTransfers(r, signer).Build(ctx, spec)` returns a `relayer.SignedTx`. The member or treasury wallet signs through Privy (`apps/backend/internal/platform/chain/privy/sign.go`), then the relayer co-signs as fee payer. `SignedTx.Signature` is final before anything is sent.
2. **Store the signature before broadcast.** Write `SignedTx.Signature` and `LastValidBlockHeight` to the row in a `uow.Do` that commits before you call `Broadcast`. After a crash, the stored signature is how recovery finds the transaction on chain instead of sending it twice.
3. **Broadcast.** `Transfers.Broadcast(ctx, signed)` sends it and fails when the RPC reports a different signature.
4. **Confirm.** A later step confirms by the stored signature and moves the row with a guarded update.
5. **Relayer floor.** The relayer must hold more than `relayer.FloorLamports` (0.001 SOL). `Relayer.CheckFloor` returns `relayer_underfunded` at or under it, and `relayer.CheckBoot` fails api and worker boot in staging and production. Never lower the floor to get a test green.

## Logs

- Inside `uow.Do`, log at `Debug` only. The transaction can roll back and retry, so an `Info` line there can describe money that never moved. `db.UnitOfWork.Do` itself logs `tx.committed` or `tx.rolled_back` at `Info`. No lint enforces this rule yet, so check it in review.
- After the commit, log the change once with before and after attributes: the balance or state before, the balance or state after, the asset and the owning ID. Register the message in the area's `msgs_<area>.go` under `apps/backend/internal/platform/observability`. `apps/backend/internal/platform/observability/registry_test.go` fails an unregistered message.
- Never log a private key, a Privy token or a raw signed transaction. The handler in `apps/backend/internal/platform/observability/redact.go` masks attrs named like `key`, `token`, `signature` or `secret`, and long base58 and base64 values. Keep attr names honest so redaction applies. `apps/backend/internal/platform/chain/relayer/redaction_test.go` proves no key, token or signature reaches a log line. Add a case there when you add a signing path.

## Tests

- **Property test.** Use `pgregory.net/rapid`, as `apps/backend/internal/platform/money/property_test.go` does. State the invariant: entries sum to zero per asset, shares never mint value, a round trip returns the input, rounding never favors the member.
- **Crash-point test.** Name the fault point in the flow's `outcomes` cell as `crash:<point>`, register a new point in `apps/backend/internal/platform/faultpoint/faultpoint.go`, and hit it with `faultpoint.Hit` where the process could die. Test it with `testkit.CrashAt` from `apps/backend/internal/testkit/crash.go`, or with a flow script like `F00RecordPingCrashAfterPublish` in `apps/backend/internal/testkit/flows/f00.go`. The run crashes at the point, restarts, and must reach the same end state with no double spend. Crash tests carry `//go:build faultpoints`.
- **Guarded-update test.** Run the same transition twice and assert the second changes zero rows and moves no money.
- **Failure outcomes.** Each `errs` code the flow can return is an outcome in the flow's file, `packages/flows/backend/<id>.tsv`, with its own `TestFlow<id>_<Command>_<Outcome>` test.
- **Ledger check.** Ledger rows go through `treasury`'s `app.Ledger` (`PostCabalTxn`, `PostUserTxn`, `SetStatus`), and tests seed a ledger with `testkit.NewLedger`. A module that owns a ledger exports `LedgerCheck(config.Config) replay.LedgerCheck` from its package and reads every mint from that configuration, never a constant. `scripts/gen-registry` registers it, so `monacoctl replay --verify` and `monacoctl verify` both run it.
- Money code gets 100% statement coverage. `apps/backend/coverage.exclude` lists no money package.

## Checklist

- [ ] Amounts are `Micros`, `SignedMicros` or `BaseUnits`, with every arithmetic error handled.
- [ ] Share math uses `money.MulDiv` and rounds in favor of the pot.
- [ ] Each balance change and its event commit in one `uow.Do`.
- [ ] Each state change is a guarded update.
- [ ] No outbound call inside a transaction.
- [ ] A transfer's signature is stored before broadcast.
- [ ] Only `Debug` logs inside `uow.Do`. One post-commit log line has before and after attrs.
- [ ] No secret, key or signed transaction reaches a log.
- [ ] A property test and a crash-point test cover the change.
