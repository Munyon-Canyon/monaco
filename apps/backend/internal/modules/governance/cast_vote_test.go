package governance_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type voteDB struct {
	proposalDB
	uow *db.UnitOfWork
	clk *testkit.Clock
}

func newVoteDB(t *testing.T) voteDB {
	t.Helper()
	d := newProposalDB(t)
	clk := testkit.NewClock(d.now.Add(time.Hour))
	return voteDB{proposalDB: d, uow: db.New(d.pool, d.ids, clk), clk: clk}
}

func (d voteDB) open(t *testing.T, voters int) (sqlc.InsertProposalParams, []uuid.UUID) {
	t.Helper()
	return d.openUnder(t, voters, "majority")
}

func (d voteDB) openUnder(t *testing.T, voters int, rule string) (sqlc.InsertProposalParams, []uuid.UUID) {
	t.Helper()
	set := make([]uuid.UUID, voters)
	for i := range set {
		set[i] = d.ids.NewV7()
	}
	p := d.buy(set...)
	p.Threshold = rule
	d.insert(t, p)
	return p, set
}

func (d voteDB) handler() *app.CastVoteHandler {
	return app.NewCastVoteHandler(d.uow, d.clk, app.NoHints{})
}

func (voteDB) cast(
	ctx context.Context, h *app.CastVoteHandler, proposal, voter uuid.UUID, choice domain.Choice,
) (app.CastVoteResult, error) {
	user, err := ids.ParseUserID(voter.String())
	if err != nil {
		return app.CastVoteResult{}, err
	}
	ctx = observability.WithActor(ctx, "user:"+voter.String())
	return h.Handle(ctx, app.CastVote{ProposalID: ids.ProposalIDFrom(proposal), VoterID: user, Choice: choice})
}

func (d voteDB) mustCast(
	t *testing.T, h *app.CastVoteHandler, proposal, voter uuid.UUID, choice domain.Choice,
) app.CastVoteResult {
	t.Helper()
	got, err := d.cast(t.Context(), h, proposal, voter, choice)
	if err != nil {
		t.Fatalf("CastVote(%s) = %v", choice, err)
	}
	return got
}

