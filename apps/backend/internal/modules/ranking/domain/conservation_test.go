package domain_test

import (
	"math"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestCheckConservation(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		nav      money.Micros
		equities []money.Micros
		ok       bool
	}{
		"exact":                      {nav: usd(100), equities: []money.Micros{usd(60), usd(40)}, ok: true},
		"one micro per member short": {nav: usd(100), equities: []money.Micros{usd(33), usd(33), usd(31)}, ok: true},
		"empty pot no members":       {nav: usd(0), ok: true},
		"too far short":              {nav: usd(100), equities: []money.Micros{usd(60), usd(37)}},
		"members exceed the pot":     {nav: usd(100), equities: []money.Micros{usd(60), usd(41)}},
		"members with no pot":        {nav: usd(0), equities: []money.Micros{usd(1)}},
		"value with no members":      {nav: usd(1)},
		"sum past uint64 still checks": {
			nav:      usd(math.MaxUint64),
			equities: []money.Micros{usd(math.MaxUint64), usd(1)},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := domain.CheckConservation(tt.nav, tt.equities)
			if tt.ok && err != nil {
				t.Fatalf("CheckConservation = %v, want nil", err)
			}
			if !tt.ok && errs.CodeOf(err) != errs.CodeConservationBroken {
				t.Fatalf("CheckConservation = %v, want conservation_broken", err)
			}
		})
	}
}
