package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type tally struct {
	ok, failed, skipped, dryRun int
	total                       uint64
}

func tallyOutcomes(outcomes []walletOutcome) tally {
	var t tally
	for _, o := range outcomes {
		switch o.status {
		case statusOK:
			t.ok++
			t.total += o.balance.Uint64()
		case statusDryRun:
			t.dryRun++
			t.total += o.balance.Uint64()
		case statusFail:
			t.failed++
		case statusSkipped:
			t.skipped++
		}
	}
	return t
}

func (t tally) exitCode() int {
	if t.failed > 0 {
		return 1
	}
	return 0
}

func printSweepRecap(out io.Writer, dest chain.SolanaAddress, dryRun bool, outcomes []walletOutcome) {
	_, _ = fmt.Fprintf(out, "\n=== Sweep summary (%s) ===\nDestination: %s\n", modeName(dryRun), dest)
	if len(outcomes) == 0 {
		_, _ = fmt.Fprintln(out, "No wallets.")
		return
	}
	t := tallyOutcomes(outcomes)
	_, _ = fmt.Fprintf(out, "Wallets: %d", len(outcomes))
	if t.failed > 0 {
		_, _ = fmt.Fprintf(out, " (%d FAIL)", t.failed)
	}
	_, _ = fmt.Fprintln(out)
	for i, o := range outcomes {
		_, _ = fmt.Fprintf(out, "[%d] %s %s\n  %s\n",
			i+1, o.source.kind, formatAddress(string(o.source.wallet.Address)), formatOutcomeLine(o))
	}
	verb := "Swept"
	if dryRun {
		verb = "Would sweep"
	}
	_, _ = fmt.Fprintf(out, "\n%s: %s across %d wallet(s)\n",
		verb, formatUSDC(money.NewBaseUnits(t.total, 6)), t.ok+t.dryRun)
	_, _ = fmt.Fprintf(out, "Wallets: ok=%d fail=%d skipped=%d dry-run=%d\n", t.ok, t.failed, t.skipped, t.dryRun)
}

func formatOutcomeLine(o walletOutcome) string {
	line := fmt.Sprintf("%s usdc-sweep amount=%s (%s raw)", o.status, formatUSDC(o.balance), o.balance)
	switch o.status {
	case statusOK:
		line += " tx=" + formatAddress(string(o.tx))
	case statusFail:
		line += " err=" + o.note
	case statusSkipped, statusDryRun:
		line += " (" + o.note + ")"
	}
	return line
}

func formatUSDC(amount money.BaseUnits) string {
	v := amount.Uint64()
	return fmt.Sprintf("%d.%06d USDC", v/1_000_000, v%1_000_000)
}

func formatAddress(addr string) string {
	if len(addr) <= 12 {
		return addr
	}
	return addr[:4] + "…" + addr[len(addr)-4:]
}

func oneLineErr(msg string) string {
	return strings.Join(strings.Fields(msg), " ")
}
