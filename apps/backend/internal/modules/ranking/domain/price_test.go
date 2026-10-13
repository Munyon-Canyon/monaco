package domain_test

import (
	"slices"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
)

func TestPriceForBoard_StaleAfterFiveMinutes(t *testing.T) {
	t.Parallel()
	now := clock.Real{}.Now().UTC()
	tests := map[string]struct {
		age   time.Duration
		stale bool
	}{
		"fresh":                 {age: time.Minute},
		"at five minutes":       {age: domain.StaleAfter},
		"past five minutes":     {age: domain.StaleAfter + time.Nanosecond, stale: true},
		"a day old, off-hours":  {age: 24 * time.Hour, stale: true},
		"a sample from the run": {},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sample := domain.Sample{Price: usd(1), At: now.Add(-tt.age)}
			got, flags := domain.PriceForBoard(&sample, now)
			var want []domain.Flag
			if tt.stale {
				want = []domain.Flag{domain.FlagStalePrices}
			}
			if got != sample || !slices.Equal(flags, want) {
				t.Fatalf("PriceForBoard = %+v, %v, want %+v, %v", got, flags, sample, want)
			}
		})
	}
}

func TestPriceForBoard_Unpriced(t *testing.T) {
	t.Parallel()
	got, flags := domain.PriceForBoard(nil, clock.Real{}.Now().UTC())
	if got != (domain.Sample{}) || !slices.Equal(flags, []domain.Flag{domain.FlagUnpricedAssets}) {
		t.Fatalf("PriceForBoard = %+v, %v, want unpriced", got, flags)
	}
}
