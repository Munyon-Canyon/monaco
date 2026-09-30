package main

import (
	"context"
	"fmt"
	"io"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

const marketUsage = "usage: monacoctl market tradable <symbol> on|off|auto"

func toolMarket(env toolEnv) tool { return marketTool(env.environ, clock.Real{}) }

func marketTool(environ []string, clk clock.Clock) tool {
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
		_, _ = fmt.Fprintf(stdout, "%s\ttradable=%t\toverride=%s\tissuer_tradable=%t\n",
			a.Symbol, a.Tradable(), a.Override, a.IssuerTradable)
		return 0
	}
	return func(args []string, stdout, stderr io.Writer) int {
		return run(map[string]command{"tradable": tradable}, nil, environ, args, stdout, stderr)
	}
}
