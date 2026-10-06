package app_test

import (
	"math"
	"testing"
	"time"

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

func window() (time.Time, time.Time) {
	t1 := clock.Real{}.Now().UTC()
	return t1.Add(-7 * 24 * time.Hour), t1
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
	t0, t1 := window()
	snap := &app.Snapshot{Value: usd(100_000_000), TotalShares: shares(100)}
	flows := []domain.Flow{{Amount: signed(100_000_000), At: t0.Add(3 * 24 * time.Hour)}}
	r, err := app.MemberRanged(t0, t1, shares(100), snap, usd(200_000_000), flows)
	if err != nil || r.Start != usd(100_000_000) {
		t.Fatalf("MemberRanged = %+v, %v, want a start of 100 USDC", r, err)
	}
	wantGain(t, r, 0, 0)
}

func TestMemberRanged_StartsAtShareUnitsAtT0NotCurrent(t *testing.T) {
	t.Parallel()
	t0, t1 := window()
	snap := &app.Snapshot{Value: usd(1_000), TotalShares: shares(100)}
	r, err := app.MemberRanged(t0, t1, shares(10), snap, usd(150), nil)
	if err != nil || r.Start != usd(100) {
		t.Fatalf("MemberRanged = %+v, %v, want 10 of 100 shares of 1000", r, err)
	}
	wantGain(t, r, 50, 5_000)
}

func TestMemberRanged_CashOutInRangeIsNotALoss(t *testing.T) {
	t.Parallel()
	t0, t1 := window()
	snap := &app.Snapshot{Value: usd(200_000_000), TotalShares: shares(100)}
	flows := []domain.Flow{{Amount: signed(-100_000_000), At: t0.Add(24 * time.Hour)}}
	r, err := app.MemberRanged(t0, t1, shares(100), snap, usd(100_000_000), flows)
	if err != nil {
		t.Fatal(err)
	}
	wantGain(t, r, 0, 0)
}

func TestMemberRanged_CabalYoungerThanRangeStartsAtZero(t *testing.T) {
	t.Parallel()
	t0, t1 := window()
	flows := []domain.Flow{{Amount: signed(100), At: t1.Add(-24 * time.Hour)}}
	r, err := app.MemberRanged(t0, t1, shares(0), nil, usd(110), flows)
	if err != nil || !r.Start.IsZero() {
		t.Fatalf("MemberRanged = %+v, %v, want a zero start", r, err)
	}
	wantGain(t, r, 10, 7_000)
}

func TestMemberRanged_SharesAboveSnapshotTotal(t *testing.T) {
	t.Parallel()
	t0, t1 := window()
	snap := &app.Snapshot{Value: usd(100), TotalShares: shares(10)}
	if _, err := app.MemberRanged(t0, t1, shares(11), snap, usd(100), nil); err == nil {
		t.Fatal("MemberRanged = nil error, want shares above the snapshot total to fail")
	}
}

func TestCabalRanged(t *testing.T) {
	t.Parallel()
	t0, t1 := window()
	flows := []domain.Flow{{Amount: signed(50), At: t0.Add(time.Hour)}}
	r := app.CabalRanged(t0, t1, &app.Snapshot{Value: usd(100)}, usd(150), flows)
	if r.Start != usd(100) {
		t.Fatalf("CabalRanged start = %v, want the snapshot value", r.Start)
	}
	wantGain(t, r, 0, 0)
	young := app.CabalRanged(t0, t1, nil, usd(150), nil)
	if !young.Start.IsZero() {
		t.Fatalf("CabalRanged start = %v, want zero without a snapshot", young.Start)
	}
}

func TestRangeFlows_KeepsOnlyTheRangeAndGroupsThem(t *testing.T) {
	t.Parallel()
	t0, t1 := window()
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
	t0, t1 := window()
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
	t0, t1 := window()
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
