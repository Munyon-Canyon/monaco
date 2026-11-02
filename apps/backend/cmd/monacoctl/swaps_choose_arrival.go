package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	tradingapp "github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	tradingsqlc "github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type chooseArrivalArgs struct {
	swap   ids.SwapID
	tx     string
	reason string
	actor  string
	yes    bool
}

func parseChooseArrival(environ []string, args []string) (chooseArrivalArgs, bool) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return chooseArrivalArgs{}, false
	}
	id, err := uuid.Parse(args[0])
	fs := flag.NewFlagSet("swaps choose-arrival", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	tx := fs.String("tx", "", "")
	reason := fs.String("reason", "", "")
	yes := fs.Bool("yes", false, "")
	if err != nil || fs.Parse(args[1:]) != nil || fs.NArg() != 0 || *tx == "" || *reason == "" {
		return chooseArrivalArgs{}, false
	}
	return chooseArrivalArgs{
		swap: ids.SwapIDFrom(id), tx: *tx, reason: *reason, actor: "operator:" + operatorOf(environ), yes: *yes,
	}, true
}

func chooseArrival(environ []string, clk clock.Clock, stdin io.Reader, ports cashPorts) command {
	return func(cfg config.Config, args []string, stdout, stderr io.Writer) int {
		parsed, ok := parseChooseArrival(environ, args)
		if !ok {
			_, _ = fmt.Fprintln(stderr, swapsUsage)
			return 2
		}
		ctx := observability.WithLogger(context.Background(), observability.NewLogger(cfg, stderr))
		pool, err := db.Open(ctx, cfg.DB)
		if err != nil {
			return fail(stderr, err)
		}
		defer pool.Close()
		step, err := tradingsqlc.New(pool).TradeStep(ctx, tradingsqlc.TradeStepParams{
			TradeID: parsed.swap.UUID(), Step: string(domain.StepBridgeOut),
		})
		if err != nil {
			return fail(stderr, fmt.Errorf("read the bridge_out step of swap %s: %w", parsed.swap.UUID(), err))
		}
		uow := db.New(pool, ids.Real{}, clk)
		adapters := ports(cfg, pool, uow, clk)
		handler := tradingapp.NewChooseBridgeArrivalHandler(
			uow, pool, clk, ids.Real{}, adapters.Wallets, adapters.Transfers,
		)
		cmd := tradingapp.ChooseBridgeArrival{
			SwapID: parsed.swap, Network: chain.Network(step.Network), TxHash: parsed.tx, Actor: parsed.actor,
			Reason:         parsed.reason,
			IdempotencyKey: "choose-arrival:" + parsed.swap.String() + ":" + strings.ToLower(parsed.tx),
		}
		plan, err := handler.Plan(ctx, cmd)
		if err != nil {
			return failWithDetail(stderr, err)
		}
		_, _ = fmt.Fprintln(stdout, "plan "+arrivalLine(plan))
		if plan.Changed && !parsed.yes && !confirmed(stdin, stderr, "Book this transfer as the bridge arrival?") {
			_, _ = fmt.Fprintln(stderr, "monacoctl: cancelled")
			return 1
		}
		booked, err := handler.Handle(ctx, cmd)
		if err != nil {
			return failWithDetail(stderr, err)
		}
		_, _ = fmt.Fprintln(stdout, "booked "+arrivalLine(booked))
		return 0
	}
}

func failWithDetail(stderr io.Writer, err error) int {
	var detail strings.Builder
	for _, a := range errs.Detail(err) {
		detail.WriteString(" " + a.Key + "=" + a.Value.String())
	}
	_, _ = fmt.Fprintf(stderr, "monacoctl: %v%s\n", err, detail.String())
	return 1
}

func arrivalLine(c tradingapp.BridgeArrivalChoice) string {
	return fmt.Sprintf("swap=%s cabal=%s network=%s arrival_tx=%s arrived_micros=%s fee_micros=%s changed=%t",
		c.SwapID.UUID(), c.CabalID.UUID(), c.Network, c.TxHash, c.Amount, c.Fee, c.Changed)
}
