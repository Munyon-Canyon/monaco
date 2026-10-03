package adapters_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func TestUnwired_FailsClosed(t *testing.T) {
	t.Parallel()
	u := adapters.Unwired{}
	cabalID := ids.CabalIDFrom(ids.Real{}.NewV7())
	userID := ids.UserIDFrom(ids.Real{}.NewV7())
	calls := map[string]func() error{
		"Positions":        func() error { _, err := u.Positions(t.Context(), cabalID); return err },
		"PotValue":         func() error { _, err := u.PotValue(t.Context(), cabalID); return err },
		"TotalShares":      func() error { _, err := u.TotalShares(t.Context(), cabalID); return err },
		"ShareUnits":       func() error { _, err := u.ShareUnits(t.Context(), cabalID, userID); return err },
		"Stake":            func() error { _, err := u.Stake(t.Context(), cabalID, userID); return err },
		"StakesOf":         func() error { _, err := u.StakesOf(t.Context(), userID); return err },
		"ShareUnitsAt":     func() error { _, err := u.ShareUnitsAt(t.Context(), cabalID, userID, time.Time{}); return err },
		"CabalPositionsAt": func() error { _, err := u.CabalPositionsAt(t.Context(), time.Time{}); return err },
		"MemberStakesAt":   func() error { _, err := u.MemberStakesAt(t.Context(), time.Time{}); return err },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := call()
			if errs.KindOf(errs.CodeOf(err)) != errs.KindUnavailable {
				t.Fatalf("error kind = %v, want unavailable", errs.KindOf(errs.CodeOf(err)))
			}
			if name == "PotValue" && errs.CodeOf(err) != errs.CodePriceUnavailable {
				t.Fatalf("PotValue code = %s, want %s", errs.CodeOf(err), errs.CodePriceUnavailable)
			}
			if name != "PotValue" && errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
				t.Fatalf("%s code = %s, want %s", name, errs.CodeOf(err), errs.CodeUpstreamUnavailable)
			}
		})
	}
}
