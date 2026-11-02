package market_test

import (
	"math"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestAsset_CloseReference(t *testing.T) {
	t.Parallel()
	closedAt := time.Date(2026, time.November, 2, 21, 0, 0, 0, time.UTC)
	step := closedAt.Add(2 * time.Hour)
	now := closedAt.Add(14 * time.Hour)
	steppedTo := func(from, to domain.Multiplier) domain.Asset {
		return domain.Asset{UIMultiplier: from, NextUIMultiplier: domain.MultiplierStep{To: to, At: step}}
	}
	one, two := domain.Multiplier{Num: 1, Den: 1}, domain.Multiplier{Num: 2, Den: 1}
	three := domain.Multiplier{Num: 3, Den: 2}
	tenth := domain.Multiplier{Num: 1, Den: 10}
	dividend := domain.Multiplier{Num: 1_003, Den: 1_000}
	huge := domain.Multiplier{Num: 1, Den: math.MaxInt64}
	cases := map[string]struct {
		asset domain.Asset
		now   time.Time
		ref   uint64
		want  uint64
	}{
		"two for one":       {steppedTo(one, two), now, 200_000_000, 100_000_000},
		"three for two":     {steppedTo(one, three), now, 300_000_000, 200_000_000},
		"one for ten":       {steppedTo(one, tenth), now, 5_000_000, 50_000_000},
		"dividend step":     {steppedTo(one, dividend), now, 200_000_000, 200_000_000},
		"step not yet due":  {steppedTo(one, two), step.Add(-time.Second), 200_000_000, 200_000_000},
		"step before close": {steppedTo(one, two), now, 200_000_000, 200_000_000},
		"no schedule":       {domain.Asset{UIMultiplier: one}, now, 200_000_000, 200_000_000},
		"unchecked asset":   {domain.Asset{}, now, 200_000_000, 200_000_000},
		"overflow":          {steppedTo(one, huge), now, math.MaxUint64, math.MaxUint64},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			at := closedAt
			if name == "step before close" {
				at = step.Add(time.Hour)
			}
			got := tc.asset.CloseReference(money.MicrosFromUint64(tc.ref), at, tc.now)
			if got.Uint64() != tc.want {
				t.Fatalf("CloseReference(%d) = %d, want %d", tc.ref, got.Uint64(), tc.want)
			}
		})
	}
}
