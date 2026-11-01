package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	treasuryapp "github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const treasuryUsage = `usage:
  monacoctl treasury refund-fund --transfer <uuid> --reason <text> [--dry-run] [--yes]
    sends a landed fund transfer back from the cabal's treasury wallet to the member's wallet,
    only while the cabal cannot be valued; a second run moves nothing`

type refundPorts func(cfg config.Config, pool *pgxpool.Pool, uow *db.UnitOfWork, clk clock.Clock) *treasuryapp.FundRefunds

func liveRefundPorts(
	cfg config.Config,
	pool *pgxpool.Pool,
	uow *db.UnitOfWork,
	clk clock.Clock,
) *treasuryapp.FundRefunds {
	d := module.Deps{Config: cfg, Clock: clk, IDs: ids.Real{}, Pool: pool, UoW: uow}
	t := treasury.New(d)
	module.NewSet(cabal.New(d), identity.New(d), t)
	return t.FundRefunds()
}

func toolTreasury(env toolEnv) tool {
	return treasuryTool(env.environ, clock.Real{}, os.Stdin, liveRefundPorts)
}

func treasuryTool(environ []string, clk clock.Clock, stdin io.Reader, ports refundPorts) tool {
	return func(args []string, stdout, stderr io.Writer) int {
		return run(map[string]command{
			"refund-fund": refundFund(environ, clk, stdin, ports),
		}, nil, environ, args, stdout, stderr)
	}
}

type refundRequest struct {
	cmd    treasuryapp.RefundFundTransfer
	dryRun bool
	yes    bool
}

func parseRefundFund(environ []string, args []string) (refundRequest, bool) {
	fs := flag.NewFlagSet("treasury refund-fund", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	transfer := fs.String("transfer", "", "")
	reason := fs.String("reason", "", "")
	dryRun := fs.Bool("dry-run", false, "")
	yes := fs.Bool("yes", false, "")
	if fs.Parse(args) != nil || fs.NArg() != 0 || *reason == "" {
		return refundRequest{}, false
	}
	id, err := uuid.Parse(*transfer)
	if err != nil {
		return refundRequest{}, false
	}
	return refundRequest{
		cmd:    treasuryapp.RefundFundTransfer{ID: id, Actor: "operator:" + operatorOf(environ), Reason: *reason},
		dryRun: *dryRun, yes: *yes,
	}, true
}

func refundFund(environ []string, clk clock.Clock, stdin io.Reader, ports refundPorts) command {
	return func(cfg config.Config, args []string, stdout, stderr io.Writer) int {
		req, ok := parseRefundFund(environ, args)
		if !ok {
			_, _ = fmt.Fprintln(stderr, treasuryUsage)
			return 2
		}
		ctx := observability.WithLogger(context.Background(), observability.NewLogger(cfg, stderr))
		ctx = auth.WithActor(ctx, auth.Actor{Kind: auth.ActorSystem, ID: "monacoctl"})
		pool, err := db.Open(ctx, cfg.DB)
		if err != nil {
			return fail(stderr, err)
		}
		defer pool.Close()
		refunds := ports(cfg, pool, db.New(pool, ids.Real{}, clk), clk)
		plan, err := refunds.Plan(ctx, req.cmd.ID)
		if err != nil {
			return fail(stderr, err)
		}
		_, _ = fmt.Fprintln(stdout, "plan "+refundLine(plan, ""))
		if req.dryRun {
			return 0
		}
		if !req.yes && !confirmed(stdin, stderr, "Send this refund?") {
			_, _ = fmt.Fprintln(stderr, "monacoctl: cancelled")
			return 1
		}
		result, err := refunds.Refund(ctx, req.cmd)
		if err != nil {
			return fail(stderr, err)
		}
		_, _ = fmt.Fprintln(stdout, refundLine(result.Plan, string(result.State)))
		return 0
	}
}

func refundLine(p treasuryapp.FundRefundPlan, state string) string {
	line := fmt.Sprintf(
		"transfer=%s cabal=%s user=%s amount_micros=%s from=%s to=%s treasury_micros=%s signature=%s",
		p.TransferID, p.CabalID.UUID(), p.UserID.UUID(), p.Amount, p.From, p.To, p.Treasury, p.Signature,
	)
	if state != "" {
		line += " state=" + state
	}
	return line
}
