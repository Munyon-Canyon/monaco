package governance_test

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type afterListClock struct {
	clock.Clock
	calls *int
	run   func()
}

func (c afterListClock) Now() time.Time {
	*c.calls++
	if *c.calls == 2 {
		c.run()
	}
	return c.Clock.Now()
}

func (d voteDB) afterList(run func()) *app.CloseReminderPoller {
	return app.NewCloseReminderPoller(d.uow, d.pool, afterListClock{Clock: d.clk, calls: new(int), run: run})
}

func (d voteDB) closeReminders() *app.CloseReminderPoller {
	return app.NewCloseReminderPoller(d.uow, d.pool, d.clk)
}

func (d voteDB) remindersTick(ctx context.Context, p *app.CloseReminderPoller) (poller.Report, error) {
	return p.Tick(observability.WithActor(ctx, "system:poller.governance.close_reminders"))
}

func (d voteDB) mustRemind(t *testing.T, p *app.CloseReminderPoller, scanned, changed int) {
	t.Helper()
	got, err := d.remindersTick(t.Context(), p)
	if err != nil || got.Scanned != scanned || got.Changed != changed {
		t.Fatalf("Tick = %+v, %v, want scanned %d changed %d", got, err, scanned, changed)
	}
}

func (d voteDB) closeReminderDues(t *testing.T, proposal uuid.UUID) []events.ProposalCloseReminderDue {
	t.Helper()
	raws := d.payloads(t, proposal, events.TypeProposalCloseReminderDue)
	out := make([]events.ProposalCloseReminderDue, 0, len(raws))
	for _, raw := range raws {
		var ev events.ProposalCloseReminderDue
		if err := json.Unmarshal(raw, &ev); err != nil {
			t.Fatal(err)
		}
		out = append(out, ev)
	}
	return out
}

func (d voteDB) dueReminder(t *testing.T) (sqlc.InsertProposalParams, []uuid.UUID) {
	t.Helper()
	p, v := d.openWithUsers(t)
	d.clk.Set(p.ExpiresAt.Add(-4 * time.Hour))
	d.mustSet(t, p.ID, v[0])
	d.clk.Set(p.ExpiresAt.Add(-domain.ReminderLead))
	return p, v
}

func (d voteDB) wantSentWithoutEvent(t *testing.T, p sqlc.InsertProposalParams, user uuid.UUID) {
	t.Helper()
	if got := d.closeReminderDues(t, p.ID); len(got) != 0 {
		t.Fatalf("events = %+v, want none", got)
	}
	if rows := d.reminderRows(t, p.ID, user); len(rows) != 1 || rows[0].SentAt == nil {
		t.Fatalf("rows = %+v, want one row marked sent", rows)
	}
}

func (d voteDB) wantNoRowAndNoEvent(t *testing.T, p sqlc.InsertProposalParams, user uuid.UUID) {
	t.Helper()
	if got := d.closeReminderDues(t, p.ID); len(got) != 0 {
		t.Fatalf("events = %+v, want none", got)
	}
	if rows := d.reminderRows(t, p.ID, user); len(rows) != 0 {
		t.Fatalf("rows = %+v, want none", rows)
	}
}

func TestCloseReminderPoller_appendsOncePerMember(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, v := d.openWithUsers(t)
	d.clk.Set(p.ExpiresAt.Add(-4 * time.Hour))
	d.mustSet(t, p.ID, v[0])
	d.mustSet(t, p.ID, v[1])
	d.mustCast(t, d.handler(), p.ID, v[2], domain.ChoiceNo)
	reminders := d.closeReminders()
	d.mustRemind(t, reminders, 0, 0)

	d.clk.Set(p.ExpiresAt.Add(-domain.ReminderLead))
	d.mustRemind(t, reminders, 2, 2)
	d.wantTwoReminders(t, p, v, [3]int{0, 1, 3})
	d.clk.Advance(time.Minute)
	d.mustRemind(t, reminders, 0, 0)
	if again := d.closeReminderDues(t, p.ID); len(again) != 2 {
		t.Fatalf("events after later ticks = %d, want 2", len(again))
	}
}

