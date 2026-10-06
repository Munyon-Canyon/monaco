package adapters_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/rankingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type stubSnapshots struct{ rows []domain.Snapshot }

func (s stubSnapshots) SnapshotsOfCabals(
	context.Context, []ids.CabalID, time.Time, time.Time,
) (map[ids.CabalID][]domain.Snapshot, error) {
	return map[ids.CabalID][]domain.Snapshot{}, nil
}

func (s stubSnapshots) SnapshotsSince(context.Context, ids.CabalID, time.Time, time.Time) ([]domain.Snapshot, error) {
	return s.rows, nil
}

type stubLedger struct{ net int64 }

func (s stubLedger) CabalContributionHistory(context.Context, ids.CabalID) ([]app.ContributionPoint, error) {
	return []app.ContributionPoint{{At: time.Unix(0, 0), NetContributed: money.SignedMicrosFromInt64(s.net)}}, nil
}

func historyHTTP(net int64, rows ...domain.Snapshot) adapters.HTTP {
	return adapters.HTTP{
		Boards: stubBoards{
			run:  true,
			asOf: time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC),
		},
		Snapshots: stubSnapshots{rows: rows},
		Ledger:    stubLedger{net: net},
		Cabals:    func(context.Context, ids.CabalID) error { return nil },
	}
}

func historyReq(rng *api.GetCabalValueHistoryParamsRange) api.GetCabalValueHistoryRequestObject {
	return api.GetCabalValueHistoryRequestObject{
		Id:     ids.Real{}.NewV7(),
		Params: api.GetCabalValueHistoryParams{Range: rng},
	}
}

func TestGetCabalValueHistory_needsASignedInUserAndAKnownRange(t *testing.T) {
	t.Parallel()
	h := historyHTTP(0)
	if _, err := h.GetCabalValueHistory(t.Context(), historyReq(nil)); errs.CodeOf(err) != errs.CodeUnauthorized {
		t.Fatalf("anonymous: err = %v, want unauthorized", err)
	}
	ctx := asActor(t.Context(), auth.ActorUser, ids.Real{}.NewV7().String())
	bad := api.GetCabalValueHistoryParamsRange("2Y")
	if _, err := h.GetCabalValueHistory(ctx, historyReq(&bad)); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("range: err = %v, want invalid_input", err)
	}
	h.Boards = stubBoards{err: errs.New(errs.CodeInternal, "test")}
	if _, err := h.GetCabalValueHistory(ctx, historyReq(nil)); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("store: err = %v, want internal", err)
	}
}

func TestGetCabalValueHistory_mapsPointsAndFailsOnUnrepresentableOnes(t *testing.T) {
	t.Parallel()
	ctx := asActor(t.Context(), auth.ActorUser, ids.Real{}.NewV7().String())
	at := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	good := domain.Snapshot{At: at, Value: money.MicrosFromUint64(7), NavPerShare: money.MicrosFromUint64(2)}
	got, err := historyHTTP(0, good).GetCabalValueHistory(ctx, historyReq(nil))
	out, ok := got.(api.GetCabalValueHistory200JSONResponse)
	if err != nil || !ok || out.Range != api.CabalValueHistoryRangeALL || len(out.Points) != 1 ||
		out.Points[0].ValueMicros != 7 || out.Points[0].NavPerShareMicros != 2 || out.Points[0].PnlMicros != 7 {
		t.Fatalf("response = %+v, %v", got, err)
	}
	for name, bad := range map[string]domain.Snapshot{
		"value": {At: at, Value: money.MicrosFromUint64(math.MaxInt64 + 10)},
		"nav":   {At: at, NavPerShare: money.MicrosFromUint64(math.MaxUint64)},
	} {
		if _, err := historyHTTP(
			20,
			bad,
		).GetCabalValueHistory(ctx, historyReq(nil)); errs.CodeOf(
			err,
		) != errs.CodeInternal {
			t.Errorf("%s: err = %v, want internal", name, err)
		}
	}
}

type stubStakes struct{ points []app.StakePoint }

func (s stubStakes) UserStakeHistory(context.Context, ids.UserID) ([]app.StakePoint, error) {
	return s.points, nil
}

func TestGetMyPnlHistory_needsASignedInUserAndAKnownRange(t *testing.T) {
	t.Parallel()
	h := historyHTTP(0)
	if _, err := h.GetMyPnlHistory(
		t.Context(),
		api.GetMyPnlHistoryRequestObject{},
	); errs.CodeOf(
		err,
	) != errs.CodeUnauthorized {
		t.Fatalf("anonymous: err = %v, want unauthorized", err)
	}
	ctx := asActor(t.Context(), auth.ActorUser, ids.Real{}.NewV7().String())
	bad := api.GetMyPnlHistoryParamsRange("2Y")
	req := api.GetMyPnlHistoryRequestObject{Params: api.GetMyPnlHistoryParams{Range: &bad}}
	if _, err := h.GetMyPnlHistory(ctx, req); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("range: err = %v, want invalid_input", err)
	}
	h.Boards = stubBoards{err: errs.New(errs.CodeInternal, "test")}
	if _, err := h.GetMyPnlHistory(ctx, api.GetMyPnlHistoryRequestObject{}); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("store: err = %v, want internal", err)
	}
}

type fixedSnapshots struct {
	stubSnapshots
	series map[ids.CabalID][]domain.Snapshot
}

func (f fixedSnapshots) SnapshotsOfCabals(
	context.Context, []ids.CabalID, time.Time, time.Time,
) (map[ids.CabalID][]domain.Snapshot, error) {
	return f.series, nil
}

func TestGetMyPnlHistory_mapsPointsAndFailsOnUnrepresentableOnes(t *testing.T) {
	t.Parallel()
	ctx := asActor(t.Context(), auth.ActorUser, ids.Real{}.NewV7().String())
	at := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	cabal := ids.CabalIDFrom(ids.Real{}.NewV7())
	run := func(value uint64, net int64) (api.GetMyPnlHistoryResponseObject, error) {
		stake := app.StakePoint{
			CabalID: cabal, At: at, ShareUnits: money.SharesUnitsFromUint64(1),
			NetContributed: money.SignedMicrosFromInt64(net),
		}
		snap := domain.Snapshot{
			At:          at,
			Value:       money.MicrosFromUint64(value),
			TotalShares: money.SharesUnitsFromUint64(1),
		}
		h := historyHTTP(0)
		h.Stakes = stubStakes{points: []app.StakePoint{stake}}
		h.Snapshots = fixedSnapshots{series: map[ids.CabalID][]domain.Snapshot{cabal: {snap}}}
		return h.GetMyPnlHistory(ctx, api.GetMyPnlHistoryRequestObject{})
	}
	got, err := run(7, 2)
	out, ok := got.(api.GetMyPnlHistory200JSONResponse)
	if err != nil || !ok || out.Range != api.MyPnlHistoryRangeALL || len(out.Points) != 1 ||
		out.Points[0].EquityMicros != 7 || out.Points[0].PnlMicros != 5 {
		t.Fatalf("response = %+v, %v", got, err)
	}
	if _, err = run(math.MaxInt64+10, 20_000); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("huge equity: err = %v, want internal", err)
	}
}
