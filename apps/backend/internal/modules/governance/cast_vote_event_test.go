package governance_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
)

func (d voteDB) voteCasts(t *testing.T, proposal uuid.UUID) []events.VoteCast {
	t.Helper()
	raws := d.payloads(t, proposal, events.TypeVoteCast)
	out := make([]events.VoteCast, 0, len(raws))
	for _, raw := range raws {
		var ev events.VoteCast
		if err := json.Unmarshal(raw, &ev); err != nil {
			t.Fatal(err)
		}
		out = append(out, ev)
	}
	return out
}

func TestCastVote_appendsOneVoteCastPerBallot(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	h := d.handler()
	p, v := d.open(t, 3)
	d.mustCast(t, h, p.ID, v[0], domain.ChoiceNo)
	d.mustCast(t, h, p.ID, v[0], domain.ChoiceYes)
	got := d.voteCasts(t, p.ID)
	if len(got) != 2 {
		t.Fatalf("vote.cast events = %+v, want one per ballot including the changed one", got)
	}
	for i, choice := range []string{"no", "yes"} {
		want := events.VoteCast{
			V: 1, ProposalID: p.ID, CabalID: p.CabalID, VoterID: v[0], Choice: choice, CastAt: d.clk.Now(),
		}
		if !got[i].CastAt.Equal(want.CastAt) {
			t.Fatalf("vote.cast[%d].cast_at = %s, want %s", i, got[i].CastAt, want.CastAt)
		}
		got[i].CastAt = want.CastAt
		if got[i] != want {
			t.Fatalf("vote.cast[%d] = %+v, want %+v", i, got[i], want)
		}
	}
}

func TestCastVote_decidingVoteAppendsVoteCastBeforeTheDecision(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, v := d.open(t, 1)
	d.mustCast(t, d.handler(), p.ID, v[0], domain.ChoiceYes)
	rows, err := d.pool.Query(t.Context(), `SELECT type FROM events WHERE aggregate_id = $1
		AND type IN ('vote.cast', 'proposal.passed') ORDER BY id`, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var typ string
		if err := rows.Scan(&typ); err != nil {
			t.Fatal(err)
		}
		got = append(got, typ)
	}
	if want := []string{"vote.cast", "proposal.passed"}; !slices.Equal(got, want) {
		t.Fatalf("events = %q, want %q", got, want)
	}
}

func TestCastVote_aRefusedBallotAppendsNoVoteCast(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, _ := d.open(t, 1)
	if _, err := d.cast(t.Context(), d.handlerFor(cabalStatus{}), p.ID, d.ids.NewV7(), domain.ChoiceYes); err == nil {
		t.Fatal("a stranger's ballot succeeded")
	}
	if got := d.voteCasts(t, p.ID); len(got) != 0 {
		t.Fatalf("vote.cast events = %+v after a refused ballot, want none", got)
	}
}