func TestCloseReminderPoller_skipsClosedOrVoted(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	reminders := d.closeReminders()

	failed, fv := d.dueReminder(t)
	d.transition(t, failed.ID, "open", "failed", "")
	passed, pv := d.dueReminder(t)
	d.transition(t, passed.ID, "open", "passed", "")
	expired, ev := d.dueReminder(t)
	d.clk.Set(expired.ExpiresAt.Add(time.Second))
	d.mustRemind(t, reminders, 3, 0)
	d.wantSentWithoutEvent(t, failed, fv[0])
	d.wantSentWithoutEvent(t, passed, pv[0])
	d.wantSentWithoutEvent(t, expired, ev[0])
	d.mustRemind(t, reminders, 0, 0)
}

func TestCloseReminderPoller_voteBetweenListAndProcess(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, v := d.dueReminder(t)
	reminders := d.afterList(func() {
		d.mustCast(t, d.handler(), p.ID, v[0], domain.ChoiceYes)
	})
	d.mustRemind(t, reminders, 1, 0)
	d.wantNoRowAndNoEvent(t, p, v[0])
}

func TestCloseReminderPoller_clearBetweenListAndProcess(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, v := d.dueReminder(t)
	reminders := d.afterList(func() {
		if _, err := d.clearReminder(t, p.ID, v[0]); err != nil {
			t.Error(err)
		}
	})
	d.mustRemind(t, reminders, 1, 0)
	d.wantNoRowAndNoEvent(t, p, v[0])
}

func TestCloseReminderPoller_closeBetweenListAndProcess(t *testing.T) {
	t.Parallel()
	closes := map[string]string{"passes": "passed", "fails": "failed", "expires": ""}
	for name, to := range closes {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := newVoteDB(t)
			p, v := d.dueReminder(t)
			reminders := d.afterList(func() {
				if to == "" {
					d.clk.Set(p.ExpiresAt)
					return
				}
				d.transition(t, p.ID, "open", to, "")
			})
			d.mustRemind(t, reminders, 1, 0)
			d.wantSentWithoutEvent(t, p, v[0])
		})
	}
}

func TestCloseReminderPoller_secondPassSendsNothing(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, v := d.dueReminder(t)
	var inner *app.CloseReminderPoller
	var concurrent poller.Report
	outer := d.afterList(func() {
		var err error
		if concurrent, err = d.remindersTick(t.Context(), inner); err != nil {
			t.Error(err)
		}
	})
	inner = d.closeReminders()
	d.mustRemind(t, outer, 1, 0)
	if concurrent.Scanned != 1 || concurrent.Changed != 1 {
		t.Fatalf("the pass that ran first = %+v, want scanned 1 changed 1", concurrent)
	}
	if got := d.closeReminderDues(t, p.ID); len(got) != 1 || got[0].UserID != v[0] {
		t.Fatalf("events = %+v, want one for %s", got, v[0])
	}
}

func (d voteDB) wantTwoReminders(t *testing.T, p sqlc.InsertProposalParams, v []uuid.UUID, tally [3]int) {
	t.Helper()
	got := d.closeReminderDues(t, p.ID)
	if len(got) != 2 {
		t.Fatalf("events = %+v, want one per member", got)
	}
	for _, ev := range got {
		want := events.ProposalCloseReminderDue{
			V: 1, ProposalID: p.ID, CabalID: p.CabalID, UserID: ev.UserID, Kind: p.Kind, Symbol: p.Symbol,
			USDCMicros: ev.USDCMicros, ExpiresAt: ev.ExpiresAt, Yes: tally[0], No: tally[1], Voters: tally[2],
		}
		if ev != want || !ev.ExpiresAt.Equal(p.ExpiresAt) || ev.USDCMicros.String() != "25000000" {
			t.Errorf("event = %+v, want %+v with 25000000 micros expiring %s", ev, want, p.ExpiresAt)
		}
	}
	users := []uuid.UUID{got[0].UserID, got[1].UserID}
	if !slices.Contains(users, v[0]) || !slices.Contains(users, v[1]) {
		t.Errorf("users = %v, want %s and %s once each", users, v[0], v[1])
	}
	for _, user := range v[:2] {
		rows := d.reminderRows(t, p.ID, user)
		if len(rows) != 1 || rows[0].SentAt == nil || !rows[0].SentAt.Equal(d.clk.Now()) {
			t.Errorf("rows of %s = %+v, want one sent at the tick's clock", user, rows)
		}
	}
}

