package domain_test

import (
	"math"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func curveSnap(at time.Time, value uint64) domain.Snapshot {
	return domain.Snapshot{
		At: at, Value: money.MicrosFromUint64(value), NavPerShare: money.MicrosFromUint64(value / 100),
		TotalShares: money.SharesUnitsFromUint64(100),
	}
}

func net(at time.Time, v int64) domain.Contribution {
	return domain.Contribution{At: at, Net: money.SignedMicrosFromInt64(v)}
}

func TestValueCurve_EmptyWithoutSnapshots(t *testing.T) {
	t.Parallel()
	got, err := domain.ValueCurve(domain.Range1D, bucketNowAt(), nil, nil)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("ValueCurve = %#v, %v, want an empty slice", got, err)
	}
}

func TestValueCurve_TakesTheLastSnapshotInEachBucketAndSkipsEarlyOnes(t *testing.T) {
	t.Parallel()
	now := bucketNowAt()
	snaps := []domain.Snapshot{
		curveSnap(now.Add(-70*time.Minute), 1_000),
		curveSnap(now.Add(-25*time.Minute), 2_000),
		curveSnap(now.Add(-24*time.Minute), 3_000),
	}
	got, err := domain.ValueCurve(domain.Range1H, now, snaps, nil)
	if err != nil || len(got) != 31 {
		t.Fatalf("ValueCurve = %d points, %v, want 31", len(got), err)
	}
	if got[0].Value.Uint64() != 1_000 || got[0].PnL.Int64() != 1_000 {
		t.Fatalf("first = %+v, want the snapshot from before the range with no contribution", got[0])
	}
	if last := got[len(got)-1]; !last.At.Equal(now) || last.Value.Uint64() != 3_000 || last.NavPerShare.Uint64() != 30 {
		t.Fatalf("last = %+v, want the newest snapshot at now", last)
	}
	young, err := domain.ValueCurve(domain.Range1H, now, snaps[1:], nil)
	if err != nil || len(young) != 14 || !young[0].At.Equal(snaps[1].At) || young[0].Value.Uint64() != 2_000 {
		t.Fatalf("young = %d points, %v, want the first snapshot and the 13 buckets after it", len(young), err)
	}
}

func TestValueCurve_AFundMidRangeIsNotAGain(t *testing.T) {
	t.Parallel()
	now := bucketNowAt()
	fund := now.Add(-30 * time.Minute)
	snaps := []domain.Snapshot{
		curveSnap(now.Add(-58*time.Minute), 1_000), curveSnap(now.Add(-32*time.Minute), 1_000),
		curveSnap(fund.Add(2*time.Minute), 3_000), curveSnap(now, 3_000),
	}
	contributed := []domain.Contribution{net(now.Add(-time.Hour), 1_000), net(fund, 3_000)}
	got, err := domain.ValueCurve(domain.Range1H, now, snaps, contributed)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range got {
		if !p.PnL.IsZero() {
			t.Fatalf("point %v pnl = %d, want 0 across the fund", p.At, p.PnL.Int64())
		}
	}
	if got[0].Value.Uint64() != 1_000 || got[len(got)-1].Value.Uint64() != 3_000 {
		t.Fatalf("values = %d..%d, want 1000..3000", got[0].Value.Uint64(), got[len(got)-1].Value.Uint64())
	}
}

func TestValueCurve_ALLKeepsTheFirstSnapshotAndASignedLoss(t *testing.T) {
	t.Parallel()
	now := bucketNowAt()
	first := now.Add(-36 * time.Hour)
	snaps := []domain.Snapshot{curveSnap(first, 400), curveSnap(now, 900)}
	got, err := domain.ValueCurve(domain.RangeAll, now, snaps, []domain.Contribution{net(first, 500)})
	if err != nil || len(got) != 3 || !got[0].At.Equal(first) {
		t.Fatalf("ValueCurve = %+v, %v, want three points from the first snapshot", got, err)
	}
	if got[0].PnL.Int64() != -100 || got[2].PnL.Int64() != 400 {
		t.Fatalf("pnl = %d, %d, want -100 and 400", got[0].PnL.Int64(), got[2].PnL.Int64())
	}
}

func TestValueCurve_FailsWhenPnLDoesNotFit(t *testing.T) {
	t.Parallel()
	now := bucketNowAt()
	_, err := domain.ValueCurve(domain.Range1H, now,
		[]domain.Snapshot{curveSnap(now, math.MaxInt64)}, []domain.Contribution{net(now, math.MinInt64)})
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("err = %v, want invalid_input", err)
	}
}
