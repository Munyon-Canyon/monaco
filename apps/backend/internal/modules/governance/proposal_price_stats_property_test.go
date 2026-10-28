package governance_test

import (
	"math"
	"math/big"
	"testing"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func drawMicros(rt *rapid.T, label string) uint64 {
	return rapid.OneOf(
		rapid.Uint64Range(0, math.MaxInt64), rapid.Uint64Range(math.MaxInt64-1_000, math.MaxInt64),
		rapid.Uint64Range(0, 1_000_000), rapid.Uint64Range(1, 3), rapid.Uint64(),
	).Draw(rt, label)
}

func checkEstimate(rt *rapid.T, usdc, price uint64, decimals uint8) {
	shares, err := app.EstimatedShares(money.MicrosFromUint64(usdc), money.MicrosFromUint64(price), decimals)
	switch {
	case price == 0:
		if shares != nil || err != nil {
			rt.Fatalf("estimate at a zero price = %v, %v, want nil", shares, err)
		}
	case err != nil:
		if shares != nil {
			rt.Fatalf("estimate that overflowed = %v with %v, want nil", shares, err)
		}
	default:
		cost := new(big.Int).Mul(new(big.Int).SetUint64(shares.Uint64()), new(big.Int).SetUint64(price))
		cost.Quo(cost, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil))
		if cost.Cmp(new(big.Int).SetUint64(usdc)) > 0 || shares.Decimals() != decimals {
			rt.Fatalf("estimate %d at %d micros, %d decimals costs %s, over the %d spent",
				shares.Uint64(), price, decimals, cost, usdc)
		}
	}
}

func checkSince(rt *rapid.T, fill, now uint64) {
	since, err := app.SinceBps(money.MicrosFromUint64(fill), money.MicrosFromUint64(now))
	switch {
	case fill == 0 || now == 0 || err != nil:
		if since != nil {
			rt.Fatalf("since(%d, %d) = %d with %v, want nil", fill, now, *since, err)
		}
	case since == nil:
		rt.Fatalf("since(%d, %d) = nil, want a value", fill, now)
	case now == fill && *since != 0:
		rt.Fatalf("since of an unchanged price = %d, want 0", *since)
	case now > fill && *since < 0, now < fill && *since > 0:
		rt.Fatalf("since(%d, %d) = %d, want the sign of now - fill", fill, now, *since)
	}
}

func TestProposalPriceStats_property(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		decimals := rapid.Uint8Range(0, 18).Draw(rt, "decimals")
		checkEstimate(rt, drawMicros(rt, "usdc"), drawMicros(rt, "price"), decimals)
		checkSince(rt, drawMicros(rt, "fill"), drawMicros(rt, "now"))
	})
}
