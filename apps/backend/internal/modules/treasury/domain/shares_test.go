package domain_test

import (
	"math"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestMintShares(t *testing.T) {
	t.Parallel()
	usd := money.MicrosFromUint64
	units := money.SharesUnitsFromUint64
	tests := map[string]struct {
		in         money.Micros
		total      money.SharesUnits
		pot        money.Micros
		want       money.SharesUnits
		code       errs.Code
		wantErrStr bool
	}{
		"first dollar buys one share": {in: usd(1_000_000), want: units(1_000_000)},
		"first deposit ignores pot":   {in: usd(7), pot: usd(99), want: units(7)},
		"pot doubled halves the mint": {in: usd(10), total: units(10), pot: usd(20), want: units(5)},
		"rounds down":                 {in: usd(1), total: units(2), pot: usd(3), want: units(0)},
		"worthless pot":               {in: usd(1), total: units(1), code: errs.CodePotValueZero},
		"overflow": {
			in: usd(math.MaxUint64), total: units(math.MaxUint64), pot: usd(1), code: errs.CodeInvalidInput,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := domain.MintShares(tt.in, tt.total, tt.pot)
			if tt.code != "" {
				if errs.CodeOf(err) != tt.code {
					t.Fatalf("MintShares = %v, %v, want %s", got, err, tt.code)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("MintShares = %v, %v, want %v", got, err, tt.want)
			}
		})
	}
}

func TestPayoutFor(t *testing.T) {
	t.Parallel()
	units := money.SharesUnitsFromUint64
	got, err := domain.PayoutFor(units(1), units(3), money.MicrosFromUint64(10))
	if err != nil || got.Uint64() != 3 {
		t.Fatalf("PayoutFor(1, 3, 10) = %v, %v, want 3", got, err)
	}
	_, err = domain.PayoutFor(units(0), units(0), money.MicrosFromUint64(10))
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("PayoutFor with no shares = %v, want invalid_input", err)
	}
}
