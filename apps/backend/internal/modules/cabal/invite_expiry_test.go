package cabal_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/metric/noop"
	"golang.org/x/sync/errgroup"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (f accessFixture) expiryTick(ctx context.Context) (poller.Report, error) {
	return app.NewInviteExpiryPoller(f.uow, f.pool, f.clock).Tick(
		observability.WithActor(ctx, "system:poller.cabal.invite_expiry"))
}

func (f accessFixture) mustExpire(t *testing.T, scanned, changed int) {
	t.Helper()
	got, err := f.expiryTick(t.Context())
	if err != nil || got.Scanned != scanned || got.Changed != changed {
		t.Fatalf("Tick = %+v, %v, want scanned %d changed %d", got, err, scanned, changed)
	}
}

func (f accessFixture) expirations(t *testing.T) []events.CabalAccessDecided {
	t.Helper()
	var out []events.CabalAccessDecided
	for _, e := range decoded[events.CabalAccessDecided](t, f, events.TypeCabalAccessDecided) {
		if e.Decision == "expired" {
			out = append(out, e)
		}
	}
	return out
}

func (f accessFixture) refuseWrites(t *testing.T, table, column string, id uuid.UUID) {
	t.Helper()
	f.exec(t, `CREATE FUNCTION refuse_`+table+`() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
		IF NEW.`+column+` = '`+id.String()+`' THEN RAISE EXCEPTION 'refused'; END IF;
		RETURN NEW; END $$`)
	f.exec(t, `CREATE TRIGGER refuse BEFORE INSERT OR UPDATE ON `+table+
		` FOR EACH ROW EXECUTE FUNCTION refuse_`+table+`()`)
}

func TestInviteExpiryPoller_ExpiresOnlyPastDue(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"))
	invitee := f.user(t)
	id := f.invite(t, c.ID, invitee, c.Creator.ID)
	accepted := f.invite(t, c.ID, f.user(t), c.Creator.ID)
	f.exec(t, `UPDATE cabal_access_requests SET status = 'approved' WHERE id = $1`, accepted)
	f.clock.Advance(6*24*time.Hour + 23*time.Hour)
	f.mustExpire(t, 0, 0)
	if got := f.accessRow(t, id).status; got != "pending" {
		t.Fatalf("status at 6 d 23 h = %s, want pending", got)
	}
	f.clock.Advance(time.Hour + time.Minute)
	f.mustExpire(t, 1, 1)
	if got := f.accessRow(t, id); got.status != "expired" || got.decidedBy != nil {
		t.Fatalf("row at 7 d 1 m = %+v, want expired with no decider", got)
	}
	want := events.CabalAccessDecided{
		V: 1, RequestID: id, CabalID: c.ID.UUID(), UserID: invitee.UUID(), Direction: "invite", Decision: "expired",
	}
	if got := f.expirations(t); len(got) != 1 || got[0] != want {
		t.Fatalf("expirations = %+v, want %+v", got, want)
	}
	f.mustExpire(t, 0, 0)
	if got := f.accessRow(t, accepted).status; got != "approved" {
		t.Fatalf("accepted invite status = %s, want it untouched", got)
	}
}

func TestInviteExpiryPoller_takesAtMostOneBatchPerTick(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool)
	f.exec(t, `INSERT INTO users (id, privy_user_id, login_provider, auth_state, auth_state_changed_at,
		account_status, created_at, updated_at)
		SELECT gen_random_uuid(), 'did:privy:due-' || g, 'sms', 'CREATED', now(), 'active', now(), now()
		FROM generate_series(1, $1::int) g`, app.InviteExpiryBatch+1)
	f.exec(t, `INSERT INTO cabal_access_requests (id, cabal_id, user_id, direction, invited_by, expires_at, created_at)
		SELECT gen_random_uuid(), $1, u.id, 'invite', $2, $3, $3
		FROM users u WHERE u.privy_user_id LIKE 'did:privy:due-%'`,
		c.ID.UUID(), c.Creator.ID.UUID(), f.clock.Now())
	f.clock.Advance(time.Minute)
	f.mustExpire(t, app.InviteExpiryBatch, app.InviteExpiryBatch)
	f.mustExpire(t, 1, 1)
	f.mustExpire(t, 0, 0)
}

