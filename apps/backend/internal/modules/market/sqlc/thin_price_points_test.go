package sqlc

import (
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestThinPricePoints_readsThroughTheTimestampIndex(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	now := clock.Real{}.Now().UTC().Truncate(time.Hour)
	mint := "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp"
	old := now.Add(-10 * 24 * time.Hour)
	_, err := pool.Exec(t.Context(), `INSERT INTO price_points (mint, ts, price_micros, source)
		VALUES ($1, $2, 1, 'jupiter'), ($1, $3, 2, 'jupiter')`, mint, old, old.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pool.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err := conn.Exec(t.Context(), `SET enable_seqscan = off`); err != nil {
		t.Fatal(err)
	}
	rows, err := conn.Query(t.Context(), "EXPLAIN "+ThinPricePointsSQL(),
		time.Time{}, now.Add(-7*24*time.Hour), "5 minutes", int32(1000))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.String(), "price_points_ts_idx") {
		t.Fatalf("plan =\n%s\nwant price_points_ts_idx", plan.String())
	}
}
