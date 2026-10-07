package testkit

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	BaselineFile   = "testdata/perf/baseline.json"
	AllocsTestName = "TestAllocs"
	allocRuns      = 100
)

var errPlanCount = errors.New("EXPLAIN returned a number of plans other than 1")

var (
	perfUpdate = flag.Bool("testkit.perf-update", false, "rewrite "+BaselineFile+" with the measured counts")
	baselineMu sync.Mutex
)

type baseline struct {
	Allocs  map[string]int64 `json:"allocs"`
	Queries map[string]int64 `json:"queries"`
	Buffers map[string]int64 `json:"buffers,omitempty"`
}

type PlanNode struct {
	NodeType     string     `json:"Node Type"`
	RelationName string     `json:"Relation Name"`
	IndexName    string     `json:"Index Name"`
	SharedHit    int64      `json:"Shared Hit Blocks"`
	SharedRead   int64      `json:"Shared Read Blocks"`
	Plans        []PlanNode `json:"Plans"`
}

func (p PlanNode) Buffers() int64 { return p.SharedHit + p.SharedRead }

func (p PlanNode) Uses(index string) bool {
	if p.IndexName == index {
		return true
	}
	for _, child := range p.Plans {
		if child.Uses(index) {
			return true
		}
	}
	return false
}

func (p PlanNode) SeqScans(table string) bool {
	if p.NodeType == "Seq Scan" && p.RelationName == table {
		return true
	}
	for _, child := range p.Plans {
		if child.SeqScans(table) {
			return true
		}
	}
	return false
}

type queryCounter struct {
	n atomic.Int64
}

var _ pgx.QueryTracer = (*queryCounter)(nil)

func (c *queryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	c.n.Add(1)
	return ctx
}

func (c *queryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func AssertAllocs(t *testing.T, name string, fn func()) {
	t.Helper()
	if !strings.HasPrefix(t.Name(), AllocsTestName) {
		t.Fatalf("testkit.AssertAllocs: %s must be named %s* in allocs_test.go, "+
			"so scripts/test-backend.sh runs it in its non-race, non-parallel pass", t.Name(), AllocsTestName)
	}
	if raceEnabled {
		t.Skip("testkit.AssertAllocs: -race makes sync.Pool drop items at random; the non-race pass checks this")
	}
	compareBaseline(t, BaselineFile, *perfUpdate, "allocs", name, int64(testing.AllocsPerRun(allocRuns, fn)))
}

func AssertQueries(t *testing.T, name string, fn func()) {
	t.Helper()
	s := current.Load()
	if s == nil {
		t.Fatal("testkit.AssertQueries: call testkit.Main(m) from this package's TestMain")
	}
	c := s.counter(t.Name())
	if c == nil {
		t.Fatalf("testkit.AssertQueries: %s has no testkit.DB; count queries on the database this test owns", t.Name())
	}
	before := c.n.Load()
	fn()
	compareBaseline(t, BaselineFile, *perfUpdate, "queries", name, c.n.Load()-before)
}

func Plan(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) PlanNode {
	t.Helper()
	return explain(t.Context(), t, pool, sql, args)
}

func AssertBuffers(t *testing.T, name string, got int64) {
	t.Helper()
	compareBaseline(t, BaselineFile, *perfUpdate, "buffers", name, got)
}

func explain(ctx context.Context, t perfT, pool *pgxpool.Pool, sql string, args []any) PlanNode {
	t.Helper()
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatalf("testkit.Plan: %v", err)
		return PlanNode{}
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var out []byte
	if err := tx.QueryRow(ctx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+sql, args...).Scan(&out); err != nil {
		t.Fatalf("testkit.Plan: %v", err)
		return PlanNode{}
	}
	plan, err := parsePlan(out)
	if err != nil {
		t.Fatalf("testkit.Plan: %v", err)
	}
	return plan
}

func parsePlan(raw []byte) (PlanNode, error) {
	var doc []struct {
		Plan PlanNode `json:"Plan"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return PlanNode{}, fmt.Errorf("parse EXPLAIN output: %w", err)
	}
	if len(doc) != 1 {
		return PlanNode{}, fmt.Errorf("%w: %d", errPlanCount, len(doc))
	}
	return doc[0].Plan, nil
}

type perfT interface {
	Helper()
	Fatalf(format string, args ...any)
	Logf(format string, args ...any)
}

func compareBaseline(t perfT, path string, update bool, kind, name string, got int64) {
	t.Helper()
	baselineMu.Lock()
	defer baselineMu.Unlock()
	b, err := readBaseline(path)
	if err != nil {
		t.Fatalf("testkit: %v", err)
		return
	}
	counts := b.Allocs
	switch kind {
	case "queries":
		counts = b.Queries
	case "buffers":
		counts = b.Buffers
	}
	want, ok := counts[name]
	switch {
	case update:
		counts[name] = got
		if err := writeBaseline(path, b); err != nil {
			t.Fatalf("testkit: %v", err)
		}
	case !ok:
		t.Fatalf(
			"testkit: no %s baseline for %q in %s; measured %d. Rerun with -testkit.perf-update and commit the file",
			kind,
			name,
			path,
			got,
		)
	case got > want:
		t.Fatalf(
			"testkit: %s for %q = %d, baseline %d. An increase needs %s updated in the same PR (-testkit.perf-update)",
			kind,
			name,
			got,
			want,
			path,
		)
	case got < want:
		t.Logf(
			"testkit: %s for %q = %d, below the baseline %d; lower it with -testkit.perf-update",
			kind,
			name,
			got,
			want,
		)
	}
}

func readBaseline(path string) (baseline, error) {
	b := baseline{Allocs: map[string]int64{}, Queries: map[string]int64{}, Buffers: map[string]int64{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return b, nil
	}
	if err != nil {
		return b, fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &b); err != nil {
		return b, fmt.Errorf("parse %s: %w", path, err)
	}
	if b.Allocs == nil {
		b.Allocs = map[string]int64{}
	}
	if b.Queries == nil {
		b.Queries = map[string]int64{}
	}
	if b.Buffers == nil {
		b.Buffers = map[string]int64{}
	}
	return b, nil
}

func writeBaseline(path string, b baseline) error {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
