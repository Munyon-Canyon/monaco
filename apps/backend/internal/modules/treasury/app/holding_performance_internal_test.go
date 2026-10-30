package app

import (
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
)

func TestSharesFor_refusesWhatItCannotConvertAndDefaultsTheMultiplierToOne(t *testing.T) {
	t.Parallel()
	tr := marksTrade(0, sideSell, 100_000_000, 1)
	names := map[domain.Asset]AssetName{
		"plain": {Decimals: 8},
		"split": {Decimals: 8, MultiplierAt: func(time.Time) (int64, int64) { return 2, 1 }},
		"zero":  {Decimals: 8, MultiplierAt: func(time.Time) (int64, int64) { return 0, 1 }},
	}
	quotes := map[string]ListingQuote{
		"nil":  {Decimals: 6},
		"flat": {Decimals: 6, SharesPerToken: new(big.Rat)},
		"pair": {Decimals: 6, SharesPerToken: big.NewRat(3, 2)},
	}
	for asset, want := range map[string]*big.Rat{
		"plain": big.NewRat(1, 1), "split": big.NewRat(2, 1), "pair": big.NewRat(150, 1),
	} {
		got, err := sharesFor(asset, names, quotes)(tr)
		if err != nil || got.Cmp(want) != 0 {
			t.Fatalf("%s shares = %v, %v, want %v", asset, got, err, want)
		}
	}
	for _, asset := range []string{"nil", "flat", "zero", "missing"} {
		if _, err := sharesFor(asset, names, quotes)(tr); errs.CodeOf(err) != errs.CodeInternal {
			t.Fatalf("%s shares error = %v, want internal", asset, err)
		}
	}
}

func wholeShare(trade) (*big.Rat, error) { return big.NewRat(1, 1), nil }

func closeOut(state *replayed, s *salesTotal, shares sharesOf, usdc uint64) error {
	if err := s.apply(state, marksTrade(0, sideBuy, 1, usdc), shares); err != nil {
		return err
	}
	return s.apply(state, marksTrade(1, sideSell, 1, usdc), shares)
}

func TestSalesTotal_refusesASideItDoesNotKnowAndASellItCannotReplay(t *testing.T) {
	t.Parallel()
	broke := func(trade) (*big.Rat, error) { return nil, errs.New(errs.CodeInternal, "test") }
	var s salesTotal
	if err := s.apply(&replayed{}, marksTrade(0, "swap", 1, 1), wholeShare); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("unknown side = %v, want internal", err)
	}
	if err := s.apply(&replayed{}, marksTrade(0, sideSell, 1, 1), wholeShare); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("sell of nothing = %v, want internal", err)
	}
	if err := closeOut(&replayed{}, &s, broke, 1); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("unreadable shares = %v, want internal", err)
	}
}

func TestSalesTotal_refusesTotalsThatOverflowOrCannotBeReadBack(t *testing.T) {
	t.Parallel()
	tiny := func(trade) (*big.Rat, error) { return big.NewRat(1, 100_000_000), nil }
	var removedSum salesTotal
	if err := closeOut(&replayed{}, &removedSum, wholeShare, math.MaxUint64); err != nil {
		t.Fatal(err)
	}
	if err := closeOut(&replayed{}, &removedSum, wholeShare, math.MaxUint64); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("removed cost past uint64 = %v, want internal", err)
	}
	proceedsSum := proceedsOnly(t, wholeShare)
	err := proceedsSum.apply(&replayed{held: 1}, marksTrade(1, sideSell, 1, math.MaxUint64), wholeShare)
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("proceeds past uint64 = %v, want internal", err)
	}
	if _, err := proceedsOnly(t, wholeShare).result(); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("realized past int64 = %v, want internal", err)
	}
	var huge salesTotal
	if err := closeOut(&replayed{}, &huge, tiny, math.MaxUint64); err != nil {
		t.Fatal(err)
	}
	if _, err := huge.result(); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("avg past uint64 = %v, want internal", err)
	}
}

func proceedsOnly(t *testing.T, shares sharesOf) *salesTotal {
	t.Helper()
	var s salesTotal
	if err := s.apply(&replayed{held: 1}, marksTrade(0, sideSell, 1, math.MaxUint64), shares); err != nil {
		t.Fatal(err)
	}
	return &s
}
