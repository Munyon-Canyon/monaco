package app

import (
	"math"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestArrivalFloor_isSentLessTheQuotedFeeAndTheSlack(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ sent, fee, want uint64 }{
		"fee, flat slack and one percent": {25_000_000, 7_500, 25_000_000 - 7_500 - 100_000 - 250_000},
		"a fee larger than the amount":    {1_000_000, 5_000_000, 1},
		"a fee that overflows":            {1_000_000, math.MaxUint64 - 10, 1},
		"slack alone covers the amount":   {100_000, 0, 1},
	} {
		got := arrivalFloor(money.MicrosFromUint64(tc.sent), money.MicrosFromUint64(tc.fee))
		if got != money.MicrosFromUint64(tc.want) {
			t.Errorf("%s: floor = %s, want %d", name, got, tc.want)
		}
	}
}
