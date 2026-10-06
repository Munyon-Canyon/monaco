package adapters_test

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type seedRow struct {
	board   string
	rank    int
	subject uuid.UUID
	bps     *int64
	value   int64
	handle  *string
}

func seedEntries(t *testing.T, q *sqlc.Queries, at time.Time, rows []seedRow) {
	t.Helper()
	wire := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		wire = append(wire, map[string]any{
			"board": r.board, "range": "ALL", "rank": r.rank, "subject_id": r.subject.String(),
			"subject_name": "Subject", "subject_handle": r.handle, "subject_created_at": at.Format(time.RFC3339Nano),
			"value_micros": r.value, "pnl_micros": -5, "return_bps": r.bps,
			"prices_as_of": at.Format(time.RFC3339Nano), "computed_at": at.Format(time.RFC3339Nano),
			"flags": []string{"stale_prices"},
		})
	}
	raw, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	if err := q.InsertLeaderboardEntries(t.Context(), raw); err != nil {
		t.Fatal(err)
	}
}

func seedRun(t *testing.T, q *sqlc.Queries, at time.Time) uuid.UUID {
	t.Helper()
	run := ids.Real{}.NewV7()
	if err := q.InsertLeaderboardRun(t.Context(), sqlc.InsertLeaderboardRunParams{
		RunID: run, AsOf: at, PricesAsOf: at.Add(-time.Minute), StartedAt: at, FinishedAt: at.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	return run
}

func cabalRows(n int) []seedRow {
	rows := make([]seedRow, 0, n)
	for i := 1; i <= n; i++ {
		bps := int64(1000 - i)
		rows = append(rows, seedRow{board: "cabals", rank: i, subject: ids.Real{}.NewV7(), bps: &bps, value: int64(i)})
	}
	return rows
}

func now() time.Time { return clock.Real{}.Now().UTC().Truncate(time.Microsecond) }

func TestBoards_CabalsPage_KeysetByRank(t *testing.T) {
	t.Parallel()
	db := testkit.DB(t)
	at := now()
	run := seedRun(t, sqlc.New(db), at.Add(-time.Second))
	seedEntries(t, sqlc.New(db), at, cabalRows(45))
	cursor, sizes := 0, []int{}
	for range 3 {
		in := app.ReadBoard{Board: "cabals", Range: domain.RangeAll, Cursor: cursor, Limit: 20}
		got, err := in.Run(t.Context(), adapters.Boards{DB: db})
		if err != nil {
			t.Fatal(err)
		}
		assertPage(t, got, run, at, cursor)
		sizes = append(sizes, len(got.Rows))
		if got.NextCursor == nil {
			break
		}
		if cursor, err = domain.DecodeCursor(*got.NextCursor); err != nil {
			t.Fatal(err)
		}
	}
	if len(sizes) != 3 || sizes[0] != 20 || sizes[1] != 20 || sizes[2] != 5 {
		t.Fatalf("page sizes = %v, want 20, 20, 5 with no cursor after the last", sizes)
	}
}

func assertPage(t *testing.T, got domain.BoardPage, run uuid.UUID, at time.Time, cursor int) {
	t.Helper()
	first := got.Rows[0]
	if got.RunID == nil || *got.RunID != run || got.Me != nil || !got.ComputedAt.Equal(at) {
		t.Fatalf("page after %d = %+v", cursor, got)
	}
	if first.Rank != cursor+1 || first.Return == nil || first.Value.String() != strconv.Itoa(cursor+1) {
		t.Fatalf("first row after %d = %+v", cursor, first)
	}
}

func TestBoards_EmptyBeforeFirstRun(t *testing.T) {
	t.Parallel()
	viewer := ids.Real{}.NewV7()
	in := app.ReadBoard{Board: "people", Range: domain.RangeAll, Limit: 20, Viewer: &viewer}
	got, err := in.Run(t.Context(), adapters.Boards{DB: testkit.DB(t)})
	if err != nil || got.RunID != nil || len(got.Rows) != 0 || got.Me != nil || got.NextCursor != nil {
		t.Fatalf("page = %+v, %v", got, err)
	}
}

func TestBoards_EmptyRangeFallsBackToRun(t *testing.T) {
	t.Parallel()
	db := testkit.DB(t)
	at := now()
	run := seedRun(t, sqlc.New(db), at)
	in := app.ReadBoard{Board: "cabals", Range: domain.Range1D, Limit: 20}
	got, err := in.Run(t.Context(), adapters.Boards{DB: db})
	if err != nil || got.RunID == nil || *got.RunID != run || len(got.Rows) != 0 ||
		!got.ComputedAt.Equal(at.Add(time.Second)) || !got.PricesAsOf.Equal(at.Add(-time.Minute)) {
		t.Fatalf("page = %+v, %v", got, err)
	}
}

func TestBoards_MeIsNilWithoutRow(t *testing.T) {
	t.Parallel()
	db := testkit.DB(t)
	at := now()
	seedRun(t, sqlc.New(db), at)
	rows := cabalRows(3)
	handle := "ada"
	rows[2].board, rows[2].handle, rows[2].bps = "people", &handle, nil
	seedEntries(t, sqlc.New(db), at, rows)
	in := app.ReadBoard{Board: "people", Range: domain.RangeAll, Limit: 20, Viewer: &rows[2].subject}
	got, err := in.Run(t.Context(), adapters.Boards{DB: db})
	me := got.Me
	if err != nil || me == nil || me.Rank != 3 || me.Return != nil || me.Subject.Handle == nil ||
		*me.Subject.Handle != "ada" || me.Subject.PictureURL != nil || me.PnL.Int64() != -5 || len(me.Flags) != 1 {
		t.Fatalf("me = %+v, %v", me, err)
	}
	other := ids.Real{}.NewV7()
	in.Viewer = &other
	if got, err = in.Run(t.Context(), adapters.Boards{DB: db}); err != nil || got.Me != nil {
		t.Fatalf("me = %+v, %v, want nil", got.Me, err)
	}
}

func TestBoards_BadStoredRowsAreInternal(t *testing.T) {
	t.Parallel()
	db := testkit.DB(t)
	bad := cabalRows(1)
	bad[0].value = -1
	seedEntries(t, sqlc.New(db), now(), bad)
	boards := adapters.Boards{DB: db}
	if _, err := boards.Page(t.Context(), "cabals", domain.RangeAll, 0, 2); errs.CodeOf(err) != errs.CodeInternal {
		t.Errorf("Page err = %v, want internal", err)
	}
	_, _, err := boards.Row(t.Context(), "cabals", domain.RangeAll, bad[0].subject)
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Errorf("Row err = %v, want internal", err)
	}
	if _, err = boards.Subjects(t.Context(), "cabals", domain.RangeAll, []uuid.UUID{bad[0].subject}); errs.CodeOf(
		err) != errs.CodeInternal {
		t.Errorf("Subjects err = %v, want internal", err)
	}
}

func TestBoards_SubjectsReadsOnlyTheNamedSubjectsInRankOrder(t *testing.T) {
	t.Parallel()
	db := testkit.DB(t)
	at := now()
	rows := cabalRows(4)
	for i := range rows {
		rows[i].board = "people"
	}
	seedEntries(t, sqlc.New(db), at, rows)
	boards := adapters.Boards{DB: db}
	got, err := boards.Subjects(t.Context(), "people", domain.RangeAll,
		[]uuid.UUID{rows[3].subject, ids.Real{}.NewV7(), rows[1].subject})
	if err != nil || len(got) != 2 || got[0].Subject.ID != rows[1].subject || got[0].Rank != 2 ||
		got[1].Subject.ID != rows[3].subject || got[1].Rank != 4 {
		t.Fatalf("subjects = %+v, %v", got, err)
	}
	if got, err = boards.Subjects(t.Context(), "people", domain.Range1D, []uuid.UUID{rows[1].subject}); err != nil ||
		len(got) != 0 {
		t.Fatalf("subjects on another range = %+v, %v, want none", got, err)
	}
}

func TestBoards_PageRejectsOutOfRangeBounds(t *testing.T) {
	t.Parallel()
	boards := adapters.Boards{DB: testkit.DB(t)}
	for _, b := range [][2]int{{-1, 5}, {1 << 40, 5}, {0, 0}, {0, 1 << 40}} {
		_, err := boards.Page(t.Context(), "cabals", domain.RangeAll, b[0], b[1])
		if errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("Page(%v) err = %v, want invalid_input", b, err)
		}
	}
}

func TestBoards_ClosedPoolIsInternal(t *testing.T) {
	t.Parallel()
	db := testkit.DB(t)
	db.Close()
	boards := adapters.Boards{DB: db}
	_, _, latest := boards.LatestRun(t.Context())
	_, page := boards.Page(t.Context(), "cabals", domain.RangeAll, 0, 2)
	_, _, row := boards.Row(t.Context(), "cabals", domain.RangeAll, ids.Real{}.NewV7())
	_, subjects := boards.Subjects(t.Context(), "people", domain.RangeAll, []uuid.UUID{ids.Real{}.NewV7()})
	for name, err := range map[string]error{"latest": latest, "page": page, "row": row, "subjects": subjects} {
		if errs.CodeOf(err) != errs.CodeInternal {
			t.Errorf("%s err = %v, want internal", name, err)
		}
	}
}
