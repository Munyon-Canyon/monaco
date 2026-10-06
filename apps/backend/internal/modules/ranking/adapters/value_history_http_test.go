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

func (s stubSnapshots) LatestValuesOf(context.Context, []ids.CabalID) (map[ids.CabalID]domain.Snapshot, error) {
	return map[ids.CabalID]domain.Snapshot{}, nil
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

type fixedLatest struct {
	stubSnapshots
	values map[ids.CabalID]domain.Snapshot
}

func (f fixedLatest) LatestValuesOf(context.Context, []ids.CabalID) (map[ids.CabalID]domain.Snapshot, error) {
	return f.values, nil
}

type stubCards map[ids.CabalID]app.CabalView

func (s stubCards) Cabals(context.Context, []ids.CabalID) (map[ids.CabalID]app.CabalView, error) {
	return s, nil
}

func TestGetMyPortfolio_needsASignedInUser(t *testing.T) {
	t.Parallel()
	h := historyHTTP(0)
	if _, err := h.GetMyPortfolio(
		t.Context(),
		api.GetMyPortfolioRequestObject{},
	); errs.CodeOf(
		err,
	) != errs.CodeUnauthorized {
		t.Fatalf("anonymous: err = %v, want unauthorized", err)
	}
	ctx := asActor(t.Context(), auth.ActorUser, ids.Real{}.NewV7().String())
	h.Boards = stubBoards{err: errs.New(errs.CodeInternal, "test")}
	if _, err := h.GetMyPortfolio(ctx, api.GetMyPortfolioRequestObject{}); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("store: err = %v, want internal", err)
	}
}

func portfolioReq(
	t *testing.T, value uint64, net int64, picture string,
) (api.GetMyPortfolioResponseObject, error) {
	t.Helper()
	ctx := asActor(t.Context(), auth.ActorUser, ids.Real{}.NewV7().String())
	at := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	cabalID := ids.CabalIDFrom(ids.Real{}.NewV7())
	stake := app.StakePoint{
		CabalID: cabalID, At: at, ShareUnits: money.SharesUnitsFromUint64(1),
		NetContributed: money.SignedMicrosFromInt64(net),
	}
	snap := domain.Snapshot{At: at, Value: money.MicrosFromUint64(value), TotalShares: money.SharesUnitsFromUint64(1)}
	h := historyHTTP(0)
	h.Stakes = stubStakes{points: []app.StakePoint{stake}}
	h.Snapshots = fixedLatest{values: map[ids.CabalID]domain.Snapshot{cabalID: snap}}
	h.Cards = stubCards{cabalID: {ID: cabalID, Name: "Alpha", PictureURL: picture}}
	return h.GetMyPortfolio(ctx, api.GetMyPortfolioRequestObject{})
}

func TestGetMyPortfolio_mapsRows(t *testing.T) {
	t.Parallel()
	got, err := portfolioReq(t, 300, 100, "https://example.test/a.png")
	out, ok := got.(api.GetMyPortfolio200JSONResponse)
	if err != nil || !ok || out.TotalValueMicros != 300 || out.PnlMicros != 200 || *out.ReturnBps != 20_000 ||
		len(out.Cabals) != 1 {
		t.Fatalf("response = %+v, %v", got, err)
	}
	if row := out.Cabals[0]; row.Cabal.Name != "Alpha" || row.Cabal.PictureUrl == nil || row.SliceBps != 10_000 ||
		row.ShareUnits != 1 || row.NetContributedMicros != 100 {
		t.Fatalf("row = %+v", row)
	}
}

func TestGetMyPortfolio_mapsNullsForNothingContributedAndNoPicture(t *testing.T) {
	t.Parallel()
	got, err := portfolioReq(t, 300, 0, "")
	out, _ := got.(api.GetMyPortfolio200JSONResponse)
	if err != nil || out.ReturnBps != nil || out.Cabals[0].ReturnBps != nil || out.Cabals[0].Cabal.PictureUrl != nil {
		t.Fatalf("with nothing contributed and no picture = %+v, %v", got, err)
	}
}

func TestGetMyPortfolio_failsOnAValueThatDoesNotFit(t *testing.T) {
	t.Parallel()
	if _, err := portfolioReq(t, math.MaxInt64+10, 20_000, ""); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("huge value: err = %v, want internal", err)
	}
}

type stubUsers struct{ known map[ids.UserID]app.UserCard }

func (s stubUsers) UsersByID(context.Context, []ids.UserID) (map[ids.UserID]app.UserCard, error) {
	return s.known, nil
}

type stubMembers []ids.CabalID

func (s stubMembers) CabalsOf(context.Context, ids.UserID) ([]ids.CabalID, error) { return s, nil }

func sharedReq(t *testing.T, value uint64, net int64, known bool) (api.GetSharedCabalsResponseObject, error) {
	t.Helper()
	viewer := ids.UserIDFrom(ids.Real{}.NewV7())
	other := ids.UserIDFrom(ids.Real{}.NewV7())
	ctx := asActor(t.Context(), auth.ActorUser, viewer.String())
	at := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	cabalID := ids.CabalIDFrom(ids.Real{}.NewV7())
	snap := domain.Snapshot{At: at, Value: money.MicrosFromUint64(value)}
	h := historyHTTP(net)
	h.Users = stubUsers{known: map[ids.UserID]app.UserCard{}}
	if known {
		h.Users = stubUsers{known: map[ids.UserID]app.UserCard{other: {ID: other}}}
	}
	h.Members = stubMembers{cabalID}
	h.Snapshots = fixedLatest{values: map[ids.CabalID]domain.Snapshot{cabalID: snap}}
	h.Cards = stubCards{cabalID: {ID: cabalID, Name: "Alpha"}}
	return h.GetSharedCabals(ctx, api.GetSharedCabalsRequestObject{Id: other.UUID()})
}

func TestGetSharedCabals_needsASignedInUser(t *testing.T) {
	t.Parallel()
	h := historyHTTP(0)
	if _, err := h.GetSharedCabals(
		t.Context(),
		api.GetSharedCabalsRequestObject{},
	); errs.CodeOf(
		err,
	) != errs.CodeUnauthorized {
		t.Fatalf("anonymous: err = %v, want unauthorized", err)
	}
}

func TestGetSharedCabals_mapsThePotFigures(t *testing.T) {
	t.Parallel()
	got, err := sharedReq(t, 300, 100, true)
	out, ok := got.(api.GetSharedCabals200JSONResponse)
	if err != nil || !ok || len(out.Cabals) != 1 || out.Cabals[0].Cabal.Name != "Alpha" ||
		out.Cabals[0].ValueMicros != 300 ||
		out.Cabals[0].PnlMicros != 200 ||
		*out.Cabals[0].ReturnBps != 20_000 {
		t.Fatalf("response = %+v, %v", got, err)
	}
}

func TestGetSharedCabals_failsForAnUnknownUserOrAValueThatDoesNotFit(t *testing.T) {
	t.Parallel()
	if _, err := sharedReq(t, 300, 100, false); errs.CodeOf(err) != errs.CodeUserNotFound {
		t.Fatalf("unknown user: err = %v, want user_not_found", err)
	}
	if _, err := sharedReq(t, math.MaxInt64+10, 20_000, true); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("huge value: err = %v, want internal", err)
	}
}
