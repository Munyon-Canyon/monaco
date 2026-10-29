package adapters_test

import (
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

func rangeAt() time.Time { return time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC) }

func dailySnapshots(final uint64) []domain.Snapshot {
	const days = 200
	at := rangeAt()
	out := make([]domain.Snapshot, 0, days+1)
	for d := days; d >= 0; d-- {
		value := uint64(1000)
		if d <= 30 {
			value = 1500
		}
		if d == 0 {
			value = final
		}
		out = append(out, domain.Snapshot{
			At: at.AddDate(0, 0, -d), Value: money.MicrosFromUint64(value),
			NavPerShare: money.MicrosFromUint64(1), TotalShares: money.SharesUnitsFromUint64(1),
		})
	}
	return out
}

func rangeHTTP(cabal ids.CabalID, final uint64) adapters.HTTP {
	at := rangeAt()
	stake := func(days int, net int64) app.StakePoint {
		return app.StakePoint{
			CabalID: cabal, At: at.AddDate(0, 0, -days), ShareUnits: money.SharesUnitsFromUint64(1),
			NetContributed: money.SignedMicrosFromInt64(net),
		}
	}
	rows := dailySnapshots(final)
	h := historyHTTP(1500, rows...)
	h.Stakes = stubStakes{points: []app.StakePoint{stake(210, 1000), stake(30, 1500)}}
	series := map[ids.CabalID][]domain.Snapshot{cabal: rows}
	h.Snapshots = fixedSnapshots{stubSnapshots: stubSnapshots{rows: rows}, series: series}
	return h
}

func startsAt(span int, at time.Time) bool { return at.Equal(rangeAt().AddDate(0, 0, -span)) }

func readPnL(t *testing.T, h adapters.HTTP, raw string) api.GetMyPnlHistory200JSONResponse {
	t.Helper()
	ctx := asActor(t.Context(), auth.ActorUser, ids.Real{}.NewV7().String())
	got, err := h.GetMyPnlHistory(ctx, api.GetMyPnlHistoryRequestObject{
		Params: api.GetMyPnlHistoryParams{Range: ptr(api.GetMyPnlHistoryParamsRange(raw))},
	})
	out, ok := got.(api.GetMyPnlHistory200JSONResponse)
	if err != nil || !ok || string(out.Range) != raw {
		t.Fatalf("pnl %s: %+v, %v", raw, got, err)
	}
	return out
}

func readValue(t *testing.T, h adapters.HTTP, raw string) api.GetCabalValueHistory200JSONResponse {
	t.Helper()
	ctx := asActor(t.Context(), auth.ActorUser, ids.Real{}.NewV7().String())
	got, err := h.GetCabalValueHistory(ctx, historyReq(ptr(api.GetCabalValueHistoryParamsRange(raw))))
	out, ok := got.(api.GetCabalValueHistory200JSONResponse)
	if err != nil || !ok || string(out.Range) != raw {
		t.Fatalf("value %s: %+v, %v", raw, got, err)
	}
	return out
}

func TestGetMyPnlHistory_servesThreeAndSixMonths(t *testing.T) {
	t.Parallel()
	h := rangeHTTP(ids.CabalIDFrom(ids.Real{}.NewV7()), 1741)
	budget := len(readPnL(t, h, "1M").Points)
	for raw, span := range map[string]int{"3M": 90, "6M": 182} {
		out := readPnL(t, h, raw)
		if len(out.Points) == 0 || len(out.Points) > budget || !startsAt(span, out.Points[0].At) {
			t.Fatalf(
				"%s: %d points from %v, want at most %d from %d days back",
				raw,
				len(out.Points),
				out.Points[0].At,
				budget,
				span,
			)
		}
	}
}

func TestGetCabalValueHistory_servesThreeAndSixMonths(t *testing.T) {
	t.Parallel()
	h := rangeHTTP(ids.CabalIDFrom(ids.Real{}.NewV7()), 1741)
	budget := len(readValue(t, h, "1M").Points)
	for raw, span := range map[string]int{"3M": 90, "6M": 182} {
		out := readValue(t, h, raw)
		if len(out.Points) == 0 || len(out.Points) > budget || !startsAt(span, out.Points[0].At) {
			t.Fatalf(
				"%s: %d points from %v, want at most %d from %d days back",
				raw,
				len(out.Points),
				out.Points[0].At,
				budget,
				span,
			)
		}
	}
}

