package treasury_test

import (
	"context"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const swapSignature = "5VERv8NMvzbJMEkV8xnrLkEaWRtSz9CosKDYjCJjBRnbJLgp8uirBgmQpjKhoR4tjF3ZpRzrFmBV6UjKdiSZkQUW"

func ledgerReads(f fixture) *app.LedgerReads {
	return treasury.New(module.Deps{Config: f.cfg, Pool: f.pool, IDs: f.ids, Clock: f.clock}).Ledger()
}

func seedFunds(t *testing.T, f fixture, user ids.UserID, cabal ids.CabalID, count int) []uuid.UUID {
	t.Helper()
	out := make([]uuid.UUID, count)
	for i := range out {
		u, c, err := f.fund(user, cabal, 10_000_000, 10, domain.TxnSettled)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.postPair(u, c); err != nil {
			t.Fatal(err)
		}
		out[i] = u.ID
		f.clock.Advance(time.Minute)
	}
	return out
}

func txnIDs(headers []port.TxnHeader) []uuid.UUID {
	out := make([]uuid.UUID, len(headers))
	for i, h := range headers {
		out[i] = h.ID
	}
	return out
}

func after(h port.TxnHeader) *port.TxnCursor { return &port.TxnCursor{At: h.CreatedAt, ID: h.ID} }

func TestLedgerReads_UserTxns_NewestFirstThenCursor(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	seeded := seedFunds(t, f, user, cabal, 3)
	reads := ledgerReads(f)
	first, err := reads.UserTxns(t.Context(), user, nil, 2)
	if err != nil || !slices.Equal(txnIDs(first), []uuid.UUID{seeded[2], seeded[1]}) {
		t.Fatalf("first page = %+v, %v", first, err)
	}
	next, err := reads.UserTxns(t.Context(), user, after(first[1]), 2)
	if err != nil || !slices.Equal(txnIDs(next), []uuid.UUID{seeded[0]}) {
		t.Fatalf("next page = %+v, %v", next, err)
	}
	other, err := reads.UserTxns(t.Context(), f.user(t), nil, 5)
	if err != nil || len(other) != 0 {
		t.Fatalf("other user = %+v, %v", other, err)
	}
}

func TestLedgerReads_UserTxns_HeaderCarriesTheRow(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	seeded := seedFunds(t, f, user, cabal, 1)
	got, err := ledgerReads(f).UserTxns(t.Context(), user, nil, 1)
	if err != nil || len(got) != 1 {
		t.Fatalf("UserTxns = %+v, %v", got, err)
	}
	want := port.TxnHeader{
		ID: seeded[0], Scope: port.TxnScopeUser, UserID: &user, CabalID: &cabal, Kind: "fund", Status: "settled",
		CreatedAt: f.clock.Now().Add(-time.Minute),
	}
	head := got[0]
	head.TransferID = nil
	if !reflect.DeepEqual(head, want) {
		t.Fatalf("header = %+v, want %+v", head, want)
	}
}

func TestLedgerReads_CabalTxns_NewestFirstThenCursor(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	seedFunds(t, f, user, cabal, 2)
	swap, err := f.swap(cabal, 5_000_000, 3)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postCabal(swap); err != nil {
		t.Fatal(err)
	}
	reads := ledgerReads(f)
	first, err := reads.CabalTxns(t.Context(), cabal, nil, 2)
	if err != nil || len(first) != 2 || first[0].ID != swap.ID {
		t.Fatalf("first page = %+v, %v", first, err)
	}
	if head := first[0]; head.Kind != "swap" || head.TxSignature != swapSignature || head.UserID != nil {
		t.Fatalf("swap header = %+v", head)
	}
	next, err := reads.CabalTxns(t.Context(), cabal, after(first[1]), 5)
	if err != nil || len(next) != 1 {
		t.Fatalf("next page = %+v, %v", next, err)
	}
}

func TestLedgerReads_TxnByID_UserEntries(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	u, c, err := f.fund(user, cabal, 10_000_000, 10, domain.TxnSettled)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
	got, found, err := ledgerReads(f).TxnByID(t.Context(), u.ID)
	if err != nil || !found || got.Scope != port.TxnScopeUser || len(got.Entries) != 4 {
		t.Fatalf("user txn = %+v, %v, %v", got, found, err)
	}
	if !reflect.DeepEqual(got.UserID, &user) || got.TransferID == nil || got.SwapID != nil {
		t.Fatalf("user txn links = %+v", got.TxnHeader)
	}
	if entry := got.Entries[0]; entry.Account != "wallet" || entry.Amount != -10_000_000 {
		t.Fatalf("first entry = %+v", entry)
	}
}

func TestLedgerReads_TxnByID_CabalEntriesAndMiss(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	u, c, err := f.fund(f.user(t), f.cabal(t), 10_000_000, 10, domain.TxnSettled)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
	reads := ledgerReads(f)
	got, found, err := reads.TxnByID(t.Context(), c.ID)
	if err != nil || !found || got.Scope != port.TxnScopeCabal || len(got.Entries) != 2 || got.UserID != nil {
		t.Fatalf("cabal txn = %+v, %v, %v", got, found, err)
	}
	if _, found, err = reads.TxnByID(t.Context(), f.ids.NewV7()); err != nil || found {
		t.Fatalf("unknown txn = %v, %v", found, err)
	}
}

func TestLedgerReads_TxnBySignature_LinksTheSwap(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	user, cabal := f.user(t), f.cabal(t)
	seedFunds(t, f, user, cabal, 1)
	swap, err := f.swap(cabal, 5_000_000, 3)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postCabal(swap); err != nil {
		t.Fatal(err)
	}
	reads := ledgerReads(f)
	got, found, err := reads.TxnBySignature(t.Context(), swapSignature)
	if err != nil || !found || got.ID != swap.ID || got.SwapID == nil || len(got.Entries) != 4 {
		t.Fatalf("by signature = %+v, %v, %v", got, found, err)
	}
	if _, found, err = reads.TxnBySignature(t.Context(), "unknown"); err != nil || found {
		t.Fatalf("unknown signature = %v, %v", found, err)
	}
}

func TestLedgerReads_TxnBySignature_PrefersTheCabalScope(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	u, c, err := f.fund(f.user(t), f.cabal(t), 10_000_000, 10, domain.TxnSettled)
	if err != nil {
		t.Fatal(err)
	}
	const shared = chain.Signature("2Z9mS6kXkEJ6bq1G1t9mYz6w3R2tPpJ7mQ1u2kYb8mVfCk4Lw8x7Dn5Hq3ZsTg6RaYe1Nc9XvWbUjP4dKfM")
	u.TxSignature, c.TxSignature = shared, shared
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
	got, found, err := ledgerReads(f).TxnBySignature(t.Context(), shared)
	if err != nil || !found || got.Scope != port.TxnScopeCabal || got.ID != c.ID {
		t.Fatalf("shared signature = %+v, %v, %v", got, found, err)
	}
}

func TestLedgerReads_Shares_UserAndCabal(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, other, cabal := f.user(t), f.user(t), f.cabal(t)
	seedFunds(t, f, user, cabal, 2)
	seedFunds(t, f, other, cabal, 1)
	reads := ledgerReads(f)
	mine, err := reads.UserShares(t.Context(), user)
	if err != nil || len(mine) != 1 || mine[0].CabalID != cabal || mine[0].UserID != user ||
		mine[0].ShareUnits.Uint64() == 0 {
		t.Fatalf("UserShares() = %+v, %v", mine, err)
	}
	all, err := reads.CabalShares(t.Context(), cabal)
	if err != nil || len(all) != 2 {
		t.Fatalf("CabalShares() = %+v, %v", all, err)
	}
	none, err := reads.CabalShares(t.Context(), f.cabal(t))
	if err != nil || len(none) != 0 {
		t.Fatalf("empty CabalShares() = %+v, %v", none, err)
	}
}

func TestLedgerReads_Shares_DecodeFailure(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	const insert = `INSERT INTO user_positions
  (user_id, cabal_id, share_units, contributed_micros, withdrawn_micros, updated_at)
VALUES ($1, $2, 99999999999999999999, 0, 0, $3)`
	if _, err := f.pool.Exec(t.Context(), insert, user.UUID(), cabal.UUID(), f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	reads := ledgerReads(f)
	if _, err := reads.UserShares(t.Context(), user); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("UserShares() error = %v", err)
	}
	if _, err := reads.CabalShares(t.Context(), cabal); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("CabalShares() error = %v", err)
	}
}

