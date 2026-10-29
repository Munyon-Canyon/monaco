package adapters_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func voteBase() time.Time { return time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC) }

type voteFixture struct {
	proposal uuid.UUID
	voters   []uuid.UUID
}

func seedVoteState(
	t *testing.T, db *pgxpool.Pool, kind, threshold string, created time.Time, window time.Duration, voters int,
) voteFixture {
	t.Helper()
	gen := ids.Real{}
	f := voteFixture{proposal: gen.NewV7()}
	exec(t, db, `INSERT INTO proposals
		(id, cabal_id, proposer_id, kind, symbol, mint, usdc_micros, quote_out_amount, threshold, status,
		 expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'AAPLx', 'mint', 25000000, 1, $5, 'open', $6, $7, $7)`,
		f.proposal, gen.NewV7(), gen.NewV7(), kind, threshold, created.Add(window), created)
	for range voters {
		voter := gen.NewV7()
		f.voters = append(f.voters, voter)
		exec(t, db, `INSERT INTO proposal_voters (proposal_id, voter_id) VALUES ($1, $2)`, f.proposal, voter)
	}
	return f
}

func vote(t *testing.T, db *pgxpool.Pool, f voteFixture, voter int, choice string) {
	t.Helper()
	exec(t, db, `INSERT INTO votes (proposal_id, voter_id, choice, cast_at, first_cast_at) VALUES ($1, $2, $3, $4, $4)`,
		f.proposal, f.voters[voter], choice, voteBase())
}

func TestVoteState_readsTheTallyAndTheVotesTheThresholdNeeds(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		threshold string
		voters    int
		need      int
	}{"majority of eight": {"majority", 8, 5}, "unanimous of three": {"unanimous", 3, 3}} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			db := testkit.DB(t)
			f := seedVoteState(t, db, "buy", tc.threshold, voteBase(), 6*time.Hour, tc.voters)
			vote(t, db, f, 0, "yes")
			vote(t, db, f, 1, "yes")
			vote(t, db, f, 2, "no")

			got, err := adapters.NewVoteStates(db).VoteState(t.Context(), ids.ProposalIDFrom(f.proposal))

			want := port.VoteState{
				Kind: "buy", Symbol: "AAPLx", Mint: "mint", Status: port.StatusOpen, Yes: 2, No: 1,
				Eligible: tc.voters, Need: tc.need, CreatedAt: voteBase(), ExpiresAt: voteBase().Add(6 * time.Hour),
			}
			got.CabalID = ids.CabalID{}
			if err != nil || got.USDCMicros.Uint64() != 25_000_000 {
				t.Fatalf("VoteState = %+v, %v, want 25 USDC", got, err)
			}
			got.USDCMicros = want.USDCMicros
			if got != want {
				t.Fatalf("VoteState = %+v, want %+v", got, want)
			}
		})
	}
}

func TestVoteState_failsForAnUnknownProposal(t *testing.T) {
	t.Parallel()

	_, err := adapters.NewVoteStates(testkit.DB(t)).VoteState(t.Context(), ids.ProposalIDFrom(ids.Real{}.NewV7()))

	if errs.CodeOf(err) != errs.CodeProposalNotFound {
		t.Fatalf("VoteState = %v, want %s", err, errs.CodeProposalNotFound)
	}
}

func TestNonVoters_returnsTheFrozenVotersWithoutABallot(t *testing.T) {
	t.Parallel()
	db := testkit.DB(t)
	f := seedVoteState(t, db, "buy", "majority", voteBase(), 6*time.Hour, 4)
	vote(t, db, f, 1, "yes")
	vote(t, db, f, 3, "no")

	got, err := adapters.NewVoteStates(db).NonVoters(t.Context(), ids.ProposalIDFrom(f.proposal))

	if err != nil || len(got) != 2 {
		t.Fatalf("NonVoters = %v, %v, want the two silent voters", got, err)
	}
	for _, u := range got {
		if u.UUID() != f.voters[0] && u.UUID() != f.voters[2] {
			t.Errorf("NonVoters returned %s, want only %s and %s", u, f.voters[0], f.voters[2])
		}
	}
}

