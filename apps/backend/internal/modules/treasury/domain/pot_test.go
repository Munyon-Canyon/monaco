package domain_test

import (
	"math"
	"math/big"
	"testing"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func sumBps(bps []int32) int64 {
	var total int64
	for _, b := range bps {
		total += int64(b)
	}
	return total
}

func firstLargest(values []uint64) int {
	largest := 0
	for i, v := range values {
		if v > values[largest] {
			largest = i
		}
	}
	return largest
}

func checkSplit(t *rapid.T, raw []uint64, bps []int32) {
	var total uint64
	for _, v := range raw {
		total += v
	}
	want := int64(10000)
	if total == 0 {
		want = 0
	}
	if sumBps(bps) != want {
		t.Fatalf("bps %v sum to %d, want %d", bps, sumBps(bps), want)
	}
	if total == 0 {
		return
	}
	largest := firstLargest(raw)
	for i, b := range bps {
		floor := new(big.Int).SetUint64(raw[i])
		floor.Mul(floor, big.NewInt(10000)).Quo(floor, new(big.Int).SetUint64(total))
		got := big.NewInt(int64(b))
		if i != largest && got.Cmp(floor) != 0 || i == largest && got.Cmp(floor) < 0 {
			t.Fatalf("row %d got %d, floor %s, largest %d", i, b, floor, largest)
		}
	}
}

func TestPotProperty_weightsSumTo10000OnTheLargestRow(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		raw := rapid.SliceOfN(rapid.Uint64Range(0, 1<<50), 1, 8).Draw(t, "values")
		values := make([]money.Micros, len(raw))
		for i, v := range raw {
			values[i] = money.MicrosFromUint64(v)
		}
		bps, err := domain.WeightsBps(values)
		if err != nil {
			t.Fatal(err)
		}
		checkSplit(t, raw, bps)
	})
}

func TestPotProperty_slicesSumTo10000AndPayoutsFitThePot(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		raw := rapid.SliceOfN(rapid.Uint64Range(0, 1<<50), 1, 8).Draw(t, "units")
		pot := money.MicrosFromUint64(rapid.Uint64Range(0, 1<<50).Draw(t, "pot"))
		units := make([]money.SharesUnits, len(raw))
		var total uint64
		for i, v := range raw {
			units[i] = money.SharesUnitsFromUint64(v)
			total += v
		}
		bps, err := domain.SlicesBps(units)
		if err != nil {
			t.Fatal(err)
		}
		checkSplit(t, raw, bps)
		if total == 0 {
			return
		}
		var paid uint64
		for _, u := range units {
			payout, err := domain.PayoutFor(u, money.SharesUnitsFromUint64(total), pot)
			if err != nil {
				t.Fatal(err)
			}
			paid += payout.Uint64()
		}
		if paid > pot.Uint64() {
			t.Fatalf("payouts %d exceed pot %d", paid, pot.Uint64())
		}
	})
}

func TestWeightsBps_tieGoesToTheFirstRow(t *testing.T) {
	t.Parallel()
	bps, err := domain.WeightsBps([]money.Micros{
		money.MicrosFromUint64(1), money.MicrosFromUint64(1), money.MicrosFromUint64(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	if bps[0] != 3334 || bps[1] != 3333 || bps[2] != 3333 {
		t.Fatalf("got %v", bps)
	}
}

func TestWeightsBps_overflowingTotalFails(t *testing.T) {
	t.Parallel()
	_, err := domain.WeightsBps([]money.Micros{money.MicrosFromUint64(math.MaxUint64), money.MicrosFromUint64(1)})
	if err == nil {
		t.Fatal("want overflow error")
	}
}

func TestReturnBps(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		pnl     int64
		net     int64
		want    int32
		present bool
	}{
		{"zero net is null", 500, 0, 0, false},
		{"negative net is null", 500, -1, 0, false},
		{"gain", 250_000, 1_000_000, 2500, true},
		{"loss is negative", -333_333, 1_000_000, -3333, true},
		{"truncates toward zero", -1, 3, -3333, true},
		{"saturates high", math.MaxInt64 / 10000, 1, math.MaxInt32, true},
		{"saturates low", math.MinInt64 / 10000, 1, math.MinInt32, true},
		{"saturates past int64", math.MaxInt64, 1, math.MaxInt32, true},
		{"saturates below int64", math.MinInt64, 1, math.MinInt32, true},
	}
	for _, c := range cases {
		got, ok := domain.ReturnBps(money.SignedMicrosFromInt64(c.pnl), money.SignedMicrosFromInt64(c.net))
		if got != c.want || ok != c.present {
			t.Errorf("%s: got (%d, %v), want (%d, %v)", c.name, got, ok, c.want, c.present)
		}
	}
}

func TestSlicesBps_zeroTotalGivesZeroSlices(t *testing.T) {
	t.Parallel()
	bps, err := domain.SlicesBps([]money.SharesUnits{{}, {}})
	if err != nil || bps[0] != 0 || bps[1] != 0 {
		t.Fatalf("got %v, %v", bps, err)
	}
}
