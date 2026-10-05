package treasury_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type cashOutPauses struct {
	paused bool
	err    error
	before func()
}

func (p cashOutPauses) IsPaused(context.Context, ids.CabalID) (app.CashOutPause, error) {
	if p.before != nil {
		p.before()
	}
	return app.CashOutPause{Paused: p.paused}, p.err
}

func cashOutHandler(f fixture, paused bool) *app.CashOutHandler {
	return cashOutHandlerWith(f, cashOutPauses{paused: paused}, newQueries(f))
}

func cashOutHandlerWith(f fixture, pauses cashOutPauses, values port.PositionsReader) *app.CashOutHandler {
	return app.NewCashOutHandler(f.uow, f.ledger, values, pauses, f.clock, f.ids, f.pool)
}

type cashOutValues struct {
	pot    money.Micros
	err    error
	before func()
}

func (v cashOutValues) Positions(context.Context, ids.CabalID) ([]port.Position, error) {
	return nil, nil
}

func (v cashOutValues) TotalShares(context.Context, ids.CabalID) (money.SharesUnits, error) {
	return money.SharesUnits{}, nil
}

func (v cashOutValues) PotValue(context.Context, ids.CabalID) (money.Micros, error) {
	if v.before != nil {
		v.before()
	}
	return v.pot, v.err
}

func cashOutFund(t *testing.T, f fixture, user ids.UserID, cabal ids.CabalID, micros int64) {
	t.Helper()
	u, c, err := f.fund(user, cabal, micros, 100, domain.TxnSettled)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
}

func TestCashOut_debitsSharesAndReservesPayout(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	cashOutFund(t, f, user, cabal, 100_000_000)
	ctx := observability.WithActor(f.ctx(), "user:"+user.String())
	result, err := cashOutHandler(f, false).Handle(ctx, app.CashOut{
		IdempotencyKey: "cash-out-1", CabalID: cabal, UserID: user, PayoutMicros: money.MicrosFromUint64(10_000_000),
	})
	if err != nil || result.ShareUnits.Uint64() != 10 || result.PayoutMicros != money.MicrosFromUint64(10_000_000) {
		t.Fatalf("CashOut = (%#v, %v)", result, err)
	}
	if shares, err := newQueries(f).ShareUnits(t.Context(), cabal, user); err != nil || shares.Uint64() != 90 {
		t.Fatalf("ShareUnits = (%v, %v)", shares, err)
	}
	if pot, err := newQueries(f).PotValue(t.Context(), cabal); err != nil || pot != money.MicrosFromUint64(90_000_000) {
		t.Fatalf("PotValue = (%v, %v)", pot, err)
	}
	if f.count(t, "cash_out_jobs") != 1 {
		t.Fatal("cash out job was not inserted")
	}
	if drift := f.drift(t); len(drift) != 0 {
		t.Fatalf("ledger drift = %#v", drift)
	}
}

func TestCashOut_refusesPausedCabal(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	_, err := cashOutHandler(f, true).Handle(f.ctx(), app.CashOut{CabalID: f.cabal(t), UserID: f.user(t), All: true})
	wantCode(t, err, errs.CodeCabalPaused)
}

