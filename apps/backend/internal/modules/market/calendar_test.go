package market_test

import (
	"maps"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
)

const wallLayout = "2006-01-02 15:04"

func newYork(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func easternTime(t *testing.T, wall string) time.Time {
	t.Helper()
	instant, err := time.ParseInLocation(wallLayout, wall, newYork(t))
	if err != nil {
		t.Fatal(err)
	}
	return instant
}

func civilDate(t *testing.T, iso string) domain.Date {
	t.Helper()
	day, err := time.Parse(time.DateOnly, iso)
	if err != nil {
		t.Fatal(err)
	}
	return domain.Date{Year: day.Year(), Month: day.Month(), Day: day.Day()}
}

func utc(month time.Month, day, hour, minute int) time.Time {
	return time.Date(2026, month, day, hour, minute, 0, 0, time.UTC)
}

func requireSession(t *testing.T, got, want domain.SessionInfo) {
	t.Helper()
	if got.State != want.State || got.Continuous != want.Continuous || got.Holiday != want.Holiday ||
		got.EarlyClose != want.EarlyClose || got.NextState != want.NextState || got.TradingDay != want.TradingDay ||
		!got.NextTransition.Equal(want.NextTransition) || !got.LastClose.Equal(want.LastClose) {
		t.Fatalf("Session = %+v, want %+v", got, want)
	}
}

func equitySession(t *testing.T, at time.Time) domain.SessionInfo {
	t.Helper()
	info, err := domain.Session(domain.KindEquity, at)
	if err != nil {
		t.Fatalf("Session(equity, %s): %v", at.Format(time.RFC3339), err)
	}
	return info
}

type wallExpectation struct {
	state      domain.State
	next       domain.State
	nextAt     string
	lastClose  string
	tradingDay string
	holiday    string
	early      bool
}

func (e wallExpectation) info(t *testing.T) domain.SessionInfo {
	t.Helper()
	return domain.SessionInfo{
		State:          e.state,
		Holiday:        e.holiday,
		EarlyClose:     e.early,
		NextState:      e.next,
		NextTransition: easternTime(t, e.nextAt),
		LastClose:      easternTime(t, e.lastClose),
		TradingDay:     civilDate(t, e.tradingDay),
	}
}

type wallCase struct {
	at   string
	want wallExpectation
}

func runWallCases(t *testing.T, cases []wallCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.at, func(t *testing.T) {
			t.Parallel()
			requireSession(t, equitySession(t, easternTime(t, tc.at)), tc.want.info(t))
		})
	}
}

func TestSession_PreMarket(t *testing.T) {
	t.Parallel()
	pre := wallExpectation{
		state: domain.StatePreMarket, next: domain.StateOpen, nextAt: "2026-09-30 09:30",
		lastClose: "2026-09-29 16:00", tradingDay: "2026-09-30",
	}
	runWallCases(t, []wallCase{
		{at: "2026-09-30 04:00", want: pre},
		{at: "2026-09-30 09:29", want: pre},
	})
}

func TestSession_RegularOpen(t *testing.T) {
	t.Parallel()
	regular := wallExpectation{
		state: domain.StateOpen, next: domain.StateAfterHours, nextAt: "2026-09-30 16:00",
		lastClose: "2026-09-29 16:00", tradingDay: "2026-09-30",
	}
	runWallCases(t, []wallCase{
		{at: "2026-09-30 09:30", want: regular},
		{at: "2026-09-30 15:59", want: regular},
	})
}

func TestSession_AfterHours(t *testing.T) {
	t.Parallel()
	after := wallExpectation{
		state: domain.StateAfterHours, next: domain.StateClosed, nextAt: "2026-09-30 20:00",
		lastClose: "2026-09-30 16:00", tradingDay: "2026-09-30",
	}
	runWallCases(t, []wallCase{
		{at: "2026-09-30 16:00", want: after},
		{at: "2026-09-30 19:59", want: after},
	})
}

