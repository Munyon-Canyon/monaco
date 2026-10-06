//go:build faultpoints

package social_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func crashAfterCommits(ctx context.Context, t *testing.T, commits int, run func(ctx context.Context)) {
	t.Helper()
	crashed := func() (p any) {
		defer func() { p = recover() }()
		run(faultpoint.ArmedAfter(ctx, faultpoint.BeforeCommit, commits))
		return nil
	}()
	if !faultpoint.IsCrash(crashed) {
		t.Fatalf("the run never crashed at before-commit, recovered %v", crashed)
	}
}

func (r renamer) delivered(t *testing.T, id uuid.UUID) int {
	t.Helper()
	var n int
	if err := r.pool.QueryRow(t.Context(), `SELECT count(*) FROM event_deliveries WHERE event_id = $1`, id).
		Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (r renamer) wantFirstBatchOnly(t *testing.T, column, renamed, stale string) {
	t.Helper()
	if got := r.count(t, column+" = '"+renamed+"'"); got != 500 {
		t.Fatalf("rows renamed before the crash = %d, want the first batch of 500", got)
	}
	if got := r.count(t, column+" = '"+stale+"'"); got != renameRows-500 {
		t.Fatalf("rows still holding %q = %d, want %d", stale, got, renameRows-500)
	}
	var lowestFirst bool
	err := r.pool.QueryRow(t.Context(), `SELECT
		(SELECT id FROM feed_objects WHERE `+column+` = $1 ORDER BY id DESC LIMIT 1)
		< (SELECT id FROM feed_objects WHERE `+column+` = $2 ORDER BY id LIMIT 1)`, renamed, stale).Scan(&lowestFirst)
	if err != nil || !lowestFirst {
		t.Fatalf("the batch that committed was not the lowest ids: %t, %v", lowestFirst, err)
	}
}

func TestFeedRename_ProfileConvergesAfterACrashBetweenBatches(t *testing.T) {
	t.Parallel()
	r := newRenamer(t)
	r = r.knowing(identity.UserCard{ID: r.alice, Handle: "alice", DisplayName: "Quillen"})
	r.seed(t, renameRows, joined(r.alice, ids.CabalIDFrom(r.gen.NewV7()), "alice"))
	ev := profileUpdated(r.alice.UUID(), "Quillen", "display_name")
	id := r.event(t, ev)

	crashAfterCommits(t.Context(), t, 1, func(ctx context.Context) {
		_, _ = r.deliverAs(ctx, t, id, profileHandler, ev)
	})

	r.wantFirstBatchOnly(t, "payload->>'actor_name'", "Quillen", "alice")
	if n := r.delivered(t, id); n != 0 {
		t.Fatalf("the crashed delivery left %d delivery rows", n)
	}
	if duplicate, err := r.deliverAs(t.Context(), t, id, profileHandler, ev); err != nil || duplicate {
		t.Fatalf("redelivery = duplicate %t, %v; want it to finish the rename", duplicate, err)
	}
	if got := r.count(t, `payload->>'actor_name' = 'Quillen' AND title = 'Quillen joined Alpha'`); got != renameRows {
		t.Fatalf("rows renamed after the redelivery = %d, want %d", got, renameRows)
	}
	if duplicate, err := r.deliverAs(t.Context(), t, id, profileHandler, ev); err != nil || !duplicate {
		t.Fatalf("second redelivery = duplicate %t, %v; want a duplicate", duplicate, err)
	}
}

func TestFeedRename_CabalConvergesAfterACrashBetweenBatches(t *testing.T) {
	t.Parallel()
	r := newRenamer(t)
	cabal := r.cabal(t, "Alpha")
	r.seed(t, renameRows, joined(r.bob, cabal, "bob"))
	ev := r.renamed(cabal, r.alice, "Quorum")
	id := r.event(t, ev)

	crashAfterCommits(t.Context(), t, 2, func(ctx context.Context) {
		_, _ = r.deliverAs(ctx, t, id, cabalHandler, ev)
	})

	r.wantFirstBatchOnly(t, "cabal_name", "Quorum", "Alpha")
	var name string
	if err := r.pool.QueryRow(t.Context(), `SELECT name FROM feed_cabals WHERE cabal_id = $1`, cabal.UUID()).
		Scan(&name); err != nil ||
		name != "Quorum" {
		t.Fatalf("feed_cabals name = %q, %v; the cabal's own rename should have committed first", name, err)
	}
	if n := r.delivered(t, id); n != 0 {
		t.Fatalf("the crashed delivery left %d delivery rows", n)
	}
	if duplicate, err := r.deliverAs(t.Context(), t, id, cabalHandler, ev); err != nil || duplicate {
		t.Fatalf("redelivery = duplicate %t, %v; want it to finish the rename", duplicate, err)
	}
	if got := r.count(t, `cabal_name = 'Quorum' AND title = 'bob joined Quorum'`); got != renameRows {
		t.Fatalf("rows renamed after the redelivery = %d, want %d", got, renameRows)
	}
}
