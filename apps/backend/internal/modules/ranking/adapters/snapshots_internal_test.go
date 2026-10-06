package adapters

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
)

func TestSnapshotFrom(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	good, err := snapshotFrom(sqlc.SnapshotsSinceRow{At: at, ValueMicros: 9, NavPerShareMicros: 3, TotalShares: 4})
	if err != nil || good.Value.Uint64() != 9 || good.NavPerShare.Uint64() != 3 || good.TotalShares.Uint64() != 4 {
		t.Fatalf("snapshotFrom = %+v, %v", good, err)
	}
	for name, row := range map[string]sqlc.SnapshotsSinceRow{
		"value":  {ValueMicros: -1},
		"nav":    {NavPerShareMicros: -1},
		"shares": {TotalShares: -1},
	} {
		if _, err := snapshotFrom(row); err == nil {
			t.Errorf("%s: snapshotFrom accepted a negative amount", name)
		}
	}
}
