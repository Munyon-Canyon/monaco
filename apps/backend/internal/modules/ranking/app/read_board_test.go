package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type fakeBoards struct {
	run     domain.Run
	hasRun  bool
	rows    []domain.Entry
	me      *domain.Entry
	failing string
}

func (f fakeBoards) LatestRun(context.Context) (domain.Run, bool, error) {
	if f.failing == "run" {
		return domain.Run{}, false, errs.New(errs.CodeInternal, "test")
	}
	return f.run, f.hasRun, nil
}

func (f fakeBoards) Page(_ context.Context, _ string, _ domain.Range, after, limit int) ([]domain.Entry, error) {
	if f.failing == "page" {
		return nil, errs.New(errs.CodeInternal, "test")
	}
	rest := f.rows[min(after, len(f.rows)):]
	return rest[:min(limit, len(rest))], nil
}

func (f fakeBoards) Row(context.Context, string, domain.Range, uuid.UUID) (domain.Entry, bool, error) {
	if f.failing == "row" {
		return domain.Entry{}, false, errs.New(errs.CodeInternal, "test")
	}
	if f.me == nil {
		return domain.Entry{}, false, nil
	}
	return *f.me, true, nil
}

func entries(n int, at time.Time) []domain.Entry {
	rows := make([]domain.Entry, 0, n)
	for i := 1; i <= n; i++ {
		rows = append(rows, domain.Entry{Rank: i, ComputedAt: at, PricesAsOf: at})
	}
	return rows
}

func TestReadBoard_pagesAndFallsBackToRunTimes(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	run := domain.Run{ID: ids.Real{}.NewV7(), FinishedAt: at.Add(time.Second), PricesAsOf: at.Add(-time.Minute)}
	viewer := ids.Real{}.NewV7()
	me := domain.Entry{Rank: 2}
	boards := fakeBoards{run: run, hasRun: true, rows: entries(3, at), me: &me}
	got, err := app.ReadBoard{Limit: 2, Viewer: &viewer}.Run(t.Context(), boards)
	if err != nil || len(got.Rows) != 2 || got.NextCursor == nil || got.Me == nil || got.Me.Rank != 2 ||
		!got.ComputedAt.Equal(at) {
		t.Fatalf("page = %+v, %v", got, err)
	}
	boards.rows = nil
	got, err = app.ReadBoard{Limit: 2}.Run(t.Context(), boards)
	if err != nil || len(got.Rows) != 0 || got.NextCursor != nil || !got.ComputedAt.Equal(run.FinishedAt) ||
		!got.PricesAsOf.Equal(run.PricesAsOf) {
		t.Fatalf("empty page = %+v, %v", got, err)
	}
}

func TestReadBoard_stopsOnStoreErrors(t *testing.T) {
	t.Parallel()
	viewer := ids.Real{}.NewV7()
	for _, failing := range []string{"run", "page", "row"} {
		boards := fakeBoards{hasRun: true, failing: failing}
		in := app.ReadBoard{Limit: 2, Viewer: &viewer}
		if _, err := in.Run(t.Context(), boards); errs.CodeOf(err) != errs.CodeInternal {
			t.Errorf("%s: err = %v, want internal", failing, err)
		}
	}
}
