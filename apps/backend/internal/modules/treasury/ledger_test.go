package treasury_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type position struct {
	Asset string
	Units string
	Cost  string
}

func (f fixture) cabalPositions(t *testing.T) []position {
	t.Helper()
	rows, err := f.pool.Query(t.Context(),
		`SELECT asset, units::text, cost_basis_micros::text FROM cabal_positions ORDER BY asset`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []position
	for rows.Next() {
		var p position
		if err := rows.Scan(&p.Asset, &p.Units, &p.Cost); err != nil {
			t.Fatal(err)
		}
		out = append(out, p)
	}
	return out
}

func (f fixture) userPosition(t *testing.T) string {
	t.Helper()
	var got string
	if err := f.pool.QueryRow(t.Context(), `SELECT concat_ws('/', share_units, contributed_micros, withdrawn_micros)
		FROM user_positions`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestLedger_postsTransfersAndSwapsAndProjectsPositions(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	u, c, err := f.fund(user, cabal, 100, 100, domain.TxnPending)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
	for _, trade := range [][2]int64{{60, 3}, {-25, -1}} {
		swap, err := f.swap(cabal, trade[0], trade[1])
		if err != nil {
			t.Fatal(err)
		}
		if err := f.postCabal(swap); err != nil {
			t.Fatal(err)
		}
	}
	want := []position{{Asset: string(usdc), Units: "65", Cost: "65"}, {Asset: string(aapl), Units: "2", Cost: "40"}}
	if got := f.cabalPositions(t); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("cabal positions = %+v, want %+v", got, want)
	}
	if got := f.userPosition(t); got != "100/100/0" {
		t.Fatalf("user position = %s, want 100 shares, 100 contributed, 0 withdrawn", got)
	}
	if drift := f.drift(t); len(drift) != 0 {
		t.Fatalf("drift = %q", drift)
	}
	assertPostedLog(t, f, cabal.String(), string(aapl), "3", "2", "-1")
	assertPostedLog(t, f, cabal.String(), string(domain.SharesAsset(cabal)), "0", "100", "100")
}

func assertPostedLog(t *testing.T, f fixture, cabal, asset, before, after, delta string) {
	t.Helper()
	for line := range strings.Lines(string(f.logs.Bytes())) {
		var got map[string]any
		if json.Unmarshal([]byte(line), &got) != nil || got["msg"] != "treasury.ledger.posted" {
			continue
		}
		if got["cabal_id"] == cabal && got["asset"] == asset && got["before"] == before && got["after"] == after &&
			got["delta"] == delta && got["level"] == "INFO" {
			return
		}
	}
	t.Fatalf("no treasury.ledger.posted line for %s %s %s -> %s (%s) in\n%s", cabal, asset, before, after, delta,
		f.logs.Bytes())
}

func TestLedger_UnbalancedRejected(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.cabal(t)
	err := f.do(func(ctx context.Context, tx db.Tx) error {
		txn, err := domain.NewCabalTxn(domain.CabalTxnHeader{ID: f.ids.NewV7(), CabalID: cabal, Kind: domain.CabalSwap},
			[]domain.CabalEntry{
				{Account: domain.CabalTreasury, Asset: usdc, Amount: amount(-60)},
				{Account: domain.CabalVenue, Asset: usdc, Amount: amount(59)},
			})
		if err != nil {
			return err
		}
		return f.ledger.PostCabalTxn(ctx, tx, txn)
	})
	wantCode(t, err, errs.CodeLedgerUnbalanced)
	for _, table := range []string{"cabal_txns", "cabal_txn_entries", "cabal_positions", "events"} {
		if n := f.count(t, table); n != 0 {
			t.Fatalf("%s has %d rows, want none", table, n)
		}
	}
}

func TestLedger_refusesAnOverdraftAndAHeaderThatSplitsATransfer(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	swap, err := f.swap(cabal, 60, 3)
	if err != nil {
		t.Fatal(err)
	}
	wantCode(t, f.postCabal(swap), errs.CodeInternal)
	u, c, err := f.fund(user, cabal, 10, 10, domain.TxnPending)
	if err != nil {
		t.Fatal(err)
	}
	c.Status = domain.TxnSettled
	wantCode(t, f.postPair(u, c), errs.CodeInternal)
	u.Status, c.Status, c.CabalID = domain.TxnSettled, domain.TxnPending, f.cabal(t)
	err = f.do(func(ctx context.Context, tx db.Tx) error {
		if err := f.ledger.PostCabalTxn(ctx, tx, c); err != nil {
			return err
		}
		return f.ledger.PostUserTxn(ctx, tx, u)
	})
	wantCode(t, err, errs.CodeInternal)
	for _, table := range []string{"cabal_txns", "user_txns", "cabal_positions", "user_positions"} {
		if n := f.count(t, table); n != 0 {
			t.Fatalf("%s has %d rows, want none", table, n)
		}
	}
}

func TestLedger_SetStatusMovesBothHeadersOnce(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	u, c, err := f.fund(user, cabal, 10, 10, domain.TxnPending)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
	for i, want := range []bool{true, false} {
		var moved bool
		err := f.do(func(ctx context.Context, tx db.Tx) error {
			moved, err = f.ledger.SetStatus(ctx, tx, u.TransferID, domain.TxnPending, domain.TxnSettled)
			return err
		})
		if err != nil || moved != want {
			t.Fatalf("SetStatus #%d = %v, %v, want %v", i, moved, err, want)
		}
	}
	var statuses string
	if err := f.pool.QueryRow(t.Context(), `SELECT (SELECT status FROM user_txns) || '/' ||
		(SELECT status FROM cabal_txns)`).Scan(&statuses); err != nil || statuses != "settled/settled" {
		t.Fatalf("statuses = %q, %v", statuses, err)
	}
	err = f.do(func(ctx context.Context, tx db.Tx) error {
		_, err := f.ledger.SetStatus(ctx, tx, u.TransferID, domain.TxnSettled, domain.TxnFailed)
		return err
	})
	wantCode(t, err, errs.CodeInvalidInput)
}

func TestLedger_SetStatusRefusesAOneSidedTransfer(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	u, _, err := f.fund(f.user(t), f.cabal(t), 10, 10, domain.TxnPending)
	if err != nil {
		t.Fatal(err)
	}
	err = f.do(func(ctx context.Context, tx db.Tx) error {
		if err := f.ledger.PostUserTxn(ctx, tx, u); err != nil {
			return err
		}
		_, err := f.ledger.SetStatus(ctx, tx, u.TransferID, domain.TxnPending, domain.TxnFailed)
		return err
	})
	wantCode(t, err, errs.CodeInternal)
	if n := f.count(t, "user_txns"); n != 0 {
		t.Fatalf("one-sided SetStatus left %d user_txns, want a rollback", n)
	}
}

func TestLedger_aDepositOutsideACabalTouchesNoPosition(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	txn, err := domain.NewUserTxn(domain.UserTxnHeader{
		ID: f.ids.NewV7(), UserID: f.user(t), Kind: domain.UserDeposit, Status: domain.TxnSettled,
	}, []domain.UserEntry{
		{Account: domain.UserExternal, Asset: usdc, Amount: amount(-5)},
		{Account: domain.UserWallet, Asset: usdc, Amount: amount(5)},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = f.do(func(ctx context.Context, tx db.Tx) error { return f.ledger.PostUserTxn(ctx, tx, txn) })
	if err != nil {
		t.Fatal(err)
	}
	var cabal *string
	if err := f.pool.QueryRow(t.Context(), `SELECT cabal_id::text FROM user_txns`).Scan(&cabal); err != nil ||
		cabal != nil || f.count(t, "user_txn_entries") != 2 || f.count(t, "user_positions") != 0 {
		t.Fatalf("deposit stored cabal %v, err %v; want a cabal-less header, two entries and no position", cabal, err)
	}
}

func TestLedger_positionDeltaErrorsStopThePost(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.cabal(t)
	two, err := domain.NewCabalTxn(domain.CabalTxnHeader{ID: f.ids.NewV7(), CabalID: cabal}, []domain.CabalEntry{
		{Account: domain.CabalTreasury, Asset: usdc, Amount: amount(-2)},
		{Account: domain.CabalTreasury, Asset: aapl, Amount: amount(1)},
		{Account: domain.CabalTreasury, Asset: "XsDoVfqeBukxuZHWhdvWHBhgEHjGNst4MLodqsJHzoB", Amount: amount(1)},
		{Account: domain.CabalVenue, Asset: usdc, Amount: amount(2)},
		{Account: domain.CabalVenue, Asset: aapl, Amount: amount(-1)},
		{Account: domain.CabalVenue, Asset: "XsDoVfqeBukxuZHWhdvWHBhgEHjGNst4MLodqsJHzoB", Amount: amount(-1)},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantCode(t, f.postCabal(two), errs.CodeInvalidInput)
	shares := domain.SharesAsset(cabal)
	huge, err := domain.NewUserTxn(domain.UserTxnHeader{ID: f.ids.NewV7(), UserID: f.user(t), CabalID: cabal},
		[]domain.UserEntry{
			{Account: domain.UserHolder, Asset: shares, Amount: amount(1 << 62)},
			{Account: domain.UserHolder, Asset: shares, Amount: amount(1 << 62)},
			{Account: domain.UserIssuer, Asset: shares, Amount: amount(-(1 << 62))},
			{Account: domain.UserIssuer, Asset: shares, Amount: amount(-(1 << 62))},
		})
	if err != nil {
		t.Fatal(err)
	}
	err = f.do(func(ctx context.Context, tx db.Tx) error { return f.ledger.PostUserTxn(ctx, tx, huge) })
	wantCode(t, err, errs.CodeInvalidInput)
}

func TestLedger_refusesATxnWithNoEntriesBeforeAnyWrite(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.cabal(t)
	cabalTxn := domain.CabalTxn{CabalTxnHeader: domain.CabalTxnHeader{
		ID: f.ids.NewV7(), CabalID: cabal, Kind: domain.CabalFund, Status: domain.TxnSettled,
	}}
	userTxn := domain.UserTxn{UserTxnHeader: domain.UserTxnHeader{
		ID: f.ids.NewV7(), UserID: f.user(t), CabalID: cabal, Kind: domain.UserFund, Status: domain.TxnSettled,
	}}
	var cabalErr, userErr error
	err := f.do(func(ctx context.Context, tx db.Tx) error {
		cabalErr = f.ledger.PostCabalTxn(ctx, tx, cabalTxn)
		userErr = f.ledger.PostUserTxn(ctx, tx, userTxn)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	wantCode(t, cabalErr, errs.CodeInvalidInput)
	wantCode(t, userErr, errs.CodeInvalidInput)
	for _, table := range []string{
		"cabal_txns", "cabal_txn_entries", "cabal_positions", "user_txns", "user_txn_entries", "user_positions", "events",
	} {
		if n := f.count(t, table); n != 0 {
			t.Fatalf("%s has %d rows after the refused posts committed, want none", table, n)
		}
	}
}

func (f fixture) spend(cabal ids.CabalID, held, release chan struct{}) error {
	swap, err := f.swap(cabal, 60, 3)
	if err != nil {
		return err
	}
	return f.do(func(ctx context.Context, tx db.Tx) error {
		if held != nil {
			if err := f.ledger.LockCabal(ctx, tx, cabal); err != nil {
				return err
			}
			close(held)
			<-release
		}
		return f.ledger.PostCabalTxn(ctx, tx, swap)
	})
}

func TestLedger_concurrentPostsOnOneCabalSerializeThroughLockCabal(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	u, c, err := f.fund(user, cabal, 100, 100, domain.TxnSettled)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
	held, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	var spenders sync.WaitGroup
	t.Cleanup(spenders.Wait)
	t.Cleanup(unblock)
	holderDone, waiterDone := make(chan error, 1), make(chan error, 1)
	spenders.Go(func() { holderDone <- f.spend(cabal, held, release) })
	<-held
	spenders.Go(func() { waiterDone <- f.spend(cabal, nil, nil) })
	testkit.Eventually(t, func() bool {
		var waiting int
		err := f.pool.QueryRow(t.Context(),
			`SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND NOT granted
			AND database = (SELECT oid FROM pg_database WHERE datname = current_database())`).Scan(&waiting)
		return err == nil && waiting == 1
	}, 10*time.Second)
	unblock()
	if err := <-holderDone; err != nil {
		t.Fatalf("holder = %v, want its spend of 60 to succeed", err)
	}
	wantCode(t, <-waiterDone, errs.CodeInternal)
	if f.count(t, "cabal_txns") != 2 || len(f.drift(t)) != 0 {
		t.Fatalf("want the fund and one swap, and no drift: %q", f.drift(t))
	}
}
