package governance_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/governanceapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func (d voteDB) retractor() *app.RetractVoteHandler {
	return app.NewRetractVoteHandler(d.uow, d.clk, app.NoHints{})
}

func (voteDB) retract(
	ctx context.Context, h *app.RetractVoteHandler, proposal, voter uuid.UUID,
) (app.RetractVoteResult, error) {
	user, err := ids.ParseUserID(voter.String())
	if err != nil {
		return app.RetractVoteResult{}, err
	}
	ctx = observability.WithActor(ctx, "user:"+voter.String())
	return h.Handle(ctx, app.RetractVote{ProposalID: ids.ProposalIDFrom(proposal), VoterID: user})
}

func (d voteDB) wantRetractCode(t *testing.T, proposal, voter uuid.UUID, want errs.Code) {
	t.Helper()
	if _, err := d.retract(t.Context(), d.retractor(), proposal, voter); errs.CodeOf(err) != want {
		t.Fatalf("RetractVote err = %v, want %s", err, want)
	}
}

func TestRetractVote_withinWindowRemovesTheBallot(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, v := d.open(t, 3)
	d.mustCast(t, d.handler(), p.ID, v[0], domain.ChoiceYes)
	d.mustCast(t, d.handler(), p.ID, v[1], domain.ChoiceNo)
	d.clk.Advance(domain.UndoWindow - time.Second)
	got, err := d.retract(t.Context(), d.retractor(), p.ID, v[0])
	if err != nil {
		t.Fatalf("RetractVote = %v", err)
	}
	if got.Status != domain.StatusOpen || got.Tally != (app.Tally{No: 1, Voters: 3, Needed: 2}) {
		t.Fatalf("result = %+v, want open with the one no left", got)
	}
	if b := d.ballots(t, p.ID); len(b) != 1 || b[v[1]] != "no" {
		t.Fatalf("ballots = %v, want only the other voter's no", b)
	}
	again := d.mustCast(t, d.handler(), p.ID, v[0], domain.ChoiceYes)
	if again.UndoUntil == nil || !again.UndoUntil.After(d.clk.Now()) {
		t.Fatalf("a fresh ballot after the undo has undo_until %v, want a new window", again.UndoUntil)
	}
}

func TestRetractVote_afterWindowIsUndoExpired(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, v := d.open(t, 3)
	d.mustCast(t, d.handler(), p.ID, v[0], domain.ChoiceYes)
	d.clk.Advance(domain.UndoWindow)
	d.wantRetractCode(t, p.ID, v[0], errs.CodeUndoExpired)
	if b := d.ballots(t, p.ID); b[v[0]] != "yes" {
		t.Fatalf("ballots = %v, want the yes kept", b)
	}
}

func TestRetractVote_changedBallotIsUndoExpired(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, v := d.open(t, 3)
	d.mustCast(t, d.handler(), p.ID, v[0], domain.ChoiceYes)
	d.clk.Advance(time.Second)
	d.mustCast(t, d.handler(), p.ID, v[0], domain.ChoiceNo)
	d.wantRetractCode(t, p.ID, v[0], errs.CodeUndoExpired)
	if b := d.ballots(t, p.ID); b[v[0]] != "no" {
		t.Fatalf("ballots = %v, want the changed no kept", b)
	}
}

func TestRetractVote_sameInstantChangeIsUndoExpired(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, v := d.open(t, 3)
	d.mustCast(t, d.handler(), p.ID, v[0], domain.ChoiceYes)
	d.mustCast(t, d.handler(), p.ID, v[0], domain.ChoiceNo)
	d.wantRetractCode(t, p.ID, v[0], errs.CodeUndoExpired)
	d.mustCast(t, d.handler(), p.ID, v[0], domain.ChoiceYes)
	d.wantRetractCode(t, p.ID, v[0], errs.CodeUndoExpired)
}

func TestRetractVote_unchangedRetryStaysUndoable(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, v := d.open(t, 3)
	first := d.mustCast(t, d.handler(), p.ID, v[0], domain.ChoiceYes)
	d.clk.Advance(5 * time.Second)
	retry := d.mustCast(t, d.handler(), p.ID, v[0], domain.ChoiceYes)
	if first.UndoUntil == nil || retry.UndoUntil == nil || !retry.UndoUntil.Equal(*first.UndoUntil) {
		t.Fatalf("undo_until after a retry = %v, want the first window %v", retry.UndoUntil, first.UndoUntil)
	}
	d.clk.Advance(domain.UndoWindow - 6*time.Second)
	if _, err := d.retract(t.Context(), d.retractor(), p.ID, v[0]); err != nil {
		t.Fatalf("RetractVote after an unchanged retry = %v, want it undone inside the first window", err)
	}
}

