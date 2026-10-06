package domain_test

import (
	"math"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func newCabalID() ids.CabalID { return ids.CabalIDFrom(ids.Real{}.NewV7()) }

func stake(cabal ids.CabalID, at time.Time, shares uint64, net int64) domain.StakePoint {
	return domain.StakePoint{
		CabalID: cabal, At: at, Shares: money.SharesUnitsFromUint64(shares), Net: money.SignedMicrosFromInt64(net),
	}
}

func potSnap(at time.Time, value, total uint64) domain.Snapshot {
	return domain.Snapshot{
		At:          at,
		Value:       money.MicrosFromUint64(value),
		TotalShares: money.SharesUnitsFromUint64(total),
	}
}

func hourly(from time.Time, hours int, value, total uint64) []domain.Snapshot {
	out := make([]domain.Snapshot, 0, hours)
	for h := range hours {
		out = append(out, potSnap(from.Add(time.Duration(h)*time.Hour), value, total))
	}
	return out
}

func TestPnLCurve_EmptyWithoutStakes(t *testing.T) {
	t.Parallel()
	got, err := domain.PnLCurve(domain.Range1D, bucketNowAt(), nil, nil)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("PnLCurve = %#v, %v, want an empty slice", got, err)
	}
}

func TestPnLCurve_AFundMidRangeAtAFlatPotIsNotAGain(t *testing.T) {
	t.Parallel()
	now := bucketNowAt()
	cabal := newCabalID()
	fund := now.Add(-90 * time.Minute)
	stakes := []domain.StakePoint{stake(cabal, now.Add(-4*time.Hour), 100, 100_000), stake(cabal, fund, 200, 200_000)}
	before := hourly(now.Add(-4*time.Hour), 3, 100_000, 100)
	after := hourly(now.Add(-time.Hour), 2, 200_000, 200)
	snaps := map[ids.CabalID][]domain.Snapshot{cabal: append(before, after...)}
	got, err := domain.PnLCurve(domain.Range1D, now, stakes, snaps)
	if err != nil || len(got) == 0 {
		t.Fatalf("PnLCurve = %d points, %v", len(got), err)
	}
	for _, p := range got {
		if !p.PnL.IsZero() {
			t.Fatalf("point %v pnl = %d, want 0 across the fund", p.At, p.PnL.Int64())
		}
	}
	if got[0].Equity.Uint64() != 100_000 || got[len(got)-1].Equity.Uint64() != 200_000 {
		t.Fatalf("equity = %d..%d, want 100000..200000", got[0].Equity.Uint64(), got[len(got)-1].Equity.Uint64())
	}
}

func TestPnLCurve_CabalsSumAndALeftCabalKeepsItsRealisedGain(t *testing.T) {
	t.Parallel()
	now := bucketNowAt()
	kept, left := newCabalID(), newCabalID()
	from := now.Add(-3 * time.Hour)
	stakes := []domain.StakePoint{
		stake(kept, from, 10, 1_000), stake(left, from, 5, 500), stake(left, now.Add(-90*time.Minute), 0, -300),
	}
	snaps := map[ids.CabalID][]domain.Snapshot{kept: hourly(from, 4, 2_000, 10), left: hourly(from, 4, 800, 5)}
	got, err := domain.PnLCurve(domain.Range1D, now, stakes, snaps)
	if err != nil || len(got) == 0 {
		t.Fatalf("PnLCurve = %d points, %v", len(got), err)
	}
	if first := got[0]; first.Equity.Uint64() != 2_800 || first.PnL.Int64() != 2_800-1_500 {
		t.Fatalf("while held = %+v, want both cabals", first)
	}
	if last := got[len(got)-1]; last.Equity.Uint64() != 2_000 || last.PnL.Int64() != 2_000-(1_000-300) {
		t.Fatalf("after the leave = %+v, want only the kept cabal's equity and the realised gain", last)
	}
}

