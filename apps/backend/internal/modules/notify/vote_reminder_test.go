package notify_test

import (
	"cmp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	reminderBody = "Buy AAPL $25: 4 yes, 1 no so far. Yours counts."
	reminderLeft = "3 votes left · ends in 4h"
)

func (w voteWorld) openAtOneThird(t *testing.T) uuid.UUID {
	t.Helper()
	return w.propose(t, w.r.clock.Now().Add(-2*time.Hour))
}

func (w voteWorld) fixtureBallots(t *testing.T, proposal uuid.UUID) {
	t.Helper()
	w.castAll(t, proposal, map[int]string{0: "yes", 1: "yes", 2: "yes", 3: "yes", 4: "no"})
}

func (w voteWorld) sentTo(r *pushRig) []ids.UserID {
	sent := r.sender.Sent()
	to := make([]ids.UserID, 0, len(sent))
	for _, p := range sent {
		to = append(to, p.UserID)
	}
	slices.SortFunc(to, func(a, b ids.UserID) int { return strings.Compare(a.String(), b.String()) })
	return to
}

func sortedUsers(users ...ids.UserID) []ids.UserID {
	slices.SortFunc(users, func(a, b ids.UserID) int { return strings.Compare(a.String(), b.String()) })
	return users
}

func TestRemindVoters_reachesTheMembersWhoHaveNotVotedAtAThirdOfTheWindowOnce(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.voteWorldOf(t, 8)
	proposal := w.openAtOneThird(t)
	w.fixtureBallots(t, proposal)
	p := w.reminders()

	wantTick(t, p, 1, 3)

	waiting := w.members[5:]
	if got, want := w.sentTo(r), sortedUsers(waiting...); !slices.Equal(got, want) {
		t.Fatalf("reminded %v, want the three who have not voted %v", got, want)
	}
	for _, push := range r.sender.Sent() {
		want := w.pushTo(push.UserID, proposal, reminderLeft, reminderBody, voteCategory)
		if !equalPush(push, want) {
			t.Fatalf("push = %+v, want %+v", push, want)
		}
	}
	r.wantCount(t, "cursor rows", 1, `SELECT count(*) FROM notify_reminder_cursor`)
	wantTick(t, p, 0, 0)
	if got := len(r.sender.Sent()); got != 3 {
		t.Fatalf("sends after the second tick = %d, want still 3", got)
	}
}

func equalPush(a, b apns.Push) bool {
	return a.UserID == b.UserID && a.Token == b.Token && a.CollapseID == b.CollapseID && a.Title == b.Title &&
		a.Body == b.Body && a.Category == b.Category && a.InterruptionLevel == b.InterruptionLevel &&
		a.ThreadID == b.ThreadID && a.MutableContent == b.MutableContent && a.Sound == b.Sound &&
		mapsEqual(a.Data, b.Data)
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestRemindVoters_waitsForTheThirdMark(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.voteWorldOf(t, 8)
	proposal := w.openAtOneThird(t)
	w.fixtureBallots(t, proposal)
	r.clock.Advance(-time.Second)
	p := w.reminders()

	wantTick(t, p, 0, 0)
	r.clock.Advance(time.Second)
	wantTick(t, p, 1, 3)
}

func TestRemindVoters_countsOneVoteLeftInTheSingular(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.voteWorldOf(t, 3)
	proposal := w.openAtOneThird(t)
	w.castAll(t, proposal, map[int]string{0: "yes", 1: "no"})

	wantTick(t, w.reminders(), 1, 1)

	sent := r.sender.Sent()
	if len(sent) != 1 || sent[0].Title != "1 vote left · ends in 4h" ||
		sent[0].Body != "Buy AAPL $25: 1 yes, 1 no so far. Yours counts." {
		t.Fatalf("sent %+v, want the singular reminder", sent)
	}
}

func TestRemindVoters_sendsNothingForAProposalDecidedBeforeTheMark(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"passed", "failed", "expired", "withdrawn", "void"} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)
			w := r.voteWorldOf(t, 8)
			proposal := w.openAtOneThird(t)
			w.fixtureBallots(t, proposal)
			r.exec(t, `UPDATE proposals SET status = $2 WHERE id = $1`, proposal, status)

			wantTick(t, w.reminders(), 0, 0)

			if len(r.sender.Sent()) != 0 {
				t.Fatalf("sent %+v, want nothing for a %s proposal", r.sender.Sent(), status)
			}
		})
	}
}

func TestRemindVoters_sendsNothingAfterTheCloseOrForARuleProposal(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.voteWorldOf(t, 8)
	closed := w.propose(t, r.clock.Now().Add(-voteWindow))
	rule := w.openAtOneThird(t)
	r.exec(
		t,
		`UPDATE proposals SET kind = 'rule', rule_change = '{"v":1,"rule":"show_sides","from":false,"to":true}', symbol = NULL, mint = NULL, quote_out_amount = NULL, usdc_micros = NULL, token_amount = NULL WHERE id = $1`,
		rule,
	)
	w.fixtureBallots(t, closed)

	wantTick(t, w.reminders(), 0, 0)

	if len(r.sender.Sent()) != 0 {
		t.Fatalf("sent %+v, want nothing", r.sender.Sent())
	}
}

