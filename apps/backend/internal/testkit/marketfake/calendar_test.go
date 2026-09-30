package marketfake_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func TestCalendarFake_returnsTheStateSetForEachAsset(t *testing.T) {
	t.Parallel()
	f := marketfake.NewCalendar()
	aapl, tsla := marketfake.AAPLx(), marketfake.TSLAx()
	closed := market.SessionInfo{
		State:     domain.StateClosed,
		LastClose: time.Date(2026, time.October, 2, 20, 0, 0, 0, time.UTC),
	}
	open := market.SessionInfo{State: domain.StateOpen, Continuous: true}
	f.SetState(aapl.ID, closed)
	f.SetState(tsla.ID, open)
	for _, at := range []time.Time{{}, time.Date(2026, time.October, 5, 14, 0, 0, 0, time.UTC)} {
		if got, err := f.Session(t.Context(), aapl.ID, at); err != nil || got != closed {
			t.Fatalf("Session(AAPLx, %v) = %+v, %v, want the closed state whatever the time", at, got, err)
		}
		if got, err := f.Session(t.Context(), tsla.ID, at); err != nil || got != open {
			t.Fatalf("Session(TSLAx, %v) = %+v, %v, want the open state", at, got, err)
		}
	}
	f.SetState(aapl.ID, open)
	if got, err := f.Session(t.Context(), aapl.ID, time.Time{}); err != nil || got != open {
		t.Fatalf("Session(AAPLx) after SetState = %+v, %v, want the replacing state", got, err)
	}
}

func TestCalendarFake_failsForAnAssetWithoutAStateAndWhereFaultsSay(t *testing.T) {
	t.Parallel()
	f := marketfake.NewCalendar()
	aapl := marketfake.AAPLx()
	if _, err := f.Session(t.Context(), aapl.ID, time.Time{}); errs.CodeOf(err) != errs.CodeAssetNotFound {
		t.Fatalf("Session(unset asset) err = %v, want asset_not_found", err)
	}
	f.SetState(aapl.ID, market.SessionInfo{State: domain.StateOpen})
	f.FailOnce("Session", errs.New(errs.CodeCalendarExpired, "test"))
	if _, err := f.Session(t.Context(), aapl.ID, time.Time{}); errs.CodeOf(err) != errs.CodeCalendarExpired {
		t.Fatalf("Session err = %v, want the scripted fault", err)
	}
	if got, err := f.Session(t.Context(), aapl.ID, time.Time{}); err != nil || got.State != domain.StateOpen {
		t.Fatalf("Session after the fault = %+v, %v, want the set state", got, err)
	}
}
