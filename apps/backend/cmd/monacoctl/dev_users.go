package main

import (
	"context"
	"fmt"
	"io"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	testflows "github.com/monaco/monaco/apps/backend/internal/testkit/flows"
)

const devUserRefused = "monacoctl dev user delete: refused: only the local dev database"

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
