package domain_test

import (
	"math"
	"slices"
	"testing"

	"pgregory.net/rapid"

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
				wantCode(t, err, tt.code)
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
	wantCode(t, err, errs.CodeInvalidInput)
}

func TestCashOutUnitsForRoundsUp(t *testing.T) {
	t.Parallel()
	got, err := domain.CashOutUnitsFor(
		money.MicrosFromUint64(1), money.SharesUnitsFromUint64(3), money.MicrosFromUint64(10),
	)
	if err != nil || got.Uint64() != 1 {
		t.Fatalf("CashOutUnitsFor = %v, %v", got, err)
	}
	_, err = domain.CashOutUnitsFor(
		money.MicrosFromUint64(1), money.SharesUnitsFromUint64(1), money.Micros{},
	)
	wantCode(t, err, errs.CodeInvalidInput)
}

func TestSliceAfterFund(t *testing.T) {
	t.Parallel()
	usd := money.MicrosFromUint64
	units := money.SharesUnitsFromUint64
	tests := map[string]struct {
		units []money.SharesUnits
		me    int
		total money.SharesUnits
		pot   money.Micros
		in    money.Micros
		want  int32
		code  errs.Code
	}{
		"first funder of an empty pot": {
			units: []money.SharesUnits{units(0)}, me: 0, pot: usd(0), in: usd(5), want: 10000,
		},
		"clean split": {
			units: []money.SharesUnits{units(35), units(65)}, me: 0, total: units(100), pot: usd(100), in: usd(1),
			want: 3564,
		},
		"remainder goes to the caller when largest": {
			units: []money.SharesUnits{units(3), units(3), units(3)}, me: 2, total: units(9), pot: usd(9), in: usd(4),
			want: 5386,
		},
		"tie breaks to the first holder": {
			units: []money.SharesUnits{units(1), units(0)}, me: 1, total: units(1), pot: usd(1), in: usd(1),
			want: 5000,
		},
		"three equal holders keep the remainder with the first": {
			units: []money.SharesUnits{units(1), units(1), units(1)}, me: 1, total: units(3), pot: usd(3), in: usd(0),
			want: 3333,
		},
		"worthless pot": {
			units: []money.SharesUnits{units(1)}, me: 0, total: units(1), in: usd(1), code: errs.CodePotValueZero,
		},
		"unknown member": {
			units: []money.SharesUnits{units(1)}, me: 1, total: units(1), pot: usd(1), in: usd(1),
			code: errs.CodeInvalidInput,
		},
		"negative member": {
			units: []money.SharesUnits{units(1)}, me: -1, total: units(1), pot: usd(1), in: usd(1),
			code: errs.CodeInvalidInput,
		},
		"units overflow": {
			units: []money.SharesUnits{units(math.MaxUint64)}, me: 0, total: units(1), pot: usd(1), in: usd(1),
			code: errs.CodeInvalidInput,
		},
		"slices overflow": {
			units: []money.SharesUnits{units(math.MaxUint64), units(math.MaxUint64)}, me: 0, total: units(1),
			pot: usd(1), in: usd(0), code: errs.CodeInvalidInput,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := domain.SliceAfterFund(tt.units, tt.me, tt.total, tt.pot, tt.in)
			if tt.code != "" {
				wantCode(t, err, tt.code)
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("SliceAfterFund = %v, %v, want %v", got, err, tt.want)
			}
		})
	}
}

func drawHolders(t *rapid.T) ([]money.SharesUnits, money.SharesUnits) {
	held := rapid.SliceOfN(rapid.Uint64Range(0, 1<<40), 1, 8).Draw(t, "units")
	all := make([]money.SharesUnits, len(held))
	var total uint64
	for i, v := range held {
		all[i] = money.SharesUnitsFromUint64(v)
		total += v
	}
	return all, money.SharesUnitsFromUint64(total)
}

func addBps(bps []int32) int32 {
	var sum int32
	for _, b := range bps {
		sum += b
	}
	return sum
}

func TestSliceAfterFundProperty(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		all, total := drawHolders(t)
		me := rapid.IntRange(0, len(all)-1).Draw(t, "me")
		pot := money.MicrosFromUint64(rapid.Uint64Range(1<<20, 1<<40).Draw(t, "pot"))
		in := money.MicrosFromUint64(rapid.Uint64Range(0, 1<<40).Draw(t, "in"))
		got, err := domain.SliceAfterFund(all, me, total, pot, in)
		if err != nil || got < 0 || got > 10000 {
			t.Fatalf("slice = %d, %v, want within [0, 10000]", got, err)
		}
		minted, err := domain.MintShares(in, total, pot)
		if err != nil {
			t.Fatal(err)
		}
		after := slices.Clone(all)
		after[me], _ = after[me].Add(minted)
		bps, err := domain.SlicesBps(after)
		if err != nil || got != bps[me] {
			t.Fatalf("slice = %d, want %d from SlicesBps (%v)", got, bps[me], err)
		}
		if !total.IsZero() || !minted.IsZero() {
			if sum := addBps(bps); sum != 10000 {
				t.Fatalf("slices sum to %d, want 10000", sum)
			}
		}
	})
}