func TestCashOut_previewsAndReadsAReservedJob(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	cashOutFund(t, f, user, cabal, 1_000_000)
	h := cashOutHandler(f, false)
	preview, err := h.Preview(f.ctx(), cabal, user)
	if err != nil || preview.ShareUnits.Uint64() != 100 || preview.SliceMicros != money.MicrosFromUint64(1_000_000) {
		t.Fatalf("Preview = (%#v, %v)", preview, err)
	}
	result, err := h.Handle(
		observability.WithActor(f.ctx(), "user:"+user.String()),
		app.CashOut{CabalID: cabal, UserID: user, All: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(
		t.Context(), "UPDATE cash_out_jobs SET result_code = $1 WHERE id = $2", "test", result.ID,
	); err != nil {
		t.Fatal(err)
	}
	job, err := h.Job(f.ctx(), cabal, user, result.ID)
	if err != nil || job.ID != result.ID || job.Status != domain.CashOutStarted ||
		job.SellUSDCMicros != (money.Micros{}) || job.ResultCode == nil {
		t.Fatalf("Job = (%#v, %v)", job, err)
	}
}

func TestCashOut_jobIsNotFoundForUnknownOrAnotherUsersID(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, other, cabal := f.user(t), f.user(t), f.cabal(t)
	cashOutFund(t, f, user, cabal, 1_000_000)
	h := cashOutHandler(f, false)
	result, err := h.Handle(
		observability.WithActor(f.ctx(), "user:"+user.String()),
		app.CashOut{CabalID: cabal, UserID: user, All: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.Job(f.ctx(), cabal, user, uuid.Nil)
	wantCode(t, err, errs.CodeNotFound)
	_, err = h.Job(f.ctx(), cabal, other, result.ID)
	wantCode(t, err, errs.CodeNotFound)
}

func TestCashOut_rejectsAMemberWithoutShares(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	_, err := cashOutHandler(f, false).Handle(f.ctx(), app.CashOut{CabalID: cabal, UserID: user, All: true})
	wantCode(t, err, errs.CodeInsufficientShares)
}

func TestCashOut_shortOfUSDCSellsTheShortfallAndReservesOnlyCashOnHand(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	alice, bob, cabal := f.user(t), f.user(t), f.cabal(t)
	cashOutFund(t, f, alice, cabal, 1_000_000)
	cashOutFund(t, f, bob, cabal, 1_000_000)
	h := cashOutHandlerWith(f, cashOutPauses{}, cashOutValues{pot: money.MicrosFromUint64(6_000_000)})

	ctx := observability.WithActor(f.ctx(), "user:"+alice.String())
	first, err := h.Handle(ctx, app.CashOut{CabalID: cabal, UserID: alice, All: true})
	if err != nil || first.PayoutMicros != money.MicrosFromUint64(3_000_000) {
		t.Fatalf("first CashOut = (%#v, %v), want a 3 USDC slice", first, err)
	}
	second, err := h.Handle(ctx, app.CashOut{CabalID: cabal, UserID: bob, All: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		job  uuid.UUID
		user ids.UserID
		sell string
	}{{first.ID, alice, "1000000"}, {second.ID, bob, "6000000"}} {
		job, err := h.Job(f.ctx(), cabal, c.user, c.job)
		if err != nil || job.SellUSDCMicros.String() != c.sell || job.Status != domain.CashOutStarted {
			t.Fatalf("Job = (%#v, %v), want started selling %s", job, err, c.sell)
		}
		var sell string
		if err := f.pool.QueryRow(t.Context(), `SELECT payload->>'sell_usdc_micros' FROM events
			WHERE type = 'cashout.started' AND aggregate_id = $1`, c.job).Scan(&sell); err != nil || sell != c.sell {
			t.Fatalf("cashout.started sell_usdc_micros = (%q, %v), want %s", sell, err, c.sell)
		}
	}
}

func TestCashOut_refusesAnotherLiveJob(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	cashOutFund(t, f, user, cabal, 1_000_000)
	h := cashOutHandler(f, false)
	ctx := observability.WithActor(f.ctx(), "user:"+user.String())
	if _, err := h.Handle(ctx, app.CashOut{
		CabalID: cabal, UserID: user, PayoutMicros: money.MicrosFromUint64(100_000),
	}); err != nil {
		t.Fatal(err)
	}
	_, err := h.Handle(ctx, app.CashOut{CabalID: cabal, UserID: user, All: true})
	wantCode(t, err, errs.CodeCashOutInProgress)
}

func TestCashOut_returnsStoreFailures(t *testing.T) {
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	cashOutFund(t, f, user, cabal, 1_000_000)
	h := cashOutHandlerWith(f, cashOutPauses{before: func() {
		if _, err := f.pool.Exec(t.Context(), "ALTER TABLE user_positions RENAME TO missing_positions"); err != nil {
			t.Fatal(err)
		}
	}}, cashOutValues{pot: money.MicrosFromUint64(1_000_000)})
	if _, err := h.Handle(f.ctx(), app.CashOut{CabalID: cabal, UserID: user, All: true}); err == nil {
		t.Fatal("CashOut error = nil")
	}
	t.Parallel()
}

func TestCashOut_returnsPauseAndValueFailures(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	cashOutFund(t, f, user, cabal, 1_000_000)
	for _, h := range []*app.CashOutHandler{
		cashOutHandlerWith(f, cashOutPauses{err: errs.New(errs.CodeInternal, "test")}, newQueries(f)),
		cashOutHandlerWith(f, cashOutPauses{}, cashOutValues{err: errs.New(errs.CodeInternal, "test")}),
	} {
		if _, err := h.Handle(f.ctx(), app.CashOut{CabalID: cabal, UserID: user, All: true}); err == nil {
			t.Fatal("CashOut error = nil")
		}
	}
}