func TestSession_ClosedOvernight(t *testing.T) {
	t.Parallel()
	beforeDawn := wallExpectation{
		state: domain.StateClosed, next: domain.StatePreMarket, nextAt: "2026-09-30 04:00",
		lastClose: "2026-09-29 16:00", tradingDay: "2026-09-30",
	}
	afterDark := wallExpectation{
		state: domain.StateClosed, next: domain.StatePreMarket, nextAt: "2026-10-01 04:00",
		lastClose: "2026-09-30 16:00", tradingDay: "2026-09-30",
	}
	runWallCases(t, []wallCase{
		{at: "2026-09-30 00:00", want: beforeDawn},
		{at: "2026-09-30 03:59", want: beforeDawn},
		{at: "2026-09-30 20:00", want: afterDark},
		{at: "2026-09-30 23:59", want: afterDark},
	})
}

func TestSession_Weekend(t *testing.T) {
	t.Parallel()
	runWallCases(t, []wallCase{
		{at: "2026-10-03 12:00", want: wallExpectation{
			state: domain.StateClosed, next: domain.StatePreMarket, nextAt: "2026-10-05 04:00",
			lastClose: "2026-10-02 16:00", tradingDay: "2026-10-03",
		}},
		{at: "2026-10-04 23:59", want: wallExpectation{
			state: domain.StateClosed, next: domain.StatePreMarket, nextAt: "2026-10-05 04:00",
			lastClose: "2026-10-02 16:00", tradingDay: "2026-10-04",
		}},
		{at: "2026-10-02 20:00", want: wallExpectation{
			state: domain.StateClosed, next: domain.StatePreMarket, nextAt: "2026-10-05 04:00",
			lastClose: "2026-10-02 16:00", tradingDay: "2026-10-02",
		}},
	})
}

func TestSession_Holiday(t *testing.T) {
	t.Parallel()
	thanksgiving := wallExpectation{
		state: domain.StateClosed, next: domain.StatePreMarket, nextAt: "2026-11-27 04:00",
		lastClose: "2026-11-25 16:00", tradingDay: "2026-11-26", holiday: "Thanksgiving Day",
	}
	runWallCases(t, []wallCase{
		{at: "2026-11-26 04:00", want: thanksgiving},
		{at: "2026-11-26 09:30", want: thanksgiving},
		{at: "2026-11-26 16:00", want: thanksgiving},
		{at: "2026-01-19 12:00", want: wallExpectation{
			state: domain.StateClosed, next: domain.StatePreMarket, nextAt: "2026-01-20 04:00",
			lastClose: "2026-01-16 16:00", tradingDay: "2026-01-19", holiday: "Martin Luther King, Jr. Day",
		}},
		{at: "2026-07-03 12:00", want: wallExpectation{
			state: domain.StateClosed, next: domain.StatePreMarket, nextAt: "2026-07-06 04:00",
			lastClose: "2026-07-02 16:00", tradingDay: "2026-07-03", holiday: "Independence Day (observed)",
		}},
		{at: "2026-07-04 12:00", want: wallExpectation{
			state: domain.StateClosed, next: domain.StatePreMarket, nextAt: "2026-07-06 04:00",
			lastClose: "2026-07-02 16:00", tradingDay: "2026-07-04",
		}},
		{at: "2027-12-24 12:00", want: wallExpectation{
			state: domain.StateClosed, next: domain.StatePreMarket, nextAt: "2027-12-27 04:00",
			lastClose: "2027-12-23 16:00", tradingDay: "2027-12-24", holiday: "Christmas Day (observed)",
		}},
		{at: "2028-07-04 12:00", want: wallExpectation{
			state: domain.StateClosed, next: domain.StatePreMarket, nextAt: "2028-07-05 04:00",
			lastClose: "2028-07-03 13:00", tradingDay: "2028-07-04", holiday: "Independence Day",
		}},
	})
}

