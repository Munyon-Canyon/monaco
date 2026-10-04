//go:build faultpoints

package ranking_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestSnapshotWriter_crashBeforeCommitConvergesToOneRun(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	writer := newWriter(pool, now)
	valuation := app.Valuation{AsOf: now, PricesAsOf: now, Entries: []app.Entry{entry(now, 1, 7)}}
	testkit.CrashAt(t, faultpoint.BeforeCommit, func(ctx context.Context) error {
		_, err := writer.Write(observability.WithActor(ctx, "system:ranking.valuation"), valuation, now, now)
		return err
	})
	for query, want := range map[string]int{
		`SELECT count(*) FROM leaderboard_runs`:                               1,
		`SELECT count(*) FROM leaderboard_entries WHERE range = 'ALL'`:        1,
		`SELECT count(*) FROM events WHERE type = 'ranking.snapshot_written'`: 1,
	} {
		if n := count(t, pool, query); n != want {
			t.Fatalf("%s = %d, want %d", query, n, want)
		}
	}
}
