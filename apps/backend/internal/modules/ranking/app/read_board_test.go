package app_test

import (
	"context"
	"slices"
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

func (f fakeBoards) Subjects(
	_ context.Context, _ string, _ domain.Range, subjects []uuid.UUID,
) ([]domain.Entry, error) {
	if f.failing == "subjects" {
		return nil, errs.New(errs.CodeInternal, "test")
	}
	var out []domain.Entry
	for _, row := range f.rows {
		if slices.Contains(subjects, row.Subject.ID) {
			out = append(out, row)
		}
	}
	return out, nil
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

type recordingLoader struct{ keys []app.PageKey }

func (r *recordingLoader) Load(
	ctx context.Context, key app.PageKey, load func(context.Context) (domain.BoardPage, error),
) (domain.BoardPage, error) {
	r.keys = append(r.keys, key)
	return load(ctx)
}

func TestReadBoard_loadsThePageThroughTheCacheKeyedByRunAndRev(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	run := domain.Run{ID: ids.Real{}.NewV7(), Rev: 4}
	loader := &recordingLoader{}
	in := app.ReadBoard{Board: "cabals", Range: domain.Range1W, Cursor: 20, Limit: 2, Pages: loader}
	got, err := in.Run(t.Context(), fakeBoards{run: run, hasRun: true, rows: entries(30, at)})
	want := app.PageKey{RunID: run.ID, Rev: 4, Board: "cabals", Range: domain.Range1W, Cursor: 20, Limit: 2}
	if err != nil || len(got.Rows) != 2 || len(loader.keys) != 1 || loader.keys[0] != want {
		t.Fatalf("page = %+v, %v, keys = %+v, want one load keyed %+v", got, err, loader.keys, want)
	}
}