func TestInviteExpiryPoller_aRefusedInviteDoesNotHoldBackTheRest(t *testing.T) {
	t.Parallel()
	for _, table := range []struct{ name, column string }{{"cabal_access_requests", "id"}, {"events", "aggregate_id"}} {
		t.Run(table.name, func(t *testing.T) {
			t.Parallel()
			f := newAccess(t)
			stuckCabal, fineCabal := testkit.NewCabal(t, f.pool), testkit.NewCabal(t, f.pool)
			stuck := f.invite(t, stuckCabal.ID, f.user(t), stuckCabal.Creator.ID)
			fine := f.invite(t, fineCabal.ID, f.user(t), fineCabal.Creator.ID)
			refused := stuck
			if table.name == "events" {
				refused = stuckCabal.ID.UUID()
			}
			f.refuseWrites(t, table.name, table.column, refused)
			f.clock.Advance(8 * 24 * time.Hour)
			got, err := f.expiryTick(t.Context())
			if err == nil || got.Scanned != 2 || got.Changed != 1 {
				t.Fatalf("Tick = %+v, %v, want an error after scanning 2 and expiring 1", got, err)
			}
			if f.accessRow(t, stuck).status != "pending" || f.accessRow(t, fine).status != "expired" ||
				len(f.expirations(t)) != 1 {
				t.Fatal("want the refused invite still pending and the other expired with one event")
			}
		})
	}
}

func TestInviteExpiryPoller_aTickThatCannotReadFails(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := f.expiryTick(ctx); err == nil {
		t.Fatal("Tick on a cancelled context succeeded")
	}
}

func TestInviteExpiryPoller_anInviteDecidedWhileTheTickWaitsIsLeftAlone(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool)
	id := f.invite(t, c.ID, f.user(t), c.Creator.ID)
	f.clock.Advance(8 * 24 * time.Hour)
	commit := f.holdWrite(t, `UPDATE cabal_access_requests SET status = 'revoked' WHERE id = $1`, id)
	type result struct {
		report poller.Report
		err    error
	}
	done := make(chan result, 1)
	var ticking sync.WaitGroup
	t.Cleanup(ticking.Wait)
	ticking.Go(func() {
		report, err := f.expiryTick(t.Context())
		done <- result{report, err}
	})
	f.waitForALockWaiter(t)
	commit()
	if got := <-done; got.err != nil || got.report.Scanned != 1 || got.report.Changed != 0 {
		t.Fatalf("Tick = %+v, want one scanned and none expired", got)
	}
	if got := f.accessRow(t, id).status; got != "revoked" || len(f.expirations(t)) != 0 {
		t.Fatalf("status = %s, want revoked with no expiry event", got)
	}
}

func TestDecideAccess_anInvitePastItsExpiryIsExpiredAndRefused(t *testing.T) {
	t.Parallel()
	for _, decision := range []app.Decision{app.Approve, app.Deny} {
		t.Run(string(decision), func(t *testing.T) {
			t.Parallel()
			f := newAccess(t)
			c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"))
			invitee := f.user(t)
			id := f.invite(t, c.ID, invitee, c.Creator.ID)
			f.clock.Advance(7*24*time.Hour + time.Second)
			_, err := f.decide(t.Context(), invitee, c.ID, id, decision)
			wantErr(t, err, errs.CodeInviteExpired)
			if got := f.accessRow(t, id); got.status != "expired" || got.decidedBy != nil {
				t.Fatalf("row = %+v, want expired in the refused decision's own unit of work", got)
			}
			if _, ok := f.membership(t, c.ID, invitee); ok || len(f.expirations(t)) != 1 {
				t.Fatal("want no member and exactly one expiry event")
			}
			_, err = f.decide(t.Context(), invitee, c.ID, id, decision)
			wantErr(t, err, errs.CodeInviteExpired)
			f.mustExpire(t, 0, 0)
		})
	}
}

