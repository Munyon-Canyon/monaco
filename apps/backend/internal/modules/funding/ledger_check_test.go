package funding_test

import (
	"context"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestLedgerCheck_namesAPauseLeftOpenOnASettledDeposit(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	id := f.detected(t, flow08USDCSig)
	check := funding.LedgerCheck(testkit.Config())
	if diffs, err := check.Check(t.Context(), f.pool); err != nil || len(diffs) != 0 || check.Name != "funding" {
		t.Fatalf("%s on a bouncing deposit = %q, %v, want no diffs", check.Name, diffs, err)
	}
	if err := f.bouncer.Start(bounceCtx(t), id); err != nil || f.status(t, id) != "returned" {
		t.Fatalf("Start = %v, status %s, want returned", err, f.status(t, id))
	}
	if diffs, err := check.Check(t.Context(), f.pool); err != nil || len(diffs) != 0 {
		t.Fatalf("returned deposit = %q, %v, want its pause resolved", diffs, err)
	}
	if _, err := f.pool.Exec(t.Context(), `UPDATE cabal_pauses SET resolved_at = NULL WHERE external_deposit_id = $1`,
		id); err != nil {
		t.Fatal(err)
	}
	diffs, err := check.Check(t.Context(), f.pool)
	if err != nil || len(diffs) != 1 || !strings.Contains(diffs[0], "returned external deposit "+id.String()) {
		t.Fatalf("reopened pause = %q, %v, want it named on the returned deposit", diffs, err)
	}
}

func TestLedgerCheck_aFailedReadIsInternal(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := funding.LedgerCheck(testkit.Config()).Check(ctx, f.pool); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("canceled check = %v, want internal", err)
	}
}
