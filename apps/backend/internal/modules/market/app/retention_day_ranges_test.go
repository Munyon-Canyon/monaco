package app

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const rangeMint = "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp"

func retentionNow() time.Time { return time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC) }

func insertPrices(t *testing.T, pool sqlc.DBTX, at time.Time, minutes []int, micros []int64) {
	t.Helper()
	for i, m := range minutes {
		_, err := pool.Exec(t.Context(),
			`INSERT INTO price_points (mint, ts, price_micros, source) VALUES ($1, $2, $3, 'jupiter')`,
			rangeMint, at.Add(time.Duration(m)*time.Minute), micros[i])
		if err != nil {
			t.Fatal(err)
		}
	}
}

func dayRange(t *testing.T, pool sqlc.DBTX, day string) (high, low int64, source string, found bool) {
	t.Helper()
	rows, err := pool.Query(t.Context(),
		`SELECT high_micros, low_micros, source FROM price_day_ranges WHERE mint = $1 AND day = $2::date`,
		rangeMint, day)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		if err := rows.Scan(&high, &low, &source); err != nil {
			t.Fatal(err)
		}
		found = true
	}
	return high, low, source, found
}

func TestRetention_anIntradaySpikeThatClosesLowerSurvivesTheThinAsTheDaysHigh(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	clk := testkit.NewClock(retentionNow())
	r := NewRetention(db.New(pool, testkit.NewIDs(6473), clk), clk)
	day := time.Date(2026, time.October, 25, 14, 0, 0, 0, time.UTC)
	insertPrices(t, pool, day, []int{0, 10, 20, 30, 40},
		[]int64{200_000_000, 215_000_000, 230_000_000, 225_000_000, 220_000_000})
	insertPrices(t, pool, retentionNow().Add(-time.Hour), []int{0}, []int64{221_000_000})

	if _, err := r.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	var left int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM price_points WHERE ts < $1`, retentionNow().Add(-2*time.Hour)).Scan(&left)
	if err != nil || left != 1 {
		t.Fatalf("raw samples left from the old hour = %d (%v), want the one earliest", left, err)
	}
	high, low, source, found := dayRange(t, pool, "2026-10-25")
	if !found || high != 230_000_000 || low != 215_000_000 || source != "samples" {
		t.Fatalf("day range = %d..%d %s found=%v, want 215e6..230e6 from samples", low, high, source, found)
	}
}

func TestRetention_aSampleTheAcceptRuleRejectsNeverBecomesADaysHighOrLow(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	clk := testkit.NewClock(retentionNow())
	r := NewRetention(db.New(pool, testkit.NewIDs(6473), clk), clk)
	day := time.Date(2026, time.October, 25, 14, 0, 0, 0, time.UTC)
	insertPrices(t, pool, day, []int{0, 10, 20, 30},
		[]int64{200_000_000, 210_000_000, 900_000_000, 10_000_000})
	insertPrices(t, pool, retentionNow().Add(-time.Hour), []int{0}, []int64{206_000_000})

	if _, err := r.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	high, low, _, found := dayRange(t, pool, "2026-10-25")
	if !found || high != 210_000_000 || low != 210_000_000 {
		t.Fatalf("day range = %d..%d found=%v, want only the accepted 210e6, the spikes left out", low, high, found)
	}
}

func TestRetention_theHoursOfOneDayMergeIntoOneRange(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	clk := testkit.NewClock(retentionNow())
	r := NewRetention(db.New(pool, testkit.NewIDs(6473), clk), clk)
	day := time.Date(2026, time.October, 25, 14, 0, 0, 0, time.UTC)
	insertPrices(t, pool, day, []int{0, 10}, []int64{200_000_000, 230_000_000})
	insertPrices(t, pool, day, []int{120, 130}, []int64{205_000_000, 190_000_000})
	insertPrices(t, pool, retentionNow().Add(-time.Hour), []int{0}, []int64{200_000_000})

	if _, err := r.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	high, low, _, found := dayRange(t, pool, "2026-10-25")
	if !found || high != 230_000_000 || low != 190_000_000 {
		t.Fatalf("day range = %d..%d found=%v, want 190e6..230e6 across both hours", low, high, found)
	}
}

func countRows(t *testing.T, pool sqlc.DBTX) (samples, ranges int) {
	t.Helper()
	err := pool.QueryRow(t.Context(),
		`SELECT (SELECT count(*) FROM price_points), (SELECT count(*) FROM price_day_ranges)`).Scan(&samples, &ranges)
	if err != nil {
		t.Fatal(err)
	}
	return samples, ranges
}

func execAll(t *testing.T, pool sqlc.DBTX, stmts ...string) {
	t.Helper()
	for _, stmt := range stmts {
		if _, err := pool.Exec(t.Context(), stmt); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRetention_aFailedRollUpLeavesTheSamplesAndTheNextTickDoesBothOnce(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	clk := testkit.NewClock(retentionNow())
	r := NewRetention(db.New(pool, testkit.NewIDs(6473), clk), clk)
	day := time.Date(2026, time.October, 25, 14, 0, 0, 0, time.UTC)
	insertPrices(t, pool, day, []int{0, 10, 20}, []int64{200_000_000, 230_000_000, 220_000_000})
	insertPrices(t, pool, retentionNow().Add(-time.Hour), []int{0}, []int64{221_000_000})
	failFunction := `CREATE FUNCTION fail_day_ranges() RETURNS trigger LANGUAGE plpgsql
		AS $$ BEGIN RAISE EXCEPTION 'fault point'; END $$`
	failTrigger := `CREATE TRIGGER fail_day_ranges BEFORE INSERT ON price_day_ranges
		FOR EACH ROW EXECUTE FUNCTION fail_day_ranges()`
	execAll(t, pool, failFunction, failTrigger)

	if _, err := r.Tick(t.Context()); err == nil {
		t.Fatal("tick succeeded with a failing roll-up")
	}
	if samples, ranges := countRows(t, pool); samples != 4 || ranges != 0 {
		t.Fatalf("after the fault: %d samples, %d ranges, want 4 and 0", samples, ranges)
	}

	execAll(t, pool, `DROP TRIGGER fail_day_ranges ON price_day_ranges`)
	for range 2 {
		if _, err := r.Tick(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if samples, ranges := countRows(t, pool); samples != 2 || ranges != 1 {
		t.Fatalf("after the retry: %d samples, %d ranges, want 2 and 1", samples, ranges)
	}
	high, low, _, _ := dayRange(t, pool, "2026-10-25")
	if high != 230_000_000 || low != 220_000_000 {
		t.Fatalf("day range = %d..%d, want 220e6..230e6", low, high)
	}
}

func TestRetention_aColdMintsDeletedSamplesAreRolledUpToo(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	clk := testkit.NewClock(retentionNow())
	r := NewRetention(db.New(pool, testkit.NewIDs(6473), clk), clk)
	day := time.Date(2026, time.October, 2, 14, 0, 0, 0, time.UTC)
	insertPrices(t, pool, day, []int{0, 10}, []int64{100_000_000, 110_000_000})
	insertPrices(t, pool, retentionNow().Add(-time.Hour), []int{0}, []int64{105_000_000})

	if _, err := r.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	high, low, source, found := dayRange(t, pool, "2026-10-02")
	if !found || high != 110_000_000 || low != 100_000_000 || source != "samples" {
		t.Fatalf("day range = %d..%d %s found=%v, want 100e6..110e6", low, high, source, found)
	}
}