func TestPnLCurve_SkipsBucketsWithoutASnapshotForAHeldCabal(t *testing.T) {
	t.Parallel()
	now := bucketNowAt()
	cabal := newCabalID()
	stakes := []domain.StakePoint{stake(cabal, now.Add(-3*time.Hour), 10, 1_000)}
	snaps := map[ids.CabalID][]domain.Snapshot{cabal: hourly(now.Add(-time.Hour), 2, 1_500, 10)}
	got, err := domain.PnLCurve(domain.Range1D, now, stakes, snaps)
	if err != nil || len(got) == 0 || got[0].At.Before(now.Add(-time.Hour)) {
		t.Fatalf("PnLCurve = %+v, %v, want points only from the first snapshot", got, err)
	}
	late := map[ids.CabalID][]domain.Snapshot{cabal: {potSnap(now.Add(-4*time.Hour), 900, 10)}}
	before := []domain.StakePoint{stake(cabal, now.Add(-5*time.Hour), 10, 1_000)}
	if got, err = domain.PnLCurve(domain.Range1D, now, before, late); err != nil || len(got) == 0 ||
		got[len(got)-1].Equity.Uint64() != 900 {
		t.Fatalf("PnLCurve = %+v, %v, want the older snapshot to keep serving", got, err)
	}
	unstaked := []domain.StakePoint{stake(cabal, now, 10, 1_000)}
	if got, err = domain.PnLCurve(domain.Range1H, now, unstaked, late); err != nil || len(got) != 0 {
		t.Fatalf("PnLCurve = %+v, %v, want no point when the stake began after the snapshot", got, err)
	}
}

func TestPnLCurve_Failures(t *testing.T) {
	t.Parallel()
	now := bucketNowAt()
	a, b, c := newCabalID(), newCabalID(), newCabalID()
	big := uint64(math.MaxInt64)
	for name, tc := range map[string]struct {
		stakes []domain.StakePoint
		snaps  map[ids.CabalID][]domain.Snapshot
	}{
		"more shares than the pot has": {
			stakes: []domain.StakePoint{stake(a, now, 11, 1)},
			snaps:  map[ids.CabalID][]domain.Snapshot{a: {potSnap(now, 5, 10)}},
		},
		"net contributed overflows": {
			stakes: []domain.StakePoint{stake(a, now, 1, math.MaxInt64), stake(b, now, 1, math.MaxInt64)},
			snaps:  map[ids.CabalID][]domain.Snapshot{a: {potSnap(now, 1, 1)}, b: {potSnap(now, 1, 1)}},
		},
		"equity overflows": {
			stakes: []domain.StakePoint{stake(a, now, 1, 0), stake(b, now, 1, 0), stake(c, now, 1, 0)},
			snaps: map[ids.CabalID][]domain.Snapshot{
				a: {potSnap(now, big, 1)}, b: {potSnap(now, big, 1)}, c: {potSnap(now, big, 1)},
			},
		},
		"pnl does not fit": {
			stakes: []domain.StakePoint{stake(a, now, 1, 0), stake(b, now, 1, 0)},
			snaps:  map[ids.CabalID][]domain.Snapshot{a: {potSnap(now, big, 1)}, b: {potSnap(now, big, 1)}},
		},
	} {
		if _, err := domain.PnLCurve(
			domain.Range1H,
			now,
			tc.stakes,
			tc.snaps,
		); errs.CodeOf(
			err,
		) != errs.CodeInvalidInput {
			t.Errorf("%s: err = %v, want invalid_input", name, err)
		}
	}
	negative := []domain.StakePoint{stake(a, now, 1, math.MinInt64), stake(b, now, 1, -1)}
	snaps := map[ids.CabalID][]domain.Snapshot{a: {potSnap(now, 1, 1)}, b: {potSnap(now, 1, 1)}}
	if _, err := domain.PnLCurve(domain.Range1H, now, negative, snaps); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Errorf("net underflows: err = %v, want invalid_input", err)
	}
}