func TestRetractVote_settledProposalIsClosed(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, v := d.open(t, 3)
	d.mustCast(t, d.handler(), p.ID, v[0], domain.ChoiceYes)
	if got := d.mustCast(t, d.handler(), p.ID, v[1], domain.ChoiceYes); got.Status != domain.StatusPassed {
		t.Fatalf("second yes status = %s, want passed", got.Status)
	}
	d.wantRetractCode(t, p.ID, v[0], errs.CodeProposalClosed)
	d.wantRetractCode(t, p.ID, v[1], errs.CodeProposalClosed)
	if b := d.ballots(t, p.ID); len(b) != 2 {
		t.Fatalf("ballots = %v, want both kept", b)
	}
	if got := d.row(t, p.ID).status.String; got != "passed" {
		t.Fatalf("status = %s, want passed", got)
	}
	if n := len(d.payloads(t, p.ID, events.TypeProposalPassed)); n != 1 {
		t.Fatalf("%d proposal.passed events, want 1", n)
	}
}

func TestRetractVote_refusals(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, v := d.open(t, 3)
	d.wantRetractCode(t, d.ids.NewV7(), v[0], errs.CodeProposalNotFound)
	d.wantRetractCode(t, p.ID, d.ids.NewV7(), errs.CodeNotAVoter)
	d.mustCast(t, d.handler(), p.ID, v[0], domain.ChoiceYes)
	d.clk.Set(p.ExpiresAt)
	d.wantRetractCode(t, p.ID, v[0], errs.CodeProposalClosed)
}

func TestRetractVote_noBallotIsVoteNotFound(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, v := d.open(t, 3)
	d.mustCast(t, d.handler(), p.ID, v[0], domain.ChoiceYes)
	d.wantRetractCode(t, p.ID, v[1], errs.CodeVoteNotFound)
	if _, err := d.retract(t.Context(), d.retractor(), p.ID, v[0]); err != nil {
		t.Fatal(err)
	}
	d.wantRetractCode(t, p.ID, v[0], errs.CodeVoteNotFound)
}

func TestRetractVote_racesADecidingVote(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, v := d.open(t, 3)
	d.mustCast(t, d.handler(), p.ID, v[0], domain.ChoiceYes)
	var undone, voted error
	var settled app.CastVoteResult
	var wg sync.WaitGroup
	wg.Go(func() { _, undone = d.retract(t.Context(), d.retractor(), p.ID, v[0]) })
	wg.Go(func() { settled, voted = d.cast(t.Context(), d.handler(), p.ID, v[1], domain.ChoiceYes) })
	wg.Wait()
	if voted != nil {
		t.Fatalf("the other member's vote err = %v, want it counted either way", voted)
	}
	switch {
	case undone == nil:
		d.wantUndoFirst(t, p.ID, v[1], settled)
	case errs.CodeOf(undone) == errs.CodeProposalClosed:
		d.wantVoteFirst(t, p.ID, settled)
	default:
		t.Fatalf("undo err = %v, want success or proposal_closed", undone)
	}
}

func (d voteDB) wantUndoFirst(t *testing.T, proposal, other uuid.UUID, vote app.CastVoteResult) {
	t.Helper()
	if vote.Status != domain.StatusOpen || vote.Tally.Yes != 1 {
		t.Fatalf("undo first: vote = %+v, want open with one yes", vote)
	}
	if b := d.ballots(t, proposal); len(b) != 1 || b[other] != "yes" {
		t.Fatalf("undo first: ballots = %v, want only the other voter's yes", b)
	}
	if n := d.terminal(t, proposal); n != 0 {
		t.Fatalf("undo first: %d terminal events, want 0", n)
	}
}

func (d voteDB) wantVoteFirst(t *testing.T, proposal uuid.UUID, vote app.CastVoteResult) {
	t.Helper()
	if vote.Status != domain.StatusPassed {
		t.Fatalf("vote first: vote = %+v, want passed", vote)
	}
	if b := d.ballots(t, proposal); len(b) != 2 {
		t.Fatalf("vote first: ballots = %v, want both kept", b)
	}
	if n := len(d.payloads(t, proposal, events.TypeProposalPassed)); n != 1 || d.terminal(t, proposal) != 1 {
		t.Fatalf("vote first: %d proposal.passed events, want exactly 1", n)
	}
}

