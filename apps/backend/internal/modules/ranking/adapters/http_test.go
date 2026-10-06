package adapters_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/rankingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type stubBoards struct {
	run     bool
	asOf    time.Time
	entries []domain.Entry
	me      *domain.Entry
	err     error
}

func (s stubBoards) LatestRun(context.Context) (domain.Run, bool, error) {
	return domain.Run{ID: ids.Real{}.NewV7(), AsOf: s.asOf}, s.run, s.err
}

func (s stubBoards) Page(context.Context, string, domain.Range, int, int) ([]domain.Entry, error) {
	return s.entries, s.err
}

func (s stubBoards) Row(context.Context, string, domain.Range, uuid.UUID) (domain.Entry, bool, error) {
	if s.me == nil {
		return domain.Entry{}, false, nil
	}
	return *s.me, true, nil
}

func (s stubBoards) Subjects(context.Context, string, domain.Range, []uuid.UUID) ([]domain.Entry, error) {
	return s.entries, s.err
}

func ptr[T any](v T) *T { return &v }

func TestGetCabalsLeaderboard_rejectsBadParams(t *testing.T) {
	t.Parallel()
	h := adapters.HTTP{Boards: stubBoards{run: true}}
	for name, params := range map[string]api.GetCabalsLeaderboardParams{
		"range":  {Range: ptr(api.GetCabalsLeaderboardParamsRange("2Y"))},
		"cursor": {Cursor: ptr("!!")},
		"limit":  {Limit: ptr(0)},
	} {
		_, err := h.GetCabalsLeaderboard(t.Context(), api.GetCabalsLeaderboardRequestObject{Params: params})
		if errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("%s: err = %v, want invalid_input", name, err)
		}
	}
}

func TestGetCabalsLeaderboard_mapsRowsAndFailsOnUnrepresentableOnes(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	bps := domain.Bps(-40)
	good := domain.Entry{
		Rank: 1, Value: money.MicrosFromUint64(7), PnL: money.SignedMicrosFromInt64(-3), Return: &bps,
		Flags: []domain.Flag{domain.FlagStalePrices}, ComputedAt: at, PricesAsOf: at,
	}
	huge := domain.Entry{Rank: 1, Value: money.MicrosFromUint64(math.MaxUint64)}
	far := domain.Entry{Rank: math.MaxInt32 + 1}
	h := adapters.HTTP{Boards: stubBoards{run: true, entries: []domain.Entry{good}}}
	got, err := h.GetCabalsLeaderboard(t.Context(), api.GetCabalsLeaderboardRequestObject{})
	page, ok := got.(api.GetCabalsLeaderboard200JSONResponse)
	if err != nil || !ok || len(page.Rows) != 1 || *page.Rows[0].ReturnBps != -40 || page.Rows[0].ValueMicros != 7 ||
		page.Rows[0].Flags[0] != api.StalePrices || page.Rows[0].Subject.Kind != api.Cabal {
		t.Fatalf("response = %+v, %v", got, err)
	}
	for _, bad := range []domain.Entry{huge, far} {
		h.Boards = stubBoards{run: true, entries: []domain.Entry{bad}}
		_, err := h.GetCabalsLeaderboard(t.Context(), api.GetCabalsLeaderboardRequestObject{})
		if errs.CodeOf(err) != errs.CodeInternal {
			t.Errorf("entry %+v: err = %v, want internal", bad, err)
		}
	}
	h.Boards = stubBoards{err: errs.New(errs.CodeInternal, "test")}
	_, err = h.GetCabalsLeaderboard(t.Context(), api.GetCabalsLeaderboardRequestObject{})
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Errorf("store error: err = %v, want internal", err)
	}
}

func asActor(ctx context.Context, kind auth.ActorKind, id string) context.Context {
	return auth.WithActor(ctx, auth.Actor{Kind: kind, ID: id})
}

func TestGetPeopleLeaderboard_needsASignedInUser(t *testing.T) {
	t.Parallel()
	h := adapters.HTTP{Boards: stubBoards{run: true}}
	req := api.GetPeopleLeaderboardRequestObject{}
	for name, tc := range map[string]struct {
		with func(context.Context) context.Context
		want errs.Code
	}{
		"anonymous": {func(ctx context.Context) context.Context { return ctx }, errs.CodeUnauthorized},
		"admin":     {func(ctx context.Context) context.Context { return asActor(ctx, auth.ActorAdmin, "a") }, errs.CodeForbidden},
		"bad id":    {func(ctx context.Context) context.Context { return asActor(ctx, auth.ActorUser, "nope") }, errs.CodeUnauthorized},
	} {
		if _, err := h.GetPeopleLeaderboard(tc.with(t.Context()), req); errs.CodeOf(err) != tc.want {
			t.Errorf("%s: err = %v, want %s", name, err, tc.want)
		}
	}
}

func people(t *testing.T, boards stubBoards, limit *int) (api.LeaderboardPage, error) {
	t.Helper()
	ctx := asActor(t.Context(), auth.ActorUser, ids.Real{}.NewV7().String())
	req := api.GetPeopleLeaderboardRequestObject{Params: api.GetPeopleLeaderboardParams{Limit: limit}}
	got, err := adapters.HTTP{Boards: boards}.GetPeopleLeaderboard(ctx, req)
	page, _ := got.(api.GetPeopleLeaderboard200JSONResponse)
	return api.LeaderboardPage(page), err
}

