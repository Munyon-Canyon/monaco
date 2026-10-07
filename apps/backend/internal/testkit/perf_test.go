package testkit_test

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func seedPlanThings(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := testkit.DB(t)
	for _, stmt := range []string{
		`CREATE TABLE plan_things (id int PRIMARY KEY, grp int NOT NULL)`,
		`INSERT INTO plan_things SELECT n, n % 10 FROM generate_series(1, 5000) n`,
		`VACUUM (ANALYZE) plan_things`,
	} {
		if _, err := pool.Exec(t.Context(), stmt); err != nil {
			t.Fatal(err)
		}
	}
	return pool
}

func TestPlanReportsTheIndexAndTheBuffersOfAPrimaryKeyLookup(t *testing.T) {
	t.Parallel()
	pool := seedPlanThings(t)
	p := testkit.Plan(t, pool, `SELECT id, grp FROM plan_things WHERE id = $1`, 2500)
	if !p.Uses("plan_things_pkey") || p.Uses("other_idx") || p.SeqScans("plan_things") {
		t.Fatalf("plan = %+v, want plan_things_pkey and no sequential scan", p)
	}
	testkit.AssertBuffers(t, "plan primary key lookup", p.Buffers())
}

func TestPlanReportsASequentialScanWhenNoIndexServesTheFilter(t *testing.T) {
	t.Parallel()
	pool := seedPlanThings(t)
	p := testkit.Plan(t, pool, `SELECT id FROM plan_things WHERE grp = $1`, 3)
	if !p.SeqScans("plan_things") || p.SeqScans("other_table") || p.Uses("plan_things_pkey") {
		t.Fatalf("plan = %+v, want a sequential scan of plan_things", p)
	}
	lookup := testkit.Plan(t, pool, `SELECT id FROM plan_things WHERE id = $1`, 1)
	if p.Buffers() <= lookup.Buffers() {
		t.Fatalf("scan read %d buffers, lookup read %d; want the scan to read more", p.Buffers(), lookup.Buffers())
	}
}

func TestPlanFindsAnIndexBelowTheRootNode(t *testing.T) {
	t.Parallel()
	pool := seedPlanThings(t)
	p := testkit.Plan(t, pool, `SELECT id, (SELECT count(*) FROM plan_things s WHERE s.id = t.id)
		FROM plan_things t WHERE grp = $1 LIMIT 3`, 4)
	if !p.Uses("plan_things_pkey") || !p.SeqScans("plan_things") {
		t.Fatalf("plan = %+v, want the pkey in a subplan and a scan of the outer table", p)
	}
}
