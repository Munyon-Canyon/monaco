package ranking_test

import (
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
	"github.com/monaco/monaco/apps/backend/internal/tools/ops/replay"
)

func twoRuns(t *testing.T) (source *pgxpool.Pool, firstRows []string) {
	t.Helper()
	s := flow19Scenario(t)
	flows.F19RunValuationOK(s)
	firstRows = entryRows(t, s.DB())
	s.When(func(s *scenario.Scenario) {
		s.Helper()
		if _, err := s.DB().Exec(s.Context(), `INSERT INTO ranking_triggers (cabal_id, reason, created_at)
			SELECT id, 'replay', now() - interval '2 seconds' FROM cabals`); err != nil {
			s.Fatalf("queue a second valuation: %v", err)
		}
	}, scenario.AwaitTickPastTimeouts("ranking.valuation")).Then(
		scenario.Eventually("two ranking.snapshot_written events", func(s *scenario.Scenario) bool {
			return len(snapshotEvents(t, s.DB())) == 2
		}),
	)
	return s.DB(), firstRows
}

func entryRows(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, _ := pool.Query(t.Context(), `SELECT to_jsonb(t)::text FROM leaderboard_entries t ORDER BY 1`)
	out, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func snapshotEvents(t *testing.T, pool *pgxpool.Pool) []uuid.UUID {
	t.Helper()
	rows, _ := pool.Query(t.Context(), `SELECT id FROM events WHERE type = 'ranking.snapshot_written' ORDER BY id`)
	out, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func replayBoards(t *testing.T, source *pgxpool.Pool, o replay.Options) (replay.Report, *pgxpool.Pool) {
	t.Helper()
	target, clk := testkit.DB(t), &replay.Clock{}
	uow := db.New(target, ids.Real{}, clk)
	targetSet := module.Set{rankingOver(module.Deps{Pool: target, UoW: uow, Clock: clk, IDs: ids.Real{}})}
	sourceSet := module.Set{rankingOver(module.Deps{Pool: source, Clock: clk, IDs: ids.Real{}})}
	o.Source, o.Target, o.UoW, o.Clock = source, target, uow, clk
	o.Handlers = replay.Projections(targetSet, sourceSet)
	o.Tables = []string{"leaderboard_entries", "leaderboard_runs", "cabal_value_snapshots"}
	rep, err := replay.Run(t.Context(), o)
	if err != nil {
		t.Fatal(err)
	}
	return rep, target
}

func TestReplay_RebuildsLeaderboards(t *testing.T) {
	t.Parallel()
	source, _ := twoRuns(t)
	t.Run("target", func(t *testing.T) {
		t.Parallel()
		rep, target := replayBoards(t, source, replay.Options{Verify: true})
		if rep.Applied != 2 || len(rep.Diffs) != 0 {
			t.Fatalf("report = %+v, want 2 applied runs and no diffs", rep)
		}
		if got, want := entryRows(t, target), entryRows(t, source); len(want) == 0 || !slices.Equal(got, want) {
			t.Fatalf("replayed board = %q, want %q", got, want)
		}
		var sourceEvents, targetEvents int
		for pool, n := range map[*pgxpool.Pool]*int{source: &sourceEvents, target: &targetEvents} {
			if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM events`).Scan(n); err != nil {
				t.Fatal(err)
			}
		}
		if sourceEvents != targetEvents {
			t.Fatalf("target holds %d events, source %d: replay appended events", targetEvents, sourceEvents)
		}
	})
}

func TestReplay_ToEarlierEvent(t *testing.T) {
	t.Parallel()
	source, firstRows := twoRuns(t)
	t.Run("target", func(t *testing.T) {
		t.Parallel()
		_, target := replayBoards(t, source, replay.Options{To: snapshotEvents(t, source)[0]})
		got := entryRows(t, target)
		if len(firstRows) == 0 || !slices.Equal(got, firstRows) {
			t.Fatalf("board at the first run = %q, want the rows that run wrote %q", got, firstRows)
		}
		if slices.Equal(got, entryRows(t, source)) {
			t.Fatal("the first run's rows equal the last run's: the test cannot tell the two runs apart")
		}
	})
}
