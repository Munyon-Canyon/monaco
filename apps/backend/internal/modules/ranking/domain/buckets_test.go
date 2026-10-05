package domain_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
)

type snap struct {
	at    time.Time
	value int
}

func snapAt(s snap) time.Time { return s.at }

func bucketNowAt() time.Time { return time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC) }

func TestBucketWidth(t *testing.T) {
	t.Parallel()
	tests := map[domain.Range]time.Duration{
		domain.Range1H:  2 * time.Minute,
		domain.Range1D:  10 * time.Minute,
		domain.Range1W:  time.Hour,
		domain.Range1M:  4 * time.Hour,
		domain.RangeAll: 24 * time.Hour,
	}
	for r, want := range tests {
		if got := domain.BucketWidth(r); got != want {
			t.Fatalf("BucketWidth(%s) = %v, want %v", r, got, want)
		}
	}
}

func assertEvenlySpaced(t *testing.T, got []time.Time, width time.Duration) {
	t.Helper()
	for i := 1; i < len(got); i++ {
		if got[i].Sub(got[i-1]) != width {
			t.Fatalf("gap at %d = %v, want %v", i, got[i].Sub(got[i-1]), width)
		}
	}
}

func TestBucketTimes(t *testing.T) {
	t.Parallel()
	now := bucketNowAt()
	tests := map[string]struct {
		r     domain.Range
		first time.Time
		count int
		start time.Time
	}{
		"1H":                {r: domain.Range1H, count: 31, start: now.Add(-time.Hour)},
		"1D":                {r: domain.Range1D, count: 145, start: now.Add(-24 * time.Hour)},
		"1W":                {r: domain.Range1W, count: 169, start: now.Add(-7 * 24 * time.Hour)},
		"1M":                {r: domain.Range1M, count: 181, start: now.Add(-30 * 24 * time.Hour)},
		"ALL from first":    {r: domain.RangeAll, first: now.Add(-10 * 24 * time.Hour), count: 11},
		"ALL young cabal":   {r: domain.RangeAll, first: now.Add(-time.Hour), count: 1},
		"ALL partial width": {r: domain.RangeAll, first: now.Add(-36 * time.Hour), count: 2},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := domain.BucketTimes(tt.r, now, tt.first)
			if len(got) != tt.count || !got[len(got)-1].Equal(now) {
				t.Fatalf("BucketTimes = %v, want %d ending at now", got, tt.count)
			}
			if !tt.start.IsZero() && !got[0].Equal(tt.start) {
				t.Fatalf("first = %v, want %v", got[0], tt.start)
			}
			assertEvenlySpaced(t, got, domain.BucketWidth(tt.r))
		})
	}
}

func TestBucketTimes_EmptyCases(t *testing.T) {
	t.Parallel()
	now := bucketNowAt()
	for name, got := range map[string][]time.Time{
		"ALL future first": domain.BucketTimes(domain.RangeAll, now, now.Add(time.Hour)),
		"unknown range":    domain.BucketTimes(domain.Range("2Y"), now, now),
	} {
		if got == nil || len(got) != 0 {
			t.Fatalf("%s: BucketTimes = %#v, want empty", name, got)
		}
	}
}

func TestLastAtOrBefore(t *testing.T) {
	t.Parallel()
	now := bucketNowAt()
	points := []snap{
		{at: now.Add(-6 * time.Hour), value: 1},
		{at: now.Add(-5 * time.Hour), value: 2},
		{at: now.Add(-1 * time.Hour), value: 3},
	}
	tests := map[string]struct {
		at   time.Time
		want int
	}{
		"before the first":     {at: now.Add(-7 * time.Hour)},
		"exactly the first":    {at: now.Add(-6 * time.Hour), want: 1},
		"between points":       {at: now.Add(-5*time.Hour + time.Minute), want: 2},
		"across a gap":         {at: now.Add(-2 * time.Hour), want: 2},
		"exactly the last":     {at: now.Add(-1 * time.Hour), want: 3},
		"after the last point": {at: now, want: 3},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := domain.LastAtOrBefore(points, tt.at, snapAt)
			if tt.want == 0 {
				if got != nil {
					t.Fatalf("LastAtOrBefore = %+v, want nil", got)
				}
				return
			}
			if got == nil || got.value != tt.want {
				t.Fatalf("LastAtOrBefore = %+v, want %d", got, tt.want)
			}
		})
	}
	if got := domain.LastAtOrBefore(nil, now, snapAt); got != nil {
		t.Fatalf("LastAtOrBefore(nil) = %+v", got)
	}
}

func TestBucketing_YoungCabalAndGap(t *testing.T) {
	t.Parallel()
	now := bucketNowAt()
	first := now.Add(-30 * time.Minute)
	points := []snap{
		{at: first, value: 10},
		{at: now.Add(-10 * time.Minute), value: 20},
	}
	var values []int
	omitted := 0
	for _, end := range domain.BucketTimes(domain.Range1H, now, first) {
		if p := domain.LastAtOrBefore(points, end, snapAt); p != nil {
			values = append(values, p.value)
			continue
		}
		omitted++
	}
	if len(values) != 16 || values[0] != 10 || values[len(values)-1] != 20 || omitted != 15 {
		t.Fatalf("values = %v, omitted = %d", values, omitted)
	}
}
