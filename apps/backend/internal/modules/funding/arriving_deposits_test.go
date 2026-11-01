package funding_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func seedOnrampSession(
	t *testing.T, f *forwardFixture, status string, completedAt *time.Time,
) {
	t.Helper()
	id := ids.Real{}.NewV7()
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO onramp_sessions
		(id, user_id, token_hash, status, created_at, expires_at, completed_at)
		VALUES ($1, $2, $3, $4, $5, $5, $6)`,
		id, f.user.ID.UUID(), id[:], status, f.clock.Now(), completedAt); err != nil {
		t.Fatal(err)
	}
}

func TestDepositArriving_IsTrueForASubmittedOrConfirmedSessionInsideTheWindowOnly(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		status    string
		completed time.Duration
		want      bool
	}{
		{"submitted just now", "submitted", 0, true},
		{"confirmed just now", "confirmed", 0, true},
		{"confirmed at the edge of the window", "confirmed", app.OnrampArrivalWindow, true},
		{"confirmed past the window", "confirmed", app.OnrampArrivalWindow + time.Second, false},
		{"cancelled just now", "cancelled", 0, false},
		{"failed just now", "failed", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newForwardFixture(t)
			completed := f.clock.Now().Add(-tc.completed)
			seedOnrampSession(t, f, tc.status, &completed)
			got, err := adapters.ArrivingDeposits{}.DepositArriving(t.Context(), f.pool, f.user.ID, f.clock.Now())
			if err != nil || got != tc.want {
				t.Fatalf("DepositArriving = %t, %v, want %t", got, err, tc.want)
			}
		})
	}
}

func TestDepositArriving_IgnoresSessionsThatAreStillCreatedOrOpenedAndOtherUsers(t *testing.T) {
	t.Parallel()
	f := newForwardFixture(t)
	seedOnrampSession(t, f, "created", nil)
	seedOnrampSession(t, f, "opened", nil)
	other := testkit.SeedUser(t, f.pool, testkit.UserOpts{})
	id := ids.Real{}.NewV7()
	now := f.clock.Now()
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO onramp_sessions
		(id, user_id, token_hash, status, created_at, expires_at, completed_at)
		VALUES ($1, $2, $3, 'submitted', $4, $4, $4)`, id, other.ID.UUID(), id[:], now); err != nil {
		t.Fatal(err)
	}
	got, err := adapters.ArrivingDeposits{}.DepositArriving(t.Context(), f.pool, f.user.ID, now)
	if err != nil || got {
		t.Fatalf("DepositArriving = %t, %v, want false", got, err)
	}
}

func TestDepositArriving_IsTrueForAPendingEVMForwardUntilItArrivesOrFails(t *testing.T) {
	t.Parallel()
	f := newForwardFixture(t)
	f.tick(t, execution("fwd", "pending"))
	check := func(want bool) {
		t.Helper()
		got, err := adapters.ArrivingDeposits{}.DepositArriving(t.Context(), f.pool, f.user.ID, f.clock.Now())
		if err != nil || got != want {
			t.Fatalf("DepositArriving = %t, %v, want %t", got, err, want)
		}
	}
	check(true)
	f.exec(t, `UPDATE evm_deposit_forwards SET status = 'arrived', solana_signature = 'sig', arrived_at = now()`)
	check(false)
	f.exec(t, `UPDATE evm_deposit_forwards SET status = 'failed', solana_signature = NULL, arrived_at = NULL`)
	check(false)
}

func TestDepositArriving_SurfacesAQueryFailure(t *testing.T) {
	t.Parallel()
	f := newForwardFixture(t)
	f.exec(t, `ALTER TABLE onramp_sessions RENAME TO onramp_sessions_gone`)
	_, err := adapters.ArrivingDeposits{}.DepositArriving(t.Context(), f.pool, f.user.ID, clock.Real{}.Now())
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("DepositArriving = %v, want internal", err)
	}
}

func (f *forwardFixture) exec(t *testing.T, sql string) {
	t.Helper()
	if _, err := f.pool.Exec(t.Context(), sql); err != nil {
		t.Fatal(err)
	}
}
