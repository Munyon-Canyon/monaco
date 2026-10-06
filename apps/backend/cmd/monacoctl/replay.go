package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/tools/ops/replay"
)

const replayUsage = "usage: monacoctl replay --into <database_url> [--to <event_id>] [--verify]"

func toolReplay(env toolEnv) tool { return replayTool(env.environ) }

func replayTool(environ []string) tool {
	return func(args []string, stdout, stderr io.Writer) int {
		fs := flag.NewFlagSet("replay", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		into := fs.String("into", "", "")
		to := fs.String("to", "", "")
		verify := fs.Bool("verify", false, "")
		toID, ok := parseFlags(fs, args, to)
		if !ok || *into == "" {
			_, _ = fmt.Fprintln(stderr, replayUsage)
			return 2
		}
		cfg, err := config.Load(environ)
		if err == nil {
			err = replay.CheckTarget(cfg.DB.URL, *into)
		}
		if err != nil {
			return fail(stderr, err)
		}
		ctx := context.Background()
		pools, err := openPools(ctx, cfg.DB, config.DB{URL: *into, MaxConns: cfg.DB.MaxConns})
		if err != nil {
			return fail(stderr, err)
		}
		defer closePools(pools)
		clk := &replay.Clock{}
		uow := db.New(pools[1], ids.Real{}, clk)
		rep, err := replay.Run(ctx, replay.Options{
			Source: pools[0], Target: pools[1], UoW: uow, Clock: clk,
			Handlers: replayHandlers(cfg, pools[0], pools[1], uow, clk),
			To:       toID, Verify: *verify, Checks: replay.LedgerChecks(cfg),
		})
		_, _ = fmt.Fprintf(
			stdout,
			"replayed %d events: %d applied, %d duplicate\n",
			rep.Events,
			rep.Applied,
			rep.Duplicates,
		)
		if err != nil {
			return fail(stderr, err)
		}
		return printDiffs(stdout, rep.Diffs, *verify)
	}
}

func printDiffs(stdout io.Writer, diffs []string, verify bool) int {
	for _, d := range diffs {
		_, _ = fmt.Fprintln(stdout, d)
	}
	if verify {
		_, _ = fmt.Fprintf(stdout, "verify: %d diffs\n", len(diffs))
	}
	if len(diffs) > 0 {
		return 1
	}
	return 0
}

func parseFlags(fs *flag.FlagSet, args []string, id *string) (uuid.UUID, bool) {
	if fs.Parse(args) != nil || fs.NArg() != 0 {
		return uuid.Nil, false
	}
	if *id == "" {
		return uuid.Nil, true
	}
	parsed, err := uuid.Parse(*id)
	return parsed, err == nil
}

func openPools(ctx context.Context, dbs ...config.DB) ([]*pgxpool.Pool, error) {
	pools := make([]*pgxpool.Pool, 0, len(dbs))
	for _, d := range dbs {
		pool, err := db.Open(ctx, d)
		if err != nil {
			closePools(pools)
			return nil, err
		}
		pools = append(pools, pool)
	}
	return pools, nil
}

func closePools(pools []*pgxpool.Pool) {
	for _, p := range pools {
		p.Close()
	}
}

func registeredSet(cfg config.Config, pool *pgxpool.Pool, uow *db.UnitOfWork, clk clock.Clock) module.Set {
	return registered.Build(module.Deps{
		Config: cfg, Clock: clk, IDs: ids.Real{}, Pool: pool, UoW: uow, HTTPClient: httpclient.New,
	})
}

func registeredConsumers(cfg config.Config, pool *pgxpool.Pool, uow *db.UnitOfWork, clk *replay.Clock) []bus.Consumer {
	return registeredSet(cfg, pool, uow, clk).Consumers()
}

func handlers(cfg config.Config, pool *pgxpool.Pool, uow *db.UnitOfWork, clk *replay.Clock) []bus.HandlerSpec {
	return replay.Handlers(registeredConsumers(cfg, pool, uow, clk))
}

func projectionDurables() map[string]bool {
	return map[string]bool{
		"admin":              true,
		"system_echo":        true,
		"social_feed":        true,
		"ranking_membership": true,
		"ranking_names":      true,
		"treasury_activity":  true,
	}
}

func projections(cfg config.Config, pool *pgxpool.Pool, uow *db.UnitOfWork, clk *replay.Clock) []bus.HandlerSpec {
	keep := projectionDurables()
	all := registeredConsumers(cfg, pool, uow, clk)
	return replay.Handlers(slices.DeleteFunc(all, func(c bus.Consumer) bool { return !keep[c.Durable] }))
}

func replayHandlers(
	cfg config.Config, source, target *pgxpool.Pool, uow *db.UnitOfWork, clk *replay.Clock,
) []bus.HandlerSpec {
	derived := replay.Projections(registeredSet(cfg, target, uow, clk), registeredSet(cfg, source, nil, clock.Real{}))
	return append(projections(cfg, target, uow, clk), derived...)
}

func fail(stderr io.Writer, err error) int {
	_, _ = fmt.Fprintf(stderr, "monacoctl: %v\n", err)
	return 1
}