func TestRemindVoters_leavesOutNonMembersAndDisabledPreferences(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.voteWorldOf(t, 8, 6)
	proposal := w.openAtOneThird(t)
	w.fixtureBallots(t, proposal)
	r.exec(
		t,
		`INSERT INTO notification_preferences (user_id, category, enabled, updated_at) VALUES ($1, 'decision_requests', false, $2)`,
		w.members[7].UUID(),
		r.clock.Now(),
	)

	wantTick(t, w.reminders(), 1, 1)

	if got, want := w.sentTo(r), []ids.UserID{w.members[5]}; !slices.Equal(got, want) {
		t.Fatalf("reminded %v, want only the member who is in the cabal and has the category on %v", got, want)
	}
}

func TestRemindVoters_skipsAProposalWithNoOneToRemindAndStillMovesOn(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.voteWorldOf(t, 3)
	proposal := w.openAtOneThird(t)
	w.castAll(t, proposal, map[int]string{0: "yes", 1: "yes", 2: "no"})
	p := w.reminders()

	wantTick(t, p, 1, 0)
	wantTick(t, p, 0, 0)

	r.wantCount(t, "cursor rows", 1, `SELECT count(*) FROM notify_reminder_cursor`)
}

func TestRemindVoters_doesNotRemindWhoGotTheDecidingPush(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.voteWorldOf(t, 8)
	proposal := w.openAtOneThird(t)
	w.castAll(t, proposal, map[int]string{0: "yes", 1: "yes", 2: "yes", 3: "no"})
	w.cast(t, proposal, w.members[4], "yes")
	e := w.voteCast(proposal, w.members[4], "yes")
	d := r.emit(t, userActor(w.members[4]), e)
	if err := handleKinds(t, r, r.sender, d, e, w.deciding()); err != nil {
		t.Fatal(err)
	}
	decided := len(r.sender.Sent())

	wantTick(t, w.reminders(), 1, 0)

	if decided != 3 || len(r.sender.Sent()) != decided {
		t.Fatalf("sends = %d after %d deciding pushes, want no reminder on top", len(r.sender.Sent()), decided)
	}
}

func TestRemindVoters_remindsEveryDueProposalInOrderAcrossBatches(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.voteWorldOf(t, 2)
	now := r.clock.Now()
	r.exec(t, `INSERT INTO proposals (id, cabal_id, proposer_id, kind, symbol, mint, usdc_micros, quote_out_amount,
			threshold, status, expires_at, created_at, updated_at)
		SELECT gen_random_uuid(), $1, $2, 'buy', 'AAPLx', $3, 25000000, 1, 'majority', 'open',
			$4::timestamptz + interval '4 hours' - n * interval '1 second',
			$4::timestamptz - interval '2 hours' - n * interval '1 second', $4
		FROM generate_series(1, 120) AS n`, w.cabalID.UUID(), w.members[0].UUID(), voteWorldMint, now)
	r.exec(t, `INSERT INTO proposal_voters (proposal_id, voter_id) SELECT id, $1 FROM proposals WHERE cabal_id = $2`,
		w.members[1].UUID(), w.cabalID.UUID())
	p := w.reminders()

	wantTick(t, p, 50, 50)
	first := cursorOf(t, r)
	r.wantCount(t, "reminded proposals due after one that was not reminded", 0, `SELECT count(*) FROM proposals a
		JOIN proposals b ON b.cabal_id = a.cabal_id AND a.created_at > b.created_at
		WHERE a.id::text IN (SELECT data->>'proposal_id' FROM notifications)
		  AND b.id::text NOT IN (SELECT data->>'proposal_id' FROM notifications)`)
	wantTick(t, p, 50, 50)
	second := cursorOf(t, r)
	wantTick(t, p, 20, 20)
	third := cursorOf(t, r)
	wantTick(t, p, 0, 0)

	if !first.before(second) || !second.before(third) {
		t.Fatalf("cursor went %+v, %+v, %+v, want it to move forward each batch", first, second, third)
	}
	r.wantCount(t, "reminder rows", 120, `SELECT count(*) FROM notifications WHERE kind = 'vote_reminder'`)

	r.clock.Advance(time.Minute)
	w.propose(t, r.clock.Now().Add(-2*time.Hour))
	wantTick(t, p, 1, 2)
}

type cursor struct {
	dueAt time.Time
	id    uuid.UUID
}

func (c cursor) before(o cursor) bool {
	return cmp.Or(c.dueAt.Compare(o.dueAt), strings.Compare(c.id.String(), o.id.String())) < 0
}

func cursorOf(t *testing.T, r *pushRig) cursor {
	t.Helper()
	var c cursor
	if err := r.pool.QueryRow(t.Context(), `SELECT due_at, proposal_id FROM notify_reminder_cursor`).
		Scan(&c.dueAt, &c.id); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestRemindVoters_theConfiguredIntervalReplacesTheMinute(t *testing.T) {
	t.Parallel()
	if got := app.NewRemindVoters(nil, app.VotePorts{}, 3*time.Second).Interval(); got != 3*time.Second {
		t.Fatalf("Interval() = %v, want the configured 3s", got)
	}
	if got := app.NewRemindVoters(nil, app.VotePorts{}, 0).Interval(); got != time.Minute {
		t.Fatalf("Interval() = %v, want the default minute", got)
	}
}