func TestDueForReminder_pagesOpenTradeProposalsByDueTimeAndID(t *testing.T) {
	t.Parallel()
	db := testkit.DB(t)
	early := seedVoteState(t, db, "buy", "majority", voteBase().Add(-3*time.Hour), 6*time.Hour, 1)
	tied := seedVoteState(t, db, "buy", "majority", voteBase().Add(-2*time.Hour), 3*time.Hour, 1)
	middle := seedVoteState(t, db, "sell", "majority", voteBase().Add(-2*time.Hour), 6*time.Hour, 1)
	later := []voteFixture{
		seedVoteState(t, db, "buy", "majority", voteBase().Add(-time.Hour), 6*time.Hour, 1),
		seedVoteState(t, db, "rule", "majority", voteBase().Add(-3*time.Hour), 6*time.Hour, 1),
		seedVoteState(t, db, "buy", "majority", voteBase().Add(-9*time.Hour), 6*time.Hour, 1),
	}
	closed := seedVoteState(t, db, "buy", "majority", voteBase().Add(-3*time.Hour), 6*time.Hour, 1)
	exec(t, db, `UPDATE proposals SET status = 'passed' WHERE id = $1`, closed.proposal)
	states := adapters.NewVoteStates(db)
	want := []port.DueReminder{
		{ProposalID: ids.ProposalIDFrom(early.proposal), DueAt: voteBase().Add(-time.Hour)},
		{ProposalID: ids.ProposalIDFrom(tied.proposal), DueAt: voteBase().Add(-time.Hour)},
		{ProposalID: ids.ProposalIDFrom(middle.proposal), DueAt: voteBase()},
	}
	if want[0].ProposalID.String() > want[1].ProposalID.String() {
		want[0], want[1] = want[1], want[0]
	}

	got, err := states.DueForReminder(t.Context(), voteBase(), port.ReminderCursor{}, 10)

	if err != nil || !slices.Equal(got, want) {
		t.Fatalf(
			"due = %+v, %v, want %+v: no future, rule, ended or closed proposal (%d of them)",
			got,
			err,
			want,
			len(later)+1,
		)
	}
	page, err := states.DueForReminder(
		t.Context(),
		voteBase(),
		port.ReminderCursor{DueAt: got[0].DueAt, ProposalID: got[0].ProposalID},
		1,
	)
	if err != nil || len(page) != 1 || page[0] != got[1] {
		t.Fatalf("page after the first = %+v, %v, want %+v", page, err, got[1])
	}
	rest, err := states.DueForReminder(
		t.Context(),
		voteBase(),
		port.ReminderCursor{DueAt: got[2].DueAt, ProposalID: got[2].ProposalID},
		10,
	)
	if err != nil || len(rest) != 0 {
		t.Fatalf("page after the last = %+v, %v, want none", rest, err)
	}
}

func TestVoteStates_failWhenTheDatabaseCallFails(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	states := adapters.NewVoteStates(testkit.DB(t))
	id := ids.ProposalIDFrom(ids.Real{}.NewV7())

	_, stateErr := states.VoteState(ctx, id)
	_, dueErr := states.DueForReminder(ctx, voteBase(), port.ReminderCursor{}, 1)
	_, nonErr := states.NonVoters(ctx, id)

	for name, err := range map[string]error{"VoteState": stateErr, "DueForReminder": dueErr, "NonVoters": nonErr} {
		if errs.CodeOf(err) != errs.CodeDBUnavailable {
			t.Errorf("%s = %v, want %s", name, err, errs.CodeDBUnavailable)
		}
	}
}

func TestVoteState_failsForAStatusItDoesNotKnow(t *testing.T) {
	t.Parallel()
	db := testkit.DB(t)
	f := seedVoteState(t, db, "buy", "majority", voteBase(), 6*time.Hour, 1)
	exec(t, db, `UPDATE proposals SET status = 'mystery' WHERE id = $1`, f.proposal)

	_, err := adapters.NewVoteStates(db).VoteState(t.Context(), ids.ProposalIDFrom(f.proposal))

	if errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("VoteState = %v, want %s", err, errs.CodeDecodeFailed)
	}
}
