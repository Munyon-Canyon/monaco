package treasury_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type ledgerView struct {
	Wallet, Balance, Position string
}

func (f fixture) view(t *testing.T, user ids.UserID, cabal ids.CabalID) ledgerView {
	t.Helper()
	q := sqlc.New(f.pool)
	wallet, err := q.WalletLedgerMicros(
		t.Context(),
		sqlc.WalletLedgerMicrosParams{UserID: user.UUID(), Asset: usdcMint},
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := q.LedgerBalances(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	v := ledgerView{Wallet: wallet.Settled, Balance: "0"}
	for _, r := range rows {
		if r.Owner == "wallet:"+user.String() && r.Asset == usdcMint {
			v.Balance = r.Balance
		}
	}
	if err := f.pool.QueryRow(t.Context(), `SELECT share_units || '/' || contributed_micros || '/' || withdrawn_micros
		FROM user_positions WHERE user_id = $1 AND cabal_id = $2`, user.UUID(), cabal.UUID()).Scan(&v.Position); err != nil {
		t.Fatal(err)
	}
	return v
}

func (f fixture) wantView(t *testing.T, user ids.UserID, cabal ids.CabalID, want ledgerView) {
	t.Helper()
	if got := f.view(t, user, cabal); got != want {
		t.Fatalf("ledger view = %+v, want %+v", got, want)
	}
	if drift := f.drift(t); len(drift) != 0 {
		t.Fatalf("ledger drift = %#v", drift)
	}
}

func beforeCashOut() ledgerView {
	return ledgerView{Wallet: "-50000000", Balance: "-50000000", Position: "100/50000000/0"}
}

func TestFailedTxn_aFailedSaleCashOutLeavesEveryViewAtItsPreCashOutValue(t *testing.T) {
	t.Parallel()
	r := newSaleRig(t, 100_000_000)
	r.deliver(t, r.started)
	r.deliver(t, r.unsold(r.f.ids.NewV7(), 500, 1))
	r.want(t, "failed", "50000000", "100", "sale_short", 100)
	r.f.wantView(t, r.alice, r.cabal, beforeCashOut())
}

func TestFailedTxn_aFailedPayoutCashOutLeavesEveryViewAtItsPreCashOutValue(t *testing.T) {
	t.Parallel()
	r := newPayoutRig(t)
	r.seed(t, 1, domain.PayoutBroadcast)
	r.chain.set(seededSig(1), refused())
	r.mustAdvance(t)
	r.wantJob(t, "failed", "payout_failed")
	r.mustAdvance(t)
	r.f.wantView(t, r.alice, r.cabal, beforeCashOut())
}

func TestFailedTxn_aShortSalePartialKeepsTheViewsAndThePendingPayoutApart(t *testing.T) {
	t.Parallel()
	r := newSaleRig(t, 80_000_000)
	r.deliver(t, r.started)
	r.deliver(t, r.sold(r.f.ids.NewV7(), 400, 15_000_000, 2))
	r.deliver(t, r.unsold(r.f.ids.NewV7(), 200, 2))
	r.want(t, "paying", "35000000", "30", "sale_short", 30)
	got := r.f.view(t, r.alice, r.cabal)
	want := ledgerView{Wallet: "-65000000", Balance: "-15000000", Position: "30/65000000/50000000"}
	if got != want {
		t.Fatalf("ledger view = %+v, want %+v", got, want)
	}
}

func TestFailedTxn_aFailedCashOutLeavesStakeHistoryUnchanged(t *testing.T) {
	t.Parallel()
	r := newSaleRig(t, 100_000_000)
	r.deliver(t, r.started)
	r.deliver(t, r.unsold(r.f.ids.NewV7(), 500, 1))
	rows, err := sqlc.New(r.f.pool).UserStakeHistory(t.Context(), r.alice.UUID())
	if err != nil || len(rows) != 1 || rows[0].ShareUnits != "100" || rows[0].NetContributedMicros != "50000000" {
		t.Fatalf("stake history = %+v, %v, want the one fund row", rows, err)
	}
	contrib, err := sqlc.New(r.f.pool).CabalContributionHistory(t.Context(), r.cabal.UUID())
	if err != nil || len(contrib) != 2 || contrib[1].NetContributedMicros != "100000000" {
		t.Fatalf("contribution history = %+v, %v, want only the two funds", contrib, err)
	}
	flows, err := sqlc.New(r.f.pool).MemberFlowsBetween(t.Context(), sqlc.MemberFlowsBetweenParams{
		FromAt: r.f.clock.Now().Add(-24 * time.Hour), ToAt: r.f.clock.Now().Add(time.Hour),
	})
	if err != nil || len(flows) != 2 {
		t.Fatalf("member flows = %+v, %v, want only the two funds", flows, err)
	}
}

func TestFailedTxn_failingATransferTwiceMovesNothingTheSecondTime(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	cashOutFund(t, f, user, cabal, 50_000_000)
	u, c, err := f.cashOut(user, cabal, 20_000_000, 40, domain.TxnPending)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
	for i, want := range []bool{true, false} {
		var moved bool
		err := f.do(func(ctx context.Context, tx db.Tx) error {
			var err error
			moved, err = f.ledger.SetStatus(ctx, tx, u.TransferID, domain.TxnPending, domain.TxnFailed)
			return err
		})
		if err != nil || moved != want {
			t.Fatalf("SetStatus #%d = %v, %v, want %v", i, moved, err, want)
		}
		f.wantView(t, user, cabal, ledgerView{Wallet: "-50000000", Balance: "-50000000", Position: "100/50000000/0"})
	}
}

func TestFailedTxn_failingAnUnpairedTxnTwiceMovesNothingTheSecondTime(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	cashOutFund(t, f, user, cabal, 50_000_000)
	u, _, err := f.cashOut(user, cabal, 20_000_000, 40, domain.TxnPending)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.do(func(ctx context.Context, tx db.Tx) error { return f.ledger.PostUserTxn(ctx, tx, u) }); err != nil {
		t.Fatal(err)
	}
	for i, want := range []bool{true, false} {
		var failed bool
		err := f.do(func(ctx context.Context, tx db.Tx) error {
			var err error
			failed, err = f.ledger.FailUnpaired(ctx, tx, u.TransferID)
			return err
		})
		if err != nil || failed != want {
			t.Fatalf("FailUnpaired #%d = %v, %v, want %v", i, failed, err, want)
		}
		f.wantView(t, user, cabal, ledgerView{Wallet: "-50000000", Balance: "-50000000", Position: "100/50000000/0"})
	}
}

func TestFailedTxn_aTransferWithoutACabalSideStillFailsAndTouchesNoPosition(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user := f.user(t)
	txn, err := domain.NewUserTxn(domain.UserTxnHeader{
		ID: f.ids.NewV7(), UserID: user, Kind: domain.UserDeposit, Status: domain.TxnPending,
		TransferID: f.ids.NewV7(),
	}, []domain.UserEntry{
		{Account: domain.UserExternal, Asset: usdc, Amount: amount(-5)},
		{Account: domain.UserWallet, Asset: usdc, Amount: amount(5)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.do(
		func(ctx context.Context, tx db.Tx) error { return f.ledger.PostUserTxn(ctx, tx, txn) },
	); err != nil {
		t.Fatal(err)
	}
	var failed bool
	err = f.do(func(ctx context.Context, tx db.Tx) error {
		var err error
		failed, err = f.ledger.FailUnpaired(ctx, tx, txn.TransferID)
		return err
	})
	if err != nil || !failed || f.count(t, "user_positions") != 0 {
		t.Fatalf("FailUnpaired = %v, %v with %d positions, want a failed deposit and no position", failed, err,
			f.count(t, "user_positions"))
	}
}

func (f fixture) pendingWallet(t *testing.T, user ids.UserID) int64 {
	t.Helper()
	var n string
	if err := f.pool.QueryRow(t.Context(), `SELECT coalesce(sum(e.amount), 0)::text FROM user_txns t
		JOIN user_txn_entries e ON e.txn_id = t.id
		WHERE t.user_id = $1 AND t.status = 'pending' AND e.account = 'wallet'`, user.UUID()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	v, err := strconv.ParseInt(n, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func (f fixture) positionImpliedWallet(t *testing.T, user ids.UserID) int64 {
	t.Helper()
	var n string
	if err := f.pool.QueryRow(t.Context(), `SELECT coalesce(sum(withdrawn_micros - contributed_micros), 0)::text
		FROM user_positions WHERE user_id = $1`, user.UUID()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	v, err := strconv.ParseInt(n, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func (f fixture) settledWallet(t *testing.T, user ids.UserID) int64 {
	t.Helper()
	row, err := sqlc.New(f.pool).WalletLedgerMicros(t.Context(),
		sqlc.WalletLedgerMicrosParams{UserID: user.UUID(), Asset: usdcMint})
	if err != nil {
		t.Fatal(err)
	}
	v, err := strconv.ParseInt(row.Settled, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestFailedTxn_aMissingPositionRowRefusesTheFailAndRollsBack(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	cashOutFund(t, f, user, cabal, 50_000_000)
	u, c, err := f.cashOut(user, cabal, 20_000_000, 40, domain.TxnPending)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(t.Context(), `DELETE FROM user_positions`); err != nil {
		t.Fatal(err)
	}
	err = f.do(func(ctx context.Context, tx db.Tx) error {
		_, err := f.ledger.SetStatus(ctx, tx, u.TransferID, domain.TxnPending, domain.TxnFailed)
		return err
	})
	wantCode(t, err, errs.CodeInternal)
	var status string
	if err := f.pool.QueryRow(t.Context(), `SELECT status FROM user_txns WHERE kind = 'cash_out'`).
		Scan(&status); err != nil ||
		status != "pending" {
		t.Fatalf("status = %q, %v, want the refused fail rolled back", status, err)
	}
}

func TestFailedTxn_aRefusedStatusWriteFailsTheCall(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	cashOutFund(t, f, user, cabal, 50_000_000)
	u, c, err := f.cashOut(user, cabal, 20_000_000, 40, domain.TxnPending)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(t.Context(), refuse("user_txns", "NEW.status = 'failed'")); err != nil {
		t.Fatal(err)
	}
	err = f.do(func(ctx context.Context, tx db.Tx) error {
		_, err := f.ledger.SetStatus(ctx, tx, u.TransferID, domain.TxnPending, domain.TxnFailed)
		return err
	})
	wantCode(t, err, errs.CodeInternal)
}

func TestFailedTxn_aFailedPayoutAfterAShortSaleUndoesTheSalesReturnToo(t *testing.T) {
	t.Parallel()
	s := newSaleRig(t, 80_000_000)
	r := (&payoutRig{f: s.f, alice: s.alice, bob: s.bob, cabal: s.cabal, job: s.job}).stubs()
	s.deliver(t, s.started)
	s.deliver(t, s.sold(s.f.ids.NewV7(), 400, 15_000_000, 2))
	s.deliver(t, s.unsold(s.f.ids.NewV7(), 200, 2))
	r.seed(t, 1, domain.PayoutBroadcast)
	r.chain.set(seededSig(1), refused())
	r.mustAdvance(t)
	r.wantJob(t, "failed", "payout_failed")
	got := r.f.view(t, r.alice, r.cabal)
	want := ledgerView{Wallet: "-50000000", Balance: "-50000000", Position: "100/65000000/15000000"}
	if got != want {
		t.Fatalf("ledger view = %+v, want %+v", got, want)
	}
	r.noDrift(t)
	r.mustAdvance(t)
	if again := r.f.view(t, r.alice, r.cabal); again != want {
		t.Fatalf("a second run moved the ledger: %+v", again)
	}
}

func TestFailedTxn_aRefusedUndoOfTheSalesReturnMovesNoMoney(t *testing.T) {
	t.Parallel()
	s := newSaleRig(t, 80_000_000)
	r := (&payoutRig{f: s.f, alice: s.alice, bob: s.bob, cabal: s.cabal, job: s.job}).stubs()
	s.deliver(t, s.started)
	s.deliver(t, s.sold(s.f.ids.NewV7(), 400, 15_000_000, 2))
	s.deliver(t, s.unsold(s.f.ids.NewV7(), 200, 2))
	r.seed(t, 1, domain.PayoutBroadcast)
	r.chain.set(seededSig(1), refused())
	before := r.f.view(t, r.alice, r.cabal)
	if _, err := r.f.pool.Exec(t.Context(), refuse("user_txns", "NEW.status = 'settled'")); err != nil {
		t.Fatal(err)
	}
	if err := r.advance(t, 0); err == nil {
		t.Fatal("Advance = nil, want the refused undo to fail the step")
	}
	r.wantJob(t, "paying", "sale_short")
	if after := r.f.view(t, r.alice, r.cabal); after != before {
		t.Fatalf("ledger view = %+v, want %+v untouched", after, before)
	}
}

func TestFailedTxn_theReplayLedgerCheckShowsNoDriftOverAFailedCashOutSwapAndDeposit(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	cashOutFund(t, f, user, cabal, 50_000_000)

	u, c, err := f.cashOut(user, cabal, 20_000_000, 40, domain.TxnPending)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
	if err := f.do(func(ctx context.Context, tx db.Tx) error {
		_, err := f.ledger.SetStatus(ctx, tx, u.TransferID, domain.TxnPending, domain.TxnFailed)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	swap, err := f.swap(cabal, 10_000_000, 5)
	if err != nil {
		t.Fatal(err)
	}
	swap.Status = domain.TxnPending
	if err := f.postCabal(swap); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(t.Context(), `UPDATE cabal_txns SET status = 'failed' WHERE kind = 'swap'`); err != nil {
		t.Fatal(err)
	}

	deposit, err := domain.NewUserTxn(domain.UserTxnHeader{
		ID: f.ids.NewV7(), UserID: user, Kind: domain.UserDeposit, Status: domain.TxnPending,
		TransferID: f.ids.NewV7(),
	}, []domain.UserEntry{
		{Account: domain.UserExternal, Asset: usdc, Amount: amount(-5)},
		{Account: domain.UserWallet, Asset: usdc, Amount: amount(5)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.do(func(ctx context.Context, tx db.Tx) error {
		if err := f.ledger.PostUserTxn(ctx, tx, deposit); err != nil {
			return err
		}
		_, err := f.ledger.FailUnpaired(ctx, tx, deposit.TransferID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	f.wantView(t, user, cabal, beforeCashOut())
}
