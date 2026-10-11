package main

import (
	"context"
	"fmt"
	"io"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const listingsUsage = "usage: monacoctl listings verify [--all] | override <listing-id> on|off|auto"

type listingVerifierFor func(config.Config) app.ListingVerifier

func toolListings(env toolEnv) tool {
	return listingsTool(env.environ, clock.Real{}, func(cfg config.Config) app.ListingVerifier {
		return liveListingVerifier(cfg)
	})
}

func liveListingVerifier(cfg config.Config) *adapters.ListingVerifier {
	rpc := httpclient.New("evm-rpc", httpclient.WithTimeout(cfg.Timeouts.RPC))
	return adapters.NewListingVerifier(rpc, cfg.EVM.RPCURL, solana.New(cfg, clock.Real{}))
}

func listingsTool(environ []string, clk clock.Clock, verifier listingVerifierFor) tool {
	verify := func(cfg config.Config, args []string, stdout, stderr io.Writer) int {
		return verifyListings(cfg, clk, verifier(cfg), args, stdout, stderr)
	}
	return func(args []string, stdout, stderr io.Writer) int {
		return run(map[string]command{"verify": verify, "override": overrideListing}, nil, environ, args, stdout,
			stderr)
	}
}

func verifyListings(
	cfg config.Config, clk clock.Clock, verifier app.ListingVerifier, args []string, stdout, stderr io.Writer,
) int {
	all := len(args) == 1 && args[0] == "--all"
	if len(args) > 0 && !all {
		_, _ = fmt.Fprintln(stderr, listingsUsage)
		return 2
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, cfg.DB)
	if err != nil {
		return fail(stderr, err)
	}
	defer pool.Close()
	report, err := app.NewListingSync(db.New(pool, ids.Real{}, clk), ids.Real{}, clk, nil, verifier).Verify(ctx, all)
	if err != nil {
		return fail(stderr, err)
	}
	for _, c := range report.Verified {
		_, _ = fmt.Fprintf(stdout, "verified\t%s\t%s\t%s\t%s\tdecimals=%d\n",
			c.ID, c.Issuer, c.Symbol, c.Network, c.VerifiedDecimals)
	}
	for _, c := range report.Failed {
		_, _ = fmt.Fprintf(stdout, "failed\t%s\t%s\t%s\t%s\t%s\n", c.ID, c.Issuer, c.Symbol, c.Network, c.Reason())
	}
	_, _ = fmt.Fprintf(stdout, "verified=%d failed=%d\n", len(report.Verified), len(report.Failed))
	return min(len(report.Failed), 1)
}

func overrideListing(cfg config.Config, args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 {
		_, _ = fmt.Fprintln(stderr, listingsUsage)
		return 2
	}
	id, idErr := uuid.Parse(args[0])
	o, err := domain.ParseOverride(args[1])
	if idErr != nil || err != nil {
		_, _ = fmt.Fprintln(stderr, listingsUsage)
		return 2
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, cfg.DB)
	if err != nil {
		return fail(stderr, err)
	}
	defer pool.Close()
	l, err := app.SetListingOverride(ctx, pool, id, o)
	if err != nil {
		return fail(stderr, err)
	}
	_, _ = fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\toverride=%s\tenabled=%t\n", l.ID, l.Issuer, l.Symbol, l.Network, o,
		l.Enabled)
	return 0
}