func TestSession_EarlyClose(t *testing.T) {
	t.Parallel()
	dayAfterThanksgiving := func(state, next domain.State, nextAt, lastClose string) wallExpectation {
		return wallExpectation{
			state: state, next: next, nextAt: nextAt, lastClose: lastClose,
			tradingDay: "2026-11-27", early: true,
		}
	}
	runWallCases(t, []wallCase{
		{at: "2026-11-27 04:00", want: dayAfterThanksgiving(
			domain.StatePreMarket, domain.StateOpen, "2026-11-27 09:30", "2026-11-25 16:00")},
		{at: "2026-11-27 12:59", want: dayAfterThanksgiving(
			domain.StateOpen, domain.StateAfterHours, "2026-11-27 13:00", "2026-11-25 16:00")},
		{at: "2026-11-27 13:00", want: dayAfterThanksgiving(
			domain.StateAfterHours, domain.StateClosed, "2026-11-27 17:00", "2026-11-27 13:00")},
		{at: "2026-11-27 16:59", want: dayAfterThanksgiving(
			domain.StateAfterHours, domain.StateClosed, "2026-11-27 17:00", "2026-11-27 13:00")},
		{at: "2026-11-27 17:00", want: dayAfterThanksgiving(
			domain.StateClosed, domain.StatePreMarket, "2026-11-30 04:00", "2026-11-27 13:00")},
		{at: "2026-12-24 12:59", want: wallExpectation{
			state: domain.StateOpen, next: domain.StateAfterHours, nextAt: "2026-12-24 13:00",
			lastClose: "2026-12-23 16:00", tradingDay: "2026-12-24", early: true,
		}},
		{at: "2026-12-24 13:00", want: wallExpectation{
			state: domain.StateAfterHours, next: domain.StateClosed, nextAt: "2026-12-24 17:00",
			lastClose: "2026-12-24 13:00", tradingDay: "2026-12-24", early: true,
		}},
		{at: "2028-11-24 12:00", want: wallExpectation{
			state: domain.StateOpen, next: domain.StateAfterHours, nextAt: "2028-11-24 13:00",
			lastClose: "2028-11-22 16:00", tradingDay: "2028-11-24", early: true,
		}},
	})
}

func TestSession_LastCloseOnMonday(t *testing.T) {
	t.Parallel()
	fridayClose := func(state, next domain.State, nextAt string) wallExpectation {
		return wallExpectation{
			state: state, next: next, nextAt: nextAt, lastClose: "2026-09-25 16:00", tradingDay: "2026-09-28",
		}
	}
	runWallCases(t, []wallCase{
		{at: "2026-09-28 08:00", want: fridayClose(domain.StatePreMarket, domain.StateOpen, "2026-09-28 09:30")},
		{at: "2026-09-28 10:00", want: fridayClose(domain.StateOpen, domain.StateAfterHours, "2026-09-28 16:00")},
		{at: "2026-09-28 16:30", want: wallExpectation{
			state: domain.StateAfterHours, next: domain.StateClosed, nextAt: "2026-09-28 20:00",
			lastClose: "2026-09-28 16:00", tradingDay: "2026-09-28",
		}},
		{at: "2026-01-20 08:00", want: wallExpectation{
			state: domain.StatePreMarket, next: domain.StateOpen, nextAt: "2026-01-20 09:30",
			lastClose: "2026-01-16 16:00", tradingDay: "2026-01-20",
		}},
	})
}

type utcCase struct {
	name string
	at   time.Time
	want domain.SessionInfo
}

