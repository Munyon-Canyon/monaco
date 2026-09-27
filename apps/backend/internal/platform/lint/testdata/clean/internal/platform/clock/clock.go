package clock

import "time"

func Now() time.Time {
	return time.Now()
}

func Seconds(d time.Duration) float64 {
	return d.Seconds()
}
