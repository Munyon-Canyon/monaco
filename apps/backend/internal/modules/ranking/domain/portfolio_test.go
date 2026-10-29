package domain_test

import (
	"math"
	"math/big"
	"slices"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func micros(vs ...uint64) []money.Micros {
	out := make([]money.Micros, 0, len(vs))
	for _, v := range vs {
		out = append(out, money.MicrosFromUint64(v))
	}
	return out
}

func sum(xs []uint64) uint64 {
	var total uint64
	for _, x := range xs {
		total += x
	}
	return total
}

func floorOf(v, total uint64) uint64 {
	q := new(big.Int).Mul(new(big.Int).SetUint64(v), big.NewInt(10_000))
	return q.Div(q, new(big.Int).SetUint64(total)).Uint64()
}

func assertFloors(t *rapid.T, raw, got []uint64, total uint64) {
	largest := slices.Index(raw, slices.Max(raw))
	for i, v := range raw {
		if floor := floorOf(v, total); got[i] < floor || (i != largest && got[i] != floor) {
			t.Fatalf(
				"Slices(%v)[%d] = %d, want the floor %d (the remainder goes to row %d)",
				raw,
				i,
				got[i],
				floor,
				largest,
			)
		}
	}
}

func TestProperty_SlicesSumTo10000(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		raw := rapid.SliceOfN(rapid.Uint64Range(0, 1<<50), 1, 40).Draw(t, "values")
		var total uint64
		for _, v := range raw {
			total += v
		}
		got, err := domain.Slices(micros(raw...), money.MicrosFromUint64(total))
		if err != nil {
			t.Fatalf("Slices(%v) = %v", raw, err)
		}
		switch want := uint64(10_000); {
		case total == 0 && sum(got) != 0:
			t.Fatalf("Slices of a zero total = %v, want all 0", got)
		case total != 0 && sum(got) != want:
			t.Fatalf("Slices(%v) = %v sums to %d, want %d", raw, got, sum(got), want)
		case total != 0:
			assertFloors(t, raw, got, total)
		}
	})
}

func TestSlices_RemainderGoesToTheFirstLargest(t *testing.T) {
	t.Parallel()
	got, err := domain.Slices(micros(1, 1, 1), money.MicrosFromUint64(3))
	if err != nil {
		t.Fatalf("Slices = %v", err)
	}
	if got[0] != 3334 || got[1] != 3333 || got[2] != 3333 {
		t.Fatalf("Slices = %v, want 3334 3333 3333", got)
	}
	if got, err = domain.Slices(nil, money.Micros{}); err != nil || len(got) != 0 {
		t.Fatalf("Slices(nil) = %v, %v", got, err)
	}
}

func TestSlices_AValueAboveTheTotalIsAnErrorNotAPanic(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		values []money.Micros
		total  uint64
	}{
		"just above":         {micros(4, 1), 3},
		"above a small one":  {micros(1, 9), 3},
		"hi word would trip": {micros(math.MaxUint64, 1), 10},
	} {
		got, err := domain.Slices(tc.values, money.MicrosFromUint64(tc.total))
		if errs.CodeOf(err) != errs.CodeInvalidInput || got != nil {
			t.Errorf("%s: Slices = %v, %v, want invalid_input", name, got, err)
		}
	}
}

func skipNone(ids.CabalID) {}

func recordSkips() (*[]ids.CabalID, func(ids.CabalID)) {
	var got []ids.CabalID
	return &got, func(id ids.CabalID) { got = append(got, id) }
}

func TestNewPortfolio_ACabalWithMoreSharesThanItsSnapshotIsSkippedAndReported(t *testing.T) {
	t.Parallel()
	now := bucketNowAt()
	a, b := newCabalID(), newCabalID()
	stakes := []domain.StakePoint{stake(a, now, 3, 1), stake(b, now, 2, 5)}
	latest := map[ids.CabalID]domain.Snapshot{a: potSnap(now, 5, 2), b: potSnap(now, 100, 4)}
	skipped, record := recordSkips()
	got, err := domain.NewPortfolio(stakes, latest, nil, record)
	if err != nil || len(got.Rows) != 1 || got.Rows[0].CabalID != b || got.Total != money.MicrosFromUint64(50) ||
		got.Rows[0].SliceBps != 10_000 {
		t.Fatalf("NewPortfolio = %+v, %v, want only the cabal %v at 50", got, err, b)
	}
	if len(*skipped) != 1 || (*skipped)[0] != a {
		t.Fatalf("skipped = %v, want [%v]", *skipped, a)
	}
}