func TestDecideAccess_anInviteAtItsExpiryInstantCanStillBeAccepted(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"))
	invitee := f.user(t)
	id := f.invite(t, c.ID, invitee, c.Creator.ID)
	f.clock.Advance(7 * 24 * time.Hour)
	if got, err := f.decide(t.Context(), invitee, c.ID, id, app.Approve); err != nil || got.Status != "approved" {
		t.Fatalf("accept at the expiry instant = %+v, %v; want approved", got, err)
	}
}

func TestDecideAccess_aFailedExpiryRollsBack(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool)
	invitee := f.user(t)
	id := f.invite(t, c.ID, invitee, c.Creator.ID)
	f.refuseWrites(t, "cabal_access_requests", "id", id)
	f.clock.Advance(8 * 24 * time.Hour)
	_, err := f.decide(t.Context(), invitee, c.ID, id, app.Approve)
	wantErr(t, err, errs.CodeInternal)
	if got := f.accessRow(t, id).status; got != "pending" {
		t.Fatalf("status = %s, want pending after the refused expiry", got)
	}
}

func TestInviteExpiryPoller_twoWorkersOnOneDatabaseExpireEachInviteOnce(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool)
	id := f.invite(t, c.ID, f.user(t), c.Creator.ID)
	f.clock.Advance(7*24*time.Hour - app.InviteExpiryInterval + time.Second)
	a := startInviteExpiryWorker(t, f)
	if got := waitForExpiryLines(t, a, 1); got[0] != "poller.tick" {
		t.Fatalf("first worker logged %v, want a tick", got)
	}
	b := startInviteExpiryWorker(t, f)
	if got := waitForExpiryLines(t, b, 1); got[0] != "poller.tick.skipped_locked" {
		t.Fatalf("second worker logged %v, want a skip while the first holds the lock", got)
	}
	f.clock.Advance(app.InviteExpiryInterval)
	if got := waitForExpiryLines(t, a, 2); got[1] != "poller.tick" {
		t.Fatalf("first worker logged %v, want a second tick", got)
	}
	if got := waitForExpiryLines(t, b, 2); got[1] != "poller.tick.skipped_locked" {
		t.Fatalf("second worker logged %v, want a second skip", got)
	}
	if got := f.accessRow(t, id).status; got != "expired" || len(f.expirations(t)) != 1 {
		t.Fatalf("status %s, want expired with exactly one expiry event", got)
	}
}

type expiryLog struct {
	mu    sync.Mutex
	lines []string
}

func (w *expiryLog) Write(p []byte) (int, error) {
	var line struct {
		Msg    string `json:"msg"`
		Poller string `json:"poller"`
	}
	if err := json.Unmarshal(p, &line); err != nil {
		return 0, err
	}
	if line.Poller == "cabal.invite_expiry" && strings.HasPrefix(line.Msg, "poller.") {
		w.mu.Lock()
		w.lines = append(w.lines, line.Msg)
		w.mu.Unlock()
	}
	return len(p), nil
}

func (w *expiryLog) seen() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.lines...)
}

func startInviteExpiryWorker(t *testing.T, f accessFixture) *expiryLog {
	t.Helper()
	pollers := cabal.New(module.Deps{Clock: f.clock, IDs: f.ids, Pool: f.pool, UoW: f.uow}).Pollers()
	if len(pollers) != 1 || pollers[0].Name() != "cabal.invite_expiry" || pollers[0].Interval() != 5*time.Minute {
		t.Fatalf("pollers = %v, want cabal.invite_expiry every 5 minutes", pollers)
	}
	runner, err := poller.NewRunner(f.pool, f.clock, noop.NewMeterProvider().Meter("t"))
	if err != nil {
		t.Fatal(err)
	}
	w := &expiryLog{}
	ctx, cancel := context.WithCancel(context.WithoutCancel(t.Context()))
	ctx = observability.WithLogger(ctx, slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug})))
	var run errgroup.Group
	run.Go(func() error { return runner.Run(ctx, pollers...) })
	t.Cleanup(func() {
		cancel()
		if err := run.Wait(); err != nil {
			t.Errorf("Run returned %v at cleanup", err)
		}
	})
	return w
}

func waitForExpiryLines(t *testing.T, w *expiryLog, n int) []string {
	t.Helper()
	testkit.Eventually(t, func() bool { return len(w.seen()) >= n }, 30*time.Second)
	return w.seen()
}
