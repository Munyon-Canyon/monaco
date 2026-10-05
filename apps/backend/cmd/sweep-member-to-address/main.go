package main

import (
	"context"
	"fmt"
	"io"
	"os"

	cabalsqlc "github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	identitysqlc "github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

type walletKind string

const (
	kindMember   walletKind = "member"
	kindTreasury walletKind = "treasury"
	kindPrivy    walletKind = "privy"
	kindExplicit walletKind = "explicit"
)

type sweepSource struct {
	kind   walletKind
	wallet chain.Wallet
}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Environ(), os.Stdin, os.Stdout, os.Stderr))
}

func run(ctx context.Context, args, environ []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags, err := parseSweepFlags(args, stderr)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}
	cfg, err := config.Load(environ)
	if err != nil {
		return fail(stderr, "config", err)
	}
	clk := clock.Real{}
	client, err := privy.New(cfg, clk)
	if err != nil {
		return fail(stderr, "privy", err)
	}
	rpc := solana.New(cfg, clk)
	payer, err := relayer.New(cfg, rpc)
	if err != nil && !flags.dryRun {
		return fail(stderr, "relayer", err)
	}
	var payerAddress chain.SolanaAddress
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "relayer: %v (the dry run goes on; a live run needs RELAYER_PRIVATE_KEY)\n", err)
	} else {
		payerAddress = payer.Address()
	}
	pool, err := db.Open(ctx, cfg.DB)
	if err != nil {
		return fail(stderr, "database", err)
	}
	defer pool.Close()
	sources, note, err := loadSweepSources(ctx, flags, identitysqlc.New(pool), cabalsqlc.New(pool), client)
	if err != nil {
		return fail(stderr, "list wallets", err)
	}
	if err := confirmSweep(stdin, stdout, confirmOpts{
		dest: flags.destination, databaseURL: cfg.DB.URL, sourceNote: note, dryRun: flags.dryRun, sources: sources,
	}); err != nil {
		return fail(stderr, "abort", err)
	}
	runner := sweepRunner{
		out:       stdout,
		dest:      flags.destination,
		dryRun:    flags.dryRun,
		relayer:   payerAddress,
		usdc:      chain.Mint{Address: chain.SolanaAddress(cfg.Solana.USDCMint), Decimals: 6},
		balances:  rpc,
		transfers: relayer.NewTransfers(payer, client),
	}
	outcomes := runner.sweep(ctx, sources)
	printSweepRecap(stdout, flags.destination, flags.dryRun, outcomes)
	t := tallyOutcomes(outcomes)
	_, _ = fmt.Fprintf(stdout, "done mode=%s swept=%d skipped=%d failed=%d dest=%s no_live_tx=%t\n",
		modeName(flags.dryRun), t.ok+t.dryRun, t.skipped, t.failed, flags.destination, flags.dryRun)
	return t.exitCode()
}

func fail(stderr io.Writer, step string, err error) int {
	_, _ = fmt.Fprintf(stderr, "%s: %v\n", step, err)
	return 1
}

func modeName(dryRun bool) string {
	if dryRun {
		return "dry-run"
	}
	return "live"
}
