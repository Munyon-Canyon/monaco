package main

import (
	"context"
	"fmt"
	"io"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

const (
	relayerUsage     = "usage: monacoctl relayer balance"
	lamportsPerSOL   = 1_000_000_000
	relayerSOLFormat = "relayer %s\nbalance %d.%09d SOL (%d lamports)\n"
)

func toolRelayer(env toolEnv) tool { return relayerTool(env.environ) }

func relayerTool(environ []string) tool {
	return func(args []string, stdout, stderr io.Writer) int {
		if len(args) != 1 || args[0] != "balance" {
			_, _ = fmt.Fprintln(stderr, relayerUsage)
			return 2
		}
		cfg, err := config.Load(environ)
		if err != nil {
			return fail(stderr, err)
		}
		rpc := solana.New(cfg, clock.Real{})
		r, err := relayer.New(cfg, rpc)
		if err != nil {
			return fail(stderr, err)
		}
		balance, err := rpc.SOLBalance(context.Background(), r.Address())
		if err != nil {
			return fail(stderr, err)
		}
		lamports := balance.Uint64()
		_, _ = fmt.Fprintf(
			stdout,
			relayerSOLFormat,
			r.Address(),
			lamports/lamportsPerSOL,
			lamports%lamportsPerSOL,
			lamports,
		)
		return 0
	}
}
