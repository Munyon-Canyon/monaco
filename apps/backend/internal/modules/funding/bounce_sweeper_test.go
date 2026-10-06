package funding_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

func (f *bounceFixture) inFlight(t *testing.T) uuid.UUID {
	t.Helper()
	f.chain.state = solana.StateProcessing
	id := f.detected(t, flow08USDCSig)
	if err := f.bouncer.Start(bounceCtx(t), id); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(app.BounceSweepAge + time.Second)
	return id
}

func (f *bounceFixture) sweeper() *app.BounceSweeper {
	return app.NewBounceSweeper(f.bouncer, app.DefaultBounceSweepTiming())
}

func (f *bounceFixture) tick(t *testing.T) poller.Report {
	t.Helper()
	report, err := f.sweeper().Tick(bounceCtx(t))
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func (f *bounceFixture) bounce(t *testing.T, id uuid.UUID) (sig string, attempts int) {
	t.Helper()
	if err := f.pool.QueryRow(t.Context(), `SELECT bounce_signature, bounce_attempts FROM external_deposits
		WHERE id = $1`, id).Scan(&sig, &attempts); err != nil {
		t.Fatal(err)
	}
	return sig, attempts
}

func TestBounceSweeper_IsAThirtySecondPoller(t *testing.T) {
	t.Parallel()
	s := app.NewBounceSweeper(nil, app.DefaultBounceSweepTiming())
	if s.Name() != "funding.bounce-sweeper" || s.Interval() != 30*time.Second {
		t.Fatalf("sweeper = %s@%s, want funding.bounce-sweeper@30s", s.Name(), s.Interval())
	}
}

func TestBounceSweeper_SweepsOnTheTimingItIsGiven(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	f.chain.state = solana.StateProcessing
	id := f.detected(t, flow08USDCSig)
	if err := f.bouncer.Start(bounceCtx(t), id); err != nil {
		t.Fatal(err)
	}
	f.chain.state = solana.StateFinalized
	if report := f.tick(t); report.Scanned != 0 {
		t.Fatalf("default report = %+v, want a bounce younger than 2m skipped", report)
	}
	s := app.NewBounceSweeper(f.bouncer, app.BounceSweepTiming{Interval: time.Second, Age: time.Second})
	report, err := s.Tick(bounceCtx(t))
	if err != nil || report.Scanned != 1 || report.Changed != 1 || s.Interval() != time.Second ||
		f.status(t, id) != "returned" {
		t.Fatalf("report = %+v, %v, interval %s, status %s, want the bounce returned on a 1s age",
			report, err, s.Interval(), f.status(t, id))
	}
}

func TestBounceSweeper_ResolvesAFinalizedBounce(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	id := f.inFlight(t)
	f.chain.state = solana.StateFinalized

	if report := f.tick(t); report.Scanned != 1 || report.Changed != 1 {
		t.Fatalf("report = %+v, want one scanned and changed", report)
	}
	if f.status(t, id) != "returned" || externalPauses(t, f.pool) != 0 ||
		countEvents(t, f.pool, events.TypeCabalExternalDepositBounced) != 1 {
		t.Fatal("want the deposit returned, the pause ended and one bounced event")
	}
}

func TestBounceSweeper_LeavesFreshAndProcessingBouncesAlone(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	f.chain.state = solana.StateProcessing
	id := f.detected(t, flow08USDCSig)
	if err := f.bouncer.Start(bounceCtx(t), id); err != nil {
		t.Fatal(err)
	}
	f.clock.Set(f.clock.Now().Add(-time.Minute))

	if report := f.tick(t); report.Scanned != 0 {
		t.Fatalf("report = %+v, want a fresh bounce skipped", report)
	}
	f.clock.Advance(time.Hour)
	if report := f.tick(t); report.Scanned != 1 || report.Changed != 0 || f.status(t, id) != "bouncing" {
		t.Fatalf("report = %+v, want a processing bounce scanned and left bouncing", report)
	}
}

func TestBounceSweeper_FailsARejectedBounce(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	id := f.inFlight(t)
	f.chain.state, f.chain.failed = solana.StateFinalized, true

	f.tick(t)

	if f.status(t, id) != "bounce_failed" || externalPauses(t, f.pool) != 1 {
		t.Fatal("want bounce_failed with the pause kept")
	}
}

func TestBounceSweeper_RebroadcastsWhileTheBlockhashIsValid(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	id := f.inFlight(t)
	f.chain.state = solana.StateNotFound

	f.tick(t)

	if sig, attempts := f.bounce(t, id); sig != "bounce-1" || attempts != 1 || len(f.transfers.sent) != 2 {
		t.Fatalf("signature %s after %d attempts and %d sends, want the same signature sent again",
			sig, attempts, len(f.transfers.sent))
	}
}

func TestBounceSweeper_ReSignsAnExpiredBounceThenGivesUpAfterThree(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	id := f.inFlight(t)
	f.chain.state, f.chain.expired = solana.StateNotFound, true

	f.tick(t)
	f.tick(t)

	if sig, attempts := f.bounce(t, id); sig != "bounce-3" || attempts != 3 || len(f.transfers.sent) != 3 {
		t.Fatalf("signature %s after %d attempts and %d sends, want bounce-3 on the third attempt",
			sig, attempts, len(f.transfers.sent))
	}
	f.tick(t)
	if f.status(t, id) != "bounce_failed" || len(f.transfers.builds) != 3 {
		t.Fatalf("status = %s after %d builds, want bounce_failed with no fourth", f.status(t, id),
			len(f.transfers.builds))
	}
}

func TestBounceSweeper_ReSignRefusedFailsTheBounce(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	id := f.inFlight(t)
	f.chain.state, f.chain.expired = solana.StateNotFound, true
	f.chain.closed = true

	if report := f.tick(t); report.Changed != 1 || f.status(t, id) != "bounce_failed" {
		t.Fatalf("report = %+v with status %s, want bounce_failed", report, f.status(t, id))
	}
}

func TestBounceSweeper_ReSignRaceStoresNothing(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	id := f.inFlight(t)
	f.chain.state, f.chain.expired = solana.StateNotFound, true
	f.transfers.onBuild = func() { exec(t, f.pool, `UPDATE external_deposits SET bounce_signature = 'other'`) }

	if report := f.tick(t); report.Changed != 0 || len(f.transfers.sent) != 1 {
		t.Fatalf("report = %+v with %d sends, want nothing stored or sent", report, len(f.transfers.sent))
	}
	if sig, _ := f.bounce(t, id); sig != "other" {
		t.Fatalf("signature = %s, want the other writer's", sig)
	}
}

func TestBounceSweeper_ReportsChainAndDecodeErrors(t *testing.T) {
	t.Parallel()
	down := errs.New(errs.CodeRPCUnavailable, "test")
	for name, arrange := range map[string]func(*bounceFixture){
		"statuses":  func(f *bounceFixture) { f.chain.err = down },
		"blockhash": func(f *bounceFixture) { f.chain.hashErr = down },
		"sign":      func(f *bounceFixture) { f.chain.expired, f.transfers.buildErr = true, down },
		"broadcast": func(f *bounceFixture) { f.chain.expired, f.transfers.sendErr = true, down },
		"decode": func(f *bounceFixture) {
			exec(t, f.pool, `UPDATE external_deposits SET bounce_signed_tx = '\x00'`)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newBounceFixture(t)
			f.inFlight(t)
			f.chain.state = solana.StateNotFound
			arrange(f)

			if _, err := f.sweeper().Tick(bounceCtx(t)); err == nil {
				t.Fatal("Tick = nil, want the error")
			}
		})
	}
}

func TestBounceSweeper_ListFailureIsInternal(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	ctx, cancel := context.WithCancel(bounceCtx(t))
	cancel()

	if _, err := f.sweeper().Tick(ctx); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Tick = %v, want internal", err)
	}
}

func TestBounceSweeper_SkipsARowThatStoppedBouncing(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	id := f.inFlight(t)
	exec(t, f.pool, `UPDATE external_deposits SET status = 'held'`)

	if changed, err := f.bouncer.Sweep(bounceCtx(t), id); changed || err != nil {
		t.Fatalf("Sweep = %v, %v, want nothing", changed, err)
	}
}