func TestSession_DSTBoundary(t *testing.T) {
	t.Parallel()
	closedOnSunday := func(at time.Time, nextOpening, lastClose time.Time, day int, month time.Month) utcCase {
		return utcCase{
			name: at.Format(time.RFC3339),
			at:   at,
			want: domain.SessionInfo{
				State: domain.StateClosed, NextState: domain.StatePreMarket, NextTransition: nextOpening,
				LastClose:  lastClose,
				TradingDay: domain.Date{Year: 2026, Month: month, Day: day},
			},
		}
	}
	springClose := utc(time.March, 6, 21, 0)
	autumnClose := utc(time.October, 30, 20, 0)
	cases := []utcCase{
		{
			name: "friday before the clocks go forward",
			at:   utc(time.March, 6, 20, 59),
			want: domain.SessionInfo{
				State: domain.StateOpen, NextState: domain.StateAfterHours, NextTransition: springClose,
				LastClose:  utc(time.March, 5, 21, 0),
				TradingDay: domain.Date{Year: 2026, Month: time.March, Day: 6},
			},
		},
		closedOnSunday(utc(time.March, 8, 6, 59), utc(time.March, 9, 8, 0), springClose, 8, time.March),
		closedOnSunday(utc(time.March, 8, 7, 0), utc(time.March, 9, 8, 0), springClose, 8, time.March),
		{
			name: "monday pre-market in daylight time",
			at:   utc(time.March, 9, 13, 29),
			want: domain.SessionInfo{
				State: domain.StatePreMarket, NextState: domain.StateOpen, NextTransition: utc(time.March, 9, 13, 30),
				LastClose:  springClose,
				TradingDay: domain.Date{Year: 2026, Month: time.March, Day: 9},
			},
		},
		{
			name: "monday open in daylight time",
			at:   utc(time.March, 9, 13, 30),
			want: domain.SessionInfo{
				State: domain.StateOpen, NextState: domain.StateAfterHours, NextTransition: utc(time.March, 9, 20, 0),
				LastClose:  springClose,
				TradingDay: domain.Date{Year: 2026, Month: time.March, Day: 9},
			},
		},
		{
			name: "friday close before the clocks go back",
			at:   autumnClose,
			want: domain.SessionInfo{
				State:          domain.StateAfterHours,
				NextState:      domain.StateClosed,
				NextTransition: utc(time.October, 31, 0, 0),
				LastClose:      autumnClose,
				TradingDay:     domain.Date{Year: 2026, Month: time.October, Day: 30},
			},
		},
		closedOnSunday(utc(time.November, 1, 5, 59), utc(time.November, 2, 9, 0), autumnClose, 1, time.November),
		closedOnSunday(utc(time.November, 1, 7, 0), utc(time.November, 2, 9, 0), autumnClose, 1, time.November),
		{
			name: "saturday just before midnight in daylight time",
			at:   utc(time.November, 1, 3, 59),
			want: domain.SessionInfo{
				State:          domain.StateClosed,
				NextState:      domain.StatePreMarket,
				NextTransition: utc(time.November, 2, 9, 0),
				LastClose:      autumnClose,
				TradingDay:     domain.Date{Year: 2026, Month: time.October, Day: 31},
			},
		},
		{
			name: "monday pre-market in standard time",
			at:   utc(time.November, 2, 14, 29),
			want: domain.SessionInfo{
				State:          domain.StatePreMarket,
				NextState:      domain.StateOpen,
				NextTransition: utc(time.November, 2, 14, 30),
				LastClose:      autumnClose,
				TradingDay:     domain.Date{Year: 2026, Month: time.November, Day: 2},
			},
		},
		{
			name: "monday open in standard time",
			at:   utc(time.November, 2, 14, 30),
			want: domain.SessionInfo{
				State:          domain.StateOpen,
				NextState:      domain.StateAfterHours,
				NextTransition: utc(time.November, 2, 21, 0),
				LastClose:      autumnClose,
				TradingDay:     domain.Date{Year: 2026, Month: time.November, Day: 2},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			requireSession(t, equitySession(t, tc.at), tc.want)
		})
	}
}

func TestSession_PreIPOAlwaysOpen(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		at         string
		tradingDay string
	}{
		"sunday noon":                     {at: "2026-10-04 12:00", tradingDay: "2026-10-04"},
		"sunday evening is monday in UTC": {at: "2026-10-04 21:00", tradingDay: "2026-10-05"},
		"thanksgiving":                    {at: "2026-11-26 10:00", tradingDay: "2026-11-26"},
		"overnight":                       {at: "2026-09-30 02:00", tradingDay: "2026-09-30"},
		"after the equity table ends":     {at: "2030-06-03 10:00", tradingDay: "2030-06-03"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := domain.Session(domain.KindPreIPO, easternTime(t, tc.at))
			if err != nil {
				t.Fatal(err)
			}
			want := domain.SessionInfo{
				State:      domain.StateOpen,
				Continuous: true,
				TradingDay: civilDate(t, tc.tradingDay),
			}
			requireSession(t, got, want)
		})
	}
}

func TestSession_BeyondTable(t *testing.T) {
	t.Parallel()
	requireExpired(t, "2028-12-29 20:00", "2028-12-31 20:00", "2029-01-02 10:00", "2030-06-03 10:00")
	if errs.KindOf(errs.CodeCalendarExpired) != errs.KindInternal || !errs.Alert(errs.CodeCalendarExpired) {
		t.Fatal("calendar_expired must be an internal error that alerts, so a missed yearly update pages")
	}
}

func TestSession_BeforeTheTableStarts(t *testing.T) {
	t.Parallel()
	requireExpired(t, "2025-06-02 10:00", "2026-01-01 12:00", "2026-01-02 12:00")
}

