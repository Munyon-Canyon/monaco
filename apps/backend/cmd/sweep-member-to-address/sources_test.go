package main

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalsqlc "github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	identitysqlc "github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

type fakeTables struct {
	members    []chain.Wallet
	treasuries []chain.Wallet
}

func (f fakeTables) ListSweepMemberWallets(context.Context) ([]identitysqlc.ListSweepMemberWalletsRow, error) {
	rows := make([]identitysqlc.ListSweepMemberWalletsRow, 0, len(f.members))
	for _, w := range f.members {
		rows = append(rows, identitysqlc.ListSweepMemberWalletsRow{PrivyWalletID: w.ID, Address: string(w.Address)})
	}
	return rows, nil
}

func (f fakeTables) ListTreasuryWallets(context.Context) ([]cabalsqlc.TreasuryWallet, error) {
	rows := make([]cabalsqlc.TreasuryWallet, 0, len(f.treasuries))
	for _, w := range f.treasuries {
		rows = append(rows, cabalsqlc.TreasuryWallet{PrivyWalletID: w.ID, Address: string(w.Address)})
	}
	return rows, nil
}

type fakePrivy struct {
	wallets []chain.Wallet
	err     error
	calls   int
}

func (f *fakePrivy) ListAppWallets(context.Context) ([]chain.Wallet, error) {
	f.calls++
	return f.wallets, f.err
}

func load(t *testing.T, flags sweepFlags, privy *fakePrivy) ([]sweepSource, string, error) {
	t.Helper()
	return loadSweepSources(t.Context(), flags, tables, tables, privy)
}

func fromDB(w chain.Wallet) chain.Wallet { return chain.Wallet{ID: w.ID, Address: w.Address} }

func TestLoadSweepSources_dbDefaultIsEveryUserAndTreasuryWallet(t *testing.T) {
	t.Parallel()
	privy := &fakePrivy{}
	sources, note, err := load(t, sweepFlags{destination: dest}, privy)
	want := []sweepSource{{kindMember, fromDB(member)}, {kindTreasury, fromDB(cabal)}}
	if err != nil || note != "postgres user_wallets + treasury_wallets" || !slices.Equal(sources, want) {
		t.Fatalf("sources = %+v, %q, %v", sources, note, err)
	}
	if privy.calls != 0 {
		t.Fatalf("the default asked Privy %d times", privy.calls)
	}
}

func TestLoadSweepSources_explicitListKeepsOrderClassifiesAndDedupes(t *testing.T) {
	t.Parallel()
	privy := &fakePrivy{wallets: []chain.Wallet{stray}}
	unknown := chain.SolanaAddress("9ixcyg5nNxGCJLtSyJYibP7EgQBw4BfpNLbDe7GQ14eh")
	flags := sweepFlags{destination: dest, sources: []chain.SolanaAddress{
		cabal.Address, member.Address, stray.Address, unknown, cabal.Address,
	}}
	sources, note, err := load(t, flags, privy)
	want := []sweepSource{
		{kindTreasury, fromDB(cabal)},
		{kindMember, fromDB(member)},
		{kindExplicit, stray},
		{kindExplicit, chain.Wallet{Address: unknown}},
	}
	if err != nil || note != "explicit wallets (4 --source)" || !slices.Equal(sources, want) {
		t.Fatalf("sources = %+v, %q, %v", sources, note, err)
	}
	if privy.calls != 1 {
		t.Fatalf("Privy listed %d times, want once for the addresses the database lacks", privy.calls)
	}
}

func TestLoadSweepSources_explicitDatabaseWalletsNeverAskPrivy(t *testing.T) {
	t.Parallel()
	privy := &fakePrivy{}
	flags := sweepFlags{destination: dest, sources: []chain.SolanaAddress{member.Address}}
	if _, _, err := load(t, flags, privy); err != nil || privy.calls != 0 {
		t.Fatalf("err = %v, privy calls = %d", err, privy.calls)
	}
}

func TestLoadSweepSources_allTakesPrivyAsTheSourceOfTruth(t *testing.T) {
	t.Parallel()
	privy := &fakePrivy{wallets: []chain.Wallet{member, stray}}
	sources, note, err := load(t, sweepFlags{destination: dest, all: true}, privy)
	want := []sweepSource{{kindMember, member}, {kindPrivy, stray}}
	if err != nil || note != "privy app wallets (--all)" || !slices.Equal(sources, want) {
		t.Fatalf("sources = %+v, %q, %v", sources, note, err)
	}
}

type failingTables struct {
	fakeTables
	members, treasuries error
}

func (f failingTables) ListSweepMemberWallets(ctx context.Context) ([]identitysqlc.ListSweepMemberWalletsRow, error) {
	rows, _ := f.fakeTables.ListSweepMemberWallets(ctx)
	return rows, f.members
}

func (f failingTables) ListTreasuryWallets(ctx context.Context) ([]cabalsqlc.TreasuryWallet, error) {
	rows, _ := f.fakeTables.ListTreasuryWallets(ctx)
	return rows, f.treasuries
}

func TestLoadSweepSources_returnsEachReadError(t *testing.T) {
	t.Parallel()
	down := errs.New(errs.CodeInternal, "test.down")
	explicit := []chain.SolanaAddress{stray.Address}
	for name, tc := range map[string]struct {
		tables failingTables
		flags  sweepFlags
		privy  error
	}{
		"user_wallets":          {failingTables{tables, down, nil}, sweepFlags{}, nil},
		"treasury_wallets":      {failingTables{tables, nil, down}, sweepFlags{}, nil},
		"privy for --all":       {failingTables{fakeTables: tables}, sweepFlags{all: true}, down},
		"privy for an explicit": {failingTables{fakeTables: tables}, sweepFlags{sources: explicit}, down},
	} {
		_, _, err := loadSweepSources(t.Context(), tc.flags, tc.tables, tc.tables, &fakePrivy{err: tc.privy})
		if !errors.Is(err, down) {
			t.Fatalf("%s: err = %v, want %v", name, err, down)
		}
	}
}
