package governance_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
)

func (d voteDB) voteRetractions(t *testing.T, proposal uuid.UUID) []events.VoteRetracted {
	t.Helper()
	raws := d.payloads(t, proposal, events.TypeVoteRetracted)
	out := make([]events.VoteRetracted, 0, len(raws))
	for _, raw := range raws {
		var ev events.VoteRetracted
		if err := json.Unmarshal(raw, &ev); err != nil {
			t.Fatal(err)
		}
		out = append(out, ev)
	}
	return out
}

func TestRetractVote_appendsOneVoteRetractedPerUndoWithTheDeletedChoice(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, v := d.open(t, 3)
	for i, choice := range []domain.Choice{domain.ChoiceYes, domain.ChoiceNo} {
		d.mustCast(t, d.handler(), p.ID, v[0], choice)
		d.clk.Advance(time.Second)
		if _, err := d.retract(t.Context(), d.retractor(), p.ID, v[0]); err != nil {
			t.Fatalf("RetractVote #%d = %v", i, err)
		}
		got := d.voteRetractions(t, p.ID)
		if len(got) != i+1 {
			t.Fatalf("after undo #%d: vote.retracted events = %+v, want %d", i, got, i+1)
		}
		last := got[i]
		if !last.RetractedAt.Equal(d.clk.Now()) {
			t.Fatalf("vote.retracted[%d].retracted_at = %s, want %s", i, last.RetractedAt, d.clk.Now())
		}
		last.RetractedAt = time.Time{}
		want := events.VoteRetracted{V: 1, ProposalID: p.ID, CabalID: p.CabalID, VoterID: v[0], Choice: string(choice)}
		if last != want {
			t.Fatalf("vote.retracted[%d] = %+v, want %+v", i, last, want)
		}
	}
}

func TestRetractVote_aRefusedUndoAppendsNoVoteRetracted(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	closed, cv := d.open(t, 1)
	d.mustCast(t, d.handler(), closed.ID, cv[0], domain.ChoiceNo)
	open, v := d.open(t, 5)
	d.mustCast(t, d.handler(), open.ID, v[0], domain.ChoiceYes)
	d.mustCast(t, d.handler(), open.ID, v[1], domain.ChoiceNo)
	d.mustCast(t, d.handler(), open.ID, v[1], domain.ChoiceYes)
	d.wantRetractCode(t, closed.ID, cv[0], errs.CodeProposalClosed)
	d.wantRetractCode(t, open.ID, d.ids.NewV7(), errs.CodeNotAVoter)
	d.wantRetractCode(t, open.ID, v[4], errs.CodeVoteNotFound)
	d.wantRetractCode(t, open.ID, v[1], errs.CodeUndoExpired)
	d.clk.Advance(domain.UndoWindow)
	d.wantRetractCode(t, open.ID, v[0], errs.CodeUndoExpired)
	for _, id := range []uuid.UUID{closed.ID, open.ID} {
		if got := d.voteRetractions(t, id); len(got) != 0 {
			t.Fatalf("vote.retracted events on %s = %+v after refused undos, want none", id, got)
		}
	}
}

func TestRetractVote_aFailedAppendLeavesTheBallotInPlace(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, v := d.open(t, 3)
	d.mustCast(t, d.handler(), p.ID, v[0], domain.ChoiceYes)
	for _, stmt := range []string{
		`CREATE FUNCTION veto() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'vetoed'; END $$`,
		`CREATE TRIGGER veto BEFORE INSERT ON events FOR EACH ROW EXECUTE FUNCTION veto()`,
	} {
		if _, err := d.pool.Exec(t.Context(), stmt); err != nil {
			t.Fatal(err)
		}
	}
	d.wantRetractCode(t, p.ID, v[0], errs.CodeInternal)
	if b := d.ballots(t, p.ID); len(b) != 1 || b[v[0]] != "yes" {
		t.Fatalf("ballots = %v, want the yes kept", b)
	}
	if got := d.voteRetractions(t, p.ID); len(got) != 0 {
		t.Fatalf("vote.retracted events = %+v after a failed append, want none", got)
	}
}
