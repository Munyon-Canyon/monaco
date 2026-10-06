package app_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func usd(v uint64) money.Micros { return money.MicrosFromUint64(v) }

func signed(v int64) money.SignedMicros { return money.SignedMicrosFromInt64(v) }

func shares(v uint64) money.SharesUnits { return money.SharesUnitsFromUint64(v) }

func window(r domain.Range) app.Window {
	t1 := clock.Real{}.Now().UTC()
	t0, ok := r.Start(t1)
	if !ok {
		t0 = t1.Add(-7 * 24 * time.Hour)
	}
	return app.Window{Range: r, T0: t0, T1: t1}
}

func snapshotAt(w app.Window, value uint64, total uint64) *app.Snapshot {
	return &app.Snapshot{At: w.T0.Add(-time.Minute), Value: usd(value), TotalShares: shares(total)}
}

func wantGain(t *testing.T, r app.Ranged, pnl int64, bps domain.Bps) {
	t.Helper()
	gain, ret, err := r.Gain()
	if err != nil || gain != signed(pnl) || ret == nil || *ret != bps {
		t.Fatalf("Gain = %v, %v, %v, want %d micros and %d bps", gain, ret, err, pnl, bps)
	}
}

func TestMemberRanged_MidRangeDepositShowsNoFakeGain(t *testing.T) {
	t.Parallel()
	w := window(domain.Range1W)
	flows := []domain.Flow{{Amount: signed(100_000_000), At: w.T0.Add(3 * 24 * time.Hour)}}
	r, err := app.MemberRanged(w, shares(100), shares(100), snapshotAt(w, 100_000_000, 100), usd(200_000_000), flows)
	if err != nil || r.Start != usd(100_000_000) {
		t.Fatalf("MemberRanged = %+v, %v, want a start of 100 USDC", r, err)
	}
	wantGain(t, r, 0, 0)
}

func TestMemberRanged_StartsAtShareUnitsAtT0NotCurrent(t *testing.T) {
	t.Parallel()
	w := window(domain.Range1W)
	r, err := app.MemberRanged(w, shares(100), shares(10), snapshotAt(w, 1_000, 100), usd(150), nil)
	if err != nil || r.Start != usd(100) {
		t.Fatalf("MemberRanged = %+v, %v, want 10 of 100 shares of 1000", r, err)
	}
	wantGain(t, r, 50, 5_000)
}

func TestMemberRanged_CashOutInRangeIsNotALoss(t *testing.T) {
	t.Parallel()
	w := window(domain.Range1W)
	flows := []domain.Flow{{Amount: signed(-100_000_000), At: w.T0.Add(24 * time.Hour)}}
	r, err := app.MemberRanged(w, shares(100), shares(100), snapshotAt(w, 200_000_000, 100), usd(100_000_000), flows)
	if err != nil {
		t.Fatal(err)
	}
	wantGain(t, r, 0, 0)
}

func TestMemberRanged_CabalYoungerThanRangeStartsAtZero(t *testing.T) {
	t.Parallel()
	w := window(domain.Range1W)
	flows := []domain.Flow{{Amount: signed(100), At: w.T1.Add(-24 * time.Hour)}}
	r, err := app.MemberRanged(w, shares(0), shares(0), nil, usd(110), flows)
	if err != nil || !r.Start.IsZero() {
		t.Fatalf("MemberRanged = %+v, %v, want a zero start", r, err)
	}
	wantGain(t, r, 10, 7_000)
}

func TestMemberRanged_ZeroSharesIgnoreAStaleSnapshot(t *testing.T) {
	t.Parallel()
	w := window(domain.Range1H)
	stale := &app.Snapshot{At: w.T0.Add(-24 * time.Hour), Value: usd(100), TotalShares: shares(10)}
	r, err := app.MemberRanged(w, shares(10), shares(0), stale, usd(5), nil)
	if err != nil || !r.Start.IsZero() {
		t.Fatalf("MemberRanged = %+v, %v, want a zero start", r, err)
	}
}