func requireExpired(t *testing.T, walls ...string) {
	t.Helper()
	for _, wall := range walls {
		t.Run(wall, func(t *testing.T) {
			t.Parallel()
			info, err := domain.Session(domain.KindEquity, easternTime(t, wall))
			if errs.CodeOf(err) != errs.CodeCalendarExpired {
				t.Fatalf("Session(%s) = %+v, %v, want calendar_expired", wall, info, err)
			}
		})
	}
}

func TestSession_LastDayOfTheTableStillAnswers(t *testing.T) {
	t.Parallel()
	runWallCases(t, []wallCase{
		{at: "2028-12-29 12:00", want: wallExpectation{
			state: domain.StateOpen, next: domain.StateAfterHours, nextAt: "2028-12-29 16:00",
			lastClose: "2028-12-28 16:00", tradingDay: "2028-12-29",
		}},
		{at: "2028-12-29 19:59", want: wallExpectation{
			state: domain.StateAfterHours, next: domain.StateClosed, nextAt: "2028-12-29 20:00",
			lastClose: "2028-12-29 16:00", tradingDay: "2028-12-29",
		}},
	})
}

func TestSession_UnknownKind(t *testing.T) {
	t.Parallel()
	_, err := domain.Session(domain.Kind("bond"), easternTime(t, "2026-09-30 10:00"))
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("Session(bond) err = %v, want invalid_input", err)
	}
}

func TestSession_TradingDayOfAnEquityIsTheNewYorkDate(t *testing.T) {
	t.Parallel()
	info := equitySession(t, easternTime(t, "2026-09-30 21:00"))
	if info.TradingDay != civilDate(t, "2026-09-30") || info.TradingDay.String() != "2026-09-30" {
		t.Fatalf("TradingDay = %v, want 2026-09-30, the New York date and not the UTC date", info.TradingDay)
	}
}

func nthWeekday(year int, month time.Month, weekday time.Weekday, n int) time.Time {
	first := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	shift := (int(weekday) - int(first.Weekday()) + 7) % 7
	return first.AddDate(0, 0, shift+7*(n-1))
}

func lastWeekday(year int, month time.Month, weekday time.Weekday) time.Time {
	last := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC)
	return last.AddDate(0, 0, -((int(last.Weekday()) - int(weekday) + 7) % 7))
}

func goodFriday(year int) time.Time {
	a := year % 19
	b := year / 100
	c := year % 100
	d := b / 4
	e := b % 4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i := c / 4
	k := c % 4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := (h + l - 7*m + 114) / 31
	day := (h+l-7*m+114)%31 + 1
	return time.Date(year, time.Month(month), day-2, 0, 0, 0, 0, time.UTC)
}

func observed(day time.Time) time.Time {
	if day.Weekday() == time.Saturday {
		return day.AddDate(0, 0, -1)
	}
	if day.Weekday() == time.Sunday {
		return day.AddDate(0, 0, 1)
	}
	return day
}

func mondayToThursday(day time.Time) bool {
	return day.Weekday() >= time.Monday && day.Weekday() <= time.Thursday
}

func nyseRuleDays(year int) map[string]string {
	days := map[string]string{}
	mark := func(day time.Time, kind string) { days[day.Format(time.DateOnly)] = kind }
	fixed := func(month time.Month, day int) time.Time { return time.Date(year, month, day, 0, 0, 0, 0, time.UTC) }
	thanksgiving := nthWeekday(year, time.November, time.Thursday, 4)
	mark(fixed(time.January, 1), "closed")
	mark(nthWeekday(year, time.January, time.Monday, 3), "closed")
	mark(nthWeekday(year, time.February, time.Monday, 3), "closed")
	mark(goodFriday(year), "closed")
	mark(lastWeekday(year, time.May, time.Monday), "closed")
	mark(observed(fixed(time.June, 19)), "closed")
	mark(observed(fixed(time.July, 4)), "closed")
	mark(nthWeekday(year, time.September, time.Monday, 1), "closed")
	mark(thanksgiving, "closed")
	mark(observed(fixed(time.December, 25)), "closed")
	mark(thanksgiving.AddDate(0, 0, 1), "early")
	for _, eve := range []time.Time{fixed(time.July, 3), fixed(time.December, 24)} {
		if mondayToThursday(eve) {
			mark(eve, "early")
		}
	}
	return days
}

