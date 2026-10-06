package adapters_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/rankingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type stubBoards struct {
	run     bool
	entries []domain.Entry
	err     error
}

func (s stubBoards) LatestRun(context.Context) (domain.Run, bool, error) {
	return domain.Run{ID: ids.Real{}.NewV7()}, s.run, s.err
}

func (s stubBoards) Page(context.Context, string, domain.Range, int, int) ([]domain.Entry, error) {
	return s.entries, s.err
}

func (stubBoards) Row(context.Context, string, domain.Range, uuid.UUID) (domain.Entry, bool, error) {
	return domain.Entry{}, false, nil
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
