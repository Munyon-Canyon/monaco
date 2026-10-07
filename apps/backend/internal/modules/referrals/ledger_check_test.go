package referrals_test

import (
	"context"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestLedgerCheck_namesWhatBreaksAReferralsInvariant(t *testing.T) {
	t.Parallel()
	f := newQualifyFixture(t, 91)
	check := referrals.LedgerCheck(testkit.Config())
	want := func(parts ...string) {
		t.Helper()
		diffs, err := check.Check(t.Context(), f.pool)
		if err != nil || len(diffs) != len(parts) {
			t.Fatalf("%s = %q, %v, want %d diffs", check.Name, diffs, err, len(parts))
		}
		for i, part := range parts {
			if !strings.Contains(diffs[i], part) {
				t.Fatalf("diff %d = %q, want %q", i, diffs[i], part)
			}
		}
	}
	want()
	id, _, referee := f.seedReferral(t)
	f.mustApply(t, verifiedUsers(referee, true), f.funded(referee, 10_000_000))
	f.wantState(t, referee, "qualified", true, 1)
	want()
	if _, err := f.pool.Exec(t.Context(), `UPDATE referrals SET qualified_at = NULL WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	want("qualified referral " + id.String() + " has no qualified_at")
	if _, err := f.pool.Exec(t.Context(), `DELETE FROM events WHERE type = 'referral.qualified'`); err != nil {
		t.Fatal(err)
	}
	want("has no qualified_at", "has 0 referral.qualified events, want 1")
	if _, err := f.pool.Exec(t.Context(), `UPDATE referrals SET qualified_at = now() WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	want("has 0 referral.qualified events, want 1")
	if _, err := f.pool.Exec(t.Context(), `ALTER TABLE referral_clicks ADD COLUMN referrer uuid`); err != nil {
		t.Fatal(err)
	}
	want("has 0 referral.qualified events, want 1", "referral_clicks has columns clicks, code, day, referrer")
}

func TestLedgerCheck_namesAnExtraRefereeRow(t *testing.T) {
	t.Parallel()
	f := newQualifyFixture(t, 92)
	_, _, referee := f.seedReferral(t)
	if _, err := f.pool.Exec(t.Context(),
		`ALTER TABLE referrals DROP CONSTRAINT referrals_referee_id_key`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(t.Context(),
		`INSERT INTO referrals (id, referrer_id, referee_id, code, code_kind, source, status, created_at)
		SELECT $1, referrer_id, referee_id, code, code_kind, source, status, created_at FROM referrals`,
		f.ids.NewV7()); err != nil {
		t.Fatal(err)
	}
	diffs, err := referrals.LedgerCheck(testkit.Config()).Check(t.Context(), f.pool)
	if err != nil || len(diffs) != 1 || !strings.Contains(diffs[0], "referee "+referee.String()+" has more than one") {
		t.Fatalf("two rows for one referee = %q, %v, want it named", diffs, err)
	}
}

func TestLedgerCheck_aFailedReadIsInternal(t *testing.T) {
	t.Parallel()
	f := newQualifyFixture(t, 93)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := referrals.LedgerCheck(testkit.Config()).Check(ctx, f.pool); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("canceled check = %v, want internal", err)
	}
}
