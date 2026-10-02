package app

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestRetention_secondBatchStartsAfterTheFirst(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	now := clock.Real{}.Now().UTC().Truncate(time.Hour)
	clk := testkit.NewClock(now)
	r := NewRetention(db.New(pool, testkit.NewIDs(559), clk), clk)
	r.batchLimit = 2
	var afters []time.Time
	r.onBatch = func(after time.Time) { afters = append(afters, after) }

	aapl := "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp"
	tsla := "XsDoVfqeBukxuZHWhdvWHBhgEHjGNst4MLodqsJHzoB"
	base := now.Add(-10 * 24 * time.Hour)
	boundary := base.Add(21 * time.Minute)
	rows := []struct {
		mint   string
		ts     time.Time
		micros int64
	}{
		{aapl, base, 1},
		{aapl, base.Add(5 * time.Minute), 2},
		{aapl, base.Add(10 * time.Minute), 3},
		{aapl, base.Add(15 * time.Minute), 4},
		{aapl, base.Add(16 * time.Minute), 5},
		{aapl, base.Add(20 * time.Minute), 6},
		{tsla, base.Add(20 * time.Minute), 7},
		{aapl, boundary, 8},
		{tsla, boundary, 9},
		{aapl, base.Add(25 * time.Minute), 10},
		{aapl, base.Add(26 * time.Minute), 11},
		{aapl, now.Add(-time.Hour), 12},
		{tsla, now.Add(-time.Hour), 13},
	}
	for _, row := range rows {
		_, err := pool.Exec(t.Context(),
			`INSERT INTO price_points (mint, ts, price_micros, source) VALUES ($1, $2, $3, 'jupiter')`,
			row.mint, row.ts, row.micros)
		if err != nil {
			t.Fatal(err)
		}
	}
	report, err := r.Tick(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if report.Changed != 4 {
		t.Fatalf("Changed = %d, want the four later samples", report.Changed)
	}
	if len(afters) < 2 || !afters[0].IsZero() || !afters[1].Equal(boundary) {
		t.Fatalf("batch cursors = %v, want the second batch to start at %s", afters, boundary.Format(time.RFC3339))
	}
	if !afters[1].After(base.Add(10 * time.Minute)) {
		t.Fatalf("second cursor %s does not move past the leading keepers", afters[1].Format(time.RFC3339))
	}
	var shared int
	err = pool.QueryRow(t.Context(), `SELECT count(*) FROM price_points WHERE ts = $1`, boundary).Scan(&shared)
	if err != nil || shared != 0 {
		t.Fatalf("rows left at the shared timestamp = %d (%v), want both mints deleted", shared, err)
	}
}