func TestHolidayTable_MatchesTheNYSERules(t *testing.T) {
	t.Parallel()
	ny := newYork(t)
	want := map[string]string{}
	for _, year := range []int{2026, 2027, 2028} {
		maps.Copy(want, nyseRuleDays(year))
	}
	delete(want, "2026-01-01")
	delete(want, "2028-01-01")
	checked := 0
	end := time.Date(2028, time.December, 30, 0, 0, 0, 0, time.UTC)
	for day := time.Date(2026, time.January, 3, 0, 0, 0, 0, time.UTC); day.Before(end); day = day.AddDate(0, 0, 1) {
		info := equitySession(t, time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, ny))
		got := ""
		if info.Holiday != "" {
			got = "closed"
		}
		if info.EarlyClose {
			got = "early"
		}
		if iso := day.Format(time.DateOnly); got != want[iso] {
			t.Errorf("%s: the table says %q, the NYSE rules say %q", iso, got, want[iso])
		}
		checked++
	}
	if checked != 1092 || len(want) != 33 {
		t.Fatalf("checked %d days against %d rule days, want 1092 and 33", checked, len(want))
	}
}

type boundary struct {
	at    time.Time
	state domain.State
}

func boundaryChain(t *testing.T, ny *time.Location) []boundary {
	t.Helper()
	var chain []boundary
	lastDay := time.Date(2028, time.December, 29, 0, 0, 0, 0, time.UTC)
	for day := time.Date(2026, time.January, 5, 0, 0, 0, 0, time.UTC); day.Before(lastDay); day = day.AddDate(0, 0, 1) {
		at := func(hour, minute int) time.Time {
			return time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, ny)
		}
		noon := equitySession(t, at(12, 0))
		if noon.State != domain.StateOpen {
			continue
		}
		closeHour, endHour := 16, 20
		if noon.EarlyClose {
			closeHour, endHour = 13, 17
		}
		chain = append(chain,
			boundary{at: at(4, 0), state: domain.StatePreMarket},
			boundary{at: at(9, 30), state: domain.StateOpen},
			boundary{at: at(closeHour, 0), state: domain.StateAfterHours},
			boundary{at: at(endHour, 0), state: domain.StateClosed},
		)
	}
	return chain
}

func requireStateChange(t *testing.T, previous, b boundary) {
	t.Helper()
	minuteBefore := equitySession(t, b.at.Add(-time.Minute))
	if minuteBefore.State != previous.state || minuteBefore.NextState != b.state ||
		!minuteBefore.NextTransition.Equal(b.at) {
		t.Fatalf("a minute before %s: %+v, want %s until then", b.at, minuteBefore, previous.state)
	}
}

func requireBoundaryDay(t *testing.T, ny *time.Location, b boundary, lastClose time.Time) domain.SessionInfo {
	t.Helper()
	local := b.at.In(ny)
	got := equitySession(t, b.at)
	if got.State != b.state || !got.LastClose.Equal(lastClose) ||
		got.TradingDay != (domain.Date{Year: local.Year(), Month: local.Month(), Day: local.Day()}) {
		t.Fatalf("at %s: %+v, want %s, last close %s and trading day %s", b.at, got, b.state, lastClose, local)
	}
	return got
}

func TestSession_FollowsTheNewYorkClockAcrossTheTable(t *testing.T) {
	t.Parallel()
	ny := newYork(t)
	chain := boundaryChain(t, ny)
	if len(chain) != 3004 {
		t.Fatalf("chain has %d boundaries, want 3004: 751 trading days with four boundaries each", len(chain))
	}
	previous := boundary{state: domain.StateClosed}
	lastClose := time.Date(2026, time.January, 2, 16, 0, 0, 0, ny)
	for i, b := range chain {
		if b.state == domain.StateAfterHours {
			lastClose = b.at
		}
		requireStateChange(t, previous, b)
		got := requireBoundaryDay(t, ny, b, lastClose)
		if i+1 < len(chain) {
			requireNextBoundary(t, got, chain[i+1])
		}
		previous = b
	}
}

func requireNextBoundary(t *testing.T, got domain.SessionInfo, next boundary) {
	t.Helper()
	if got.NextState != next.state || !got.NextTransition.Equal(next.at) {
		t.Fatalf("%+v, want next %s at %s", got, next.state, next.at)
	}
}
