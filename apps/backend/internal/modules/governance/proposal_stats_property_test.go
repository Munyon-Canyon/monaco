package governance_test

import (
	"math"
	"math/big"
	"testing"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func exactShare(value, pot uint64) *big.Int {
	exact := new(big.Int).Mul(new(big.Int).SetUint64(value), big.NewInt(10_000))
	return exact.Quo(exact, new(big.Int).SetUint64(pot))
}

func checkShare(rt *rapid.T, value, pot uint64, got *int64, err error) {
	rt.Helper()
	exact := exactShare(value, pot)
	switch {
	case err != nil && exact.IsUint64():
		rt.Fatalf("share(%d, %d) = %v, want %s", value, pot, err, exact)
	case err != nil:
	case got == nil && exact.IsInt64():
		rt.Fatalf("share(%d, %d) = nil, want %s", value, pot, exact)
	case got != nil && big.NewInt(*got).Cmp(exact) != 0:
		rt.Fatalf("share(%d, %d) = %d, want %s rounded down", value, pot, *got, exact)
	case got != nil && value <= pot && *got > 10_000:
		rt.Fatalf("share(%d, %d) = %d above 10000", value, pot, *got)
	}
}

func TestTreasuryShareBps_property(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		amount := rapid.OneOf(
			rapid.Uint64Range(0, math.MaxInt64), rapid.Uint64Range(math.MaxInt64-1_000, math.MaxInt64),
			rapid.Uint64Range(0, 1_000_000),
		)
		value, pot := amount.Draw(rt, "value"), amount.Draw(rt, "pot")
		got, err := app.TreasuryShareBps(money.MicrosFromUint64(value), money.MicrosFromUint64(pot))
		if pot == 0 {
			if got != nil || err != nil {
				rt.Fatalf("zero pot share = %v, %v, want nil", got, err)
			}
			return
		}
		checkShare(rt, value, pot, got, err)
	})
}

func TestTreasuryShareBps_wholePotIsTenThousand(t *testing.T) {
	t.Parallel()
	full, err := app.TreasuryShareBps(money.MicrosFromUint64(7), money.MicrosFromUint64(7))
	if err != nil || full == nil || *full != 10_000 {
		t.Fatalf("value = pot share = %v, %v, want 10000", full, err)
	}
}
