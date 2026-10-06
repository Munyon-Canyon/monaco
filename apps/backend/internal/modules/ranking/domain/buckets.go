package domain

import (
	"slices"
	"sort"
	"time"
)

func BucketWidth(r Range) time.Duration {
	switch r {
	case Range1H:
		return 2 * time.Minute
	case Range1D:
		return 10 * time.Minute
	case Range1W:
		return time.Hour
	case Range1M:
		return 4 * time.Hour
	case RangeAll:
	}
	return 24 * time.Hour
}

func BucketTimes(r Range, now, first time.Time) []time.Time {
	start, ok := r.Start(now)
	if !ok {
		if r != RangeAll {
			return []time.Time{}
		}
		start = first
	}
	width := BucketWidth(r)
	if now.Before(start) {
		return []time.Time{}
	}
	count := int(now.Sub(start)/width) + 1
	out := make([]time.Time, count)
	for i := range out {
		out[count-1-i] = now.Add(-time.Duration(i) * width)
	}
	if first.After(start) {
		start = first
	}
	return withLead(out, start, now)
}

func withLead(out []time.Time, lead, now time.Time) []time.Time {
	i := sort.Search(len(out), func(i int) bool { return !out[i].Before(lead) })
	if lead.After(now) || (i < len(out) && out[i].Equal(lead)) {
		return out
	}
	return slices.Insert(out, i, lead)
}

func LastAtOrBefore[T any](points []T, t time.Time, at func(T) time.Time) *T {
	i := sort.Search(len(points), func(i int) bool { return at(points[i]).After(t) })
	if i == 0 {
		return nil
	}
	return &points[i-1]
}