func TestLedgerReads_CabalHoldings_DecodeFailure(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.cabal(t)
	const insert = `INSERT INTO cabal_positions (cabal_id, asset, units, cost_basis_micros, updated_at)
VALUES ($1, 'Mint', 99999999999999999999, 1, $2)`
	if _, err := f.pool.Exec(t.Context(), insert, cabal.UUID(), f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := ledgerReads(f).CabalHoldings(t.Context(), cabal); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("CabalHoldings() error = %v", err)
	}
}

func TestLedgerReads_CabalHoldings_RawUnitsWithoutTheCatalog(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	seedFunds(t, f, user, cabal, 1)
	const insert = `INSERT INTO cabal_positions (cabal_id, asset, units, cost_basis_micros, updated_at)
VALUES ($1, 'UnlistedMint1111111111111111111111111111111', 7, 1, $2)`
	if _, err := f.pool.Exec(t.Context(), insert, cabal.UUID(), f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	got, err := ledgerReads(f).CabalHoldings(t.Context(), cabal)
	if err != nil || len(got) != 2 || got[0].Units.Uint64() != 10_000_000 || got[1].Units.Uint64() != 7 ||
		got[1].Mint != "UnlistedMint1111111111111111111111111111111" {
		t.Fatalf("CabalHoldings() = %+v, %v", got, err)
	}
	none, err := ledgerReads(f).CabalHoldings(t.Context(), f.cabal(t))
	if err != nil || len(none) != 0 {
		t.Fatalf("empty CabalHoldings() = %+v, %v", none, err)
	}
}

func TestLedgerReads_DatabaseDown_ReportsDBUnavailable(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	reads := ledgerReads(f)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	user, cabal := f.user(t), f.cabal(t)
	calls := map[string]func() error{
		"UserTxns":       func() error { _, err := reads.UserTxns(ctx, user, nil, 1); return err },
		"CabalTxns":      func() error { _, err := reads.CabalTxns(ctx, cabal, nil, 1); return err },
		"TxnByID":        func() error { _, _, err := reads.TxnByID(ctx, f.ids.NewV7()); return err },
		"TxnBySignature": func() error { _, _, err := reads.TxnBySignature(ctx, "sig"); return err },
		"UserShares":     func() error { _, err := reads.UserShares(ctx, user); return err },
		"CabalShares":    func() error { _, err := reads.CabalShares(ctx, cabal); return err },
		"CabalHoldings":  func() error { _, err := reads.CabalHoldings(ctx, cabal); return err },
	}
	for name, call := range calls {
		if got := errs.CodeOf(call()); got != errs.CodeDBUnavailable {
			t.Errorf("%s code = %q", name, got)
		}
	}
}

func TestLedgerReads_QueryCount(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	u, c, err := f.fund(user, cabal, 10_000_000, 10, domain.TxnSettled)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
	reads := ledgerReads(f)
	check := func(name string, call func() error) {
		t.Helper()
		testkit.AssertQueries(t, name, func() {
			if err := call(); err != nil {
				t.Fatal(err)
			}
		})
	}
	check("LedgerReads UserTxns", func() error { _, err := reads.UserTxns(t.Context(), user, nil, 20); return err })
	check("LedgerReads CabalTxns", func() error { _, err := reads.CabalTxns(t.Context(), cabal, nil, 20); return err })
	check("LedgerReads TxnByID", func() error { _, _, err := reads.TxnByID(t.Context(), u.ID); return err })
	check(
		"LedgerReads TxnBySignature",
		func() error { _, _, err := reads.TxnBySignature(t.Context(), "sig"); return err },
	)
	check("LedgerReads UserShares", func() error { _, err := reads.UserShares(t.Context(), user); return err })
	check("LedgerReads CabalShares", func() error { _, err := reads.CabalShares(t.Context(), cabal); return err })
	check("LedgerReads CabalHoldings", func() error { _, err := reads.CabalHoldings(t.Context(), cabal); return err })
}
