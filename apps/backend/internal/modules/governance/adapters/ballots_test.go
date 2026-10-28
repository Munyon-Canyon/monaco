package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/modules/governance/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func exec(t *testing.T, db *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := db.Exec(t.Context(), sql, args...); err != nil {
		t.Fatal(err)
	}
}

func TestBallots_returnsEveryFrozenVoterWithANilChoiceForTheSilent(t *testing.T) {
	t.Parallel()
	db := testkit.DB(t)
	gen := ids.Real{}
	proposal, voted, silent := gen.NewV7(), gen.NewV7(), gen.NewV7()
	castAt := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	exec(t, db, `INSERT INTO proposals
		(id, cabal_id, proposer_id, kind, symbol, mint, usdc_micros, quote_out_amount, threshold, status,
		 expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, 'buy', 'AAPLx', 'mint', 5000000, 1, 'majority', 'open', $4, $5, $5)`,
		proposal, gen.NewV7(), voted, castAt.Add(24*time.Hour), castAt)
	for _, voter := range []uuid.UUID{voted, silent} {
		exec(t, db, `INSERT INTO proposal_voters (proposal_id, voter_id) VALUES ($1, $2)`, proposal, voter)
	}
	exec(
		t,
		db,
		`INSERT INTO votes (proposal_id, voter_id, choice, cast_at, first_cast_at) VALUES ($1, $2, 'no', $3, $3)`,
		proposal,
		voted,
		castAt,
	)

	got, err := adapters.NewBallots(db).Ballots(t.Context(), ids.ProposalIDFrom(proposal))
	if err != nil || len(got) != 2 {
		t.Fatalf("Ballots() = %d rows, %v, want 2", len(got), err)
	}
	byVoter := map[uuid.UUID]port.Ballot{}
	for _, b := range got {
		byVoter[b.VoterID.UUID()] = b
	}
	if b := byVoter[voted]; b.Choice == nil || *b.Choice != "no" || b.CastAt == nil || !b.CastAt.Equal(castAt) {
		t.Errorf("voted ballot = %+v, want no at %s", b, castAt)
	}
	if b := byVoter[silent]; b.Choice != nil || b.CastAt != nil {
		t.Errorf("silent ballot = %+v, want nil choice and nil cast_at", b)
	}
}

func TestBallots_failsWhenTheDatabaseCallFails(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := adapters.NewBallots(testkit.DB(t)).Ballots(ctx, ids.ProposalIDFrom(ids.Real{}.NewV7())); err == nil {
		t.Fatal("Ballots() error = nil")
	}
}
