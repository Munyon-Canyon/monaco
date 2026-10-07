package testkit

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
)

type Queryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type PlanNode struct {
	NodeType     string     `json:"Node Type"`
	RelationName string     `json:"Relation Name"`
	IndexName    string     `json:"Index Name"`
	SharedHit    int64      `json:"Shared Hit Blocks"`
	SharedRead   int64      `json:"Shared Read Blocks"`
	Plans        []PlanNode `json:"Plans"`
}

func (n PlanNode) Blocks() int64 { return n.SharedHit + n.SharedRead }

func (n PlanNode) Nodes() []PlanNode {
	out := make([]PlanNode, 0, 1+len(n.Plans))
	out = append(out, n)
	for _, c := range n.Plans {
		out = append(out, c.Nodes()...)
	}
	return out
}

func AssertPlanWork(t *testing.T, db Queryer, name, index string, maxBlocks int64, sql string, args ...any) {
	t.Helper()
	explain := func() PlanNode {
		var plan []struct {
			Plan PlanNode `json:"Plan"`
		}
		row := db.QueryRow(t.Context(), "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+sql, args...)
		if err := row.Scan(&plan); err != nil || len(plan) != 1 {
			t.Fatalf("%s: EXPLAIN = %v, %v", name, plan, err)
		}
		return plan[0].Plan
	}
	explain()
	top := explain()
	t.Logf("%s touched %d shared blocks", name, top.Blocks())
	nodes := top.Nodes()
	for _, n := range nodes {
		if n.NodeType == "Seq Scan" {
			t.Fatalf("%s plan seq-scans %s", name, n.RelationName)
		}
	}
	if !slices.ContainsFunc(nodes, func(n PlanNode) bool { return n.IndexName == index }) {
		t.Fatalf("%s plan does not use %s", name, index)
	}
	if top.Blocks() > maxBlocks {
		t.Fatalf("%s touched %d shared blocks, want at most %d", name, top.Blocks(), maxBlocks)
	}
}
