package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/monaco/monaco/apps/backend/internal/modules/governance"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/coingecko"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

const backfillPricesUsage = `usage: monacoctl backfill prices (--mint <mint> | --all | --queue-all)
  --all backfills the hot listed mints: Popular, held by a cabal, or named by an open proposal
  --queue-all queues every listed, tradable mint and calls nothing; the worker's market.backfill drains the queue`

type priceHistory func(config.Config) app.PriceHistory

func coinGecko(cfg config.Config) *coingecko.Client {
	return coingecko.New(httpclient.New("coingecko", coingecko.Options(cfg)...), cfg.CoinGecko.APIKey)
}

type pricesMode struct {
	mint          string
	all, queueAll bool
}

func parsePricesMode(args []string) (pricesMode, bool) {
	fs := flag.NewFlagSet("backfill prices", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var m pricesMode
	fs.StringVar(&m.mint, "mint", "", "")
	fs.BoolVar(&m.all, "all", false, "")
	fs.BoolVar(&m.queueAll, "queue-all", false, "")
	if fs.Parse(args) != nil || fs.NArg() != 0 {
		return m, false
	}
	modes := 0
	for _, on := range []bool{m.mint != "", m.all, m.queueAll} {
		if on {
			modes++
		}
	}
	return m, modes == 1
}

func backfillPrices(environ []string, history priceHistory, args []string, stdout, stderr io.Writer) int {
	mode, ok := parsePricesMode(args)
	if !ok {
		_, _ = fmt.Fprintln(stderr, backfillPricesUsage)
		return 2
	}
	cfg, err := config.Load(environ)
	if err != nil {
		return fail(stderr, err)
	}
	source := history(cfg)
	if !mode.queueAll && !source.Configured() {
		_, _ = fmt.Fprintln(stderr, "monacoctl: COINGECKO_API_KEY is not set")
		return 1
	}
	parsed, err := parseOptionalMint(mode.mint)
	if err != nil {
		return fail(stderr, err)
	}
	ctx := context.Background()
	pools, err := openPools(ctx, cfg.DB)
	if err != nil {
		return fail(stderr, err)
	}
	defer closePools(pools)
	d := module.Deps{
		Config: cfg, Pool: pools[0], Clock: clock.Real{}, IDs: ids.Real{},
		UoW: db.New(pools[0], ids.Real{}, clock.Real{}),
	}
	m := market.New(d)
	module.NewSet(m, treasury.New(d), governance.New(d))
	backfill := m.Backfill(source)
	if mode.queueAll {
		queued, err := backfill.QueueAll(ctx)
		if err != nil {
			return fail(stderr, err)
		}
		_, _ = fmt.Fprintf(stdout, "backfill prices: queued %d mints\n", queued)
		return 0
	}
	var res app.BackfillResult
	if mode.all {
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
