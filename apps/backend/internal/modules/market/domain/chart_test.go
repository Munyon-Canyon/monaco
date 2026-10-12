package domain

import (
	"slices"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

func TestParseChartRange_namesTheWindow(t *testing.T) {
	t.Parallel()
	day := 24 * time.Hour
	cases := []struct {
		raw     string
		bucket  time.Duration
		window  time.Duration
		bounded bool
	}{
		{"1D", 5 * time.Minute, day, true},
		{"1W", time.Hour, 7 * day, true},
		{"1M", time.Hour, 30 * day, true},
		{"3M", time.Hour, 90 * day, true},
		{"6M", 24 * time.Hour, 180 * day, true},
		{"1Y", 24 * time.Hour, 365 * day, true},
		{"ALL", 24 * time.Hour, 0, false},
	}
	for _, tc := range cases {
		got, err := ParseChartRange(tc.raw)
		window, bounded := got.Window()
		if err != nil || got.Bucket() != tc.bucket || window != tc.window || bounded != tc.bounded {
			t.Fatalf("%s = %s %v bucket %s window %s %v", tc.raw, got, err, got.Bucket(), window, bounded)
		}
	}
	if _, err := ParseChartRange("nope"); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("nope = %v", err)
	}
	if ChartRange("nope").Bucket() != day {
		t.Fatal("default bucket")
	}
	if _, bounded := ChartRange("nope").Window(); bounded {
		t.Fatal("default window")
	}
}

func TestChartRange_allowsTheIntervalsOfItsTable(t *testing.T) {
	t.Parallel()
	all := []ChartInterval{Interval5m, Interval15m, Interval30m, Interval1h, Interval4h, Interval1d, Interval1w}
	table := map[ChartRange][]ChartInterval{
		Chart1D:  {Interval5m, Interval15m, Interval30m, Interval1h, Interval4h, Interval1d},
		Chart1W:  {Interval5m, Interval15m, Interval30m, Interval1h, Interval4h, Interval1d},
		Chart1M:  {Interval15m, Interval30m, Interval1h, Interval4h, Interval1d},
		Chart3M:  {Interval30m, Interval1h, Interval4h, Interval1d, Interval1w},
		Chart6M:  {Interval1h, Interval4h, Interval1d, Interval1w},
		Chart1Y:  {Interval4h, Interval1d, Interval1w},
		ChartAll: {Interval4h, Interval1d, Interval1w},
	}
	for span, allowed := range table {
		for _, interval := range all {
			if got := span.Allows(interval); got != slices.Contains(allowed, interval) {
				t.Fatalf("%s allows %s = %v", span, interval, got)
			}
		}
		if !span.Allows(span.DefaultInterval()) || span.Bucket() != span.DefaultInterval().Duration() {
			t.Fatalf("%s default %s is not allowed", span, span.DefaultInterval())
		}
	}
	if ChartRange("nope").Allows(Interval1d) {
		t.Fatal("an unknown range allows nothing")
	}
}

func TestParseChartInterval_namesTheWidth(t *testing.T) {
	t.Parallel()
	widths := map[string]time.Duration{
		"5m": 5 * time.Minute, "15m": 15 * time.Minute, "30m": 30 * time.Minute,
		"1h": time.Hour, "4h": 4 * time.Hour, "1d": 24 * time.Hour, "1w": 7 * 24 * time.Hour,
	}
	for raw, width := range widths {
		got, err := ParseChartInterval(raw)
		if err != nil || got.Duration() != width {
			t.Fatalf("%s = %s %v", raw, got, err)
		}
	}
	for _, raw := range []string{"1m", "15M", "", "2h", "1W"} {
		if _, err := ParseChartInterval(raw); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Fatalf("%q = %v", raw, err)
		}
	}
}

func TestChartInterval_anUnknownWidthFallsBackToADay(t *testing.T) {
	t.Parallel()
	if got := ChartInterval("").Duration(); got != 24*time.Hour {
		t.Fatalf("duration = %s", got)
	}
}