func TestMemberRanged_UnusableStartIsASkip(t *testing.T) {
	t.Parallel()
	w := window(domain.Range1D)
	tests := map[string]struct {
		shares, ledger uint64
		snap           *app.Snapshot
	}{
		"shares at t0 but no snapshot": {shares: 5, ledger: 10},
		"snapshot older than the run cadence": {
			shares: 5, ledger: 10,
			snap: &app.Snapshot{
				At:          w.T0.Add(-domain.MaxSnapshotGap(w.Range) - time.Second),
				Value:       usd(100),
				TotalShares: shares(10),
			},
		},
		"snapshot after t0": {
			shares: 5, ledger: 10,
			snap: &app.Snapshot{At: w.T0.Add(time.Second), Value: usd(100), TotalShares: shares(10)},
		},
		"shares above the snapshot total":          {shares: 11, ledger: 10, snap: snapshotAt(w, 100, 10)},
		"a deposit between the snapshot and t0":    {shares: 100, ledger: 150, snap: snapshotAt(w, 100, 100)},
		"a withdrawal between the snapshot and t0": {shares: 20, ledger: 60, snap: snapshotAt(w, 100, 100)},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r, err := app.MemberRanged(w, shares(tt.ledger), shares(tt.shares), tt.snap, usd(100), nil)
			if !errors.Is(err, domain.ErrUnusableStart) || errs.CodeOf(err) != errs.CodeInvalidInput {
				t.Fatalf("MemberRanged error = %v, want ErrUnusableStart as invalid input", err)
			}
			if !r.End.IsZero() || !r.Start.IsZero() {
				t.Fatalf("MemberRanged = %+v, want no row to rank on a skip", r)
			}
		})
	}
}

func TestMemberRanged_SnapshotGapBoundIsInclusive(t *testing.T) {
	t.Parallel()
	for _, rng := range []domain.Range{domain.Range1H, domain.Range1D, domain.Range1W, domain.Range1M} {
		w := window(rng)
		edge := &app.Snapshot{At: w.T0.Add(-domain.MaxSnapshotGap(rng)), Value: usd(100), TotalShares: shares(10)}
		if _, err := app.MemberRanged(w, shares(10), shares(10), edge, usd(100), nil); err != nil {
			t.Fatalf("%s: MemberRanged at the gap bound = %v, want a usable start", rng, err)
		}
	}
}

func TestRanged_ADepositBetweenSnapshotAndT0IsASkipNotAFakeGain(t *testing.T) {
	t.Parallel()
	w := window(domain.Range1D)
	snap := snapshotAt(w, 100, 100)
	_, err := app.MemberRanged(w, shares(150), shares(100), snap, usd(200), nil)
	if !errors.Is(err, domain.ErrUnusableStart) {
		t.Fatalf(
			"MemberRanged = %v, want the member skipped: 100 of 150 shares at t0 is not 100 of the snapshot's 100",
			err,
		)
	}
	if _, err := app.CabalRanged(w, shares(150), snap, usd(200), nil); !errors.Is(err, domain.ErrUnusableStart) {
		t.Fatalf("CabalRanged = %v, want the cabal skipped when the snapshot total is not the ledger total", err)
	}
}

func TestRanged_AWindowThatDoesNotStartBeforeItEndsFails(t *testing.T) {
	t.Parallel()
	w := window(domain.Range1D)
	w.T1 = w.T0
	_, memberErr := app.MemberRanged(w, shares(10), shares(0), nil, usd(1), nil)
	_, cabalErr := app.CabalRanged(w, shares(0), nil, usd(1), nil)
	if memberErr == nil || errors.Is(memberErr, domain.ErrUnusableStart) || cabalErr == nil ||
		errors.Is(cabalErr, domain.ErrUnusableStart) {
		t.Fatalf("errors = %v, %v, want a real failure that is not a skip", memberErr, cabalErr)
	}
}

func TestRanged_NoStartAndNoFlowsIsNoGain(t *testing.T) {
	t.Parallel()
	w := window(domain.Range1D)
	r := app.Ranged{T0: w.T0, T1: w.T1, End: usd(500)}
	gain, ret, err := r.Gain()
	if err != nil || !gain.IsZero() || ret != nil {
		t.Fatalf("Gain = %v, %v, %v, want no gain and no return", gain, ret, err)
	}
}

