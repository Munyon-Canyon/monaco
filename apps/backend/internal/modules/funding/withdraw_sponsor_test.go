package funding_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
)

const (
	sponsorFull = relayer.SponsorCostLamports
	sponsorFee  = relayer.NetworkFeeLamports
)

func userBudget(lamports uint64) func(*app.WithdrawDeps) {
	return func(d *app.WithdrawDeps) { d.Sponsor = relayer.SponsorBudgets{UserDaily: lamports} }
}

func (f *withdrawFixture) skipped(t *testing.T) []bool {
	t.Helper()
	out := make([]bool, 0, len(f.transfers.builds))
	for _, b := range f.transfers.builds {
		out = append(out, b.SkipCreate)
	}
	return out
}

func (f *withdrawFixture) reservations(t *testing.T) (live, released int) {
	t.Helper()
	err := f.pool.QueryRow(t.Context(), `SELECT count(*) FILTER (WHERE released_at IS NULL),
		count(*) FILTER (WHERE released_at IS NOT NULL) FROM relayer_sponsor_reservations`).Scan(&live, &released)
	if err != nil {
		t.Fatal(err)
	}
	return live, released
}

func (f *withdrawFixture) wantSkipped(t *testing.T, want ...bool) {
	t.Helper()
	if got := f.skipped(t); !slices.Equal(got, want) {
		t.Fatalf("builds skip create = %v, want %v", got, want)
	}
}

func TestWithdraw_AFreshDestinationCreatesTheAccountAndAConfirmedOneDoesNot(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 100_000_000)
	to := freshDestination(0)
	for range 2 {
		if _, err := f.handle(f.ctx(t.Context()), to); err != nil {
			t.Fatal(err)
		}
	}
	f.wantSkipped(t, false, false)
	if _, err := f.pool.Exec(t.Context(),
		`UPDATE withdrawals SET status = 'confirmed', completed_at = now() WHERE to_address = $1`, to); err != nil {
		t.Fatal(err)
	}
	if _, err := f.handle(f.ctx(t.Context()), to); err != nil {
		t.Fatal(err)
	}
	f.wantSkipped(t, false, false, true)
}

func TestWithdraw_PastTheUserBudgetTheTransferSkipsTheAccountThenIsRefused(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 100_000_000, userBudget(sponsorFull+sponsorFee))
	for n := range 2 {
		if _, err := f.handle(f.ctx(t.Context()), freshDestination(n)); err != nil {
			t.Fatalf("withdraw %d: %v", n, err)
		}
	}
	f.wantSkipped(t, false, true)
	f.assertRefusedBeforeSigning(t, freshDestination(2))
	if got := len(f.rows(t)); got != 2 {
		t.Fatalf("withdrawal rows = %d, want the refused one rolled back", got)
	}
	if live, _ := f.reservations(t); live != 6 {
		t.Fatalf("live reservations = %d, want 3 per accepted withdrawal", live)
	}
}

func TestWithdraw_AFailedBuildGivesTheReservationBack(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 100_000_000, userBudget(sponsorFull))
	f.transfers.buildErr = errs.New(errs.CodePrivyUnavailable, "test")
	for n := range 3 {
		if _, err := f.handle(f.ctx(t.Context()), freshDestination(n)); errs.CodeOf(err) != errs.CodePrivyUnavailable {
			t.Fatalf("Handle err = %v, want privy_unavailable", err)
		}
	}
	if live, released := f.reservations(t); live != 0 || released != 9 {
		t.Fatalf("reservations live %d released %d, want 0 and 9", live, released)
	}
	f.transfers.buildErr = nil
	if _, err := f.handle(f.ctx(t.Context()), freshDestination(3)); err != nil {
		t.Fatalf("withdraw after the failures: %v", err)
	}
	f.wantSkipped(t, false, false, false, false)
}

func TestWithdraw_FailsClosedWhenBudgetStorageIsDown(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 100_000_000)
	if _, err := f.pool.Exec(t.Context(),
		`ALTER TABLE relayer_sponsor_reservations RENAME TO relayer_sponsor_reservations_down`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.handle(f.ctx(t.Context()), freshDestination(0)); errs.CodeOf(err) != errs.CodeDBUnavailable ||
		len(f.transfers.builds) != 0 || len(f.rows(t)) != 0 {
		t.Fatalf("Handle err = %v builds = %d rows = %d, want db_unavailable, no signature and no row",
			err, len(f.transfers.builds), len(f.rows(t)))
	}
}

func TestWithdraw_ABuildFailureThatCannotReleaseTheReservationIsReported(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 100_000_000)
	f.transfers.buildErr = errs.New(errs.CodePrivyUnavailable, "test")
	f.transfers.beforeSign = func() {
		if _, err := f.pool.Exec(t.Context(),
			`ALTER TABLE relayer_sponsor_reservations RENAME TO relayer_sponsor_reservations_down`); err != nil {
			t.Error(err)
		}
	}
	_, err := f.handle(f.ctx(t.Context()), freshDestination(0))
	if err == nil || !strings.Contains(err.Error(), string(errs.CodeDBUnavailable)) ||
		!strings.Contains(err.Error(), string(errs.CodePrivyUnavailable)) {
		t.Fatalf("Handle err = %v, want both the build failure and the release failure", err)
	}
	if rows := f.rows(t); len(rows) != 1 || rows[0].status != "created" {
		t.Fatalf("rows = %+v, want the withdrawal left created for the stale sweep", rows)
	}
}

func TestWithdraw_AResumedWithdrawalKeepsItsReservation(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 9_000_000, userBudget(sponsorFull))
	f.transfers.beforeSign = func() { panic("killed while signing") }
	func() {
		defer func() { _ = recover() }()
		_, _ = f.withdrawKeyed(t, "h1")
	}()
	f.transfers.beforeSign = nil
	if _, err := f.withdrawKeyed(t, "h1"); err != nil {
		t.Fatal(err)
	}
	if live, _ := f.reservations(t); live != 3 {
		t.Fatalf("live reservations = %d, want the one reservation of the resumed withdrawal", live)
	}
	f.wantSkipped(t, false, false)
}

func TestWithdraw_AResumedWithdrawalWithoutAReservationReservesAgain(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 9_000_000)
	f.transfers.beforeSign = func() { panic("killed while signing") }
	func() {
		defer func() { _ = recover() }()
		_, _ = f.withdrawKeyed(t, "h1")
	}()
	f.transfers.beforeSign = nil
	if _, err := f.pool.Exec(t.Context(), `DELETE FROM relayer_sponsor_reservations`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.withdrawKeyed(t, "h1"); err != nil {
		t.Fatal(err)
	}
	if live, _ := f.reservations(t); live != 3 {
		t.Fatalf("live reservations = %d, want the resumed withdrawal to reserve again", live)
	}
}

func TestWithdraw_AnEVMWithdrawalReservesNoSolanaSponsorship(t *testing.T) {
	t.Parallel()
	f := newEVMFixture(t, &privyActions{})
	if _, err := f.withdrawEVM(f.ctx(t.Context())); err != nil {
		t.Fatal(err)
	}
	if live, released := f.reservations(t); live != 0 || released != 0 {
		t.Fatalf("reservations live %d released %d, want none for an EVM destination", live, released)
	}
}
