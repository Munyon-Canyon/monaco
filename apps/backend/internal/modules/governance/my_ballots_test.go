package governance_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestMyBallots_saysHowTheViewerVotedAndWhetherTheyCanStillVote(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	gen := ids.Real{}
	viewer, other, cabal := gen.NewV7(), gen.NewV7(), gen.NewV7()
	created := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	future := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	past := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	insert := func(status string, expires time.Time, voters ...uuid.UUID) uuid.UUID {
		id := gen.NewV7()
		if _, err := pool.Exec(t.Context(), `INSERT INTO proposals
			(id, cabal_id, proposer_id, kind, symbol, mint, usdc_micros, quote_out_amount, threshold, status,
			 expires_at, created_at, updated_at)
			VALUES ($1, $2, $3, 'buy', 'AAPLx', 'mint', 5000000, 1, 'majority', $4, $5, $6, $6)`,
			id, cabal, other, status, expires, created); err != nil {
			t.Fatal(err)
		}
		for _, v := range voters {
			if _, err := pool.Exec(t.Context(),
				`INSERT INTO proposal_voters (proposal_id, voter_id) VALUES ($1, $2)`, id, v); err != nil {
				t.Fatal(err)
			}
		}
		return id
	}
	vote := func(proposal uuid.UUID, choice string) {
		if _, err := pool.Exec(t.Context(),
			`INSERT INTO votes (proposal_id, voter_id, choice, cast_at, first_cast_at) VALUES ($1, $2, $3, $4, $4)`,
			proposal, viewer, choice, created); err != nil {
			t.Fatal(err)
		}
	}
	silent := insert("open", future, viewer, other)
	yes := insert("open", future, viewer)
	vote(yes, "yes")
	no := insert("open", future, viewer)
	vote(no, "no")
	closed := insert("passed", future, viewer)
	vote(closed, "yes")
	lapsed := insert("open", past, viewer)
	outsider := insert("open", future, other)
	asked := make([]ids.ProposalID, 0, 6)
	for _, id := range []uuid.UUID{silent, yes, no, closed, lapsed, outsider} {
		asked = append(asked, ids.ProposalIDFrom(id))
	}

	got, err := app.NewQueries(pool).MyBallots(t.Context(), ids.UserIDFrom(viewer), asked)
	if err != nil {
		t.Fatal(err)
	}
	want := map[uuid.UUID]port.ViewerBallot{
		silent: {CanVote: true}, yes: {Choice: "yes", CanVote: true}, no: {Choice: "no", CanVote: true},
		closed: {Choice: "yes"}, lapsed: {},
	}
	if len(got) != len(want) {
		t.Fatalf("MyBallots = %v, want %d rows without the outsider proposal", got, len(want))
	}
	for id, w := range want {
		if g := got[ids.ProposalIDFrom(id)]; g != w {
			t.Errorf("proposal %s = %+v, want %+v", id, g, w)
		}
	}
}

func TestMyBallots_failsWhenTheReadFails(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	gen := ids.Real{}
	if _, err := pool.Exec(t.Context(), `ALTER TABLE proposal_voters RENAME TO proposal_voters_gone`); err != nil {
		t.Fatal(err)
	}
	_, err := app.NewQueries(pool).
		MyBallots(t.Context(), ids.UserIDFrom(gen.NewV7()), []ids.ProposalID{ids.ProposalIDFrom(gen.NewV7())})
	if errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("MyBallots error = %v, want %s", err, errs.CodeDBUnavailable)
	}
}
