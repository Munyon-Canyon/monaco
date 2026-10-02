package domain

import "time"

func FirstAcceptedOnDay(samples []Sample, dayStart time.Time) (Sample, bool) {
	var seen []Sample
	for _, s := range samples {
		seen = append(seen, s)
		from := 0
		if extra := len(seen) - HoldWindow; extra > 0 {
			from = extra
		}
		window := seen[from:]
		newest := make([]Sample, len(window))
		for i := range window {
			newest[len(window)-1-i] = window[i]
		}
		accepted, _ := Accept(newest)
		if accepted.ObservedAt.Before(dayStart) {
			continue
		}
		return accepted, true
	}
	return Sample{}, false
}
