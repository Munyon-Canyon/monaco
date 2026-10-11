package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"

	tradingapp "github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

func listFailedBridgeBacks(clk clock.Clock) command {
	return func(cfg config.Config, args []string, stdout, stderr io.Writer) int {
		if len(args) != 0 {
			_, _ = fmt.Fprintln(stderr, swapsUsage)
			return 2
		}
		ctx := observability.WithLogger(context.Background(), observability.NewLogger(cfg, stderr))
		var failed []tradingapp.FailedBridgeBack
		pool, err := db.Open(ctx, cfg.DB)
		if err == nil {
			defer pool.Close()
			failed, err = tradingapp.NewRetryBridgeBackHandler(db.New(pool, ids.Real{}, clk), pool, clk, ids.Real{}).
				ListFailed(ctx)
		}
		if err != nil {
			return fail(stderr, err)
		}
		if len(failed) == 0 {
			_, _ = fmt.Fprintln(stdout, "no failed bridge-backs")
		}
		for _, f := range failed {
			_, _ = fmt.Fprintf(stdout, "swap=%s cabal=%s source=%s network=%s amount_micros=%s attempt=%d error=%s "+
				"failed_at=%s\n", f.SwapID.UUID(), f.CabalID.UUID(), f.Source, f.Network, f.Amount, f.Attempt, f.Error,
				f.FailedAt.UTC().Format(time.RFC3339))
		}
		return 0
	}
}

func retryBridgeBack(environ []string, clk clock.Clock, stdin io.Reader) command {
	return func(cfg config.Config, args []string, stdout, stderr io.Writer) int {
		fs := flag.NewFlagSet("swaps retry-bridge-back", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		swap := fs.String("swap", "", "")
		reason := fs.String("reason", "", "")
		yes := fs.Bool("yes", false, "")
		if fs.Parse(args) != nil || fs.NArg() != 0 || *reason == "" {
			_, _ = fmt.Fprintln(stderr, swapsUsage)
			return 2
		}
		id, err := uuid.Parse(*swap)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, swapsUsage)
			return 2
		}
		cmd := tradingapp.RetryBridgeBack{
			SwapID: ids.SwapIDFrom(id), Actor: "operator:" + operatorOf(environ), Reason: *reason,
		}
		ctx := observability.WithLogger(context.Background(), observability.NewLogger(cfg, stderr))
		var handler *tradingapp.RetryBridgeBackHandler
		var before domain.TradeStep
		pool, err := db.Open(ctx, cfg.DB)
		if err == nil {
			defer pool.Close()
			handler = tradingapp.NewRetryBridgeBackHandler(db.New(pool, ids.Real{}, clk), pool, clk, ids.Real{})
			before, err = handler.Step(ctx, cmd.SwapID)
		}
		if err != nil {
			return fail(stderr, err)
		}
		_, _ = fmt.Fprintln(stdout, "before "+bridgeBackLine(before))
		if !*yes && !confirmed(stdin, stderr, "Retry this bridge-back?") {
			_, _ = fmt.Fprintln(stderr, "monacoctl: cancelled")
			return 1
		}
		after, err := handler.Handle(ctx, cmd)
		if err != nil {
			return fail(stderr, err)
		}
		_, _ = fmt.Fprintln(stdout, "after "+bridgeBackLine(after))
		return 0
	}
}

func bridgeBackLine(s domain.TradeStep) string {
	return fmt.Sprintf("swap=%s cabal=%s network=%s amount_micros=%s status=%s attempt=%d privy_action_id=%s "+
		"prior_privy_action_ids=%s error=%s", s.SwapID.UUID(), s.CabalID.UUID(), s.Network, s.Amount, s.Status,
		s.Attempt, s.PrivyActionID, strings.Join(s.PriorPrivyActionIDs, ","), s.Error)
}
