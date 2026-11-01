package app

import (
	"math"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestArrivalFloor_allowsTheQuotedFeePlusTheSlackAndNoMore(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ estimate, sent, want uint64 }{
		"fee plus a flat and a percent slack": {estimate: 7_500, sent: 25_000_000, want: 25_000_000 - 7_500 - 100_000 - 250_000},
		"a zero quote still leaves the slack": {estimate: 0, sent: 1_000_000, want: 1_000_000 - 100_000 - 10_000},
		"a fee above the amount":              {estimate: 2_000_000, sent: 1_000_000, want: 1},
		"a slack above the amount":            {estimate: 0, sent: 50_000, want: 1},
		"an overflowing quote":                {estimate: math.MaxUint64, sent: 1_000_000, want: 1},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := arrivalFloor(money.MicrosFromUint64(tc.sent), money.MicrosFromUint64(tc.estimate))

			if got != money.MicrosFromUint64(tc.want) {
				t.Fatalf("arrivalFloor = %v, want %d", got, tc.want)
			}
		})
	}
}
