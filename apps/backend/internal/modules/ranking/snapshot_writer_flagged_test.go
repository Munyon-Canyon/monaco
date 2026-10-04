package ranking_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	flaggedRowsKept  = `SELECT count(*) FROM leaderboard_entries WHERE range = 'ALL' AND value_micros IN (5, 3)`
	peopleRowKept    = `SELECT count(*) FROM leaderboard_entries WHERE range = 'ALL' AND value_micros = 9`
	flaggedRunsCount = `SELECT count(*) FROM leaderboard_runs`
)

type flaggedRun struct {
	pool       *pgxpool.Pool
	writer     app.SnapshotWriter
	cabal      ids.CabalID
	now, later time.Time
}

func seedFlaggedRun(t *testing.T) flaggedRun {
	t.Helper()
	pool := testkit.DB(t)
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	run := flaggedRun{
		pool: pool, writer: newWriter(pool, now), cabal: ids.CabalIDFrom(ids.Real{}.NewV7()),
		now: now, later: now.Add(time.Minute),
	}
	handle, picture, bps := "h", "p", int64(250)
	cabalRow := entry(now, 1, 5)
	cabalRow.Board, cabalRow.SubjectID, cabalRow.ReturnBps = "cabals", run.cabal.UUID(), &bps
	cabalRow.SubjectHandle, cabalRow.SubjectPictureURL = &handle, &picture
	memberRow := entry(now, 1, 3)
	memberRow.Board = app.MembersBoard(run.cabal.UUID())
	first := app.Valuation{AsOf: now, PricesAsOf: now, Entries: []app.Entry{cabalRow, memberRow, entry(now, 1, 9)}}
	if _, err := run.writer.Write(run.ctx(t), first, now, now); err != nil {
		t.Fatal(err)
	}
	return run
}

func (r flaggedRun) ctx(t *testing.T) context.Context {
	t.Helper()
	return observability.WithActor(t.Context(), "system:ranking.valuation")
}

func (r flaggedRun) second(entries []app.Entry) app.Valuation {
	return app.Valuation{
		AsOf: r.later, PricesAsOf: r.later, Flagged: []app.CabalValue{{CabalID: r.cabal}}, Entries: entries,
	}
}

func TestSnapshotWriterRefusesToDropAFlaggedCabalsPreviousRows(t *testing.T) {
	t.Parallel()
	run := seedFlaggedRun(t)
	valuation := run.second([]app.Entry{entry(run.later, 1, 7)})
	if _, err := run.writer.Write(run.ctx(t), valuation, run.later, run.later); err == nil {
		t.Fatal("Write() dropping a flagged cabal's rows = nil, want a refusal")
	}
	for query, want := range map[string]int{flaggedRowsKept: 2, peopleRowKept: 1, flaggedRunsCount: 1} {
		if n := count(t, run.pool, query); n != want {
			t.Fatalf("%s = %d, want %d", query, n, want)
		}
	}
}

func TestSnapshotWriterKeepsAFlaggedCabalsRowsWhenTheRunCarriesThem(t *testing.T) {
	t.Parallel()
	run := seedFlaggedRun(t)
	carried, err := app.PreviousEntries(run.ctx(t), sqlc.New(run.pool), []ids.CabalID{run.cabal})
	if err != nil || len(carried) != 2 || carried[1].SubjectHandle == nil || carried[1].ReturnBps == nil ||
		carried[1].SubjectPictureURL == nil {
		t.Fatalf("PreviousEntries() = %+v, %v, want the cabal row and the member row", carried, err)
	}
	valuation := run.second(append([]app.Entry{entry(run.later, 1, 7)}, carried...))
	if _, err := run.writer.Write(run.ctx(t), valuation, run.later, run.later); err != nil {
		t.Fatal(err)
	}
	for query, want := range map[string]int{flaggedRowsKept: 2, peopleRowKept: 0, flaggedRunsCount: 2} {
		if n := count(t, run.pool, query); n != want {
			t.Fatalf("%s = %d, want %d", query, n, want)
		}
	}
}
