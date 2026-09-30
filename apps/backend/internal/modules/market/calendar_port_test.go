package market_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func preIPOFixture() market.Asset {
	a := marketfake.JPSTx()
	a.Symbol, a.Issuer, a.Kind = "SPACEX", domain.IssuerPreStocks, domain.KindPreIPO
	return a
}

func portSession(t *testing.T, calendar market.Calendar, id market.AssetID, at time.Time) market.SessionInfo {
	t.Helper()
	info, err := calendar.Session(t.Context(), id, at)
	if err != nil {
		t.Fatalf("Session(%s, %s): %v", id, at.Format(time.RFC3339), err)
	}
	return info
}

func requireSundaySessions(t *testing.T, calendar market.Calendar, equity, preIPO market.AssetID) {
	t.Helper()
	sunday := easternTime(t, "2026-10-04 12:00")
	closed := wallExpectation{
		state: domain.StateClosed, next: domain.StatePreMarket, nextAt: "2026-10-05 04:00",
		lastClose: "2026-10-02 16:00", tradingDay: "2026-10-04",
	}
	requireSession(t, portSession(t, calendar, equity, sunday), closed.info(t))
	continuous := domain.SessionInfo{State: domain.StateOpen, Continuous: true, TradingDay: civilDate(t, "2026-10-04")}
	requireSession(t, portSession(t, calendar, preIPO, sunday), continuous)
}

func TestCalendar_Session_followsTheAssetKind(t *testing.T) {
	t.Parallel()
	equity, preIPO := marketfake.AAPLx(), preIPOFixture()
	calendar := app.NewCalendar(marketfake.NewCatalog(equity, preIPO))
	requireSundaySessions(t, calendar, equity.ID, preIPO.ID)
}

func TestCalendar_Session_followsTheInjectedClock(t *testing.T) {
	t.Parallel()
	equity := marketfake.AAPLx()
	calendar := app.NewCalendar(marketfake.NewCatalog(equity))
	clock := testkit.NewClock(easternTime(t, "2026-09-30 09:29"))
	if got := portSession(t, calendar, equity.ID, clock.Now()); got.State != domain.StatePreMarket {
		t.Fatalf("Session at 09:29 = %+v, want pre_market", got)
	}
	clock.Advance(time.Minute)
	if got := portSession(t, calendar, equity.ID, clock.Now()); got.State != domain.StateOpen {
		t.Fatalf("Session at 09:30 = %+v, want open", got)
	}
}

func TestCalendar_Session_unknownAssetIsNotFound(t *testing.T) {
	t.Parallel()
	calendar := app.NewCalendar(marketfake.NewCatalog(marketfake.AAPLx()))
	info, err := calendar.Session(t.Context(), preIPOFixture().ID, easternTime(t, "2026-09-30 10:00"))
	if errs.CodeOf(err) != errs.CodeAssetNotFound {
		t.Fatalf("Session(unknown) = %+v, %v, want asset_not_found", info, err)
	}
}

func TestCalendar_Session_passesOnACatalogFailure(t *testing.T) {
	t.Parallel()
	equity := marketfake.AAPLx()
	catalog := marketfake.NewCatalog(equity)
	catalog.FailOnce("AssetByID", errs.New(errs.CodeDBUnavailable, "test"))
	calendar := app.NewCalendar(catalog)
	at := easternTime(t, "2026-09-30 10:00")
	if _, err := calendar.Session(t.Context(), equity.ID, at); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("Session err = %v, want the catalog's db_unavailable", err)
	}
	if got := portSession(t, calendar, equity.ID, at); got.State != domain.StateOpen {
		t.Fatalf("Session after the fault = %+v, want open", got)
	}
}

func TestCalendar_Session_returnsCalendarExpiredBeyondTheTable(t *testing.T) {
	t.Parallel()
	equity, preIPO := marketfake.AAPLx(), preIPOFixture()
	calendar := app.NewCalendar(marketfake.NewCatalog(equity, preIPO))
	beyond := easternTime(t, "2028-01-03 10:00")
	if _, err := calendar.Session(t.Context(), equity.ID, beyond); errs.CodeOf(err) != errs.CodeCalendarExpired {
		t.Fatalf("Session(equity, 2028) err = %v, want calendar_expired", err)
	}
	if got := portSession(t, calendar, preIPO.ID, beyond); got.State != domain.StateOpen || !got.Continuous {
		t.Fatalf("Session(pre-IPO, 2028) = %+v, want open and continuous", got)
	}
}

func TestModule_Calendar_Session_readsTheKindFromPostgres(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	equity, preIPO := marketfake.AAPLx(), preIPOFixture()
	seededAt := easternTime(t, "2026-09-30 12:00")
	insert(t, pool, stamped(equity, seededAt), nil)
	insert(t, pool, stamped(preIPO, seededAt), nil)
	calendar := market.New(module.Deps{Pool: pool}).Calendar()
	requireSundaySessions(t, calendar, equity.ID, preIPO.ID)
	unknown, err := domain.ParseAssetID("01920000-0000-7000-8000-0000000000ff")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := calendar.Session(t.Context(), unknown, seededAt); errs.CodeOf(err) != errs.CodeAssetNotFound {
		t.Fatalf("Session(unknown) err = %v, want asset_not_found", err)
	}
}
