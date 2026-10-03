package treasury_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

func TestLedger_aCanceledContextStopsEveryWriteAtItsFirstStatement(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	u, c, err := f.fund(f.user(t), f.cabal(t), 10, 10, domain.TxnPending)
	if err != nil {
		t.Fatal(err)
	}
	writes := map[string]func(ctx context.Context, tx db.Tx) error{
		"post cabal": func(ctx context.Context, tx db.Tx) error { return f.ledger.PostCabalTxn(ctx, tx, c) },
		"post user":  func(ctx context.Context, tx db.Tx) error { return f.ledger.PostUserTxn(ctx, tx, u) },
		"set status": func(ctx context.Context, tx db.Tx) error {
			_, err := f.ledger.SetStatus(ctx, tx, u.TransferID, domain.TxnPending, domain.TxnSettled)
			return err
		},
	}
	for name, write := range writes {
		ctx, cancel := context.WithCancel(f.ctx())
		err := f.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
			cancel()
			return write(ctx, tx)
		})
		t.Logf("%s: %v", name, err)
		wantCode(t, err, errs.CodeDBUnavailable)
	}
}

func TestLedger_theDatabaseRefusesDuplicateHeaders(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	u, c, err := f.fund(user, cabal, 10, 10, domain.TxnSettled)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
	wantCode(t, f.postCabal(c), errs.CodeInternal)
	err = f.do(func(ctx context.Context, tx db.Tx) error { return f.ledger.PostUserTxn(ctx, tx, u) })
	wantCode(t, err, errs.CodeInternal)
	if drift := f.drift(t); len(drift) != 0 || f.count(t, "user_txns") != 1 || f.count(t, "cabal_txns") != 1 {
		t.Fatalf("drift %q; want only the first fund pair stored", drift)
	}
}

func TestLedgerReturnsHeaderWriteErrors(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	u, c, err := f.fund(user, cabal, 10, 10, domain.TxnSettled)
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"user_txns", "cabal_txns"} {
		if _, err := f.pool.Exec(t.Context(), "ALTER TABLE "+table+" RENAME TO "+table+"_gone"); err != nil {
			t.Fatal(err)
		}
		var got error
		if table == "user_txns" {
			got = f.do(func(ctx context.Context, tx db.Tx) error { return f.ledger.PostUserTxn(ctx, tx, u) })
		} else {
			got = f.do(func(ctx context.Context, tx db.Tx) error { return f.ledger.PostCabalTxn(ctx, tx, c) })
		}
		if got == nil {
			t.Fatalf("%s write error = nil", table)
		}
	}
}

func TestLedger_theDatabaseRefusesUnknownAccountsAndShareOverdrafts(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	u, c, err := f.fund(user, cabal, 10, 10, domain.TxnSettled)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
	head := domain.CabalTxnHeader{ID: f.ids.NewV7(), CabalID: cabal, Kind: domain.CabalFund, Status: domain.TxnSettled}
	bogus, err := domain.NewCabalTxn(head, []domain.CabalEntry{
		{Account: "vault", Asset: usdc, Amount: amount(1)},
		{Account: domain.CabalTreasury, Asset: usdc, Amount: amount(-1)},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantCode(t, f.postCabal(bogus), errs.CodeInternal)
	deposit := domain.UserTxnHeader{
		ID: f.ids.NewV7(), UserID: user, Kind: domain.UserDeposit, Status: domain.TxnSettled,
	}
	strange, err := domain.NewUserTxn(deposit, []domain.UserEntry{
		{Account: "vault", Asset: usdc, Amount: amount(1)},
		{Account: domain.UserWallet, Asset: usdc, Amount: amount(-1)},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = f.do(func(ctx context.Context, tx db.Tx) error { return f.ledger.PostUserTxn(ctx, tx, strange) })
	wantCode(t, err, errs.CodeInternal)
	out, back, err := f.cashOut(user, cabal, 1, 11, domain.TxnSettled)
	if err != nil {
		t.Fatal(err)
	}
	wantCode(t, f.postPair(out, back), errs.CodeInternal)
	if drift := f.drift(t); len(drift) != 0 || f.count(t, "user_txns") != 1 || f.count(t, "cabal_txns") != 1 {
		t.Fatalf("drift %q; want only the first fund pair stored", drift)
	}
}
