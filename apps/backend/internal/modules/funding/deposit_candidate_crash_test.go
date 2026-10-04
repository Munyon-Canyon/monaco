//go:build faultpoints

package funding_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestDepositCandidateResolver_crashBeforeCommitCreditsOnceAfterRedelivery(t *testing.T) {
	t.Parallel()
	f := newCandidateFixture(t)
	f.record(t, f.candidate())
	e := f.event()
	resolver := candidateResolver(f, candidateRPC{})
	testkit.CrashAt(t, faultpoint.BeforeCommit, func(ctx context.Context) error {
		return f.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
			return resolver.Apply(ctx, tx, e, app.DepositCandidateResolution{Amount: money.MicrosFromUint64(1)}, f.now)
		})
	})
	var deposits int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM deposits`).Scan(&deposits); err != nil {
		t.Fatal(err)
	}
	if deposits != 1 {
		t.Fatalf("deposits after redelivery = %d, want 1", deposits)
	}
}
