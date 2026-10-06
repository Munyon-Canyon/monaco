package adapters

import (
	"testing"
	"time"
)

func TestSnapshotFrom(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	good, err := snapshotFrom(at, 9, 3, 4)
	if err != nil || good.Value.Uint64() != 9 || good.NavPerShare.Uint64() != 3 || good.TotalShares.Uint64() != 4 {
		t.Fatalf("snapshotFrom = %+v, %v", good, err)
	}
	for name, row := range map[string][3]int64{
		"value":  {-1, 1, 1},
		"nav":    {1, -1, 1},
		"shares": {1, 1, -1},
	} {
		if _, err := snapshotFrom(at, row[0], row[1], row[2]); err == nil {
			t.Errorf("%s: snapshotFrom accepted a negative amount", name)
		}
	}
}
