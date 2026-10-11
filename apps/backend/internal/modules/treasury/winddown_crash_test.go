//go:build faultpoints

package treasury_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestWindDown_CrashBeforeTheStartCommitsPaysEachMemberOnce(t *testing.T) {
	t.Parallel()
	r := newWindDownRig(t, thirds()...)
	testkit.CrashAt(t, faultpoint.BeforeCommit, func(ctx context.Context) error {
		return r.f.uow.Do(observability.WithActor(ctx, "system:consumer.treasury.winddown"), r.startIn)
	})
	if r.jobs(t) != 3 || r.held(t) != 0 || r.scalar(t, `SELECT count(*) FROM cabal_winddowns`) != 1 {
		t.Fatalf("after the restart: %d jobs, %d shares held, want 3 jobs and none held", r.jobs(t), r.held(t))
	}
	if drift := r.f.drift(t); len(drift) != 0 {
		t.Fatalf("ledger drift = %v", drift)
	}
}

func TestWindDown_CrashBeforeTheCompletionCommitsAppendsWoundDownOnce(t *testing.T) {
	t.Parallel()
	r := newWindDownRig(t, thirds()...)
	if err := r.start(); err != nil {
		t.Fatal(err)
	}
	r.finish(t)
	testkit.CrashAt(t, faultpoint.BeforeCommit, func(ctx context.Context) error {
		_, err := r.poller.Tick(observability.WithActor(ctx, "system:poller.treasury.winddown"))
		return err
	})
	if got := r.scalar(t, `SELECT count(*) FROM events WHERE type = 'cabal.wound_down'`); got != 1 {
		t.Fatalf("cabal.wound_down events = %d, want 1", got)
	}
}