func TestCloseReminderPoller_skipsAMemberWhoVotedEvenIfARowSurvived(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, v := d.dueReminder(t)
	d.mustCast(t, d.handler(), p.ID, v[0], domain.ChoiceYes)
	if _, err := d.pool.Exec(t.Context(),
		`INSERT INTO proposal_close_reminders (proposal_id, user_id, due_at, created_at) VALUES ($1, $2, $3, $3)`,
		p.ID, v[0], d.clk.Now()); err != nil {
		t.Fatal(err)
	}
	d.mustRemind(t, d.closeReminders(), 1, 0)
	d.wantSentWithoutEvent(t, p, v[0])
}

func TestCloseReminderPoller_aFailedStepLeavesTheReminderUnsent(t *testing.T) {
	t.Parallel()
	cases := map[string][]string{
		"locking the proposal":  {`ALTER TABLE proposals RENAME COLUMN price_comparison TO moved`},
		"locking the reminder":  {`ALTER TABLE proposal_close_reminders RENAME COLUMN sent_at TO moved`},
		"claiming the reminder": refuseStatements("UPDATE", "proposal_close_reminders"),
		"reading the ballot":    {`ALTER TABLE votes RENAME COLUMN voter_id TO moved`},
		"counting the ballots":  {`ALTER TABLE votes RENAME COLUMN choice TO moved`},
		"appending the event":   refuseStatements("INSERT", "events"),
	}
	for name, statements := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := newVoteDB(t)
			p, v := d.dueReminder(t)
			reminders := d.afterList(func() { d.breakWith(t, statements) })
			got, err := d.remindersTick(t.Context(), reminders)
			if errs.CodeOf(err) != errs.CodeInternal || got.Scanned != 1 || got.Changed != 0 {
				t.Fatalf("Tick = %+v, %v, want scanned 1 changed 0 and an internal error", got, err)
			}
			if events := d.closeReminderDues(t, p.ID); len(events) != 0 {
				t.Fatalf("events = %+v, want none", events)
			}
			if name == "locking the reminder" {
				return
			}
			if rows := d.reminderRows(t, p.ID, v[0]); len(rows) != 1 || rows[0].SentAt != nil {
				t.Fatalf("rows = %+v, want the reminder still unsent", rows)
			}
		})
	}
}

func TestCloseReminderPoller_aFailedListScansNothing(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, _ := d.dueReminder(t)
	d.breakWith(t, []string{`ALTER TABLE proposal_close_reminders RENAME COLUMN due_at TO moved`})
	got, err := d.remindersTick(t.Context(), d.closeReminders())
	if err == nil || got.Scanned != 0 || got.Changed != 0 {
		t.Fatalf("Tick = %+v, %v, want an error with nothing scanned", got, err)
	}
	if events := d.closeReminderDues(t, p.ID); len(events) != 0 {
		t.Fatalf("events = %+v, want none", events)
	}
}

func TestCloseReminderPoller_intervalFollowsEveryWhenSet(t *testing.T) {
	t.Parallel()
	p := app.NewCloseReminderPoller(nil, nil, nil)
	if got := p.Interval(); got != app.CloseReminderInterval {
		t.Fatalf("Interval() = %v, want the default %v", got, app.CloseReminderInterval)
	}
	p.Every = time.Second
	if got := p.Interval(); got != time.Second {
		t.Fatalf("Interval() with Every = %v, want 1s", got)
	}
}
