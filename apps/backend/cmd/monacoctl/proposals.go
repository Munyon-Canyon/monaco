package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/governance"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

const proposalsUsage = "usage:\n  monacoctl proposals void --proposal <id> --reason <text>\n" +
	"  monacoctl proposals retry-trade --proposal <id> --reason <text>"

type proposalAct func(*governance.Module) func(context.Context, governance.ProposalID, string) error

func toolProposals(env toolEnv) tool { return proposalsTool(env.environ, clock.Real{}) }

func proposalsTool(environ []string, clk clock.Clock) tool {
	ops := func(name, done string, act proposalAct) command {
		return func(cfg config.Config, args []string, stdout, stderr io.Writer) int {
			return proposalOp(cfg, clk, environ, name, done, act, args, stdout, stderr)
		}
	}
	void := func(m *governance.Module) func(context.Context, governance.ProposalID, string) error {
		return m.VoidFromOps
	}
	retry := func(m *governance.Module) func(context.Context, governance.ProposalID, string) error {
		return m.RetryBlockedTradeFromOps
	}
	return func(args []string, stdout, stderr io.Writer) int {
		return run(map[string]command{
			"void":        ops("void", "voided", void),
			"retry-trade": ops("retry-trade", "retry requested for", retry),
		}, nil, environ, args, stdout, stderr)
	}
}

func proposalOp(
	cfg config.Config, clk clock.Clock, environ []string, name, done string, act proposalAct, args []string,
	stdout, stderr io.Writer,
) int {
	fs := flag.NewFlagSet("proposals "+name, flag.ContinueOnError)
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
	id := ids.ProposalIDFrom(raw)
	ctx := auth.WithActor(
		context.Background(),
		auth.Actor{Kind: auth.ActorSystem, ID: "operator:" + operatorOf(environ)},
	)
	pool, err := db.Open(ctx, cfg.DB)
	if err != nil {
		return fail(stderr, err)
	}
	defer pool.Close()
	deps := module.Deps{Config: cfg, Clock: clk, IDs: ids.Real{}, Pool: pool, UoW: db.New(pool, ids.Real{}, clk)}
	if err := act(governance.New(deps))(ctx, id, *reason); err != nil {
		return fail(stderr, err)
	}
	_, _ = fmt.Fprintf(stdout, "%s %s\n", done, id)
	return 0
}
