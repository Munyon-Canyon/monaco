package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	chainprivy "github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	testflows "github.com/monaco/monaco/apps/backend/internal/testkit/flows"
)

const (
	pruneMinAge          = 24 * time.Hour
	devUserRefused       = "monacoctl dev user delete: refused: only the local dev database"
	devUsersPruneRefused = "monacoctl dev users prune: refused: only the local dev database"
)

var throwawayEmail = regexp.MustCompile(`^dev-[0-9a-f]{8}@example\.com$`)

func devUser(cfg config.Config, args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 || args[0] != "delete" {
		_, _ = fmt.Fprintln(stderr, devUsage)
		return 2
	}
	id, err := ids.ParseUserID(args[1])
	if err != nil {
		_, _ = fmt.Fprintln(stderr, devUserSubjectLine)
		return 2
	}
	if !cfg.LocalDev() {
		_, _ = fmt.Fprintln(stderr, devUserRefused)
		return 1
	}
	return reportDevUserDelete(
		id,
		testflows.DeleteDevUser(context.Background(), cfg, id, newBalances(cfg)),
		stdout,
		stderr,
	)
}

func reportDevUserDelete(id ids.UserID, err error, stdout, stderr io.Writer) int {
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl dev user delete: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "deleted dev user %s\n", id)
	return 0
}

func devUsers(cfg config.Config, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "prune" {
		_, _ = fmt.Fprintln(stderr, devUsage)
		return 2
	}
	fs := flag.NewFlagSet("dev users prune", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dry := fs.Bool("dry-run", false, "list the Privy users prune would delete (the default)")
	apply := fs.Bool("apply", false, "delete them")
	if err := fs.Parse(args[1:]); err != nil || (*dry && *apply) || fs.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, devUsage)
		return 2
	}
	if !cfg.LocalDev() {
		_, _ = fmt.Fprintln(stderr, devUsersPruneRefused)
		return 1
	}
	failed, err := pruneDevUsers(context.Background(), cfg, *apply, newBalances(cfg), stdout)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl dev users prune: %v\n", err)
		return 1
	}
	if failed > 0 {
		_, _ = fmt.Fprintf(stderr, "monacoctl dev users prune: %d deletes failed\n", failed)
		return 1
	}
	return 0
}

type chainReads interface {
	SOLBalance(ctx context.Context, addr chain.SolanaAddress) (money.BaseUnits, error)
	TokenBalance(ctx context.Context, owner chain.SolanaAddress, mint chain.Mint) (money.BaseUnits, error)
}

type balances struct {
	reads chainReads
	usdc  chain.Mint
}

func newBalances(cfg config.Config) balances {
	return balances{
		reads: solana.New(cfg, clock.Real{}),
		usdc:  chain.Mint{Address: chain.SolanaAddress(cfg.Solana.USDCMint), Decimals: qaUSDCDecimals},
	}
}

func (b balances) Balances(ctx context.Context, address chain.SolanaAddress) (uint64, uint64, error) {
	const op = "monacoctl.Balances"
	usdc, err := b.reads.TokenBalance(ctx, address, b.usdc)
	if err != nil {
		return 0, 0, errs.Wrap(err, errs.CodeOf(err), op)
	}
	sol, err := b.reads.SOLBalance(ctx, address)
	if err != nil {
		return 0, 0, errs.Wrap(err, errs.CodeOf(err), op)
	}
	return usdc.Uint64(), sol.Uint64(), nil
}

type pruneTally struct{ matched, deleted, skipped, failed int }

func pruneDevUsers(
	ctx context.Context, cfg config.Config, apply bool, wallets app.WalletBalances, stdout io.Writer,
) (int, error) {
	clk := clock.Real{}
	client, err := chainprivy.New(cfg, clk)
	if err != nil {
		return 0, err
	}
	users, err := client.ListUsers(ctx)
	if err != nil {
		return 0, err
	}
	cutoff := clk.Now().Add(-pruneMinAge)
	var tally pruneTally
	for _, u := range users {
		if throwawayEmail.MatchString(u.Email) && u.Identities == 1 && !u.CreatedAt.IsZero() &&
			u.CreatedAt.Before(cutoff) {
			tally.matched++
			pruneOne(ctx, client, wallets, u, apply, &tally, stdout)
		}
	}
	_, _ = fmt.Fprintf(stdout, "%d of %d Privy users match: %d deleted, %d skipped as funded, %d failed\n",
		tally.matched, len(users), tally.deleted, tally.skipped, tally.failed)
	return tally.failed, nil
}

func pruneOne(
	ctx context.Context, client *chainprivy.Client, wallets app.WalletBalances, u chainprivy.ListedUser, apply bool,
	tally *pruneTally, stdout io.Writer,
) {
	usdc, lamports, err := walletFunds(ctx, client, wallets, u.ID)
	if err != nil {
		tally.failed++
		_, _ = fmt.Fprintf(stdout, "%s %s failed: %v\n", u.ID, u.Email, err)
		return
	}
	_, _ = fmt.Fprintf(stdout, "%s %s created %s usdc_micros %d lamports %d", u.ID, u.Email,
		u.CreatedAt.Format(time.RFC3339), usdc, lamports)
	switch {
	case usdc > 0 || lamports > app.DustLamports:
		tally.skipped++
		_, _ = fmt.Fprintln(stdout, " skipped: funded, sweep it first")
	case !apply:
		_, _ = fmt.Fprintln(stdout)
	default:
		if err := client.DeleteUser(ctx, u.ID); err != nil && errs.CodeOf(err) != errs.CodeNotFound {
			tally.failed++
			_, _ = fmt.Fprintf(stdout, " failed: %v\n", err)
			return
		}
		tally.deleted++
		_, _ = fmt.Fprintln(stdout, " deleted")
	}
}

func walletFunds(
	ctx context.Context, client *chainprivy.Client, wallets app.WalletBalances, id chainprivy.UserID,
) (uint64, uint64, error) {
	addresses, err := client.ListUserWallets(ctx, id)
	if err != nil {
		return 0, 0, err
	}
	if len(addresses) == 0 {
		return 0, 0, errs.New(
			errs.CodeNotFound,
			"monacoctl.walletFunds",
			slog.String("reason", "no wallet found, skipped"),
		)
	}
	var usdc, lamports uint64
	for _, address := range addresses {
		u, l, err := wallets.Balances(ctx, address)
		if err != nil {
			return 0, 0, err
		}
		usdc += u
		lamports = max(lamports, l)
	}
	return usdc, lamports, nil
}
