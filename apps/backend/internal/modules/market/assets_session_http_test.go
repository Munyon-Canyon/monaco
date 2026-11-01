package market_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func TestAssets_afterTheCloseTodayKeepsTheSessionChangeAndAfterHoursGetsItsOwnLine(t *testing.T) {
	t.Parallel()
	monday := time.Date(2026, 3, 9, 21, 0, 0, 0, time.UTC)
	s := newMarketAPI(t, monday)
	s.seedFixtures(t)
	mint := marketfake.AAPLx().Mint.String()
	s.price(t, mint, -72*time.Hour, 188_000_000)
	s.price(t, mint, -time.Hour, 200_000_000)
	s.price(t, mint, -time.Minute, 200_400_000)
	page := pageOf(t, s.get(t, "/v1/assets?q=apple")).Assets
	if len(page) != 1 || page[0].ChangeBps == nil || *page[0].ChangeBps != 659 ||
		page[0].AfterHoursChangeBps == nil || *page[0].AfterHoursChangeBps != 20 {
		t.Fatalf("Monday 17:00 = %+v, want +659 bps today and +20 bps after hours", page)
	}
	detail := detailOf(t, s.get(t, "/v1/assets/AAPLx"))
	if detail.ChangeBps == nil || *detail.ChangeBps != 659 ||
		detail.AfterHoursChangeBps == nil || *detail.AfterHoursChangeBps != 20 {
		t.Fatalf("detail = %+v", detail)
	}
	chart := chartOf(t, s.get(t, "/v1/assets/AAPLx/chart?range=1D"))
	if chart.PreviousCloseMicros == nil || *chart.PreviousCloseMicros != 188_000_000 {
		t.Fatalf("chart previous close = %v, want 188000000", chart.PreviousCloseMicros)
	}
}

func TestAssets_onASaturdayTodayIsFridaysSessionAndTheWeekendDriftIsAfterHours(t *testing.T) {
	t.Parallel()
	saturday := time.Date(2026, 3, 7, 15, 0, 0, 0, time.UTC)
	s := newMarketAPI(t, saturday)
	s.seedFixtures(t)
	mint := marketfake.AAPLx().Mint.String()
	s.price(t, mint, -42*time.Hour, 188_000_000)
	s.price(t, mint, -18*time.Hour, 200_000_000)
	s.price(t, mint, -time.Minute, 200_400_000)
	page := pageOf(t, s.get(t, "/v1/assets?q=apple")).Assets
	if len(page) != 1 || page[0].ChangeBps == nil || *page[0].ChangeBps != 659 ||
		page[0].AfterHoursChangeBps == nil || *page[0].AfterHoursChangeBps != 20 {
		t.Fatalf("Saturday = %+v, want +659 bps today and +20 bps after hours", page)
	}
}

func TestAssets_duringTheSessionTodayIsMeasuredFromYesterdaysCloseWithNoAfterHoursLine(t *testing.T) {
	t.Parallel()
	monday := time.Date(2026, 3, 9, 15, 0, 0, 0, time.UTC)
	s := newMarketAPI(t, monday)
	s.seedFixtures(t)
	mint := marketfake.AAPLx().Mint.String()
	s.price(t, mint, -66*time.Hour, 188_000_000)
	s.price(t, mint, -time.Minute, 197_000_000)
	page := pageOf(t, s.get(t, "/v1/assets?q=apple")).Assets
	if len(page) != 1 || page[0].ChangeBps == nil || *page[0].ChangeBps != 478 || page[0].AfterHoursChangeBps != nil {
		t.Fatalf("open session = %+v, want +478 bps and no after-hours line", page)
	}
}

func TestAssets_onAHolidayTodayIsTheLastSessionAndTheCalendarBeforeItIsTheBase(t *testing.T) {
	t.Parallel()
	holiday := time.Date(2026, 1, 19, 17, 0, 0, 0, time.UTC)
	s := newMarketAPI(t, holiday)
	s.seedFixtures(t)
	mint := marketfake.AAPLx().Mint.String()
	s.price(t, mint, -92*time.Hour, 188_000_000)
	s.price(t, mint, -68*time.Hour, 200_000_000)
	s.price(t, mint, -time.Minute, 199_000_000)
	page := pageOf(t, s.get(t, "/v1/assets?q=apple")).Assets
	if len(page) != 1 || page[0].ChangeBps == nil || *page[0].ChangeBps != 585 ||
		page[0].AfterHoursChangeBps == nil || *page[0].AfterHoursChangeBps != -50 {
		t.Fatalf("holiday = %+v, want +585 bps today and -50 bps after hours", page)
	}
}
