package governance_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/modules/governance"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/bucket"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func sept(d, h int) time.Time { return time.Date(2026, 9, d, h, 0, 0, 0, time.UTC) }

type board struct {
	t    *testing.T
	pool *pgxpool.Pool
	gen  *testkit.IDs
}

func newBoard(t *testing.T) board {
	t.Helper()
	return board{t: t, pool: testkit.DB(t), gen: testkit.NewIDs(1)}
}

func (b board) dashboard() app.Dashboard {
	return governance.New(module.Deps{Pool: b.pool}).DashboardOn(b.pool)
}

func (b board) event(typ string, proposal uuid.UUID, at time.Time) {
	b.t.Helper()
	if _, err := b.pool.Exec(b.t.Context(), `INSERT INTO events
		(id, aggregate_type, aggregate_id, type, payload, actor_type, actor_id, created_at)
		VALUES ($1, 'proposal', $2, $3, '{"v":1}', 'system', 'test', $4)`, b.gen.NewV7(), proposal, typ, at); err != nil {
		b.t.Fatal(err)
	}
}

func (b board) proposal(cabal uuid.UUID, status string, at time.Time, voters, voted int) {
	b.t.Helper()
	id := b.gen.NewV7()
	if _, err := b.pool.Exec(b.t.Context(), `INSERT INTO proposals
		(id, cabal_id, proposer_id, kind, symbol, mint, usdc_micros, quote_out_amount, threshold, status,
		 expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, 'buy', 'AAPLx', 'mint', 5000000, 1, 'majority', $4, $5, $5, $5)`,
		id, cabal, b.gen.NewV7(), status, at.Add(24*time.Hour)); err != nil {
		b.t.Fatal(err)
	}
	for i := range voters {
		voter := b.gen.NewV7()
		if _, err := b.pool.Exec(b.t.Context(),
			`INSERT INTO proposal_voters (proposal_id, voter_id) VALUES ($1, $2)`, id, voter); err != nil {
			b.t.Fatal(err)
		}
		if i >= voted {
			continue
		}
		if _, err := b.pool.Exec(b.t.Context(),
			`INSERT INTO votes (proposal_id, voter_id, choice, cast_at) VALUES ($1, $2, 'yes', $3)`,
			id, voter, at.Add(time.Hour)); err != nil {
			b.t.Fatal(err)
		}
	}
}

func TestDashboard_ProposalCounts_GroupsEventsPerBucket(t *testing.T) {
	t.Parallel()
	b := newBoard(t)
	p := b.gen.NewV7()
	for _, e := range []struct {
		typ string
		at  time.Time
	}{
		{"proposal.created", sept(1, 9)},
		{"proposal.created", sept(1, 20)},
		{"proposal.passed", sept(1, 21)},
		{"proposal.failed", sept(3, 1)},
		{"proposal.expired", sept(3, 2)},
		{"proposal.execution_blocked", sept(8, 0)},
		{"proposal.voided", sept(3, 3)},
		{"proposal.created", sept(15, 0)},
	} {
		b.event(e.typ, p, e.at)
	}
	tests := map[string]struct {
		size bucket.Size
		want []port.ProposalBucket
	}{
		"day": {bucket.Day, []port.ProposalBucket{
			{Start: sept(1, 0), Created: 2, Passed: 1},
			{Start: sept(3, 0), Failed: 1, Expired: 1},
			{Start: sept(8, 0), ExecutionBlocked: 1},
		}},
		"week": {bucket.Week, []port.ProposalBucket{
			{Start: time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC), Created: 2, Passed: 1, Failed: 1, Expired: 1},
			{Start: sept(7, 0), ExecutionBlocked: 1},
		}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := b.dashboard().ProposalCounts(t.Context(), sept(1, 0), sept(15, 0), tt.size)
			if err != nil || len(got) != len(tt.want) {
				t.Fatalf("ProposalCounts() = %+v, %v, want %+v", got, err, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("bucket %d = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestDashboard_MedianTimeToPass(t *testing.T) {
	t.Parallel()
	b := newBoard(t)
	for _, hours := range []int{1, 3, 5} {
		p := b.gen.NewV7()
		b.event("proposal.created", p, sept(2, 0))
		b.event("proposal.passed", p, sept(2, hours))
	}
	late := b.gen.NewV7()
	b.event("proposal.created", late, sept(2, 0))
	b.event("proposal.passed", late, sept(20, 0))
	got, err := b.dashboard().MedianTimeToPass(t.Context(), sept(1, 0), sept(10, 0))
	if err != nil || got.Passed != 3 || got.Median != 3*time.Hour {
		t.Fatalf("MedianTimeToPass() = %+v, %v, want 3 passed at a 3h median", got, err)
	}
	none, err := b.dashboard().MedianTimeToPass(t.Context(), sept(21, 0), sept(22, 0))
	if err != nil || none != (port.PassTime{}) {
		t.Fatalf("MedianTimeToPass() over an empty range = %+v, %v, want zero", none, err)
	}
}

func TestDashboard_VoteParticipation(t *testing.T) {
	t.Parallel()
	b := newBoard(t)
	first, second := b.gen.NewV7(), b.gen.NewV7()
	b.proposal(first, "passed", sept(2, 0), 3, 2)
	b.proposal(first, "open", sept(3, 0), 3, 1)
	b.proposal(second, "failed", sept(4, 0), 2, 0)
	b.proposal(second, "failed", sept(20, 0), 2, 2)
	got, err := b.dashboard().VoteParticipation(t.Context(), sept(1, 0), sept(10, 0))
	want := map[ids.CabalID]port.CabalParticipation{
		ids.CabalIDFrom(first):  {CabalID: ids.CabalIDFrom(first), Proposals: 2, Eligible: 6, Voted: 3},
		ids.CabalIDFrom(second): {CabalID: ids.CabalIDFrom(second), Proposals: 1, Eligible: 2, Voted: 0},
	}
	if err != nil || len(got) != len(want) {
		t.Fatalf("VoteParticipation() = %+v, %v, want %+v", got, err, want)
	}
	for _, p := range got {
		if p != want[p.CabalID] {
			t.Errorf("participation = %+v, want %+v", p, want[p.CabalID])
		}
	}
}

func TestDashboard_OpenCount(t *testing.T) {
	t.Parallel()
	b := newBoard(t)
	cabal := b.gen.NewV7()
	for _, status := range []string{"open", "open", "passed", "expired", "open"} {
		b.proposal(cabal, status, sept(2, 0), 0, 0)
	}
	if got, err := b.dashboard().OpenCount(t.Context()); err != nil || got != 3 {
		t.Fatalf("OpenCount() = %d, %v, want 3", got, err)
	}
}

func TestDashboard_ReadsFailOnACancelledContext(t *testing.T) {
	t.Parallel()
	d := newBoard(t).dashboard()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := d.ProposalCounts(ctx, sept(1, 0), sept(2, 0), bucket.Day); err == nil {
		t.Error("ProposalCounts() error = nil on a cancelled context")
	}
	if _, err := d.MedianTimeToPass(ctx, sept(1, 0), sept(2, 0)); err == nil {
		t.Error("MedianTimeToPass() error = nil on a cancelled context")
	}
	if _, err := d.VoteParticipation(ctx, sept(1, 0), sept(2, 0)); err == nil {
		t.Error("VoteParticipation() error = nil on a cancelled context")
	}
	if _, err := d.OpenCount(ctx); err == nil {
		t.Error("OpenCount() error = nil on a cancelled context")
	}
}