func TestCabalRanged(t *testing.T) {
	t.Parallel()
	w := window(domain.Range1W)
	flows := []domain.Flow{{Amount: signed(50), At: w.T0.Add(time.Hour)}}
	r, err := app.CabalRanged(w, shares(100), snapshotAt(w, 100, 100), usd(150), flows)
	if err != nil || r.Start != usd(100) {
		t.Fatalf("CabalRanged = %+v, %v, want the snapshot value as the start", r, err)
	}
	wantGain(t, r, 0, 0)
	young, err := app.CabalRanged(w, shares(0), nil, usd(150), nil)
	if err != nil || !young.Start.IsZero() {
		t.Fatalf("CabalRanged = %+v, %v, want a zero start for a cabal with no shares at t0", young, err)
	}
}

func TestCabalRanged_UnusableStartIsASkip(t *testing.T) {
	t.Parallel()
	w := window(domain.Range1W)
	stale := &app.Snapshot{At: w.T0.Add(-24 * time.Hour), Value: usd(100), TotalShares: shares(100)}
	for name, snap := range map[string]*app.Snapshot{"no snapshot": nil, "stale snapshot": stale} {
		if _, err := app.CabalRanged(w, shares(100), snap, usd(150), nil); !errors.Is(err, domain.ErrUnusableStart) {
			t.Fatalf("%s: CabalRanged error = %v, want ErrUnusableStart", name, err)
		}
	}
}

func TestMemberStartsSumToTheCabalStart(t *testing.T) {
	t.Parallel()
	w := window(domain.Range1D)
	snap := snapshotAt(w, 1_000_003, 7)
	holders := []uint64{1, 2, 4}
	members := make([]app.Ranged, 0, len(holders))
	for _, held := range holders {
		r, err := app.MemberRanged(w, shares(7), shares(held), snap, usd(0), nil)
		if err != nil {
			t.Fatal(err)
		}
		members = append(members, r)
	}
	sum, err := app.SumRanged(w.T0, w.T1, members)
	if err != nil {
		t.Fatal(err)
	}
	cabal, err := app.CabalRanged(w, shares(7), snap, usd(0), nil)
	if err != nil {
		t.Fatal(err)
	}
	gap, err := cabal.Start.Delta(sum.Start)
	if diff := gap.Int64(); err != nil || diff < 0 || diff >= int64(len(holders)) {
		t.Fatalf("members start at %v, cabal at %v, want a gap below one micro per member", sum.Start, cabal.Start)
	}
}

func TestRangeFlows_KeepsOnlyTheRangeAndGroupsThem(t *testing.T) {
	t.Parallel()
	w := window(domain.Range1W)
	t0, t1 := w.T0, w.T1
	g := ids.Real{}
	alice, bob := ids.UserIDFrom(g.NewV7()), ids.UserIDFrom(g.NewV7())
	one, two := ids.CabalIDFrom(g.NewV7()), ids.CabalIDFrom(g.NewV7())
	in := t0.Add(time.Hour)
	flows := []treasury.MemberFlow{
		{UserID: alice, CabalID: one, Amount: signed(5), At: t0},
		{UserID: alice, CabalID: one, Amount: signed(10), At: in},
		{UserID: bob, CabalID: one, Amount: signed(-3), At: t1},
		{UserID: alice, CabalID: two, Amount: signed(7), At: in},
		{UserID: bob, CabalID: two, Amount: signed(9), At: t1.Add(time.Nanosecond)},
	}
	byMember, byCabal := app.RangeFlows(flows, t0, t1)
	if len(byMember) != 3 || len(byMember[app.MemberKey{UserID: alice, CabalID: one}]) != 1 {
		t.Fatalf("byMember = %+v, want three members with the flow at t0 and the flow after t1 left out", byMember)
	}
	if got := byCabal[one]; len(got) != 2 || got[0].Amount != signed(10) || got[1].Amount != signed(-3) {
		t.Fatalf("byCabal[one] = %+v, want the two flows in the range", got)
	}
	if len(byCabal[two]) != 1 {
		t.Fatalf("byCabal[two] = %+v, want one flow", byCabal[two])
	}
}

