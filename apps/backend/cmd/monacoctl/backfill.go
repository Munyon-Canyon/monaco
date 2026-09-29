package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/tools/ops/replay"
)

const backfillUsage = "usage: monacoctl backfill --consumer <handler> --types <t1,t2> [--since <event_id>]"

func toolBackfill(env toolEnv) tool { return backfillTool(env.environ) }

func backfillTool(environ []string) tool {
	return func(args []string, stdout, stderr io.Writer) int {
		fs := flag.NewFlagSet("backfill", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		handler := fs.String("consumer", "", "")
		types := fs.String("types", "", "")
		since := fs.String("since", "", "")
		sinceID, ok := parseFlags(fs, args, since)
		if !ok || *handler == "" || *types == "" {
			_, _ = fmt.Fprintln(stderr, backfillUsage)
			return 2
		}
		cfg, err := config.Load(environ)
		if err != nil {
			return fail(stderr, err)
		}
		ctx := context.Background()
		pools, err := openPools(ctx, cfg.DB)
		if err != nil {
			return fail(stderr, err)
		}
		defer closePools(pools)
		clk := &replay.Clock{}
		uow := db.New(pools[0], ids.Real{}, clk)
		handlers := projections(cfg, pools[0], uow, clk)
		i := slices.IndexFunc(handlers, func(h bus.HandlerSpec) bool { return h.Name == *handler })
		if i < 0 {
			_, _ = fmt.Fprintf(stderr, "monacoctl: no registered handler %q\n", *handler)
			return 1
		}
		var typs []events.Type
		for t := range strings.SplitSeq(*types, ",") {
			typs = append(typs, events.Type(t))
		}
		rep, err := replay.Backfill(ctx, replay.BackfillOptions{
			Pool: pools[0], UoW: uow, Clock: clk, Now: clock.Real{}, Handler: handlers[i], Types: typs, Since: sinceID,
		})
		_, _ = fmt.Fprintf(stdout, "backfill %s: %d events, %d applied, %d duplicate\n",
			*handler, rep.Events, rep.Applied, rep.Duplicates)
		if err != nil {
			return fail(stderr, err)
		}
		return 0
	}
}
