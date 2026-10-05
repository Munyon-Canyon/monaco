package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/coingecko"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const backfillPricesUsage = "usage: monacoctl backfill prices (--mint <mint> | --all)"

type priceHistory func(config.Config) app.PriceHistory

func coinGecko(cfg config.Config) *coingecko.Client {
	return coingecko.New(httpclient.New("coingecko", coingecko.Options(cfg)...), cfg.CoinGecko.APIKey)
}

func backfillPrices(environ []string, history priceHistory, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("backfill prices", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	mint := fs.String("mint", "", "")
	all := fs.Bool("all", false, "")
	if fs.Parse(args) != nil || fs.NArg() != 0 || (*mint != "") == *all {
		_, _ = fmt.Fprintln(stderr, backfillPricesUsage)
		return 2
	}
	cfg, err := config.Load(environ)
	if err != nil {
		return fail(stderr, err)
	}
	source := history(cfg)
	if !source.Configured() {
		_, _ = fmt.Fprintln(stderr, "monacoctl: COINGECKO_API_KEY is not set")
		return 1
	}
	parsed, err := parseOptionalMint(*mint)
	if err != nil {
		return fail(stderr, err)
	}
	ctx := context.Background()
	pools, err := openPools(ctx, cfg.DB)
	if err != nil {
		return fail(stderr, err)
	}
	defer closePools(pools)
	backfill := app.NewBackfill(db.New(pools[0], ids.Real{}, clock.Real{}), pools[0], clock.Real{}, source)
	var res app.BackfillResult
	if *all {
		res, err = backfill.RunAll(ctx)
	} else {
		res, err = backfill.Run(ctx, []string{parsed.String()})
	}
	_, _ = fmt.Fprintf(stdout, "backfill prices: %d mints, %d calls, %d rows inserted\n",
		res.Mints, res.Calls, res.Rows)
	if err != nil {
		return fail(stderr, err)
	}
	return 0
}

func parseOptionalMint(raw string) (domain.Mint, error) {
	if raw == "" {
		return domain.Mint{}, nil
	}
	return domain.ParseMint(raw)
}
