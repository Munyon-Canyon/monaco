package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"

	tradingapp "github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

func listParked(clk clock.Clock) command {
	return func(cfg config.Config, args []string, stdout, stderr io.Writer) int {
		if len(args) != 0 {
			_, _ = fmt.Fprintln(stderr, swapsUsage)
			return 2
		}
		ctx := observability.WithLogger(context.Background(), observability.NewLogger(cfg, stderr))
		var parked []tradingapp.ParkedSwap
		pool, err := db.Open(ctx, cfg.DB)
		if err == nil {
			defer pool.Close()
			parked, err = tradingapp.NewResolveParkedSwapHandler(db.New(pool, ids.Real{}, clk), pool, clk, ids.Real{}, nil, nil).
				ListParked(ctx)
		}
		if err != nil {
			return fail(stderr, err)
		}
		if len(parked) == 0 {
			_, _ = fmt.Fprintln(stdout, "no parked swaps")
		}
		for _, p := range parked {
			_, _ = fmt.Fprintln(stdout, parkedLine(p))
		}
		return 0
	}
}

func resolveParked(
	action tradingapp.ParkedAction, environ []string, clk clock.Clock, stdin io.Reader, ports cashPorts,
) command {
	return func(cfg config.Config, args []string, stdout, stderr io.Writer) int {
		fs := flag.NewFlagSet("swaps "+string(action), flag.ContinueOnError)
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
		cmd := tradingapp.ResolveParkedSwap{
			SwapID: ids.SwapIDFrom(id), Action: action, Actor: "operator:" + operatorOf(environ), Reason: *reason,
		}
		return runParked(cfg, clk, stdin, ports, cmd, *yes, stdout, stderr)
	}
}

func runParked(
	cfg config.Config, clk clock.Clock, stdin io.Reader, ports cashPorts, cmd tradingapp.ResolveParkedSwap, yes bool,
	stdout, stderr io.Writer,
) int {
	ctx := observability.WithLogger(context.Background(), observability.NewLogger(cfg, stderr))
	var conn *bus.Conn
	var handler *tradingapp.ResolveParkedSwapHandler
	var before tradingapp.ParkedSwap
	pool, err := db.Open(ctx, cfg.DB)
	if err == nil {
		defer pool.Close()
		conn, err = bus.Connect(ctx, cfg.NATS, bus.ProcessMonacoctl)
	}
	if err == nil {
		defer conn.Close(ctx)
		uow := db.New(pool, ids.Real{}, clk)
		handler = tradingapp.NewResolveParkedSwapHandler(
			uow, pool, clk, ids.Real{}, conn, ports(cfg, pool, uow, clk).Receipts,
		)
		before, err = handler.Parked(ctx, cmd.SwapID)
	}
	if err != nil {
		return fail(stderr, err)
	}
	_, _ = fmt.Fprintln(stdout, "before "+parkedLine(before))
	if !yes && !confirmed(stdin, stderr, "Run "+string(cmd.Action)+" on this swap?") {
		_, _ = fmt.Fprintln(stderr, "monacoctl: cancelled")
		return 1
	}
	after, err := handler.Handle(ctx, cmd)
	if err != nil {
		return fail(stderr, err)
	}
	_, _ = fmt.Fprintln(stdout, "after "+parkedLine(after))
	return 0
}

func parkedLine(p tradingapp.ParkedSwap) string {
	at := ""
	if !p.ParkedAt.IsZero() {
		at = p.ParkedAt.UTC().Format(time.RFC3339)
	}
	return fmt.Sprintf("swap=%s cabal=%s status=%s parked_at=%s code=%s bridge_out=%s swap_step=%s bridge_back=%s",
		p.ID.UUID(), p.CabalID.UUID(), p.Status, at, p.Code, p.BridgeOut, p.Swap, p.BridgeBack)
}
