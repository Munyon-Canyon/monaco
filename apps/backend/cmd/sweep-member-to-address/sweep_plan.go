package main

import (
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type verdict string

const (
	verdictSweep           verdict = "sweep"
	verdictWouldSweep      verdict = "would sweep"
	verdictSkipRelayer     verdict = "fee payer; do not drain"
	verdictSkipDestination verdict = "same as destination"
	verdictSkipZero        verdict = "zero USDC"
	verdictNoPrivyWallet   verdict = "not a Privy wallet of this app"
)

func (v verdict) skipped() bool {
	return v == verdictSkipRelayer || v == verdictSkipDestination || v == verdictSkipZero
}

func planSweep(src sweepSource, balance money.BaseUnits, dest, relayer chain.SolanaAddress, dryRun bool) verdict {
	switch {
	case src.wallet.Address == relayer:
		return verdictSkipRelayer
	case src.wallet.Address == dest:
		return verdictSkipDestination
	case balance.IsZero():
		return verdictSkipZero
	case src.wallet.ID == "":
		return verdictNoPrivyWallet
	case dryRun:
		return verdictWouldSweep
	}
	return verdictSweep
}
