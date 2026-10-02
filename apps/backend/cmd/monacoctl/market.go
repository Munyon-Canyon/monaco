package main

import (
	"context"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const marketUsage = "usage: monacoctl market tradable <symbol> on|off|auto | thin-prices"

type retentionLock func(context.Context, *pgxpool.Pool) (bool, func(), error)

func toolMarket(env toolEnv) tool { return marketTool(env.environ, clock.Real{}) }

func marketTool(environ []string, clk clock.Clock) tool {
	return marketToolWithLock(environ, clk, holdRetention)
}

func holdRetention(ctx context.Context, pool *pgxpool.Pool) (bool, func(), error) {
	lock := db.NewLock(pool, "poller:market.retention")
	held, err := lock.Hold(ctx)
	if err != nil || !held {
		return held, nil, err
	}
	return true, func() { _ = lock.Release(ctx) }, nil
}

func marketToolWithLock(environ []string, clk clock.Clock, lock retentionLock) tool {
	tradable := func(cfg config.Config, args []string, stdout, stderr io.Writer) int {
		if len(args) != 2 {
			_, _ = fmt.Fprintln(stderr, marketUsage)
			return 2
		}
		override, err := domain.ParseOverride(args[1])
		if err != nil {
			_, _ = fmt.Fprintln(stderr, marketUsage)
			return 2
		}
		ctx := context.Background()
		pool, err := db.Open(ctx, cfg.DB)
		if err != nil {
			return fail(stderr, err)
		}
		defer pool.Close()
		a, err := app.SetTradableOverride(ctx, pool, clk.Now(), args[0], override)
		if err != nil {
			return fail(stderr, err)
		}
		_, _ = fmt.Fprintf(stdout, "%s\ttradable=%t\toverride=%s\tissuer_tradable=%t\tchain_checked=%t\n",
			a.Symbol, a.Tradable(), a.Override, a.IssuerTradable, a.ChainChecked)
		return 0
	}
	thin := func(cfg config.Config, args []string, stdout, stderr io.Writer) int {
		return thinPrices(clk, lock, cfg, args, stdout, stderr)
	}
	return func(args []string, stdout, stderr io.Writer) int {
		return run(map[string]command{"tradable": tradable, "thin-prices": thin}, nil, environ, args, stdout, stderr)
	}
}

func thinPrices(clk clock.Clock, lock retentionLock, cfg config.Config, args []string, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		_, _ = fmt.Fprintln(stderr, marketUsage)
		return 2
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, cfg.DB)
	if err != nil {
		return fail(stderr, err)
	}
	defer pool.Close()
	held, release, err := lock(ctx, pool)
	if err != nil {
		return fail(stderr, err)
	}
	if !held {
		_, _ = fmt.Fprintln(stdout, "skipped: the worker holds the lock")
		return 0
	}
	defer release()
	report, err := app.NewRetention(db.New(pool, ids.Real{}, clk), clk).Tick(ctx)
	if err != nil {
		return fail(stderr, err)
	}
	_, _ = fmt.Fprintf(stdout, "deleted=%d\n", report.Changed)
	return 0
}
