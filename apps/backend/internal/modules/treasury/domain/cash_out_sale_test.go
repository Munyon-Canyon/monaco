package domain_test

import (
	"math"
	"math/big"
	"testing"

	"github.com/google/uuid"
	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func settle(t *testing.T, units, slice, paid uint64) domain.SaleSettlement {
	t.Helper()
	s, err := domain.SettleSale(
		money.SharesUnitsFromUint64(units), money.MicrosFromUint64(slice), money.MicrosFromUint64(paid),
	)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSettleSale_branches(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name               string
		units, slice, paid uint64
		returned, unpaid   uint64
		status             domain.CashOutStatus
	}{
		{"covered", 100, 3_000_000, 3_000_000, 0, 0, domain.CashOutPaying},
		{"partial floors the returned units", 100, 3_000_000, 2_000_001, 33, 999_999, domain.CashOutPaying},
		{"one unit short keeps every unit burned", 100, 3_000_000, 2_999_999, 0, 1, domain.CashOutPaying},
		{"nothing raised returns every unit", 100, 3_000_000, 0, 100, 3_000_000, domain.CashOutFailed},
	}
	for _, c := range cases {
		got := settle(t, c.units, c.slice, c.paid)
		if got.Returned.Uint64() != c.returned || got.Unpaid.Uint64() != c.unpaid || got.Status != c.status ||
			got.Paid.Uint64() != c.paid {
			t.Fatalf("%s: SettleSale = %+v, want returned %d unpaid %d status %s", c.name, got, c.returned,
				c.unpaid, c.status)
		}
	}
}

func TestSettleSale_refusesImpossibleInputs(t *testing.T) {
	t.Parallel()
	one := money.SharesUnitsFromUint64(1)
	if _, err := domain.SettleSale(one, money.MicrosFromUint64(1), money.MicrosFromUint64(2)); errs.CodeOf(err) !=
		errs.CodeInvalidInput {
		t.Fatalf("paid above the slice: err = %v, want invalid_input", err)
	}
	if _, err := domain.SettleSale(one, money.Micros{}, money.Micros{}); err == nil {
		t.Fatal("a zero slice: err = nil")
	}
}

func TestSettleSale_neverPaysMoreThanTheSliceOrMorePerUnit(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		units := rapid.Uint64Range(1, math.MaxInt64).Draw(t, "units")
		slice := rapid.Uint64Range(1, math.MaxInt64).Draw(t, "slice")
		paid := rapid.Uint64Range(0, slice).Draw(t, "paid")
		s, err := domain.SettleSale(
			money.SharesUnitsFromUint64(units), money.MicrosFromUint64(slice), money.MicrosFromUint64(paid),
		)
		if err != nil {
			t.Fatal(err)
		}
		burned := units - s.Returned.Uint64()
		perUnit := new(big.Int).Mul(new(big.Int).SetUint64(burned), new(big.Int).SetUint64(slice))
		owed := new(big.Int).Mul(new(big.Int).SetUint64(paid), new(big.Int).SetUint64(units))
		switch {
		case s.Paid.Uint64()+s.Unpaid.Uint64() != slice || s.Paid.Uint64() > slice:
			t.Fatalf("paid %d + unpaid %d != slice %d", s.Paid.Uint64(), s.Unpaid.Uint64(), slice)
		case s.Returned.Uint64() > units:
			t.Fatalf("returned %d of %d units", s.Returned.Uint64(), units)
		case perUnit.Cmp(owed) < 0:
			t.Fatalf("burned %d units for %d of a %d slice over %d units: the member is paid above the rate",
				burned, paid, slice, units)
		case (paid == 0) != (s.Status == domain.CashOutFailed):
			t.Fatalf("paid %d ended %s", paid, s.Status)
		}
	})
}

func TestCashOutEnd(t *testing.T) {
	t.Parallel()
	slice := money.MicrosFromUint64(10)
	if got := domain.CashOutEnd(slice, slice); got != domain.CashOutComplete {
		t.Fatalf("full payout ends %s", got)
	}
	if got := domain.CashOutEnd(money.MicrosFromUint64(9), slice); got != domain.CashOutCompletePartial {
		t.Fatalf("short payout ends %s", got)
	}
}

func TestCashOutReturn_reCreditsTheUnpaidUSDCAndTheReturnedUnits(t *testing.T) {
	t.Parallel()
	cabal := ids.CabalIDFrom(uuid.UUID{1})
	h := domain.UserTxnHeader{
		ID: uuid.UUID{2}, CabalID: cabal, Kind: domain.UserCashOut,
		Status: domain.TxnSettled,
	}
	txn, err := domain.CashOutReturn(h, usdc, settle(t, 100, 3_000_000, 2_000_001))
	if err != nil {
		t.Fatal(err)
	}
	delta, err := txn.PositionDelta()
	if err != nil || delta.Shares.Int64() != 33 || delta.Contributed.Uint64() != 999_999 || !delta.Withdrawn.IsZero() ||
		len(txn.Entries()) != 4 {
		t.Fatalf("delta = (%+v, %v) over %d entries, want 33 units and 999999 USDC back into the stake", delta, err,
			len(txn.Entries()))
	}
	txn, err = domain.CashOutReturn(h, usdc, settle(t, 100, 3_000_000, 2_999_999))
	if err != nil || len(txn.Entries()) != 2 {
		t.Fatalf("no units back: %d entries, %v; want only the USDC pair", len(txn.Entries()), err)
	}
	huge := domain.SaleSettlement{Unpaid: money.MicrosFromUint64(math.MaxUint64)}
	if _, err := domain.CashOutReturn(h, usdc, huge); err == nil {
		t.Fatal("unpaid above int64: err = nil")
	}
	huge = domain.SaleSettlement{
		Unpaid:   money.MicrosFromUint64(1),
		Returned: money.SharesUnitsFromUint64(math.MaxUint64),
	}
	if _, err := domain.CashOutReturn(h, usdc, huge); err == nil {
		t.Fatal("returned units above int64: err = nil")
	}
}
