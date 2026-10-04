package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	tradingapp "github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const swapsUsage = "usage: monacoctl swaps force-resolve --signature <sig> --to confirmed|failed [--out-amount <units>] --reason <text> [--yes]"

func toolSwaps(env toolEnv) tool { return swapsTool(env.environ, clock.Real{}, os.Stdin) }

func swapsTool(environ []string, clk clock.Clock, stdin io.Reader) tool {
	forceResolve := func(cfg config.Config, args []string, stdout, stderr io.Writer) int {
		fs := flag.NewFlagSet("swaps force-resolve", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		signature := fs.String("signature", "", "")
		to := fs.String("to", "", "")
		outAmount := fs.String("out-amount", "", "")
		reason := fs.String("reason", "", "")
		yes := fs.Bool("yes", false, "")
		if fs.Parse(args) != nil || fs.NArg() != 0 || *signature == "" || *reason == "" ||
			(*to != string(domain.StatusConfirmed) && *to != string(domain.StatusFailed)) ||
			(*to == string(domain.StatusConfirmed) && *outAmount == "") ||
			(*to == string(domain.StatusFailed) && *outAmount != "") {
			_, _ = fmt.Fprintln(stderr, swapsUsage)
			return 2
		}
		amount, err := parseForceResolveAmount(*outAmount)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, swapsUsage)
			return 2
		}
		return resolveSwap(
			cfg, clk, stdin, chain.Signature(*signature), domain.Status(*to), amount, *reason, *yes, stdout, stderr,
		)
	}
	return func(args []string, stdout, stderr io.Writer) int {
		return run(map[string]command{"force-resolve": forceResolve}, nil, environ, args, stdout, stderr)
	}
}

func parseForceResolveAmount(raw string) (uint64, error) {
	if raw == "" {
		return 0, nil
	}
	amount, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse out amount: %w", err)
	}
	return amount, nil
}

func resolveSwap(
	cfg config.Config,
	clk clock.Clock,
	stdin io.Reader,
	signature chain.Signature,
	to domain.Status,
	amount uint64,
	reason string,
	yes bool, stdout, stderr io.Writer,
) int {
	ctx := observability.WithLogger(context.Background(), observability.NewLogger(cfg, stderr))
	pool, err := db.Open(ctx, cfg.DB)
	if err != nil {
		return fail(stderr, err)
	}
	defer pool.Close()
	queries := trading.New(module.Deps{Pool: pool}).Queries()
	before, err := queries.SwapBySignature(ctx, signature)
	if err != nil {
		return fail(stderr, err)
	}
	printSwap(stdout, "before", before)
	if !yes && !confirmed(stdin, stderr) {
		_, _ = fmt.Fprintln(stderr, "monacoctl: cancelled")
		return 1
	}
	conn, err := bus.Connect(ctx, cfg.NATS, bus.ProcessMonacoctl)
	if err != nil {
		return fail(stderr, err)
	}
	defer conn.Close(ctx)
	units, err := forceResolveUnits(before.OutDecimals, amount, to)
	if err != nil {
		return fail(stderr, err)
	}
	handler := tradingapp.NewForceResolveSwapHandler(db.New(pool, ids.Real{}, clk), pool, clk, conn)
	after, err := handler.Handle(ctx, tradingapp.ForceResolveSwap{
		Signature: signature, To: to, OutAmount: units, Reason: reason, Actor: "system:monacoctl",
		IdempotencyKey: "force-resolve:" + string(signature) + ":" + string(to),
	})
	if errs.CodeOf(err) == errs.CodeSwapNotStuck {
		notStuck := err
		after, err = queries.SwapBySignature(ctx, signature)
		if err == nil {
			printSwap(stdout, "after", after)
			if matchesResolvedSwap(after, to, amount) {
				return 0
			}
			err = notStuck
		}
	}
	if err != nil {
		return fail(stderr, err)
	}
	printSwap(stdout, "after", after)
	return 0
}

func matchesResolvedSwap(after trading.SwapView, to domain.Status, amount uint64) bool {
	return after.Status == to && (to != domain.StatusConfirmed || after.OutAmount == amount)
}

func forceResolveUnits(decimals int16, amount uint64, to domain.Status) (money.BaseUnits, error) {
	if to != domain.StatusConfirmed {
		return money.BaseUnits{}, nil
	}
	if decimals < 0 || decimals > 255 {
		return money.BaseUnits{}, errs.New(errs.CodeDecodeFailed, "monacoctl.swaps.forceResolve")
	}
	return money.NewBaseUnits(amount, uint8(decimals)), nil
}

func confirmed(stdin io.Reader, stderr io.Writer) bool {
	_, _ = fmt.Fprint(stderr, "Force-resolve this swap? [y/N] ")
	line, _ := bufio.NewReader(stdin).ReadString('\n')
	return strings.EqualFold(strings.TrimSpace(line), "y")
}

func printSwap(out io.Writer, label string, swap trading.SwapView) {
	_, _ = fmt.Fprintf(out, "%s swap=%s signature=%s status=%s out_amount=%d decimals=%d\n",
		label, swap.ID, swap.TxSignature, swap.Status, swap.OutAmount, swap.OutDecimals)
}