func idsInOrder(n int) []ids.CabalID {
	out := make([]ids.CabalID, 0, n)
	for range n {
		out = append(out, newCabalID())
	}
	slices.SortFunc(out, compareIDs)
	return out
}

func compareIDs(a, b ids.CabalID) int {
	switch {
	case a.String() < b.String():
		return -1
	case a.String() > b.String():
		return 1
	}
	return 0
}

func assertBigRow(t *testing.T, row domain.PortfolioRow, cabal ids.CabalID) {
	t.Helper()
	if row.CabalID != cabal || row.Value.Uint64() != 2_000 || row.Shares.Uint64() != 10 || row.Net.Int64() != 1_000 ||
		row.PnL.Int64() != 1_000 || row.Return == nil || *row.Return != 10_000 || row.SliceBps != 8334 {
		t.Fatalf("big row = %+v, want the cabal's stake as of its snapshot", row)
	}
}

func TestNewPortfolio_RowsTotalsAndSlices(t *testing.T) {
	t.Parallel()
	now := bucketNowAt()
	cabals := idsInOrder(4)
	big, small, left, unvalued := cabals[0], cabals[1], cabals[2], cabals[3]
	stakes := []domain.StakePoint{
		stake(big, now.Add(-2*time.Hour), 10, 1_000), stake(big, now.Add(time.Hour), 20, 5_000),
		stake(small, now, 5, 500), stake(left, now.Add(-time.Hour), 5, 500), stake(left, now, 0, -100),
		stake(unvalued, now, 1, 1),
	}
	latest := map[ids.CabalID]domain.Snapshot{
		big: potSnap(now, 2_000, 10), small: potSnap(now, 400, 5), left: potSnap(now, 900, 5),
	}
	got, err := domain.NewPortfolio(stakes, latest, nil, skipNone)
	if err != nil || len(got.Rows) != 2 {
		t.Fatalf("NewPortfolio = %+v, %v, want the two held and valued cabals", got, err)
	}
	assertBigRow(t, got.Rows[0], big)
	if second := got.Rows[1]; second.CabalID != small || second.SliceBps != 1666 || second.PnL.Int64() != -100 {
		t.Fatalf("second = %+v", second)
	}
	if got.Total.Uint64() != 2_400 || got.PnL.Int64() != 900 || got.Return == nil || *got.Return != 6_000 {
		t.Fatalf("totals = %+v", got)
	}
}

func TestNewPortfolio_TiesBreakByCabalIDAndAnEmptyPortfolioHasNoReturn(t *testing.T) {
	t.Parallel()
	now := bucketNowAt()
	cabals := idsInOrder(2)
	stakes := []domain.StakePoint{stake(cabals[1], now, 1, 0), stake(cabals[0], now, 1, 0)}
	latest := map[ids.CabalID]domain.Snapshot{cabals[0]: potSnap(now, 50, 1), cabals[1]: potSnap(now, 50, 1)}
	got, err := domain.NewPortfolio(stakes, latest, nil, skipNone)
	if err != nil || got.Rows[0].CabalID != cabals[0] || got.Rows[0].SliceBps != 5000 || got.Return != nil {
		t.Fatalf("NewPortfolio = %+v, %v, want the lowest id first and no return with nothing contributed", got, err)
	}
	if empty, err := domain.NewPortfolio(
		nil,
		nil,
		nil,
		skipNone,
	); err != nil || empty.Rows == nil || len(empty.Rows) != 0 ||
		!empty.Total.IsZero() ||
		empty.Return != nil {
		t.Fatalf("empty = %+v, %v", empty, err)
	}
}

func TestNewPortfolio_Failures(t *testing.T) {
	t.Parallel()
	now := bucketNowAt()
	a, b, c := newCabalID(), newCabalID(), newCabalID()
	huge := uint64(math.MaxInt64)
	for name, tc := range map[string]struct {
		stakes []domain.StakePoint
		latest map[ids.CabalID]domain.Snapshot
	}{
		"a row's pnl does not fit": {
			stakes: []domain.StakePoint{stake(a, now, 1, 0)},
			latest: map[ids.CabalID]domain.Snapshot{a: potSnap(now, math.MaxUint64, 1)},
		},
		"the total overflows": {
			stakes: []domain.StakePoint{stake(a, now, 1, 0), stake(b, now, 1, 0), stake(c, now, 1, 0)},
			latest: map[ids.CabalID]domain.Snapshot{a: potSnap(now, huge, 1), b: potSnap(now, huge, 1), c: potSnap(now, huge, 1)},
		},
		"the net contributed overflows": {
			stakes: []domain.StakePoint{stake(a, now, 1, math.MaxInt64), stake(b, now, 1, math.MaxInt64)},
			latest: map[ids.CabalID]domain.Snapshot{a: potSnap(now, 1, 1), b: potSnap(now, 1, 1)},
		},
		"the total pnl does not fit": {
			stakes: []domain.StakePoint{stake(a, now, 1, 0), stake(b, now, 1, 0)},
			latest: map[ids.CabalID]domain.Snapshot{a: potSnap(now, huge, 1), b: potSnap(now, huge, 1)},
		},
	} {
		if _, err := domain.NewPortfolio(
			tc.stakes,
			tc.latest,
			nil,
			skipNone,
		); errs.CodeOf(
			err,
		) != errs.CodeInvalidInput {
			t.Errorf("%s: err = %v, want invalid_input", name, err)
		}
	}
}

