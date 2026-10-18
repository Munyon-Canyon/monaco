package app

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestRetention_secondBatchStartsAfterTheFirst(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	now := time.Date(2026, time.October, 17, 12, 0, 0, 0, time.UTC)
	clk := testkit.NewClock(now)
	r := NewRetention(db.New(pool, testkit.NewIDs(559), clk), clk)
	r.batchLimit = 2
	var afters []time.Time
	r.onBatch = func(after time.Time) { afters = append(afters, after) }

	aapl := "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp"
	tsla := "XsDoVfqeBukxuZHWhdvWHBhgEHjGNst4MLodqsJHzoB"
	base := now.Add(-3 * 24 * time.Hour)
	at := func(minutes int) time.Time { return base.Add(time.Duration(minutes) * 12 * time.Minute) }
	boundary := at(21)
	rows := []struct {
		mint   string
		ts     time.Time
		micros int64
	}{
		{aapl, at(0), 1},
		{aapl, at(5), 2},
		{aapl, at(10), 3},
		{aapl, at(15), 4},
		{aapl, at(16), 5},
		{aapl, at(20), 6},
		{tsla, at(20), 7},
		{aapl, boundary, 8},
		{tsla, boundary, 9},
		{aapl, at(25), 10},
		{aapl, at(26), 11},
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
	if !afters[1].After(at(10)) {
		t.Fatalf("second cursor %s does not move past the leading keepers", afters[1].Format(time.RFC3339))
	}
	var shared int
	err = pool.QueryRow(t.Context(), `SELECT count(*) FROM price_points WHERE ts = $1`, boundary).Scan(&shared)
	if err != nil || shared != 0 {
		t.Fatalf("rows left at the shared timestamp = %d (%v), want both mints deleted", shared, err)
	}
}
