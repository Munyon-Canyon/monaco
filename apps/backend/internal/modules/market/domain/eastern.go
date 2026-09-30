package domain

import "time"

const (
	standardOffset = -5 * time.Hour
	daylightOffset = -4 * time.Hour
	changeover     = 2 * time.Hour
)

func nthSunday(year int, month time.Month, n int) time.Time {
	first := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	toSunday := (7 - int(first.Weekday())) % 7
	return first.AddDate(0, 0, toSunday+7*(n-1))
}

func easternOffset(at time.Time) time.Duration {
	year := at.UTC().Year()
	daylightStarts := nthSunday(year, time.March, 2).Add(changeover - standardOffset)
	daylightEnds := nthSunday(year, time.November, 1).Add(changeover - daylightOffset)
	if !at.Before(daylightStarts) && at.Before(daylightEnds) {
		return daylightOffset
	}
	return standardOffset
}

func wallClock(at time.Time) time.Time {
	utc := at.UTC()
	return utc.Add(easternOffset(utc))
}

func instantOf(wall time.Time) time.Time {
	return wall.Add(-easternOffset(wall.Add(-standardOffset)))
}

func (d Date) atExchange(offset time.Duration) time.Time {
	return instantOf(d.midnight().Add(offset))
}
