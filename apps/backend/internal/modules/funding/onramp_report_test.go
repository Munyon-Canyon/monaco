package funding_test

import (
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (f onrampFixture) opened(t *testing.T, suggested *money.Micros) uuid.UUID {
	t.Helper()
	created, token := f.start(t, suggested)
	if _, err := f.exchange.Handle(t.Context(), token); err != nil {
		t.Fatal(err)
	}
	return created.ID
}

func (f onrampFixture) reportAs(
	t *testing.T, user ids.UserID, id uuid.UUID, to domain.OnrampStatus,
) (app.OnrampSession, error) {
	t.Helper()
	ctx := observability.WithActor(t.Context(), "user:"+user.String())
	return f.report.Handle(ctx, app.ReportOnrampStatus{SessionID: id, UserID: user, To: to, Provider: "moonpay"})
}

func TestReportOnrampStatus_movesAnOpenedSessionAndHintsTheOwner(t *testing.T) {
	t.Parallel()
	f := newOnrampFixture(t)
	suggested := money.MicrosFromUint64(25_000_000)
	id := f.opened(t, &suggested)
	f.clock.Advance(3 * time.Minute)
	got, err := f.reportAs(t, f.user.ID, id, domain.OnrampConfirmed)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != id || got.Status != domain.OnrampConfirmed || *got.SuggestedAmount != suggested ||
		!got.CompletedAt.Equal(f.clock.Now()) {
		t.Fatalf("report = %+v, want confirmed now", got)
	}
	f.assertConfirmedRow(t)
	evs := onrampEvents(t, f.pool)
	last := evs[len(evs)-1]
	if len(evs) != 3 || *last.From != "opened" || last.To != "confirmed" || *last.Provider != "moonpay" {
		t.Fatalf("events = %+v, want created, opened, then confirmed by moonpay", evs)
	}
	if want := "user." + f.user.ID.String() + ".onramp_changed"; !slices.Equal(f.hints.keys, []string{want}) {
		t.Fatalf("hints = %v, want [%s]", f.hints.keys, want)
	}
}

func (f onrampFixture) assertConfirmedRow(t *testing.T) {
	t.Helper()
	var status, provider string
	var completed time.Time
	if err := f.pool.QueryRow(t.Context(), `SELECT status, provider, completed_at FROM onramp_sessions`).
		Scan(&status, &provider, &completed); err != nil {
		t.Fatal(err)
	}
	if status != "confirmed" || provider != "moonpay" || !completed.Equal(f.clock.Now()) {
		t.Fatalf("row = %s %s %s, want confirmed by moonpay now", status, provider, completed)
	}
}

func TestReportOnrampStatus_refusesWithoutMovingTheSession(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		run  func(t *testing.T, f onrampFixture, id uuid.UUID) error
		want errs.Code
	}{
		{"another user", func(t *testing.T, f onrampFixture, id uuid.UUID) error {
			t.Helper()
			other := testkit.SeedUser(t, f.pool, testkit.UserOpts{})
			_, err := f.reportAs(t, other.ID, id, domain.OnrampConfirmed)
			return err
		}, errs.CodeForbidden},
		{"an unknown session", func(t *testing.T, f onrampFixture, _ uuid.UUID) error {
			t.Helper()
			_, err := f.reportAs(t, f.user.ID, f.ids.NewV7(), domain.OnrampConfirmed)
			return err
		}, errs.CodeNotFound},
		{"confirmed then cancelled", func(t *testing.T, f onrampFixture, id uuid.UUID) error {
			t.Helper()
			if _, err := f.reportAs(t, f.user.ID, id, domain.OnrampConfirmed); err != nil {
				t.Fatal(err)
			}
			_, err := f.reportAs(t, f.user.ID, id, domain.OnrampCancelled)
			return err
		}, errs.CodeOnrampInvalidTransition},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newOnrampFixture(t)
			id := f.opened(t, nil)
			before := len(onrampEvents(t, f.pool))
			if err := tt.run(t, f, id); errs.CodeOf(err) != tt.want {
				t.Fatalf("report = %v, want %s", err, tt.want)
			}
			if got := len(onrampEvents(t, f.pool)) - before; got > 1 {
				t.Fatalf("%d events appended, want at most the one accepted move", got)
			}
		})
	}
}

