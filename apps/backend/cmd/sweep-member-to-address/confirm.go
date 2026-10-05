package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

const ackPhrase = "I UNDERSTAND THIS MAY MESS WITH PROD"

var (
	errRejected    = errors.New("confirmation rejected")
	errAborted     = errors.New("confirmation aborted")
	errDestAborted = errors.New("destination confirmation aborted")
	errDestDiffers = errors.New("destination confirmation mismatch")
)

type confirmOpts struct {
	dest        chain.SolanaAddress
	databaseURL string
	sourceNote  string
	dryRun      bool
	sources     []sweepSource
}

func confirmSweep(in io.Reader, out io.Writer, opts confirmOpts) error {
	p := func(format string, args ...any) { _, _ = fmt.Fprintf(out, format, args...) }
	p("\n")
	if opts.dryRun {
		p("DRY-RUN: no transactions. Balances + plan only.\n")
	} else {
		p("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!\n")
		p("DANGER: ops USDC sweep. Can empty live wallets.\n")
		p("A swept treasury no longer matches its cabal's ledger.\n")
		p("Use only if you know DATABASE_URL / Privy app + dest are correct.\n")
		p("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!\n")
	}
	p("DATABASE_URL=%s\n", redactDatabaseURL(opts.databaseURL))
	p("destination=%s\nsource=%s\n", opts.dest, opts.sourceNote)
	p("dry_run=%t\nsources=%d\n", opts.dryRun, len(opts.sources))
	for _, src := range opts.sources {
		p("  - %s %s\n", src.kind, src.wallet.Address)
	}
	if opts.dryRun {
		return nil
	}
	p("\nType exactly: %s\n", ackPhrase)
	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		return abortErr(scanner, errAborted)
	}
	if strings.TrimSpace(scanner.Text()) != ackPhrase {
		return errRejected
	}
	p("Type destination wallet address again:\n")
	if !scanner.Scan() {
		return abortErr(scanner, errDestAborted)
	}
	if strings.TrimSpace(scanner.Text()) != string(opts.dest) {
		return errDestDiffers
	}
	return nil
}

func abortErr(scanner *bufio.Scanner, aborted error) error {
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read confirmation: %w", err)
	}
	return aborted
}

func redactDatabaseURL(raw string) string {
	return "***@" + raw[strings.LastIndex(raw, "@")+1:]
}
