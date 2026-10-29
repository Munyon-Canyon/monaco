package notify_test

import (
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const decidingBody = "Yours decides it: Buy AAPL $25 stands at 4 yes, 1 no and needs one more yes. Closes in 6h."

func (w voteWorld) deliver(t *testing.T, e events.VoteCast) error {
	t.Helper()
	d := w.r.emit(t, userActor(ids.UserIDFrom(e.VoterID)), e)
	return handleKinds(t, w.r, w.r.sender, d, e, w.deciding())
}

func TestVoteDeciding_pushesTheMembersWhoHaveNotVotedOnceWhenOneYesPassesIt(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.voteWorldOf(t, 8)
	proposal := w.proposeNow(t)
	w.castAll(t, proposal, map[int]string{0: "yes", 1: "yes", 2: "yes", 3: "no"})
	w.cast(t, proposal, w.members[4], "yes")

	if err := w.deliver(t, w.voteCast(proposal, w.members[4], "yes")); err != nil {
		t.Fatal(err)
	}

	waiting := w.members[5:]
	if got, want := w.sentTo(r), sortedUsers(waiting...); !slices.Equal(got, want) {
		t.Fatalf("pushed %v, want the three who have not voted %v", got, want)
	}
	for _, push := range r.sender.Sent() {
		want := w.pushTo(push.UserID, proposal, cabalName, decidingBody, deciderCategory)
		if !equalPush(push, want) {
			t.Fatalf("push = %+v, want %+v", push, want)
		}
	}

	w.cast(t, proposal, w.members[5], "no")
	if err := w.deliver(t, w.voteCast(proposal, w.members[5], "no")); err != nil {
		t.Fatal(err)
	}
	w.cast(t, proposal, w.members[5], "yes")
	if err := w.deliver(t, w.voteCast(proposal, w.members[5], "yes")); err != nil {
		t.Fatal(err)
	}
	w.cast(t, proposal, w.members[5], "no")
	if err := w.deliver(t, w.voteCast(proposal, w.members[5], "no")); err != nil {
		t.Fatal(err)
	}

	if got := len(r.sender.Sent()); got != 3 {
		t.Fatalf("sends = %d after the tally moved and came back, want the 3 from the first push only", got)
	}
	r.wantCount(t, "deciding rows", 3, `SELECT count(*) FROM notifications WHERE kind = 'vote_deciding'`)
}

func TestVoteDeciding_waitsUntilExactlyOneYesIsMissing(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		ballots map[int]string
		last    int
	}{
		"two yes short":               {map[int]string{0: "yes", 1: "yes", 2: "no"}, 2},
		"a yes that already decided":  {map[int]string{0: "yes", 1: "yes", 2: "yes", 3: "yes", 4: "yes"}, 4},
		"a no with the yes still low": {map[int]string{0: "yes", 1: "no", 2: "no", 3: "no"}, 3},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)
			w := r.voteWorldOf(t, 8)
			proposal := w.proposeNow(t)
			w.castAll(t, proposal, tc.ballots)

			if err := w.deliver(t, w.voteCast(proposal, w.members[tc.last], tc.ballots[tc.last])); err != nil {
				t.Fatal(err)
			}

			if len(r.sender.Sent()) != 0 {
				t.Fatalf("sent %+v, want nothing", r.sender.Sent())
			}
		})
	}
}

func TestVoteDeciding_pushesNobodyOnceTheProposalIsClosedExpiredOrARule(t *testing.T) {
	t.Parallel()
	for name, update := range map[string]string{
		"passed":         `UPDATE proposals SET status = 'passed' WHERE id = $1`,
		"rule":           `UPDATE proposals SET kind = 'rule', rule_change = '{"v":1,"rule":"show_sides","from":false,"to":true}', symbol = NULL, mint = NULL, quote_out_amount = NULL, usdc_micros = NULL, token_amount = NULL WHERE id = $1`,
		"past its close": `UPDATE proposals SET expires_at = created_at WHERE id = $1`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)
			w := r.voteWorldOf(t, 8)
			proposal := w.proposeNow(t)
			w.castAll(t, proposal, map[int]string{0: "yes", 1: "yes", 2: "yes", 3: "no", 4: "yes"})
			r.exec(t, update, proposal)

			if err := w.deliver(t, w.voteCast(proposal, w.members[4], "yes")); err != nil {
				t.Fatal(err)
			}

			if len(r.sender.Sent()) != 0 {
				t.Fatalf("sent %+v, want nothing", r.sender.Sent())
			}
		})
	}
}

func TestVoteDeciding_aRedeliveredVoteCastPushesNobodyTwice(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.voteWorldOf(t, 8)
	proposal := w.proposeNow(t)
	w.castAll(t, proposal, map[int]string{0: "yes", 1: "yes", 2: "yes", 3: "no", 4: "yes"})
	e := w.voteCast(proposal, w.members[4], "yes")
	d := r.emit(t, userActor(w.members[4]), e)

	for range 2 {
		if err := handleKinds(t, r, r.sender, d, e, w.deciding()); err != nil {
			t.Fatal(err)
		}
	}

	if got := len(r.sender.Sent()); got != 3 {
		t.Fatalf("sends = %d, want 3", got)
	}
	r.wantCount(t, "deciding rows", 3, `SELECT count(*) FROM notifications WHERE kind = 'vote_deciding'`)
	r.wantRecorded(t, d, 1)
}

func TestVoteDeciding_decisionRequestsOffSuppressesIt(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.voteWorldOf(t, 8)
	proposal := w.proposeNow(t)
	w.castAll(t, proposal, map[int]string{0: "yes", 1: "yes", 2: "yes", 3: "no", 4: "yes"})
	r.exec(
		t,
		`INSERT INTO notification_preferences (user_id, category, enabled, updated_at) VALUES ($1, 'decision_requests', false, $2)`,
		w.members[6].UUID(),
		r.clock.Now(),
	)

	if err := w.deliver(t, w.voteCast(proposal, w.members[4], "yes")); err != nil {
		t.Fatal(err)
	}

	if got, want := w.sentTo(r), sortedUsers(w.members[5], w.members[7]); !slices.Equal(got, want) {
		t.Fatalf("pushed %v, want the two who have the category on %v", got, want)
	}
}
