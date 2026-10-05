package main

import (
	"context"
	"fmt"
	"io"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type tokenBalances interface {
	TokenBalance(ctx context.Context, owner chain.SolanaAddress, mint chain.Mint) (money.BaseUnits, error)
}

type transfers interface {
	Build(ctx context.Context, spec relayer.TransferSpec) (relayer.SignedTx, error)
	Broadcast(ctx context.Context, tx relayer.SignedTx) error
}

type sweepRunner struct {
	out       io.Writer
	dest      chain.SolanaAddress
	dryRun    bool
	relayer   chain.SolanaAddress
	usdc      chain.Mint
	balances  tokenBalances
	transfers transfers
}

type outcomeStatus string

const (
	statusOK      outcomeStatus = "ok"
	statusFail    outcomeStatus = "FAIL"
	statusSkipped outcomeStatus = "skipped"
	statusDryRun  outcomeStatus = "dry-run"
)

type walletOutcome struct {
	source  sweepSource
	balance money.BaseUnits
	status  outcomeStatus
	note    string
	tx      chain.Signature
}

func (r sweepRunner) sweep(ctx context.Context, sources []sweepSource) []walletOutcome {
	outcomes := make([]walletOutcome, 0, len(sources))
	for _, src := range sources {
		outcomes = append(outcomes, r.sweepOne(ctx, src))
	}
	return outcomes
}

func (r sweepRunner) sweepOne(ctx context.Context, src sweepSource) walletOutcome {
	from := src.wallet.Address
	balance, err := r.balances.TokenBalance(ctx, from, r.usdc)
	if err != nil {
		return r.failed(walletOutcome{source: src}, fmt.Sprintf("balance: %v", err))
	}
	o := walletOutcome{source: src, balance: balance}
	v := planSweep(src, balance, r.dest, r.relayer, r.dryRun)
	switch {
	case v.skipped():
		r.printf("skip %s %s balance=%s (%s)\n", src.kind, from, formatUSDC(balance), v)
		o.status, o.note = statusSkipped, string(v)
		return o
	case v == verdictNoPrivyWallet:
		return r.failed(o, string(v))
	case v == verdictWouldSweep:
		r.printf("dry-run would sweep %s from=%s dest=%s mint=%s amount=%s no_tx_sent\n",
			src.kind, from, r.dest, r.usdc.Address, balance)
		o.status, o.note = statusDryRun, "no tx sent"
		return o
	}
	spec := relayer.TransferSpec{FromWallet: src.wallet, To: r.dest, Mint: r.usdc, Amount: balance}
	tx, err := r.transfers.Build(ctx, spec)
	if err != nil {
		return r.failed(o, fmt.Sprintf("build: %v", err))
	}
	if err := r.transfers.Broadcast(ctx, tx); err != nil {
		return r.failed(o, fmt.Sprintf("broadcast: %v", err))
	}
	r.printf("ok usdc-sweep %s from=%s dest=%s mint=%s amount=%s tx=%s\n",
		src.kind, from, r.dest, r.usdc.Address, balance, tx.Signature)
	o.status, o.tx = statusOK, tx.Signature
	return o
}

func (r sweepRunner) failed(o walletOutcome, msg string) walletOutcome {
	r.printf("fail usdc-sweep %s from=%s err=%s\n", o.source.kind, o.source.wallet.Address, oneLineErr(msg))
	o.status, o.note = statusFail, oneLineErr(msg)
	return o
}

func (r sweepRunner) printf(format string, args ...any) {
	_, _ = fmt.Fprintf(r.out, format, args...)
}
