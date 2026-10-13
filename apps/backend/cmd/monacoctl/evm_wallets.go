package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

const (
	evmWalletsUsage = "usage: monacoctl ops evm-wallets [--dry-run] [--users] [--cabals]"
	evmWalletPace   = 200 * time.Millisecond
)

type evmWalletHolder struct {
	id     string
	held   bool
	ensure func(context.Context) (chain.EVMAddress, error)
}

type evmWalletKind struct {
	name, plural string
	list         func(context.Context) ([]evmWalletHolder, error)
}

type evmWalletTally struct{ created, existing, failed int }

func evmWalletsCommand(clk clock.Clock) command {
	return func(cfg config.Config, args []string, stdout, stderr io.Writer) int {
		fs := flag.NewFlagSet("ops evm-wallets", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		dryRun := fs.Bool("dry-run", false, "")
		users := fs.Bool("users", false, "")
		cabals := fs.Bool("cabals", false, "")
		if fs.Parse(args) != nil || fs.NArg() != 0 {
			_, _ = fmt.Fprintln(stderr, evmWalletsUsage)
			return 2
		}
		if !*users && !*cabals {
			*users, *cabals = true, true
		}
		ctx := context.Background()
		pool, err := db.Open(ctx, cfg.DB)
		if err != nil {
			return fail(stderr, err)
		}
		defer pool.Close()
		set := registeredSet(cfg, pool, db.New(pool, ids.Real{}, clk), clk)
		b := evmWalletBackfill{clock: clk, dryRun: *dryRun, stdout: stdout, stderr: stderr}
		return b.run(ctx, evmWalletKinds(set, *users, *cabals))
	}
}

func evmWalletKinds(set module.Set, users, cabals bool) []evmWalletKind {
	var (
		identityModule *identity.Module
		cabalModule    *cabal.Module
	)
	for _, m := range set {
		switch m := m.(type) {
		case *identity.Module:
			identityModule = m
		case *cabal.Module:
			cabalModule = m
		}
	}
	var kinds []evmWalletKind
	if users && identityModule != nil {
		kinds = append(kinds, evmWalletKind{name: "user", plural: "users", list: userEVMWallets(identityModule)})
	}
	if cabals && cabalModule != nil {
		kinds = append(kinds, evmWalletKind{name: "cabal", plural: "cabals", list: cabalEVMWallets(cabalModule)})
	}
	return kinds
}

func userEVMWallets(m *identity.Module) func(context.Context) ([]evmWalletHolder, error) {
	return func(ctx context.Context) ([]evmWalletHolder, error) {
		rows, err := m.EVMWalletBackfillUsers(ctx)
		if err != nil {
			return nil, err
		}
		wallets := m.EVMDepositWallets()
		out := make([]evmWalletHolder, len(rows))
		for i, r := range rows {
			out[i] = evmWalletHolder{
				id:   r.User.String(),
				held: r.Held,
				ensure: func(ctx context.Context) (chain.EVMAddress, error) {
					return wallets.EnsureEVMDepositWallet(ctx, r.User)
				},
			}
		}
		return out, nil
	}
}

func cabalEVMWallets(m *cabal.Module) func(context.Context) ([]evmWalletHolder, error) {
	return func(ctx context.Context) ([]evmWalletHolder, error) {
		rows, err := m.EVMWalletBackfillCabals(ctx)
		if err != nil {
			return nil, err
		}
		wallets := m.TreasuryEVMWallets()
		out := make([]evmWalletHolder, len(rows))
		for i, r := range rows {
			out[i] = evmWalletHolder{
				id:   r.Cabal.String(),
				held: r.Held,
				ensure: func(ctx context.Context) (chain.EVMAddress, error) {
					w, err := wallets.EnsureTreasuryEVMWallet(ctx, r.Cabal)
					return w.Address, err
				},
			}
		}
		return out, nil
	}
}

type evmWalletBackfill struct {
	clock          clock.Clock
	dryRun         bool
	stdout, stderr io.Writer
	ensured        int
}

func (b *evmWalletBackfill) run(ctx context.Context, kinds []evmWalletKind) int {
	parts := make([]string, 0, len(kinds))
	failed := false
	for _, k := range kinds {
		holders, err := k.list(ctx)
		if err != nil {
			return fail(b.stderr, err)
		}
		t := b.tally(ctx, k, holders)
		failed = failed || t.failed > 0
		if b.dryRun {
			parts = append(parts, fmt.Sprintf("%s missing=%d existing=%d", k.plural, t.created, t.existing))
			continue
		}
		parts = append(
			parts,
			fmt.Sprintf("%s created=%d existing=%d failed=%d", k.plural, t.created, t.existing, t.failed),
		)
	}
	label := "evm-wallets"
	if b.dryRun {
		label += " dry run"
	}
	_, _ = fmt.Fprintf(b.stdout, "%s: %s\n", label, strings.Join(parts, "; "))
	if failed {
		return 1
	}
	return 0
}

func (b *evmWalletBackfill) tally(ctx context.Context, k evmWalletKind, holders []evmWalletHolder) evmWalletTally {
	var t evmWalletTally
	for _, h := range holders {
		switch {
		case h.held:
			t.existing++
		case b.dryRun:
			t.created++
		default:
			addr, err := b.ensure(ctx, h)
			if err != nil {
				t.failed++
				_, _ = fmt.Fprintf(b.stderr, "evm-wallets: %s %s: %v\n", k.name, h.id, err)
				continue
			}
			t.created++
			_, _ = fmt.Fprintf(b.stdout, "%s\t%s\t%s\n", k.name, h.id, addr)
		}
	}
	return t
}

func (b *evmWalletBackfill) ensure(ctx context.Context, h evmWalletHolder) (chain.EVMAddress, error) {
	if b.ensured > 0 {
		<-b.clock.After(evmWalletPace)
	}
	b.ensured++
	return h.ensure(ctx)
}
