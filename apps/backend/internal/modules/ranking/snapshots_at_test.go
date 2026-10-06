package ranking_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func insertSnapshot(t *testing.T, q *sqlc.Queries, cabal ids.CabalID, at time.Time, value int64) {
	t.Helper()
	if err := q.InsertCabalValueSnapshot(t.Context(), sqlc.InsertCabalValueSnapshotParams{
		CabalID: cabal.UUID(), At: at, ValueMicros: value, NavPerShareMicros: value / 100, TotalShares: 100,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotsAt_LatestAtOrBeforeEachBucket(t *testing.T) {
	t.Parallel()
	q := sqlc.New(testkit.DB(t))
	old, young := ids.CabalIDFrom(ids.Real{}.NewV7()), ids.CabalIDFrom(ids.Real{}.NewV7())
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	insertSnapshot(t, q, old, base, 100)
	insertSnapshot(t, q, old, base.Add(time.Hour), 200)
	insertSnapshot(t, q, old, base.Add(3*time.Hour), 300)
	insertSnapshot(t, q, young, base.Add(2*time.Hour), 50)
	ts := []time.Time{base.Add(-time.Minute), base.Add(time.Hour), base.Add(2 * time.Hour), base.Add(4 * time.Hour)}
	rows, err := q.SnapshotsAt(t.Context(), ts)
	if err != nil {
		t.Fatal(err)
	}
	type key struct {
		cabal  ids.CabalID
		bucket int32
	}
	got := map[key]int64{}
	for _, row := range rows {
		got[key{ids.CabalIDFrom(row.CabalID), row.Bucket}] = row.ValueMicros
	}
	want := map[key]int64{
		{old, 1}: 200, {old, 2}: 200, {old, 3}: 300,
		{young, 2}: 50, {young, 3}: 50,
	}
	if len(got) != len(want) || len(rows) != len(want) {
		t.Fatalf("SnapshotsAt = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("SnapshotsAt = %v, want %v", got, want)
		}
	}
}

func TestSnapshotsAt_NoTimesNoRows(t *testing.T) {
	t.Parallel()
	rows, err := sqlc.New(testkit.DB(t)).SnapshotsAt(t.Context(), nil)
	if err != nil || len(rows) != 0 {
		t.Fatalf("SnapshotsAt(nil) = %v, %v", rows, err)
	}
}
