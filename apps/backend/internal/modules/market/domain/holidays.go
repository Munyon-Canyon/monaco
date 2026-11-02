package domain

import "time"

type exception struct {
	month time.Month
	day   int
	name  string
	early bool
}

func fullClosure(month time.Month, day int, name string) exception {
	return exception{month: month, day: day, name: name}
}

func earlyClose(month time.Month, day int, name string) exception {
	return exception{month: month, day: day, name: name, early: true}
}

func (e exception) matches(date Date) bool { return e.month == date.Month && e.day == date.Day }

func (e exception) schedule() schedule {
	if e.early {
		return schedule{windows: earlyCloseHours(), early: true}
	}
	return schedule{holiday: e.name}
}

func nyseHolidays() map[int][]exception {
	return map[int][]exception{
		2026: {
			fullClosure(time.January, 1, "New Year's Day"),
			fullClosure(time.January, 19, "Martin Luther King, Jr. Day"),
			fullClosure(time.February, 16, "Washington's Birthday"),
			fullClosure(time.April, 3, "Good Friday"),
			fullClosure(time.May, 25, "Memorial Day"),
			fullClosure(time.June, 19, "Juneteenth National Independence Day"),
			fullClosure(time.July, 3, "Independence Day (observed)"),
			fullClosure(time.September, 7, "Labor Day"),
			fullClosure(time.November, 26, "Thanksgiving Day"),
			earlyClose(time.November, 27, "Day after Thanksgiving"),
			earlyClose(time.December, 24, "Christmas Eve"),
			fullClosure(time.December, 25, "Christmas Day"),
		},
		2027: {
			fullClosure(time.January, 1, "New Year's Day"),
			fullClosure(time.January, 18, "Martin Luther King, Jr. Day"),
			fullClosure(time.February, 15, "Washington's Birthday"),
			fullClosure(time.March, 26, "Good Friday"),
			fullClosure(time.May, 31, "Memorial Day"),
			fullClosure(time.June, 18, "Juneteenth National Independence Day (observed)"),
			fullClosure(time.July, 5, "Independence Day (observed)"),
			fullClosure(time.September, 6, "Labor Day"),
			fullClosure(time.November, 25, "Thanksgiving Day"),
			earlyClose(time.November, 26, "Day after Thanksgiving"),
			fullClosure(time.December, 24, "Christmas Day (observed)"),
		},
		2028: {
			fullClosure(time.January, 17, "Martin Luther King, Jr. Day"),
			fullClosure(time.February, 21, "Washington's Birthday"),
			fullClosure(time.April, 14, "Good Friday"),
			fullClosure(time.May, 29, "Memorial Day"),
			fullClosure(time.June, 19, "Juneteenth National Independence Day"),
			earlyClose(time.July, 3, "Day before Independence Day"),
			fullClosure(time.July, 4, "Independence Day"),
			fullClosure(time.September, 4, "Labor Day"),
			fullClosure(time.November, 23, "Thanksgiving Day"),
			earlyClose(time.November, 24, "Day after Thanksgiving"),
			fullClosure(time.December, 25, "Christmas Day"),
		},
		2029: {
			fullClosure(time.January, 1, "New Year's Day"),
			fullClosure(time.January, 15, "Martin Luther King, Jr. Day"),
			fullClosure(time.February, 19, "Washington's Birthday"),
			fullClosure(time.March, 30, "Good Friday"),
			fullClosure(time.May, 28, "Memorial Day"),
			fullClosure(time.June, 19, "Juneteenth National Independence Day"),
			earlyClose(time.July, 3, "Day before Independence Day"),
			fullClosure(time.July, 4, "Independence Day"),
			fullClosure(time.September, 3, "Labor Day"),
			fullClosure(time.November, 22, "Thanksgiving Day"),
			earlyClose(time.November, 23, "Day after Thanksgiving"),
			earlyClose(time.December, 24, "Christmas Eve"),
			fullClosure(time.December, 25, "Christmas Day"),
		},
	}
}

const CalendarWarning = 90 * 24 * time.Hour

func CalendarEnd() time.Time {
	last := 0
	for year := range nyseHolidays() {
		last = max(last, year)
	}
	return Date{Year: last + 1, Month: time.January, Day: 1}.Start()
}

func CalendarEnding(now time.Time) (time.Time, bool) {
	end := CalendarEnd()
	return end, !now.Before(end.Add(-CalendarWarning))
}
