package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/governance"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

const proposalsUsage = "usage: monacoctl proposals void --proposal <id> --reason <text>"

func toolProposals(env toolEnv) tool { return proposalsTool(env.environ, clock.Real{}) }

func proposalsTool(environ []string, clk clock.Clock) tool {
	void := func(cfg config.Config, args []string, stdout, stderr io.Writer) int {
		fs := flag.NewFlagSet("proposals void", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		rawID := fs.String("proposal", "", "")
		reason := fs.String("reason", "", "")
		if fs.Parse(args) != nil || fs.NArg() != 0 {
			_, _ = fmt.Fprintln(stderr, proposalsUsage)
			return 2
		}
		raw, err := uuid.Parse(*rawID)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, proposalsUsage)
			return 2
		}
		id, ctx := ids.ProposalIDFrom(raw), context.Background()
		pool, err := db.Open(ctx, cfg.DB)
		if err != nil {
			return fail(stderr, err)
		}
		defer pool.Close()
		deps := module.Deps{Config: cfg, Clock: clk, IDs: ids.Real{}, Pool: pool, UoW: db.New(pool, ids.Real{}, clk)}
		if err := governance.New(deps).VoidFromOps(ctx, id, *reason); err != nil {
			return fail(stderr, err)
		}
		_, _ = fmt.Fprintf(stdout, "voided %s\n", id)
		return 0
	}
	return func(args []string, stdout, stderr io.Writer) int {
		return run(map[string]command{"void": void}, nil, environ, args, stdout, stderr)
	}
}
