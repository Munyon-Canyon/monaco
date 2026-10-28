package adapters_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const aaplxMint = "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp"

func TestBallots_everyFrozenVoterComesBackWithItsChoiceOrNone(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	q := sqlc.New(pool)
	gen := testkit.NewIDs(1)
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	voted, idle, outsider := gen.NewV7(), gen.NewV7(), gen.NewV7()
	proposal, other := gen.NewV7(), gen.NewV7()
	insertProposal(t, q, proposal, now, voted, idle)
	insertProposal(t, q, other, now, outsider)
	castAt := now.Add(time.Minute)
	for _, b := range []sqlc.UpsertBallotParams{
		{ProposalID: proposal, VoterID: voted, Choice: "yes", CastAt: castAt},
		{ProposalID: other, VoterID: outsider, Choice: "no", CastAt: castAt},
	} {
		if err := q.UpsertBallot(t.Context(), b); err != nil {
			t.Fatal(err)
		}
	}

	got, err := adapters.NewBallots(pool).Ballots(t.Context(), ids.ProposalIDFrom(proposal))

	if err != nil || len(got) != 2 {
		t.Fatalf("Ballots = %v, %v, want the two frozen voters and nobody from another proposal", got, err)
	}
	byVoter := map[ids.UserID]port.Ballot{}
	for _, b := range got {
		byVoter[b.VoterID] = b
	}
	yes := byVoter[ids.UserIDFrom(voted)]
	if yes.Choice == nil || *yes.Choice != "yes" || yes.CastAt == nil || !yes.CastAt.Equal(castAt) {
		t.Errorf("voter who cast = %+v, want yes at %s", yes, castAt)
	}
	none := byVoter[ids.UserIDFrom(idle)]
	if none.Choice != nil || none.CastAt != nil {
		t.Errorf("eligible voter who has not voted = %+v, want a nil choice and a nil cast time", none)
	}
}

func TestBallots_aProposalWithoutVotersIsEmptyAndAClosedDatabaseNaks(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	ballots := adapters.NewBallots(pool)
	unknown := ids.ProposalIDFrom(testkit.NewIDs(2).NewV7())
	if got, err := ballots.Ballots(t.Context(), unknown); err != nil || len(got) != 0 {
		t.Fatalf("Ballots of an unknown proposal = %v, %v, want none and no error", got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := ballots.Ballots(ctx, unknown); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("Ballots on a cancelled context err = %v, want db_unavailable so a consumer naks", err)
	}
}

func insertProposal(t *testing.T, q *sqlc.Queries, id uuid.UUID, now time.Time, voters ...uuid.UUID) {
	t.Helper()
	gen := testkit.NewIDs(3)
	n, err := q.InsertProposal(t.Context(), sqlc.InsertProposalParams{
		VoterIds: voters, ID: id, CabalID: gen.NewV7(), ProposerID: gen.NewV7(), Kind: "buy", Symbol: "AAPLx",
		Mint: aaplxMint, UsdcMicros: pgtype.Int8{Int64: 25_000_000, Valid: true}, QuoteOutAmount: 105_000_000,
		ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now, Threshold: "majority",
	})
	if err != nil || n != int64(len(voters)) {
		t.Fatalf("InsertProposal = %d voters, %v, want %d", n, err, len(voters))
	}
}
