package analytics_test

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/analyticsapi"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type proposalSeed struct {
	t    *testing.T
	pool *pgxpool.Pool
	ids  *testkit.IDs
}

func (s proposalSeed) event(typ string, proposal uuid.UUID, when time.Time) {
	s.t.Helper()
	if _, err := s.pool.Exec(s.t.Context(), `INSERT INTO events
		(id, aggregate_type, aggregate_id, type, payload, actor_type, actor_id, created_at)
		VALUES ($1, 'proposal', $2, $3, '{"v":1}', 'system', 'test', $4)`,
		s.ids.NewV7(), proposal, typ, when); err != nil {
		s.t.Fatal(err)
	}
}

func (s proposalSeed) proposal(cabal uuid.UUID, status string, when time.Time, voters, voted int) {
	s.t.Helper()
	id := s.ids.NewV7()
	if _, err := s.pool.Exec(s.t.Context(), `INSERT INTO proposals
		(id, cabal_id, proposer_id, kind, symbol, mint, usdc_micros, quote_out_amount, threshold, status,
		 expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, 'buy', 'AAPLx', 'mint', 5000000, 1, 'majority', $4, $5, $5, $5)`,
		id, cabal, s.ids.NewV7(), status, when.Add(24*time.Hour)); err != nil {
		s.t.Fatal(err)
	}
	for i := range voters {
		voter := s.ids.NewV7()
		if _, err := s.pool.Exec(s.t.Context(),
			`INSERT INTO proposal_voters (proposal_id, voter_id) VALUES ($1, $2)`, id, voter); err != nil {
			s.t.Fatal(err)
		}
		if i >= voted {
			continue
		}
		if _, err := s.pool.Exec(s.t.Context(),
			`INSERT INTO votes (proposal_id, voter_id, choice, cast_at) VALUES ($1, $2, 'yes', $3)`,
			id, voter, when.Add(time.Hour)); err != nil {
			s.t.Fatal(err)
		}
	}
}

func seedGovernance(t *testing.T, pool *pgxpool.Pool) (first, second uuid.UUID) {
	t.Helper()
	s := proposalSeed{t: t, pool: pool, ids: testkit.NewIDs(5)}
	for _, hours := range []int{1, 3, 5} {
		p := s.ids.NewV7()
		s.event("proposal.created", p, at(2, 0, 0))
		s.event("proposal.passed", p, at(2, hours, 0))
	}
	failed := s.ids.NewV7()
	s.event("proposal.created", failed, at(9, 0, 0))
	s.event("proposal.failed", failed, at(9, 6, 0))
	s.event("proposal.expired", failed, at(9, 7, 0))
	s.event("proposal.execution_blocked", failed, at(10, 0, 0))
	s.event("proposal.passed", failed, at(20, 0, 0))
	first, second = s.ids.NewV7(), s.ids.NewV7()
	s.proposal(first, "passed", at(2, 0, 0), 3, 2)
	s.proposal(first, "open", at(3, 0, 0), 3, 1)
	s.proposal(second, "failed", at(4, 0, 0), 2, 0)
	s.proposal(second, "open", at(20, 0, 0), 2, 2)
	return first, second
}

func governanceWindow() url.Values {
	return window("2026-09-01", "2026-09-15", "day")
}

func TestDashboard_Governance_MedianTimeToPass(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	seedGovernance(t, pool)
	h := productHandler(t, pool)
	var got api.GovernanceDashboard
	decode(t, dashboardGet(t, h, "governance", viewerToken, governanceWindow()), &got)
	if got.PassedInRange != 3 || got.MedianSecondsToPass == nil || *got.MedianSecondsToPass != 10_800 ||
		got.OpenProposals != 2 {
		t.Fatalf("passed %d median %v open %d, want 3 passed, a 10800 s median and 2 open",
			got.PassedInRange, got.MedianSecondsToPass, got.OpenProposals)
	}
	var quiet api.GovernanceDashboard
	decode(t, dashboardGet(t, h, "governance", viewerToken, window("2026-08-01", "2026-08-02", "week")), &quiet)
	if quiet.MedianSecondsToPass != nil || quiet.PassedInRange != 0 || len(quiet.Buckets) != 0 {
		t.Fatalf("quiet range = %+v, want no median and no buckets", quiet)
	}
}

func TestDashboard_Governance_Buckets(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	seedGovernance(t, pool)
	var got api.GovernanceDashboard
	decode(t, dashboardGet(t, productHandler(t, pool), "governance", viewerToken, governanceWindow()), &got)
	want := []api.GovernanceBucket{
		{BucketStart: at(2, 0, 0), Created: 3, Passed: 3},
		{BucketStart: at(9, 0, 0), Created: 1, Failed: 1, Expired: 1},
		{BucketStart: at(10, 0, 0), ExecutionBlocked: 1},
	}
	if len(got.Buckets) != len(want) {
		t.Fatalf("buckets = %+v, want %+v", got.Buckets, want)
	}
	for i := range want {
		if got.Buckets[i] != want[i] {
			t.Errorf("bucket %d = %+v, want %+v", i, got.Buckets[i], want[i])
		}
	}
}

func TestDashboard_Governance_Participation(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	first, second := seedGovernance(t, pool)
	var got api.GovernanceDashboard
	decode(t, dashboardGet(t, productHandler(t, pool), "governance", viewerToken, governanceWindow()), &got)
	want := map[uuid.UUID]api.GovernanceParticipation{
		first:  {CabalId: first, Proposals: 2, EligibleVotes: 6, VotesCast: 3, ParticipationBps: 5000},
		second: {CabalId: second, Proposals: 1, EligibleVotes: 2, VotesCast: 0, ParticipationBps: 0},
	}
	if len(got.Participation) != len(want) {
		t.Fatalf("participation = %+v, want %+v", got.Participation, want)
	}
	for _, row := range got.Participation {
		if row != want[row.CabalId] {
			t.Errorf("participation = %+v, want %+v", row, want[row.CabalId])
		}
	}
}

func TestDashboard_Governance_RefusesABadWindow(t *testing.T) {
	t.Parallel()
	w := dashboardGet(t, productHandler(t, testkit.DB(t)), "governance", viewerToken,
		window("2026-09-02", "2026-09-01", "day"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d %s, want 400", w.Code, w.Body)
	}
}

func TestDashboard_Governance_QueryCount(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	seedGovernance(t, pool)
	h := productHandler(t, pool)
	testkit.AssertQueries(t, "analytics GetGovernanceDashboard", func() {
		w := dashboardGet(t, h, "governance", viewerToken, governanceWindow())
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d %s", w.Code, w.Body)
		}
	})
}
