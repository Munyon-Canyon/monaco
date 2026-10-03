package domain

import (
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
