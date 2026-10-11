package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

const simulationUsage = "usage: monacoctl market simulation-account [--timeout 2m] [--poll 3s]"

type delegationWait struct {
	timeout, poll time.Duration
}

func simulationAccount(clk clock.Clock) command {
	return func(cfg config.Config, args []string, stdout, stderr io.Writer) int {
		fs := flag.NewFlagSet("market simulation-account", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		timeout := fs.Duration("timeout", 2*time.Minute, "how long to wait for each chain's delegation")
		poll := fs.Duration("poll", 3*time.Second, "how often to read each chain's code while waiting")
		if err := fs.Parse(args); err != nil || fs.NArg() != 0 || *poll <= 0 {
			_, _ = fmt.Fprintln(stderr, simulationUsage)
			return 2
		}
		ctx := context.Background()
		pool, err := db.Open(ctx, cfg.DB)
		if err != nil {
			return fail(stderr, err)
		}
		defer pool.Close()
		wallets, err := privy.New(cfg, clk)
		if err != nil {
			return fail(stderr, err)
		}
		a, created, err := app.SetUpSimulationAccount(ctx, pool, wallets, clk.Now())
		if err != nil {
			return fail(stderr, err)
		}
		_, _ = fmt.Fprintf(stdout, "wallet %s\naddress %s\n%s\n", a.WalletID, a.Address, map[bool]string{
			true: "created", false: "reused",
		}[created])
		code := adapters.NewEVMCode(module.Deps{Config: cfg})
		undelegated := 0
		for _, n := range code.Networks() {
			d, err := delegateOn(ctx, wallets, code, a, n, delegationWait{timeout: *timeout, poll: *poll})
			_, _ = fmt.Fprintln(stdout, delegationLine(d, err))
			if err != nil || !d.Delegated() {
				undelegated++
			}
		}
		return min(undelegated, 1)
	}
}

func delegateOn(
	ctx context.Context, wallets multichain.EVMWallets, code app.AccountCode, a app.SimulationAccount,
	n chain.Network, wait delegationWait,
) (app.Delegation, error) {
	d, err := app.DelegateSimulationAccount(ctx, wallets, code, a, n)
	if err != nil || d.Delegated() {
		return d, err
	}
	ctx, cancel := context.WithTimeout(ctx, wait.timeout)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return d, nil
		case <-time.After(wait.poll):
		}
		next, err := app.ReadDelegation(ctx, code, n, a.Address)
		next.Sent = d.Sent
		if err != nil || next.Delegated() {
			return next, err
		}
		d = next
	}
}

func delegationLine(d app.Delegation, err error) string {
	if err != nil {
		return fmt.Sprintf("%s\terror=%v", d.Network, err)
	}
	delegate := "none"
	if d.Delegated() {
		delegate = d.Delegate.String()
	}
	sent := "none"
	if d.Sent != "" {
		sent = d.Sent
	}
	return fmt.Sprintf("%s\tcode=%s\tdelegate=%s\tsent=%s", d.Network, d.Code, delegate, sent)
}