func TestGetMyPnlHistory_returnsTheRangesGainWithoutTheDeposit(t *testing.T) {
	t.Parallel()
	ctx := asActor(t.Context(), auth.ActorUser, ids.Real{}.NewV7().String())
	cabal := ids.CabalIDFrom(ids.Real{}.NewV7())
	for name, tc := range map[string]struct {
		final uint64
		pnl   int64
		bps   int64
	}{
		"deposit only":     {final: 1500, pnl: 0, bps: 0},
		"deposit and gain": {final: 1741, pnl: 241, bps: 1606},
		"deposit and loss": {final: 1425, pnl: -75, bps: -500},
	} {
		got, err := rangeHTTP(cabal, tc.final).GetMyPnlHistory(ctx, api.GetMyPnlHistoryRequestObject{
			Params: api.GetMyPnlHistoryParams{Range: ptr(api.GetMyPnlHistoryParamsRange("3M"))},
		})
		out, ok := got.(api.GetMyPnlHistory200JSONResponse)
		if err != nil || !ok || out.PnlMicros != tc.pnl || out.ReturnBps != tc.bps {
			t.Errorf(
				"%s: pnl %d, return %d (%v), want %d and %d",
				name,
				out.PnlMicros,
				out.ReturnBps,
				err,
				tc.pnl,
				tc.bps,
			)
		}
	}
}

func TestLeaderboards_stillRefuseThreeAndSixMonths(t *testing.T) {
	t.Parallel()
	ctx := asActor(t.Context(), auth.ActorUser, ids.Real{}.NewV7().String())
	h := historyHTTP(0)
	h.Cabals = cabalCheck(nil)
	for _, raw := range []string{"3M", "6M"} {
		calls := map[string]func() error{
			"people": func() error {
				_, err := h.GetPeopleLeaderboard(ctx, api.GetPeopleLeaderboardRequestObject{
					Params: api.GetPeopleLeaderboardParams{Range: ptr(api.GetPeopleLeaderboardParamsRange(raw))},
				})
				return err
			},
			"cabals": func() error {
				_, err := h.GetCabalsLeaderboard(ctx, api.GetCabalsLeaderboardRequestObject{
					Params: api.GetCabalsLeaderboardParams{Range: ptr(api.GetCabalsLeaderboardParamsRange(raw))},
				})
				return err
			},
			"cabal members": func() error {
				_, err := h.GetCabalLeaderboard(ctx, api.GetCabalLeaderboardRequestObject{
					Id:     ids.Real{}.NewV7(),
					Params: api.GetCabalLeaderboardParams{Range: ptr(api.GetCabalLeaderboardParamsRange(raw))},
				})
				return err
			},
		}
		for name, call := range calls {
			if err := call(); errs.CodeOf(err) != errs.CodeInvalidInput {
				t.Errorf("%s %s: err = %v, want invalid_input", name, raw, err)
			}
		}
	}
}

func TestGetMyPnlHistory_failsWhenTheRangesGainOverflows(t *testing.T) {
	t.Parallel()
	ctx := asActor(t.Context(), auth.ActorUser, ids.Real{}.NewV7().String())
	cabal := ids.CabalIDFrom(ids.Real{}.NewV7())
	at := rangeAt()
	stake := func(from time.Time, net int64) app.StakePoint {
		return app.StakePoint{
			CabalID: cabal, At: from, ShareUnits: money.SharesUnitsFromUint64(1),
			NetContributed: money.SignedMicrosFromInt64(net),
		}
	}
	snap := func(from time.Time, value uint64) domain.Snapshot {
		return domain.Snapshot{
			At: from, Value: money.MicrosFromUint64(value), TotalShares: money.SharesUnitsFromUint64(1),
		}
	}
	start := at.Add(-time.Hour)
	h := historyHTTP(0)
	h.Stakes = stubStakes{points: []app.StakePoint{stake(start, math.MaxInt64), stake(at, 0)}}
	h.Snapshots = fixedSnapshots{series: map[ids.CabalID][]domain.Snapshot{
		cabal: {snap(start, 0), snap(at, math.MaxInt64)},
	}}
	req := api.GetMyPnlHistoryRequestObject{
		Params: api.GetMyPnlHistoryParams{Range: ptr(api.GetMyPnlHistoryParamsRange("1H"))},
	}
	if _, err := h.GetMyPnlHistory(ctx, req); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("err = %v, want invalid_input", err)
	}
}