func TestReportOnrampStatus_refusesASessionThatWasNeverOpened(t *testing.T) {
	t.Parallel()
	f := newOnrampFixture(t)
	created, _ := f.start(t, nil)
	if _, err := f.reportAs(t, f.user.ID, created.ID, domain.OnrampConfirmed); errs.CodeOf(err) !=
		errs.CodeOnrampInvalidTransition {
		t.Fatalf("report = %v, want onramp_invalid_transition", err)
	}
	assertOnlyCreated(t, f.pool)
}

func TestReportOnrampStatus_rollsBackWhenAStepFails(t *testing.T) {
	t.Parallel()
	for _, setup := range []string{
		`UPDATE onramp_sessions SET suggested_amount_micros = 99999999999999999999`,
		`ALTER TABLE events RENAME TO events_gone`,
		`ALTER TABLE onramp_sessions RENAME TO onramp_sessions_gone`,
	} {
		t.Run(setup, func(t *testing.T) {
			t.Parallel()
			f := newOnrampFixture(t)
			id := f.opened(t, nil)
			f.exec(t, setup)
			if _, err := f.reportAs(t, f.user.ID, id, domain.OnrampConfirmed); errs.CodeOf(err) != errs.CodeInternal {
				t.Fatalf("report = %v, want internal", err)
			}
			if len(f.hints.keys) != 0 {
				t.Fatalf("hints = %v, want none after a rollback", f.hints.keys)
			}
		})
	}
}

func TestGetOnrampSession_readsTheOwnersSessionOnly(t *testing.T) {
	t.Parallel()
	f := newOnrampFixture(t)
	suggested := money.MicrosFromUint64(5_000_000)
	id := f.opened(t, &suggested)
	got, err := app.GetOnrampSession(t.Context(), f.pool, id, f.user.ID)
	if err != nil || got.Status != domain.OnrampOpened || *got.SuggestedAmount != suggested ||
		got.CompletedAt != nil || !got.CreatedAt.Equal(f.clock.Now()) {
		t.Fatalf("get = %+v, %v, want the open session", got, err)
	}
	ctx := observability.WithActor(t.Context(), "user:"+f.user.ID.String())
	cmd := app.ReportOnrampStatus{SessionID: id, UserID: f.user.ID, To: domain.OnrampSubmitted}
	if _, err := f.report.Handle(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	if evs := onrampEvents(t, f.pool); evs[len(evs)-1].Provider != nil {
		t.Fatalf("submitted event provider = %v, want null when the page names none", *evs[len(evs)-1].Provider)
	}
	got, err = app.GetOnrampSession(t.Context(), f.pool, id, f.user.ID)
	if err != nil || got.Status != domain.OnrampSubmitted || got.CompletedAt == nil {
		t.Fatalf("get = %+v, %v, want submitted with a completion time", got, err)
	}
	other := testkit.SeedUser(t, f.pool, testkit.UserOpts{})
	if _, err := app.GetOnrampSession(t.Context(), f.pool, id, other.ID); errs.CodeOf(err) != errs.CodeNotFound {
		t.Fatalf("get by another user = %v, want not_found", err)
	}
}

func TestGetOnrampSession_failsOnABrokenRow(t *testing.T) {
	t.Parallel()
	for _, setup := range []string{
		`UPDATE onramp_sessions SET suggested_amount_micros = 99999999999999999999`,
		`ALTER TABLE onramp_sessions RENAME TO onramp_sessions_gone`,
	} {
		t.Run(setup, func(t *testing.T) {
			t.Parallel()
			f := newOnrampFixture(t)
			created, _ := f.start(t, nil)
			f.exec(t, setup)
			if _, err := app.GetOnrampSession(t.Context(), f.pool, created.ID, f.user.ID); errs.CodeOf(err) !=
				errs.CodeInternal {
				t.Fatalf("get = %v, want internal", err)
			}
		})
	}
}
