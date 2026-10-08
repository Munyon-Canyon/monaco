package analytics_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/bucket"
)

func day(d int) time.Time { return time.Date(2026, 9, d, 0, 0, 0, 0, time.UTC) }

func TestParseWindow(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		from, to, size string
		want           app.Window
		bad            bool
	}{
		"dates": {
			"2026-09-01", "2026-09-08", "day",
			app.Window{From: day(1), To: day(8), Size: bucket.Day},
			false,
		},
		"instants": {
			"2026-09-01T02:00:00+02:00", "2026-09-08T00:00:00Z", "week",
			app.Window{From: day(1), To: day(8), Size: bucket.Week},
			false,
		},
		"exactly 400 days": {
			"2025-01-01", "2026-02-05", "day",
			app.Window{
				From: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC),
				Size: bucket.Day,
			},
			false,
		},
		"401 days":       {"2025-01-01", "2026-02-06", "day", app.Window{}, true},
		"empty range":    {"2026-09-01", "2026-09-01", "day", app.Window{}, true},
		"reversed":       {"2026-09-02", "2026-09-01", "day", app.Window{}, true},
		"bad from":       {"soon", "2026-09-01", "day", app.Window{}, true},
		"bad to":         {"2026-09-01", "", "day", app.Window{}, true},
		"bad bucket":     {"2026-09-01", "2026-09-02", "hour", app.Window{}, true},
		"everything bad": {"", "", "", app.Window{}, true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := app.ParseWindow(tt.from, tt.to, tt.size)
			if tt.bad {
				if errs.CodeOf(err) != errs.CodeInvalidInput || err == nil {
					t.Fatalf("ParseWindow() error = %v, want invalid_input", err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("ParseWindow() = %+v, %v, want %+v", got, err, tt.want)
			}
		})
	}
}
