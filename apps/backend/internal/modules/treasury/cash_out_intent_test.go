package treasury_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

func cashOutIntent(user ids.UserID, hash string) db.IntentKey {
	return db.IntentKey{
		ActorKey: "user:" + user.String(), Key: "k1", Operation: app.CashOutOperation, RequestHash: []byte(hash),
	}
}

func TestCashOut_aReplayWithTheSameKeyReturnsTheFirstJob(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	cashOutFund(t, f, user, cabal, 100_000_000)
	ctx := observability.WithActor(f.ctx(), "user:"+user.String())
	h := cashOutHandler(f, false)
	cmd := app.CashOut{
		Intent: cashOutIntent(user, "h1"), CabalID: cabal, UserID: user,
		PayoutMicros: money.MicrosFromUint64(10_000_000),
	}
	first, err := h.Handle(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	again, err := h.Handle(ctx, cmd)
	if err != nil || again.ID != first.ID || again.ShareUnits != first.ShareUnits ||
		again.PayoutMicros != first.PayoutMicros || !again.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("replay = (%+v, %v), want the first job %+v", again, err, first)
	}
	if f.count(t, "cash_out_jobs") != 1 || f.positionUnits(t, cabal, user) != 90 {
		t.Fatalf("jobs = %d shares = %d, want 1 job and only 10 shares burned",
			f.count(t, "cash_out_jobs"), f.positionUnits(t, cabal, user))
	}
}

func TestCashOut_aReplayAfterTheJobFinishedPartiallyReturnsTheFirstJobNotANewOne(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	cashOutFund(t, f, user, cabal, 100_000_000)
	ctx := observability.WithActor(f.ctx(), "user:"+user.String())
	h := cashOutHandler(f, false)
	cmd := app.CashOut{
		Intent: cashOutIntent(user, "h1"), CabalID: cabal, UserID: user,
		PayoutMicros: money.MicrosFromUint64(10_000_000),
	}
	first, err := h.Handle(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(t.Context(),
		`UPDATE cash_out_jobs SET status = 'partial', result_code = 'partial' WHERE id = $1`, first.ID); err != nil {
		t.Fatal(err)
	}
	again, err := h.Handle(ctx, cmd)
	if err != nil || again.ID != first.ID {
		t.Fatalf("replay after partial = (%+v, %v), want the first job %v", again, err, first.ID)
	}
	if f.count(t, "cash_out_jobs") != 1 || f.positionUnits(t, cabal, user) != 90 {
		t.Fatalf("jobs = %d shares = %d, want 1 job and 90 shares left",
			f.count(t, "cash_out_jobs"), f.positionUnits(t, cabal, user))
	}
}

func TestCashOut_aReplayIgnoresAPauseThatStartedAfterTheFirstJob(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	cashOutFund(t, f, user, cabal, 100_000_000)
	ctx := observability.WithActor(f.ctx(), "user:"+user.String())
	cmd := app.CashOut{
		Intent: cashOutIntent(user, "h1"), CabalID: cabal, UserID: user,
		PayoutMicros: money.MicrosFromUint64(10_000_000),
	}
	first, err := cashOutHandler(f, false).Handle(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	again, err := cashOutHandler(f, true).Handle(ctx, cmd)
	if err != nil || again.ID != first.ID {
		t.Fatalf("replay while paused = (%+v, %v), want the first job %v", again, err, first.ID)
	}
}

func TestCashOut_theSameKeyWithAnotherRequestIsAMismatch(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	cashOutFund(t, f, user, cabal, 100_000_000)
	ctx := observability.WithActor(f.ctx(), "user:"+user.String())
	h := cashOutHandler(f, false)
	cmd := app.CashOut{
		Intent: cashOutIntent(user, "h1"), CabalID: cabal, UserID: user,
		PayoutMicros: money.MicrosFromUint64(10_000_000),
	}
	if _, err := h.Handle(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	cmd.Intent = cashOutIntent(user, "h2")
	_, err := h.Handle(ctx, cmd)
	wantCode(t, err, errs.CodeIdempotencyMismatch)
	if f.count(t, "cash_out_jobs") != 1 {
		t.Fatalf("jobs = %d, want 1", f.count(t, "cash_out_jobs"))
	}
}
