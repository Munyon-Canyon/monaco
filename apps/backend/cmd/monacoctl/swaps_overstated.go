package main

import (
	"context"
	"fmt"
	"io"

	chainadapters "github.com/monaco/monaco/apps/backend/internal/modules/trading/adapters/chain"
	tradingapp "github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

func listOverstatedSells(clk clock.Clock) command {
	return func(cfg config.Config, args []string, stdout, stderr io.Writer) int {
		if len(args) != 0 {
			_, _ = fmt.Fprintln(stderr, swapsUsage)
			return 2
		}
		ctx := observability.WithLogger(context.Background(), observability.NewLogger(cfg, stderr))
		usdc := chain.Mint{Address: chain.SolanaAddress(cfg.Solana.USDCMint), Decimals: 6}
		var report tradingapp.OverstatedSellsReport
		pool, err := db.Open(ctx, cfg.DB)
		if err == nil {
			defer pool.Close()
			report, err = tradingapp.OverstatedSells(ctx, pool, chainadapters.NewReader(solana.New(cfg, clk)), usdc)
		}
		if err != nil {
			return fail(stderr, err)
		}
		if len(report.Sells) == 0 {
			_, _ = fmt.Fprintln(stdout, "no overstated sells")
		}
		for _, s := range report.Sells {
			_, _ = fmt.Fprintf(stdout, "swap=%s cabal=%s symbol=%s signature=%s recorded=%d chain=%d overstated=%d\n",
				s.SwapID.UUID(), s.CabalID.UUID(), s.Symbol, s.Signature, s.Recorded, s.OnChain, s.Overstated())
		}
		for _, c := range report.Cabals {
			_, _ = fmt.Fprintf(stdout, "cabal=%s sells=%d overstated_micros=%d\n", c.CabalID.UUID(), c.Sells, c.Micros)
		}
		for _, s := range report.Unread {
			_, _ = fmt.Fprintf(stdout, "unread swap=%s cabal=%s signature=%s recorded=%d code=%s\n",
				s.SwapID.UUID(), s.CabalID.UUID(), s.Signature, s.Recorded, s.Code)
		}
		return 0
	}
}
