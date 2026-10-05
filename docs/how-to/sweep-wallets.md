# Sweep wallets

`scripts/sweep-wallets.sh` moves all mainnet USDC out of Monaco-controlled Privy wallets to one address. Use it to recover test USDC that a crashed or stopped QA run left in a member wallet or a cabal treasury.

Sweeping is cleanup for throwaway QA users and cabals. It is not a product path. Members move money with fund, cash out and withdraw in the app.

## Before you run it

- Run it against the local or staging database only. The script reads `DATABASE_URL` and the Privy app from `.env.local`.
- A swept treasury no longer matches its cabal's ledger. The cabal still records holdings and shares that the treasury no longer has. Sweep only cabals you will throw away.
- A swept member wallet drops that user's platform balance to zero.
- The script moves USDC only. It does not sell stock tokens or move SOL. Cash out a cabal first, so its treasury holds only USDC.
- The relayer (`RELAYER_PRIVATE_KEY`) pays the SOL fees, so it needs SOL. The script never drains the relayer.

## Run it

Run every command from the repo root. The script loads `.env.local` through dotenvx.

1. Do a dry run first. It reads balances, prints the plan and sends nothing:

        scripts/sweep-wallets.sh --destination <solana_address> --dry-run

2. Check the dry-run output. Each source prints one line: `dry-run would sweep` with its amount, `skip` with its balance and the reason, or `fail` with the error.

3. Run the same command without `--dry-run`:

        scripts/sweep-wallets.sh --destination <solana_address> --source <wallet>

4. At the first prompt, type this phrase exactly:

        I UNDERSTAND THIS MAY MESS WITH PROD

5. At the second prompt, paste the destination address again. A wrong phrase or a different address stops the run before it sends anything. There is no `--yes`.

6. Open each printed `tx=` signature on [solscan.io](https://solscan.io) and check that it landed. The script broadcasts each transfer and does not wait for confirmation.

The wrapper runs this command, which you can also run directly:

    scripts/with-dotenv-local.sh go run -C apps/backend ./cmd/sweep-member-to-address \
      --destination <solana_address> --dry-run

## Flags

| Flag | Required | Meaning |
| --- | --- | --- |
| `--destination` | yes | The Solana address that receives the USDC. |
| `--source` | no | Sweep only this wallet. Repeat the flag, or comma-separate addresses in one value. |
| `--all` | no | Sweep every Solana wallet in the Privy app (paginated `GET /v1/wallets?chain_type=solana`), not only the database rows. |
| `--dry-run` | no | Print balances and the plan. Send nothing and skip the prompts. |

You cannot combine `--all` with `--source`.

## Which wallets it sweeps

- With no `--source` and no `--all`, it sweeps every row of `user_wallets` (labelled `member`) and `treasury_wallets` (labelled `treasury`).
- With `--source`, it sweeps only the listed addresses. An address in neither table is labelled `explicit`, and the script looks up its Privy wallet id in the Privy app. Use this after `just reset db`, when the tables are empty but the Privy wallets still hold USDC.
- With `--all`, it sweeps every wallet Privy returns. A wallet that is in neither table is labelled `privy`.

The script skips a wallet that holds no USDC, a wallet that is the destination, and the relayer. It fails a wallet that is not a Privy wallet of this app, and goes on with the rest. It exits 1 when any wallet failed.

## Env

`.env.local` must set:

- `DATABASE_URL`
- `PRIVY_APP_ID`, `PRIVY_APP_SECRET` and `PRIVY_VERIFICATION_KEY`
- `PRIVY_AUTHORIZATION_PRIVATE_KEY` and `PRIVY_AUTHORIZATION_KEY_ID`, which sign for the wallets
- `RELAYER_PRIVATE_KEY`, the fee payer
- `SOLANA_RPC_URL`, optional, which defaults to the public mainnet RPC