func TestSumRanged_WeighsEveryFlowOnce(t *testing.T) {
	t.Parallel()
	w := window(domain.Range1W)
	t0, t1 := w.T0, w.T1
	deposit := domain.Flow{Amount: signed(100_000_000), At: t0.Add(3 * 24 * time.Hour)}
	parts := []app.Ranged{
		{T0: t0, T1: t1, Start: usd(100_000_000), End: usd(100_000_000)},
		{T0: t0, T1: t1, End: usd(100_000_000), Flows: []domain.Flow{deposit}},
	}
	sum, err := app.SumRanged(t0, t1, parts)
	if err != nil || sum.Start != usd(100_000_000) || sum.End != usd(200_000_000) || len(sum.Flows) != 1 {
		t.Fatalf("SumRanged = %+v, %v, want start 100, end 200 and the one flow", sum, err)
	}
	wantGain(t, sum, 0, 0)
}

func TestSumRanged_Overflow(t *testing.T) {
	t.Parallel()
	w := window(domain.Range1W)
	t0, t1 := w.T0, w.T1
	maxed := app.Ranged{T0: t0, T1: t1, Start: usd(math.MaxUint64)}
	one := app.Ranged{T0: t0, T1: t1, Start: usd(1)}
	if _, err := app.SumRanged(t0, t1, []app.Ranged{maxed, one}); err == nil {
		t.Fatal("SumRanged start overflow = nil error")
	}
	maxed, one = app.Ranged{T0: t0, T1: t1, End: usd(math.MaxUint64)}, app.Ranged{T0: t0, T1: t1, End: usd(1)}
	if _, err := app.SumRanged(t0, t1, []app.Ranged{maxed, one}); err == nil {
		t.Fatal("SumRanged end overflow = nil error")
	}
}

type rangedFixture struct {
	users  []ids.UserID
	cabals []ids.CabalID
	flows  []treasury.MemberFlow
}

func newRangedFixture(cabals, users int) rangedFixture {
	g := ids.Real{}
	now := clock.Real{}.Now().UTC()
	f := rangedFixture{
		users:  make([]ids.UserID, users),
		cabals: make([]ids.CabalID, cabals),
		flows:  make([]treasury.MemberFlow, users),
	}
	for i := range f.cabals {
		f.cabals[i] = ids.CabalIDFrom(g.NewV7())
	}
	for i := range f.users {
		f.users[i] = ids.UserIDFrom(g.NewV7())
		f.flows[i] = treasury.MemberFlow{
			UserID: f.users[i], CabalID: f.cabals[i%cabals], Amount: signed(1_000_000),
			At: now.Add(-time.Duration(i%600) * time.Minute),
		}
	}
	return f
}

func (f rangedFixture) compute(b *testing.B, rng domain.Range) {
	b.Helper()
	w := window(rng)
	byMember, byCabal := app.RangeFlows(f.flows, w.T0, w.T1)
	snap := snapshotAt(w, 100_000_000, 10)
	for i, user := range f.users {
		key := app.MemberKey{UserID: user, CabalID: f.cabals[i%len(f.cabals)]}
		r, err := app.MemberRanged(w, shares(10), shares(1), snap, usd(11_000_000), byMember[key])
		if err != nil {
			b.Fatal(err)
		}
		if _, _, err := r.Gain(); err != nil {
			b.Fatal(err)
		}
	}
	for _, cabal := range f.cabals {
		r, err := app.CabalRanged(w, shares(10), snap, usd(110_000_000), byCabal[cabal])
		if err != nil {
			b.Fatal(err)
		}
		if _, _, err := r.Gain(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRangedCompute_500Cabals_5000Members(b *testing.B) {
	f := newRangedFixture(500, 5000)
	b.ResetTimer()
	for b.Loop() {
		for _, rng := range []domain.Range{domain.Range1H, domain.Range1D, domain.Range1W, domain.Range1M} {
			f.compute(b, rng)
		}
	}
}
