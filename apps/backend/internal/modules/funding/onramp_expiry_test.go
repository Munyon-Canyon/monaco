package funding_test

import (
	"maps"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

func (f onrampFixture) tick(t *testing.T) (poller.Report, error) {
	t.Helper()
	return f.expiry.Tick(observability.WithActor(t.Context(), "system:poller."+f.expiry.Name()))
}

func (f onrampFixture) statusOf(t *testing.T, id uuid.UUID) string {
	t.Helper()
	var status string
	if err := f.pool.QueryRow(t.Context(), `SELECT status FROM onramp_sessions WHERE id = $1`, id).
		Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func TestOnrampExpiryPoller_expiresLapsedLinksAndStaleOpenSessionsOnce(t *testing.T) {
	t.Parallel()
	f := newOnrampFixture(t)
	lapsed, _ := f.start(t, nil)
	staleOpen := f.opened(t, nil)
	f.clock.Advance(app.OnrampOpenedTTL - time.Minute)
	freshOpen := f.opened(t, nil)
	fresh, _ := f.start(t, nil)
	f.clock.Advance(time.Minute)
	report, err := f.tick(t)
	if err != nil || report.Scanned != 2 || report.Changed != 2 {
		t.Fatalf("tick = %+v, %v, want two expired", report, err)
	}
	for id, want := range map[uuid.UUID]string{
		lapsed.ID: "expired", staleOpen: "expired", freshOpen: "opened", fresh.ID: "created",
	} {
		if got := f.statusOf(t, id); got != want {
			t.Errorf("session %s = %s, want %s", id, got, want)
		}
	}
	if got, want := expiredFrom(t, f), map[uuid.UUID]string{lapsed.ID: "created", staleOpen: "opened"}; !maps.Equal(
		got, want) {
		t.Fatalf("expired events = %v, want %v", got, want)
	}
	if report, err := f.tick(t); err != nil || report.Changed != 0 {
		t.Fatalf("second tick = %+v, %v, want nothing to expire", report, err)
	}
}

func TestOnrampExpiryPoller_runsEveryMinuteUnderItsName(t *testing.T) {
	t.Parallel()
	p := app.NewOnrampExpiryPoller(nil, nil)
	if p.Name() != "funding.onramp-expiry" || p.Interval() != time.Minute {
		t.Fatalf("poller = %s every %s", p.Name(), p.Interval())
	}
}

func TestOnrampExpiryPoller_rollsBackTheBatchWhenAStepFails(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		setup string
		ctx   bool
	}{
		{"an amount past uint64", `UPDATE onramp_sessions SET suggested_amount_micros = 99999999999999999999`, true},
		{"the session table is gone", `ALTER TABLE onramp_sessions RENAME TO onramp_sessions_gone`, true},
		{"no poller actor", ``, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newOnrampFixture(t)
			created, _ := f.start(t, nil)
			f.clock.Advance(11 * time.Minute)
			if tt.setup != "" {
				f.exec(t, tt.setup)
			}
			var err error
			if tt.ctx {
				_, err = f.tick(t)
			} else {
				_, err = f.expiry.Tick(t.Context())
			}
			if errs.CodeOf(err) != errs.CodeInternal {
				t.Fatalf("tick = %v, want internal", err)
			}
			if tt.name != "the session table is gone" && f.statusOf(t, created.ID) != "created" {
				t.Fatal("the session moved although the tick failed")
			}
		})
	}
}

func expiredFrom(t *testing.T, f onrampFixture) map[uuid.UUID]string {
	t.Helper()
	out := map[uuid.UUID]string{}
	for _, e := range onrampEvents(t, f.pool) {
		if e.To == "expired" {
			out[e.SessionID] = *e.From
		}
	}
	return out
}
