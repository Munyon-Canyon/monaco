package adapters

import (
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
)

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