type membershipCase struct {
	funded, noSnapshot, zeroShares, never, oversold ids.CabalID
	got                                             domain.Portfolio
	skipped                                         []ids.CabalID
}

func membershipPortfolio(t *testing.T) membershipCase {
	t.Helper()
	now := bucketNowAt()
	c := idsInOrder(5)
	m := membershipCase{funded: c[0], noSnapshot: c[1], zeroShares: c[2], never: c[3], oversold: c[4]}
	stakes := []domain.StakePoint{
		stake(m.funded, now, 5, 500), stake(m.noSnapshot, now, 1, 1), stake(m.zeroShares, now, 0, 0),
		stake(m.oversold, now, 3, 1),
	}
	latest := map[ids.CabalID]domain.Snapshot{
		m.funded: potSnap(now, 2_000, 10), m.zeroShares: potSnap(now, 700, 7), m.oversold: potSnap(now, 5, 2),
	}
	skipped, record := recordSkips()
	members := []ids.CabalID{m.never, m.zeroShares, m.funded, m.noSnapshot, m.oversold}
	var err error
	if m.got, err = domain.NewPortfolio(stakes, latest, members, record); err != nil {
		t.Fatalf("NewPortfolio: %v", err)
	}
	m.skipped = *skipped
	return m
}

func TestNewPortfolio_ListsEveryMembershipAfterTheValuedRowsAndTotalsIgnoreThem(t *testing.T) {
	t.Parallel()
	m := membershipPortfolio(t)
	if len(m.got.Rows) != 5 || m.got.Total.Uint64() != 1_000 || m.got.PnL.Int64() != 500 {
		t.Fatalf("NewPortfolio = %+v, want five rows and totals from the funded cabal only", m.got)
	}
	if first := m.got.Rows[0]; first.CabalID != m.funded || first.PotValue.Uint64() != 2_000 ||
		first.Value.Uint64() != 1_000 || first.SliceBps != 10_000 {
		t.Fatalf("first = %+v, want the funded cabal with its pot", first)
	}
	if len(m.skipped) != 1 || m.skipped[0] != m.oversold {
		t.Fatalf("skipped = %v, want [%v]", m.skipped, m.oversold)
	}
}

func TestNewPortfolio_AZeroRowKeepsItsPotAndNothingElse(t *testing.T) {
	t.Parallel()
	m := membershipPortfolio(t)
	wantPots := map[ids.CabalID]uint64{m.noSnapshot: 0, m.zeroShares: 700, m.never: 0, m.oversold: 5}
	zero := m.got.Rows[1:]
	if !slices.IsSortedFunc(zero, func(a, b domain.PortfolioRow) int { return compareIDs(a.CabalID, b.CabalID) }) {
		t.Errorf("zero rows = %v, want them in id order", zero)
	}
	for _, row := range zero {
		if row.PotValue.Uint64() != wantPots[row.CabalID] || !row.Value.IsZero() || !row.Shares.IsZero() ||
			!row.Net.IsZero() || !row.PnL.IsZero() || row.Return != nil || row.SliceBps != 0 || row.OwnershipBps != 0 {
			t.Errorf("zero row %v = %+v, want a zero row at pot %d", row.CabalID, row, wantPots[row.CabalID])
		}
	}
}

func TestNewPortfolio_AMembershipWithNoStakesIsAZeroRow(t *testing.T) {
	t.Parallel()
	only := newCabalID()
	got, err := domain.NewPortfolio(nil, nil, []ids.CabalID{only, only}, skipNone)
	if err != nil || len(got.Rows) != 1 || got.Rows[0].CabalID != only || !got.Total.IsZero() || got.Return != nil {
		t.Fatalf("NewPortfolio = %+v, %v, want one zero row and no return", got, err)
	}
}
