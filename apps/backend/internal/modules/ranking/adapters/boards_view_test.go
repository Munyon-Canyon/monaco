package adapters_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/rankingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestBoards_ViewReadsTheDefaultSortAndReportsAMissingRow(t *testing.T) {
	t.Parallel()
	db := testkit.DB(t)
	rows := cabalRows(2)
	seedEntries(t, sqlc.New(db), now(), rows)
	boards := adapters.Boards{DB: db}
	page, err := boards.ViewPage(t.Context(), "cabals", domain.RangeAll, domain.View{}, 0, 5)
	if err != nil || len(page) != 2 || page[0].Subject.ID != rows[0].subject {
		t.Fatalf("page = %+v, %v, want both rows by return", page, err)
	}
	row, ok, err := boards.ViewRow(t.Context(), "cabals", domain.RangeAll, domain.View{}, rows[1].subject)
	if err != nil || !ok || row.Rank != 2 {
		t.Fatalf("row = %+v, %v, %v, want rank 2", row, ok, err)
	}
	if _, ok, err = boards.ViewRow(
		t.Context(), "cabals", domain.RangeAll, domain.View{}, ids.Real{}.NewV7()); err != nil || ok {
		t.Fatalf("missing row = %v, %v, want none", ok, err)
	}
}

func TestBoards_ViewBadStoredRowsAreInternal(t *testing.T) {
	t.Parallel()
	db := testkit.DB(t)
	bad := cabalRows(1)
	bad[0].value = -1
	seedEntries(t, sqlc.New(db), now(), bad)
	boards := adapters.Boards{DB: db}
	if _, err := boards.ViewPage(
		t.Context(), "cabals", domain.RangeAll, domain.View{}, 0, 2); errs.CodeOf(err) != errs.CodeInternal {
		t.Errorf("ViewPage err = %v, want internal", err)
	}
	_, _, err := boards.ViewRow(t.Context(), "cabals", domain.RangeAll, domain.View{}, bad[0].subject)
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Errorf("ViewRow err = %v, want internal", err)
	}
}

func TestBoards_ViewOnAClosedPoolIsInternal(t *testing.T) {
	t.Parallel()
	db := testkit.DB(t)
	db.Close()
	boards := adapters.Boards{DB: db}
	_, page := boards.ViewPage(t.Context(), "cabals", domain.RangeAll, domain.View{}, 0, 2)
	_, _, row := boards.ViewRow(t.Context(), "cabals", domain.RangeAll, domain.View{}, ids.Real{}.NewV7())
	for name, err := range map[string]error{"page": page, "row": row} {
		if errs.CodeOf(err) != errs.CodeInternal {
			t.Errorf("%s err = %v, want internal", name, err)
		}
	}
}

func TestGetPeopleLeaderboard_rejectsAnUnknownSort(t *testing.T) {
	t.Parallel()
	h := adapters.HTTP{Boards: stubBoards{run: true}}
	sort := api.GetPeopleLeaderboardParamsSort("size")
	_, err := h.GetPeopleLeaderboard(
		asActor(t.Context(), auth.ActorUser, ids.Real{}.NewV7().String()),
		api.GetPeopleLeaderboardRequestObject{Params: api.GetPeopleLeaderboardParams{Sort: &sort}},
	)
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Errorf("err = %v, want invalid_input", err)
	}
}

func TestBoards_ViewPageRejectsOutOfRangeBounds(t *testing.T) {
	t.Parallel()
	boards := adapters.Boards{DB: testkit.DB(t)}
	for _, b := range [][2]int{{-1, 5}, {1 << 40, 5}, {0, 0}, {0, 1 << 40}} {
		_, err := boards.ViewPage(t.Context(), "cabals", domain.RangeAll, domain.View{}, b[0], b[1])
		if errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("ViewPage(%v) err = %v, want invalid_input", b, err)
		}
	}
	_, err := boards.ViewPage(t.Context(), "cabals", domain.RangeAll, domain.View{MinMembers: -1}, 0, 5)
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Errorf("ViewPage min members err = %v, want invalid_input", err)
	}
}