func (d voteDB) payloads(t *testing.T, proposal uuid.UUID, typ events.Type) []json.RawMessage {
	t.Helper()
	rows, err := d.pool.Query(t.Context(),
		`SELECT payload FROM events WHERE aggregate_id = $1 AND type = $2 ORDER BY id`, proposal, string(typ))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []json.RawMessage
	for rows.Next() {
		var p json.RawMessage
		if err := rows.Scan(&p); err != nil {
			t.Fatal(err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func (d voteDB) terminal(t *testing.T, proposal uuid.UUID) int {
	t.Helper()
	var n int
	if err := d.pool.QueryRow(t.Context(), `SELECT count(*) FROM events WHERE aggregate_id = $1
		AND type IN ('proposal.passed', 'proposal.failed', 'proposal.expired')`, proposal).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (d voteDB) ballots(t *testing.T, proposal uuid.UUID) map[uuid.UUID]string {
	t.Helper()
	rows, err := d.pool.Query(t.Context(), `SELECT voter_id, choice FROM votes WHERE proposal_id = $1`, proposal)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[uuid.UUID]string{}
	for rows.Next() {
		var voter uuid.UUID
		var choice string
		if err := rows.Scan(&voter, &choice); err != nil {
			t.Fatal(err)
		}
		out[voter] = choice
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func wantResult(t *testing.T, got app.CastVoteResult, status domain.Status, tally app.Tally, mine domain.Choice) {
	t.Helper()
	if got.Status != status || got.Tally != tally || got.MyBallot != mine {
		t.Fatalf("result = %+v, want status %s, tally %+v, my ballot %s", got, status, tally, mine)
	}
}

func TestCastVote_MajorityPasses(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	h := d.handler()
	p, v := d.open(t, 4)
	wantResult(t, d.mustCast(t, h, p.ID, v[0], domain.ChoiceYes), domain.StatusOpen,
		app.Tally{Yes: 1, Voters: 4, Needed: 3}, domain.ChoiceYes)
	wantResult(t, d.mustCast(t, h, p.ID, v[1], domain.ChoiceNo), domain.StatusOpen,
		app.Tally{Yes: 1, No: 1, Voters: 4, Needed: 3}, domain.ChoiceNo)
	wantResult(t, d.mustCast(t, h, p.ID, v[2], domain.ChoiceYes), domain.StatusOpen,
		app.Tally{Yes: 2, No: 1, Voters: 4, Needed: 3}, domain.ChoiceYes)
	if n := d.terminal(t, p.ID); n != 0 {
		t.Fatalf("%d terminal events before the deciding vote, want 0", n)
	}
	got := d.mustCast(t, h, p.ID, v[3], domain.ChoiceYes)
	wantResult(t, got, domain.StatusPassed, app.Tally{Yes: 3, No: 1, Voters: 4, Needed: 3}, domain.ChoiceYes)
	if got.ProposalID.UUID() != p.ID {
		t.Fatalf("result proposal = %s, want %s", got.ProposalID, p.ID)
	}
	if r := d.row(t, p.ID); r.status.String != "passed" || !r.updatedAt.Equal(d.clk.Now()) {
		t.Fatalf("row = %+v, want passed at %s", r, d.clk.Now())
	}
	passed := d.payloads(t, p.ID, events.TypeProposalPassed)
	if len(passed) != 1 || d.terminal(t, p.ID) != 1 {
		t.Fatalf("passed events = %s, want exactly one terminal event", passed)
	}
	var ev events.ProposalPassed
	if err := json.Unmarshal(passed[0], &ev); err != nil {
		t.Fatal(err)
	}
	want := events.ProposalPassed{
		V: 1, ProposalID: p.ID, CabalID: p.CabalID, ProposerID: p.ProposerID, Kind: "buy", Symbol: "AAPLx",
		Mint: aaplxMint, USDCMicros: money.MicrosFromUint64(25_000_000), QuoteOutAmount: 105_000_000,
	}
	if ev != want {
		t.Fatalf("proposal.passed = %+v, want %+v", ev, want)
	}
	_, err := d.cast(t.Context(), h, p.ID, v[1], domain.ChoiceYes)
	if errs.CodeOf(err) != errs.CodeProposalClosed {
		t.Fatalf("a vote after the decision err = %v, want proposal_closed", err)
	}
}

func TestCastVote_sellPassesWithItsTokenAmount(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	voter := d.ids.NewV7()
	p := d.buy(voter)
	p.Kind, p.UsdcMicros, p.TokenAmount = "sell", pgtype.Int8{}, pgtype.Int8{Int64: 3_000_000, Valid: true}
	d.insert(t, p)
	d.mustCast(t, d.handler(), p.ID, voter, domain.ChoiceYes)
	var ev events.ProposalPassed
	if err := json.Unmarshal(d.payloads(t, p.ID, events.TypeProposalPassed)[0], &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Kind != "sell" || ev.TokenAmount != 3_000_000 || ev.USDCMicros.Uint64() != 0 {
		t.Fatalf("proposal.passed = %+v, want a sell of 3000000 units and no USDC", ev)
	}
}

func TestCastVote_UnanimousFailsOnFirstNo(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	h := d.handler()
	p, v := d.openUnder(t, 3, "unanimous")
	d.mustCast(t, h, p.ID, v[0], domain.ChoiceYes)
	got := d.mustCast(t, h, p.ID, v[1], domain.ChoiceNo)
	wantResult(t, got, domain.StatusFailed, app.Tally{Yes: 1, No: 1, Voters: 3, Needed: 3}, domain.ChoiceNo)
	if r := d.row(t, p.ID); r.status.String != "failed" {
		t.Fatalf("status = %s, want failed", r.status.String)
	}
	failed := d.payloads(t, p.ID, events.TypeProposalFailed)
	var ev events.ProposalFailed
	if len(failed) != 1 || json.Unmarshal(failed[0], &ev) != nil || d.terminal(t, p.ID) != 1 {
		t.Fatalf("failed events = %s, want exactly one terminal event", failed)
	}
	if ev != (events.ProposalFailed{V: 1, ProposalID: p.ID, CabalID: p.CabalID}) {
		t.Fatalf("proposal.failed = %+v", ev)
	}
}

func TestCastVote_openProposalKeepsItsThresholdAfterARuleChange(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	c := testkit.NewCabal(t, d.pool, testkit.WithMembers(3))
	p, v := d.open(t, 3)
	p.CabalID = c.ID.UUID()
	move := `UPDATE proposals SET cabal_id = $2 WHERE id = $1`
	if _, err := d.pool.Exec(t.Context(), move, p.ID, p.CabalID); err != nil {
		t.Fatal(err)
	}
	tighten := `UPDATE cabals SET threshold = 'unanimous' WHERE id = $1`
	if _, err := d.pool.Exec(t.Context(), tighten, p.CabalID); err != nil {
		t.Fatal(err)
	}
	h := d.handler()
	d.mustCast(t, h, p.ID, v[0], domain.ChoiceYes)
	got := d.mustCast(t, h, p.ID, v[1], domain.ChoiceYes)
	wantResult(t, got, domain.StatusPassed, app.Tally{Yes: 2, Voters: 3, Needed: 2}, domain.ChoiceYes)
}

func TestCastVote_ChangeBallotWhileOpen(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	h := d.handler()
	p, v := d.open(t, 3)
	d.mustCast(t, h, p.ID, v[0], domain.ChoiceNo)
	got := d.mustCast(t, h, p.ID, v[0], domain.ChoiceYes)
	wantResult(t, got, domain.StatusOpen, app.Tally{Yes: 1, Voters: 3, Needed: 2}, domain.ChoiceYes)
	if b := d.ballots(t, p.ID); len(b) != 1 || b[v[0]] != "yes" {
		t.Fatalf("ballots = %v, want one yes from the voter who changed", b)
	}
	got = d.mustCast(t, h, p.ID, v[0], domain.ChoiceYes)
	wantResult(t, got, domain.StatusOpen, app.Tally{Yes: 1, Voters: 3, Needed: 2}, domain.ChoiceYes)
}

func TestCastVote_VoterSetFrozen(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	h := d.handler()
	p, _ := d.open(t, 2)
	joinedLater := d.ids.NewV7()
	_, err := d.cast(t.Context(), h, p.ID, joinedLater, domain.ChoiceYes)
	if errs.CodeOf(err) != errs.CodeNotAVoter {
		t.Fatalf("a vote from a member outside the frozen voter set err = %v, want not_a_voter", err)
	}
	if b := d.ballots(t, p.ID); len(b) != 0 {
		t.Fatalf("ballots = %v, want none", b)
	}
}

func TestCastVote_refusals(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	h := d.handler()
	_, err := d.cast(t.Context(), h, d.ids.NewV7(), d.ids.NewV7(), domain.ChoiceYes)
	if errs.CodeOf(err) != errs.CodeProposalNotFound {
		t.Errorf("a vote on an unknown proposal err = %v, want proposal_not_found", err)
	}
	p, v := d.open(t, 2)
	d.transition(t, p.ID, "open", "withdrawn", "")
	if _, err := d.cast(t.Context(), h, p.ID, v[0], domain.ChoiceYes); errs.CodeOf(err) != errs.CodeProposalClosed {
		t.Errorf("a vote on a withdrawn proposal err = %v, want proposal_closed", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := d.cast(ctx, h, p.ID, v[0], domain.ChoiceYes); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Errorf("a vote on a cancelled context err = %v, want db_unavailable", err)
	}
}

func TestCastVote_aRefusedWriteRollsTheBallotBack(t *testing.T) {
	t.Parallel()
	refusals := map[string]string{
		"ballot":     `CREATE TRIGGER refuse BEFORE INSERT ON votes FOR EACH ROW EXECUTE FUNCTION refuse()`,
		"transition": `CREATE TRIGGER refuse BEFORE UPDATE ON proposals FOR EACH ROW EXECUTE FUNCTION refuse()`,
		"event":      `CREATE TRIGGER refuse BEFORE INSERT ON events FOR EACH ROW EXECUTE FUNCTION refuse()`,
	}
	for name, trigger := range refusals {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := newVoteDB(t)
			p, v := d.open(t, 1)
			if _, err := d.pool.Exec(t.Context(), `CREATE FUNCTION refuse() RETURNS trigger LANGUAGE plpgsql
				AS $$ BEGIN RAISE EXCEPTION 'refused'; END $$`); err != nil {
				t.Fatal(err)
			}
			if _, err := d.pool.Exec(t.Context(), trigger); err != nil {
				t.Fatal(err)
			}
			_, err := d.cast(t.Context(), d.handler(), p.ID, v[0], domain.ChoiceYes)
			if err == nil {
				t.Fatal("CastVote succeeded through a refused write")
			}
			if b, r := d.ballots(t, p.ID), d.row(t, p.ID); len(b) != 0 || r.status.String != "open" {
				t.Fatalf("ballots %v, status %s after the refusal, want none and open", b, r.status.String)
			}
		})
	}
}

func TestCastVote_ConcurrentDecidingVotes(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	h := d.handler()
	p, v := d.open(t, 20)
	results := make([]app.CastVoteResult, len(v))
	failures := make([]error, len(v))
	var wg sync.WaitGroup
	for i, voter := range v {
		wg.Go(func() { results[i], failures[i] = d.cast(t.Context(), h, p.ID, voter, domain.ChoiceYes) })
	}
	wg.Wait()
	passed, closed := 0, 0
	for i, err := range failures {
		switch {
		case err == nil && results[i].Status == domain.StatusPassed:
			passed++
		case errs.CodeOf(err) == errs.CodeProposalClosed:
			closed++
		case err != nil:
			t.Fatalf("vote %d err = %v", i, err)
		}
	}
	if passed != 1 || closed != 9 {
		t.Fatalf("%d votes passed it and %d found it closed, want 1 and 9", passed, closed)
	}
	if n := len(d.payloads(t, p.ID, events.TypeProposalPassed)); n != 1 || d.terminal(t, p.ID) != 1 {
		t.Fatalf("%d proposal.passed events, want exactly 1", n)
	}
	if b := d.ballots(t, p.ID); len(b) != 11 {
		t.Fatalf("%d ballots stored, want the 11 cast before the decision", len(b))
	}
}
