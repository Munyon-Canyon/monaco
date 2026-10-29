package domain_test

import (
	"math"
	"math/big"
	"testing"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestTradePriceMicros_pricesOneDisplayShareAndRoundsDown(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		usdc, units uint64
		decimals    uint8
		num, den    int64
		want        uint64
	}{
		"one share at 1/1":               {181_200_000, 100_000_000, 8, 1, 1, 181_200_000},
		"two tokens at 3/2 are 3 shares": {300_000_000, 200_000_000, 8, 3, 2, 100_000_000},
		"six decimals":                   {10_000_000, 4_000_000, 6, 1, 1, 2_500_000},
		"a third rounds down":            {10_000_000, 6_000_000, 6, 1, 1, 1_666_666},
		"shares past uint64":             {1, math.MaxUint64, 8, 2, 1, 0},
		"split multiplier past uint64":   {100_000_000, 105_000_000, 8, 10032690125398187, 10000000000000000, 94_927_775},
	} {
		got, ok, err := domain.TradePriceMicros(
			money.MicrosFromUint64(c.usdc), money.NewBaseUnits(c.units, c.decimals), c.num, c.den,
		)
		if err != nil || !ok || got.Uint64() != c.want {
			t.Fatalf("%s: price = %v, %v, %v, want %d", name, got, ok, err, c.want)
		}
	}
}

func TestTradePriceMicros_hasNoPriceForZeroUnitsAndRefusesWhatItCannotHold(t *testing.T) {
	t.Parallel()
	if _, ok, err := domain.TradePriceMicros(
		money.MicrosFromUint64(1),
		money.NewBaseUnits(0, 8),
		1,
		1,
	); ok ||
		err != nil {
		t.Fatalf("zero units = %v, %v, want no price and no error", ok, err)
	}
	for name, c := range map[string]struct {
		usdc, units uint64
		decimals    uint8
		num, den    int64
	}{
		"zero numerator":       {1, 1, 8, 0, 1},
		"negative denominator": {1, 1, 8, 1, -1},
		"twenty decimals":      {1, 1, 20, 1, 1},
		"price past uint64":    {math.MaxUint64, 1, 8, 1, 1},
		"scale past uint64":    {1, 1, 19, 1, 2},
	} {
		if _, ok, err := domain.TradePriceMicros(
			money.MicrosFromUint64(c.usdc), money.NewBaseUnits(c.units, c.decimals), c.num, c.den,
		); err == nil || ok {
			t.Fatalf("%s: ok = %v, err = %v, want an error", name, ok, err)
		}
	}
}

func TestTradePriceMicros_neverPaysMoreThanTheFillAndLosesUnderOneShareStep(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		usdc := rapid.Uint64Range(0, 1e13).Draw(t, "usdc")
		units := rapid.Uint64Range(1, 1e15).Draw(t, "units")
		decimals := rapid.Uint8Range(0, 9).Draw(t, "decimals")
		num := rapid.Int64Range(1, 1000).Draw(t, "num")
		den := rapid.Int64Range(1, 1000).Draw(t, "den")
		price, ok, err := domain.TradePriceMicros(
			money.MicrosFromUint64(usdc),
			money.NewBaseUnits(units, decimals),
			num,
			den,
		)
		if err != nil {
			price := new(big.Int).Mul(new(big.Int).SetUint64(usdc), big.NewInt(den))
			price.Mul(price, new(big.Int).SetUint64(money.OneWhole(decimals)))
			if price.Quo(price, new(big.Int).Mul(new(big.Int).SetUint64(units), big.NewInt(num))).IsUint64() {
				t.Fatalf("price of %d for %d units = %v, want a price that fits uint64", usdc, units, err)
			}
			return
		}
		if !ok {
			t.Fatal("no price for non-zero units")
		}
		shares := new(big.Rat).SetFrac(
			new(big.Int).Mul(new(big.Int).SetUint64(units), big.NewInt(num)),
			new(big.Int).Mul(new(big.Int).SetUint64(money.OneWhole(decimals)), big.NewInt(den)),
		)
		paid := new(big.Rat).Mul(new(big.Rat).SetInt(new(big.Int).SetUint64(price.Uint64())), shares)
		fill := new(big.Rat).SetInt(new(big.Int).SetUint64(usdc))
		gap := new(big.Rat).Sub(fill, paid)
		if gap.Sign() < 0 || gap.Cmp(shares) >= 0 {
			t.Fatalf("price %d × %s shares = %s, want within one share step under %d", price.Uint64(),
				shares.RatString(), paid.RatString(), usdc)
		}
	})
}
