package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

var (
	errNotAnAddress   = errors.New("not a Solana address")
	errUsage          = errors.New("usage")
	errNoDestination  = errors.New("--destination is required")
	errAllWithSources = errors.New("--all cannot be combined with --source")
)

type sweepFlags struct {
	destination chain.SolanaAddress
	all         bool
	dryRun      bool
	sources     []chain.SolanaAddress
}

type sourceFlagValues []chain.SolanaAddress

func (values *sourceFlagValues) String() string {
	return fmt.Sprint([]chain.SolanaAddress(*values))
}

func (values *sourceFlagValues) Set(raw string) error {
	for part := range strings.SplitSeq(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		addr, err := chain.ParseAddress(part)
		if err != nil {
			return fmt.Errorf("--source %q: %w", part, errNotAnAddress)
		}
		*values = append(*values, addr)
	}
	return nil
}

func parseSweepFlags(args []string, errOut io.Writer) (sweepFlags, error) {
	fs := flag.NewFlagSet("sweep-member-to-address", flag.ContinueOnError)
	fs.SetOutput(errOut)
	dest := fs.String("destination", "", "Solana address that receives all swept USDC")
	all := fs.Bool("all", false, "use Privy as source of truth: every Solana wallet in this app")
	dryRun := fs.Bool("dry-run", false, "list balances and planned sweeps; send no transactions")
	var sources sourceFlagValues
	fs.Var(&sources, "source", "Solana wallet to drain (repeatable; comma-separated in one value)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(errOut,
			"usage: %s --destination <wallet_address> [--source <addr>] [--all] [--dry-run]\n", fs.Name())
		_, _ = fmt.Fprintln(errOut, "default: sweep every user_wallets and treasury_wallets row")
		_, _ = fmt.Fprintln(errOut, "--source: drain only the listed wallet(s); repeat or comma-separate")
		_, _ = fmt.Fprintln(errOut, "--all: sweep every Solana wallet Privy returns for this app")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return sweepFlags{}, fmt.Errorf("%w: %w", errUsage, err)
	}
	if fs.NArg() != 0 {
		return sweepFlags{}, fmt.Errorf("%w: unexpected args: %s (use --destination / --source)",
			errUsage, strings.Join(fs.Args(), " "))
	}
	if strings.TrimSpace(*dest) == "" {
		fs.Usage()
		return sweepFlags{}, errNoDestination
	}
	destination, err := chain.ParseAddress(strings.TrimSpace(*dest))
	if err != nil {
		return sweepFlags{}, fmt.Errorf("--destination %q: %w", *dest, errNotAnAddress)
	}
	if *all && len(sources) > 0 {
		return sweepFlags{}, errAllWithSources
	}
	return sweepFlags{destination: destination, all: *all, dryRun: *dryRun, sources: sources}, nil
}
