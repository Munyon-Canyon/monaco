package domain

import "time"

type Date struct {
	Year  int
	Month time.Month
	Day   int
}

func dateOf(t time.Time) Date {
	year, month, day := t.Date()
	return Date{Year: year, Month: month, Day: day}
}

func (d Date) midnight() time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC)
}

func (d Date) weekday() time.Weekday { return d.midnight().Weekday() }

func (d Date) addDays(n int) Date { return dateOf(d.midnight().AddDate(0, 0, n)) }

func (d Date) String() string { return d.midnight().Format(time.DateOnly) }
