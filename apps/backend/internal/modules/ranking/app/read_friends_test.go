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

type fakeFollows struct {
	following []ids.UserID
	err       error
	viewers   []ids.UserID
}

func (f *fakeFollows) FollowingIDs(_ context.Context, viewer ids.UserID) ([]ids.UserID, error) {
	f.viewers = append(f.viewers, viewer)
	return f.following, f.err
}

func people(n int, at time.Time) []domain.Entry {
	rows := entries(n, at)
	for i := range rows {
		rows[i].Subject.ID = ids.Real{}.NewV7()
	}
	return rows
}

func userOf(row domain.Entry) ids.UserID { return ids.UserIDFrom(row.Subject.ID) }

type friendsFixture struct {
	rows   []domain.Entry
	run    domain.Run
	boards fakeBoards
	viewer uuid.UUID
	reads  *fakeFollows
}

func newFriendsFixture() friendsFixture {
	at := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	rows := people(6, at)
	run := domain.Run{ID: ids.Real{}.NewV7(), FinishedAt: at, PricesAsOf: at}
	return friendsFixture{
		rows: rows, run: run, boards: fakeBoards{run: run, hasRun: true, rows: rows}, viewer: rows[4].Subject.ID,
		reads: &fakeFollows{following: []ids.UserID{userOf(rows[3]), userOf(rows[1]), userOf(rows[4])}},
	}
}

func (f friendsFixture) read(t *testing.T, cursor int) domain.BoardPage {
	t.Helper()
	in := app.ReadBoard{
		Board: "people", Range: domain.RangeAll, Cursor: cursor, Limit: 2, Viewer: &f.viewer, Friends: f.reads,
	}
	page, err := in.Run(t.Context(), f.boards)
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func TestReadBoard_friendsRenumbersTheFirstPage(t *testing.T) {
	t.Parallel()
	f := newFriendsFixture()
	page := f.read(t, 0)
	if len(page.Rows) != 2 || page.Rows[0].Subject.ID != f.rows[1].Subject.ID || page.Rows[0].Rank != 1 ||
		page.Rows[1].Subject.ID != f.rows[3].Subject.ID || page.Rows[1].Rank != 2 {
		t.Fatalf("rows = %+v", page.Rows)
	}
	if page.NextCursor == nil || *page.NextCursor != domain.EncodeCursor(2) || page.Me == nil || page.Me.Rank != 3 {
		t.Fatalf("cursor = %v, me = %+v", page.NextCursor, page.Me)
	}
}

func TestReadBoard_friendsPagesOverTheRenumberedList(t *testing.T) {
	t.Parallel()
	f := newFriendsFixture()
	last := f.read(t, 2)
	if len(last.Rows) != 1 || last.Rows[0].Subject.ID != f.viewer || last.Rows[0].Rank != 3 || last.NextCursor != nil ||
		last.Me == nil || last.Me.Rank != 3 {
		t.Fatalf("last page = %+v", last)
	}
	past := f.read(t, 3)
	if len(past.Rows) != 0 || past.NextCursor != nil || past.Me == nil || !past.ComputedAt.Equal(f.run.FinishedAt) {
		t.Fatalf("page past the end = %+v", past)
	}
}

func TestReadBoard_friendsCallsThePortOncePerRead(t *testing.T) {
	t.Parallel()
	f := newFriendsFixture()
	f.read(t, 0)
	f.read(t, 2)
	if len(f.reads.viewers) != 2 || f.reads.viewers[0] != ids.UserIDFrom(f.viewer) {
		t.Fatalf("port calls = %v, want one per read for the viewer", f.reads.viewers)
	}
}

func TestReadBoard_friendsOfAViewerWhoFollowsNobodyIsTheViewerAlone(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	rows := people(3, at)
	boards := fakeBoards{run: domain.Run{ID: ids.Real{}.NewV7()}, hasRun: true, rows: rows}
	ranked := rows[2].Subject.ID
	in := app.ReadBoard{Limit: 20, Viewer: &ranked, Friends: &fakeFollows{}}
	got, err := in.Run(t.Context(), boards)
	if err != nil || len(got.Rows) != 1 || got.Rows[0].Rank != 1 || got.Rows[0].Subject.ID != ranked ||
		got.Me == nil || got.Me.Rank != 1 {
		t.Fatalf("page = %+v, %v", got, err)
	}
	unranked := ids.Real{}.NewV7()
	in.Viewer = &unranked
	got, err = in.Run(t.Context(), boards)
	if err != nil || len(got.Rows) != 0 || got.Me != nil || got.NextCursor != nil {
		t.Fatalf("unranked page = %+v, %v", got, err)
	}
}

func TestReadBoard_friendsStopsOnErrorsAndBeforeTheFirstRun(t *testing.T) {
	t.Parallel()
	viewer := ids.Real{}.NewV7()
	down := errs.New(errs.CodeUpstreamUnavailable, "test")
	run := domain.Run{ID: ids.Real{}.NewV7()}
	for name, tc := range map[string]struct {
		boards  fakeBoards
		follows app.Follows
		viewer  *uuid.UUID
		want    errs.Code
	}{
		"social":    {fakeBoards{run: run, hasRun: true}, &fakeFollows{err: down}, &viewer, errs.CodeUpstreamUnavailable},
		"subjects":  {fakeBoards{run: run, hasRun: true, failing: "subjects"}, &fakeFollows{}, &viewer, errs.CodeInternal},
		"no viewer": {fakeBoards{run: run, hasRun: true}, &fakeFollows{}, nil, errs.CodeInternal},
		"unwired":   {fakeBoards{run: run, hasRun: true}, app.UnwiredFollows{}, &viewer, errs.CodeUpstreamUnavailable},
	} {
		_, err := app.ReadBoard{Limit: 20, Viewer: tc.viewer, Friends: tc.follows}.Run(t.Context(), tc.boards)
		if errs.CodeOf(err) != tc.want {
			t.Errorf("%s: err = %v, want %s", name, err, tc.want)
		}
	}
	follows := &fakeFollows{}
	got, err := app.ReadBoard{Limit: 20, Viewer: &viewer, Friends: follows}.Run(t.Context(), fakeBoards{})
	if err != nil || got.RunID != nil || len(got.Rows) != 0 || len(follows.viewers) != 0 {
		t.Fatalf("page before the first run = %+v, %v, port calls %d", got, err, len(follows.viewers))
	}
}
