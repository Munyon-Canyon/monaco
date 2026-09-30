package domain_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
)

func TestParseRange(t *testing.T) {
	t.Parallel()
	for _, want := range []domain.Range{domain.Range1H, domain.Range1D, domain.Range1W, domain.Range1M, domain.RangeAll} {
		if got, err := domain.ParseRange(string(want)); err != nil || got != want {
			t.Errorf("ParseRange(%q) = %q, %v", want, got, err)
		}
	}
	for _, raw := range []string{"", "1h", "all", "2D", "1Y"} {
		if _, err := domain.ParseRange(raw); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("ParseRange(%q) err = %v, want invalid_input", raw, err)
		}
	}
}

func TestRangeStart(t *testing.T) {
	t.Parallel()
	now := clock.Real{}.Now().UTC()
	tests := map[domain.Range]time.Duration{
		domain.Range1H: time.Hour,
		domain.Range1D: 24 * time.Hour,
		domain.Range1W: 7 * 24 * time.Hour,
		domain.Range1M: 30 * 24 * time.Hour,
	}
	for r, span := range tests {
		if got, ok := r.Start(now); !ok || !got.Equal(now.Add(-span)) {
			t.Errorf("%s.Start = %v, %v, want %v", r, got, ok, now.Add(-span))
		}
	}
	if got, ok := domain.RangeAll.Start(now); ok || !got.IsZero() {
		t.Errorf("ALL.Start = %v, %v, want zero, false", got, ok)
	}
}