func TestGetPeopleLeaderboard_carriesTheCallersRankedRowAsMe(t *testing.T) {
	t.Parallel()
	bps := domain.Bps(12)
	page, err := people(t, stubBoards{run: true, me: &domain.Entry{Rank: 41, Return: &bps}}, nil)
	if err != nil || page.Me == nil || page.Me.Rank != 41 || page.Me.Subject.Kind != api.User {
		t.Fatalf("page = %+v, %v", page, err)
	}
	page, err = people(t, stubBoards{run: true, me: &domain.Entry{Rank: 41}}, nil)
	if err != nil || page.Me != nil {
		t.Fatalf("unranked me = %+v, %v, want nil", page.Me, err)
	}
}

func TestGetPeopleLeaderboard_failsOnBadInputAndUnrepresentableRows(t *testing.T) {
	t.Parallel()
	bps := domain.Bps(12)
	huge := domain.Entry{Rank: 1, Return: &bps, Value: money.MicrosFromUint64(math.MaxUint64)}
	for name, tc := range map[string]struct {
		boards stubBoards
		limit  *int
		want   errs.Code
	}{
		"huge me":     {stubBoards{run: true, me: &huge}, nil, errs.CodeInternal},
		"store error": {stubBoards{err: errs.New(errs.CodeInternal, "test")}, nil, errs.CodeInternal},
		"bad limit":   {stubBoards{run: true}, ptr(99), errs.CodeInvalidInput},
	} {
		if _, err := people(t, tc.boards, tc.limit); errs.CodeOf(err) != tc.want {
			t.Errorf("%s: err = %v, want %s", name, err, tc.want)
		}
	}
}

func cabalCheck(err error) app.CabalCheck {
	return func(context.Context, ids.CabalID) error { return err }
}

func TestGetCabalLeaderboard_checksTheCabalAndKeepsAnUnrankedMe(t *testing.T) {
	t.Parallel()
	ctx := asActor(t.Context(), auth.ActorUser, ids.Real{}.NewV7().String())
	req := api.GetCabalLeaderboardRequestObject{Id: ids.Real{}.NewV7()}
	h := adapters.HTTP{Boards: stubBoards{run: true, me: &domain.Entry{Rank: 2}}, Cabals: cabalCheck(nil)}
	got, err := h.GetCabalLeaderboard(ctx, req)
	page, ok := got.(api.GetCabalLeaderboard200JSONResponse)
	if err != nil || !ok || page.Me == nil || page.Me.Rank != 2 || page.Me.ReturnBps != nil {
		t.Fatalf("response = %+v, %v", got, err)
	}
	h.Cabals = cabalCheck(errs.New(errs.CodeCabalNotFound, "test"))
	if _, err = h.GetCabalLeaderboard(ctx, req); errs.CodeOf(err) != errs.CodeCabalNotFound {
		t.Fatalf("unknown cabal: err = %v, want cabal_not_found", err)
	}
	if _, err = h.GetCabalLeaderboard(t.Context(), req); errs.CodeOf(err) != errs.CodeUnauthorized {
		t.Fatalf("anonymous: err = %v, want unauthorized", err)
	}
	h.Cabals = cabalCheck(nil)
	req.Params.Limit = ptr(0)
	if _, err = h.GetCabalLeaderboard(ctx, req); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("bad limit: err = %v, want invalid_input", err)
	}
}

type stubFollows struct {
	ids []ids.UserID
	err error
}

func (s stubFollows) FollowingIDs(context.Context, ids.UserID) ([]ids.UserID, error) {
	return s.ids, s.err
}

func TestGetPeopleLeaderboard_filterSelectsTheFriendsBoardAndRejectsOthers(t *testing.T) {
	t.Parallel()
	me := ids.Real{}.NewV7()
	ctx := asActor(t.Context(), auth.ActorUser, me.String())
	bps := domain.Bps(7)
	row := domain.Entry{Rank: 9, Subject: domain.Subject{ID: me}, Return: &bps}
	h := adapters.HTTP{Boards: stubBoards{run: true, entries: []domain.Entry{row}}, Follows: stubFollows{}}
	for _, filter := range []api.GetPeopleLeaderboardParamsFilter{
		api.GetPeopleLeaderboardParamsFilterAll, api.GetPeopleLeaderboardParamsFilterFriends,
	} {
		got, err := h.GetPeopleLeaderboard(ctx, api.GetPeopleLeaderboardRequestObject{
			Params: api.GetPeopleLeaderboardParams{Filter: &filter},
		})
		page, _ := got.(api.GetPeopleLeaderboard200JSONResponse)
		if err != nil || len(page.Rows) != 1 {
			t.Fatalf("filter %s: page = %+v, %v", filter, page, err)
		}
		wantRank := int32(9)
		if filter == api.GetPeopleLeaderboardParamsFilterFriends {
			wantRank = 1
		}
		if page.Rows[0].Rank != wantRank {
			t.Errorf("filter %s: rank = %d, want %d", filter, page.Rows[0].Rank, wantRank)
		}
	}
	enemies := api.GetPeopleLeaderboardParamsFilter("enemies")
	_, err := h.GetPeopleLeaderboard(ctx, api.GetPeopleLeaderboardRequestObject{
		Params: api.GetPeopleLeaderboardParams{Filter: &enemies},
	})
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Errorf("filter enemies: err = %v, want invalid_input", err)
	}
}
