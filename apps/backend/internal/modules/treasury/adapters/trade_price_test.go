package adapters

import (
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/google/uuid"
	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
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
	} {
		got, ok, err := tradePriceMicros(
			money.MicrosFromUint64(c.usdc), money.NewBaseUnits(c.units, c.decimals), c.num, c.den,
		)
		if err != nil || !ok || got.Uint64() != c.want {
			t.Fatalf("%s: price = %v, %v, %v, want %d", name, got, ok, err, c.want)
		}
	}
}

func TestTradePriceMicros_hasNoPriceForZeroUnitsAndRefusesWhatItCannotHold(t *testing.T) {
	t.Parallel()
	if _, ok, err := tradePriceMicros(money.MicrosFromUint64(1), money.NewBaseUnits(0, 8), 1, 1); ok || err != nil {
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
		"scale past uint64":    {1, 1, 19, 1, 2},
		"shares past uint64":   {1, math.MaxUint64, 8, 2, 1},
		"price past uint64":    {math.MaxUint64, 1, 8, 1, 1},
	} {
		if _, ok, err := tradePriceMicros(
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
		price, ok, err := tradePriceMicros(money.MicrosFromUint64(usdc), money.NewBaseUnits(units, decimals), num, den)
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

func TestActivityPrice_pricesConfirmedTradesAtTheMultiplierOfTheirTime(t *testing.T) {
	t.Parallel()
	change := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	split := app.Asset{
		UIMultiplierNum: 1, UIMultiplierDen: 1, NextUIMultiplierNum: 3, NextUIMultiplierDen: 2,
		NextUIMultiplierAt: change,
	}
	asset := &app.AssetName{Symbol: "NVDAx", Decimals: 8, MultiplierAt: split.UIMultiplierAt}
	usdc, units := int64(300_000_000), int64(200_000_000)
	buy := app.ActivityView{
		ID: uuid.UUID{1}, Kind: domain.ActivityBuy, Status: domain.ActivityConfirmed, Asset: asset,
		USDCMicros: &usdc, Units: &units, OccurredAt: change.Add(-time.Hour),
	}
	after := buy
	after.Kind, after.OccurredAt = domain.ActivitySell, change
	for name, c := range map[string]struct {
		view app.ActivityView
		want int64
	}{
		"before the split, 2 shares": {buy, 150_000_000},
		"after the split, 3 shares":  {after, 100_000_000},
	} {
		got, err := wireActivity(c.view)
		if err != nil || got.PriceMicros == nil || *got.PriceMicros != c.want {
			t.Fatalf("%s: price = %v, %v, want %d", name, got.PriceMicros, err, c.want)
		}
	}
	unpriced := map[string]func(*app.ActivityView){
		"fund":            func(v *app.ActivityView) { v.Kind = domain.ActivityFund },
		"pending":         func(v *app.ActivityView) { v.Status = domain.ActivityPending },
		"unknown stock":   func(v *app.ActivityView) { v.Asset = nil },
		"listing stock":   func(v *app.ActivityView) { v.Asset = &app.AssetName{Symbol: "AAPLon"} },
		"usdc not known":  func(v *app.ActivityView) { v.USDCMicros = nil },
		"units not known": func(v *app.ActivityView) { v.Units = nil },
		"no units at all": func(v *app.ActivityView) { zero := int64(0); v.Units = &zero },
	}
	for name, edit := range unpriced {
		v := buy
		edit(&v)
		if got, err := wireActivity(v); err != nil || got.PriceMicros != nil {
			t.Fatalf("%s: price = %v, %v, want null", name, got.PriceMicros, err)
		}
	}
}

func TestActivityPrice_refusesAPriceTheWireCannotHold(t *testing.T) {
	t.Parallel()
	one := int64(1)
	for name, usdc := range map[string]int64{"past uint64": math.MaxInt64, "past int64": 100_000_000_000} {
		v := app.ActivityView{
			Kind: domain.ActivitySell, Status: domain.ActivityConfirmed, USDCMicros: &usdc, Units: &one,
			Asset: &app.AssetName{Decimals: 8, MultiplierAt: func(time.Time) (int64, int64) { return 1, 1 }},
		}
		if _, err := wireActivity(v); err == nil {
			t.Fatalf("%s: wireActivity = nil error, want a refusal", name)
		}
	}
}