func TestRetractVote_aFailedStepLeavesTheBallotInPlace(t *testing.T) {
	t.Parallel()
	const trigger = `CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN %s END $$`
	cases := map[string]struct {
		setup []string
		want  errs.Code
	}{
		"reading the ballot": {[]string{`ALTER TABLE votes RENAME COLUMN first_cast_at TO moved`}, errs.CodeInternal},
		"deleting the ballot": {[]string{
			fmt.Sprintf(trigger, "refuse", "RAISE EXCEPTION 'refused';"),
			`CREATE TRIGGER refuse BEFORE DELETE ON votes FOR EACH ROW EXECUTE FUNCTION refuse()`,
		}, errs.CodeInternal},
		"a delete that removes nothing": {[]string{
			fmt.Sprintf(trigger, "skip", "RETURN NULL;"),
			`CREATE TRIGGER skip BEFORE DELETE ON votes FOR EACH ROW EXECUTE FUNCTION skip()`,
		}, errs.CodeUndoExpired},
		"counting what is left": {[]string{
			fmt.Sprintf(trigger, "lose", "PERFORM set_config('search_path', 'pg_catalog', true); RETURN NULL;"),
			`CREATE TRIGGER lose AFTER DELETE ON votes FOR EACH ROW EXECUTE FUNCTION lose()`,
		}, errs.CodeInternal},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := newVoteDB(t)
			p, v := d.open(t, 3)
			d.mustCast(t, d.handler(), p.ID, v[0], domain.ChoiceYes)
			for _, stmt := range tc.setup {
				if _, err := d.pool.Exec(t.Context(), stmt); err != nil {
					t.Fatal(err)
				}
			}
			d.wantRetractCode(t, p.ID, v[0], tc.want)
			if b := d.ballots(t, p.ID); len(b) != 1 || b[v[0]] != "yes" {
				t.Fatalf("ballots = %v, want the yes kept", b)
			}
		})
	}
}

func TestRetractVote_overHTTPRefusesCallersThatAreNotAUser(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		actor *auth.Actor
		want  errs.Code
	}{
		"no actor": {nil, errs.CodeUnauthorized},
		"service":  {&auth.Actor{Kind: auth.ActorService, ID: "a1"}, errs.CodeForbidden},
	} {
		ctx := t.Context()
		if tc.actor != nil {
			ctx = auth.WithActor(ctx, *tc.actor)
		}
		_, err := (adapters.HTTP{}).DeleteProposalVote(ctx, api.DeleteProposalVoteRequestObject{})
		if errs.CodeOf(err) != tc.want {
			t.Errorf("%s: DeleteProposalVote err = %v, want %s", name, err, tc.want)
		}
	}
}

func TestRetractVote_overHTTPTakesTheBallotBack(t *testing.T) {
	t.Parallel()
	s := scenario.New(t, withGovernance())
	c := testkit.NewCabal(t, s.DB(), testkit.WithMembers(3))
	id, now := testkit.NewIDs(21).NewV7(), clock.Real{}.Now().UTC()
	params := sqlc.InsertProposalParams{
		ID: id, CabalID: c.ID.UUID(), ProposerID: c.Creator.ID.UUID(), Kind: "buy", Symbol: "AAPLx",
		Mint: aaplxMint, UsdcMicros: pgtype.Int8{Int64: 5_000_000, Valid: true}, QuoteOutAmount: 21_000_000,
		ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now, Threshold: "majority",
	}
	for _, m := range c.Members {
		params.VoterIds = append(params.VoterIds, m.ID.UUID())
	}
	if _, err := sqlc.New(s.DB()).InsertProposal(t.Context(), params); err != nil {
		t.Fatal(err)
	}
	votes := "/v1/proposals/" + id.String() + "/votes"
	s.Given(scenario.AsSeededUser("creator", c.Creator.ID)).
		When(
			scenario.Post(votes, `{"choice":"yes"}`),
			scenario.ExpectStatus(http.StatusOK),
			expectUndoWindow(t, now),
			scenario.Delete(votes),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("status", "open"),
			scenario.ExpectJSON("tally", map[string]int{"yes": 0, "no": 0, "voters": 3, "needed": 2}),
			scenario.Get("/v1/proposals/"+id.String()),
			scenario.ExpectJSON("my_ballot", nil),
			scenario.Delete(votes),
			scenario.ExpectStatus(http.StatusNotFound),
			scenario.ExpectProblem(errs.CodeVoteNotFound),
		).
		Then(scenario.ExpectStatus(http.StatusNotFound))
}

func expectUndoWindow(t *testing.T, from time.Time) scenario.Step {
	t.Helper()
	return scenario.ExpectField("undo_until", func(_ *scenario.Scenario, raw json.RawMessage) {
		var until time.Time
		if err := json.Unmarshal(raw, &until); err != nil {
			t.Fatalf("undo_until %s: %v", raw, err)
		}
		if window := until.Sub(from); window < domain.UndoWindow || window > domain.UndoWindow+time.Minute {
			t.Fatalf("undo_until %s is %s after the request began, want about %s", raw, window, domain.UndoWindow)
		}
	})
}
