package governance_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/governanceapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (d voteDB) reminders() *app.ReminderHandler { return app.NewReminderHandler(d.uow, d.clk) }

func (d proposalDB) users(t *testing.T, n int) []uuid.UUID {
	t.Helper()
	out := make([]uuid.UUID, n)
	for i := range out {
		out[i] = testkit.SeedUser(t, d.pool, testkit.UserOpts{}).ID.UUID()
	}
	return out
}

func (d voteDB) openWithUsers(t *testing.T) (sqlc.InsertProposalParams, []uuid.UUID) {
	t.Helper()
	set := d.users(t, 3)
	p := d.buy(set...)
	d.insert(t, p)
	return p, set
}

func (d voteDB) setReminder(t *testing.T, proposal, user uuid.UUID) (app.ReminderResult, error) {
	t.Helper()
	cmd := app.SetReminder{ProposalID: ids.ProposalIDFrom(proposal), UserID: ids.UserIDFrom(user)}
	return d.reminders().Set(t.Context(), cmd)
}

func (d voteDB) clearReminder(t *testing.T, proposal, user uuid.UUID) (app.ReminderResult, error) {
	t.Helper()
	cmd := app.ClearReminder{ProposalID: ids.ProposalIDFrom(proposal), UserID: ids.UserIDFrom(user)}
	return d.reminders().Clear(t.Context(), cmd)
}

type reminderRow struct {
	DueAt  time.Time
	SentAt *time.Time
}

func (d proposalDB) reminderRows(t *testing.T, proposal, user uuid.UUID) []reminderRow {
	t.Helper()
	rows, err := d.pool.Query(t.Context(),
		`SELECT due_at, sent_at FROM proposal_close_reminders WHERE proposal_id = $1 AND user_id = $2`, proposal, user)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []reminderRow
	for rows.Next() {
		var r reminderRow
		if err := rows.Scan(&r.DueAt, &r.SentAt); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func (d voteDB) mustSet(t *testing.T, proposal, user uuid.UUID) app.ReminderResult {
	t.Helper()
	got, err := d.setReminder(t, proposal, user)
	if err != nil {
		t.Fatalf("SetReminder = %v", err)
	}
	return got
}

func TestSetReminder_storesDueAnHourBeforeClose(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, v := d.openWithUsers(t)
	d.clk.Set(p.ExpiresAt.Add(-4 * time.Hour))
	want := p.ExpiresAt.Add(-domain.ReminderLead)
	got := d.mustSet(t, p.ID, v[0])
	if !got.ReminderSet || got.DueAt == nil || !got.DueAt.Equal(want) {
		t.Fatalf("result = %+v, want set and due %v", got, want)
	}
	d.clk.Advance(time.Minute)
	if again := d.mustSet(t, p.ID, v[0]); !again.ReminderSet || !again.DueAt.Equal(want) {
		t.Fatalf("second result = %+v, want the same due %v", again, want)
	}
	rows := d.reminderRows(t, p.ID, v[0])
	if len(rows) != 1 || !rows[0].DueAt.Equal(want) || rows[0].SentAt != nil {
		t.Fatalf("rows = %+v, want one unsent row due %v", rows, want)
	}
	if other := d.reminderRows(t, p.ID, v[1]); len(other) != 0 {
		t.Fatalf("another member's rows = %+v, want none", other)
	}
}

func TestSetReminder_underAnHourIsDueNow(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, v := d.openWithUsers(t)
	now := p.ExpiresAt.Add(-30 * time.Minute)
	d.clk.Set(now)
	got := d.mustSet(t, p.ID, v[0])
	if !got.ReminderSet || got.DueAt == nil || !got.DueAt.Equal(now) {
		t.Fatalf("result = %+v, want set and due now %v", got, now)
	}
	if rows := d.reminderRows(t, p.ID, v[0]); len(rows) != 1 || !rows[0].DueAt.Equal(now) {
		t.Fatalf("rows = %+v, want one row due %v", rows, now)
	}
}

func TestSetReminder_resetsASentReminder(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, v := d.openWithUsers(t)
	d.mustSet(t, p.ID, v[0])
	if _, err := d.pool.Exec(t.Context(), `UPDATE proposal_close_reminders SET sent_at = $2 WHERE proposal_id = $1`,
		p.ID, d.clk.Now()); err != nil {
		t.Fatal(err)
	}
	d.mustSet(t, p.ID, v[0])
	if rows := d.reminderRows(t, p.ID, v[0]); len(rows) != 1 || rows[0].SentAt != nil {
		t.Fatalf("rows = %+v, want the sent mark cleared", rows)
	}
}

func TestSetReminder_refusals(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, v := d.openWithUsers(t)
	outsider := d.users(t, 1)[0]
	closed, cv := d.openWithUsers(t)
	if n := d.transition(t, closed.ID, "open", "failed", ""); n != 1 {
		t.Fatalf("transition = %d rows, want 1", n)
	}
	expired, ev := d.openWithUsers(t)
	open := d.clk.Now()
	for _, tc := range []struct {
		name     string
		proposal uuid.UUID
		user     uuid.UUID
		at       time.Time
		want     errs.Code
	}{
		{"not a voter", p.ID, outsider, open, errs.CodeNotAVoter},
		{"decided", closed.ID, cv[0], open, errs.CodeProposalClosed},
		{"expired", expired.ID, ev[0], expired.ExpiresAt, errs.CodeProposalClosed},
		{"unknown", d.ids.NewV7(), v[0], open, errs.CodeProposalNotFound},
	} {
		d.clk.Set(tc.at)
		if _, err := d.setReminder(t, tc.proposal, tc.user); errs.CodeOf(err) != tc.want {
			t.Errorf("%s: SetReminder err = %v, want %s", tc.name, err, tc.want)
		}
		if rows := d.reminderRows(t, tc.proposal, tc.user); len(rows) != 0 {
			t.Errorf("%s: rows = %+v, want none", tc.name, rows)
		}
	}
}

func TestSetReminder_alreadyVotedIsANoOp(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, v := d.openWithUsers(t)
	d.mustCast(t, d.handler(), p.ID, v[0], domain.ChoiceYes)
	got := d.mustSet(t, p.ID, v[0])
	if got.ReminderSet || got.DueAt != nil {
		t.Fatalf("result = %+v, want not set", got)
	}
	if rows := d.reminderRows(t, p.ID, v[0]); len(rows) != 0 {
		t.Fatalf("rows = %+v, want none", rows)
	}
}

func TestClearReminder_isAnIdempotentSelfCleanup(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, v := d.openWithUsers(t)
	outsider := d.users(t, 1)[0]
	d.mustSet(t, p.ID, v[0])
	d.mustSet(t, p.ID, v[1])
	clearAs := func(name string, proposal, user uuid.UUID) {
		t.Helper()
		got, err := d.clearReminder(t, proposal, user)
		if err != nil || got.ReminderSet || got.DueAt != nil || got.ProposalID != ids.ProposalIDFrom(proposal) {
			t.Fatalf("%s: ClearReminder = %+v, %v, want 200 with reminder_set false", name, got, err)
		}
	}
	clearAs("open", p.ID, v[0])
	if rows := d.reminderRows(t, p.ID, v[0]); len(rows) != 0 {
		t.Fatalf("rows after clear = %+v, want none", rows)
	}
	if rows := d.reminderRows(t, p.ID, v[1]); len(rows) != 1 {
		t.Fatalf("another member's rows = %+v, want kept", rows)
	}
	clearAs("no row", p.ID, v[0])
	clearAs("non-voter", p.ID, outsider)
	d.clk.Set(p.ExpiresAt.Add(time.Hour))
	clearAs("closed", p.ID, v[1])
	if rows := d.reminderRows(t, p.ID, v[1]); len(rows) != 0 {
		t.Fatalf("rows after clearing a closed proposal = %+v, want none", rows)
	}
	if _, err := d.clearReminder(t, d.ids.NewV7(), v[0]); errs.CodeOf(err) != errs.CodeProposalNotFound {
		t.Fatalf("ClearReminder on an unknown proposal err = %v, want proposal_not_found", err)
	}
}

func TestCastVote_deletesTheCallersReminder(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, v := d.openWithUsers(t)
	d.mustSet(t, p.ID, v[0])
	d.mustSet(t, p.ID, v[1])
	d.mustCast(t, d.handler(), p.ID, v[0], domain.ChoiceYes)
	if rows := d.reminderRows(t, p.ID, v[0]); len(rows) != 0 {
		t.Fatalf("voter's rows = %+v, want deleted with the ballot", rows)
	}
	if rows := d.reminderRows(t, p.ID, v[1]); len(rows) != 1 {
		t.Fatalf("another member's rows = %+v, want kept", rows)
	}
	if _, err := d.retract(t.Context(), d.retractor(), p.ID, v[0]); err != nil {
		t.Fatalf("RetractVote = %v", err)
	}
	if rows := d.reminderRows(t, p.ID, v[0]); len(rows) != 0 {
		t.Fatalf("rows after undo = %+v, want the reminder not restored", rows)
	}
}

func TestGetProposal_reminderSet(t *testing.T) {
	t.Parallel()
	d := newReadsDB(t)
	clk := testkit.NewClock(d.now.Add(time.Hour))
	handler := app.NewReminderHandler(db.New(d.pool, d.ids, clk), clk)
	users := d.users(t, 2)
	d.caller = users[0]
	id := d.seed(t, d.now, users[1])
	if got := d.get(t, id, users[0]); got.ReminderSet {
		t.Fatal("ReminderSet = true before asking, want false")
	}
	if _, err := handler.Set(
		t.Context(),
		app.SetReminder{ProposalID: ids.ProposalIDFrom(id), UserID: ids.UserIDFrom(users[0])},
	); err != nil {
		t.Fatalf("SetReminder = %v", err)
	}
	if got := d.get(t, id, users[0]); !got.ReminderSet {
		t.Fatal("ReminderSet = false after asking, want true")
	}
	if got := d.get(t, id, users[1]); got.ReminderSet {
		t.Fatal("ReminderSet = true for another member, want false")
	}
	if got := d.get(t, id, d.ids.NewV7()); got.ReminderSet {
		t.Fatal("ReminderSet = true for a non-member, want false")
	}
	if _, err := d.pool.Exec(t.Context(), `UPDATE proposal_close_reminders SET sent_at = $2 WHERE proposal_id = $1`,
		id, clk.Now()); err != nil {
		t.Fatal(err)
	}
	if got := d.get(t, id, users[0]); got.ReminderSet {
		t.Fatal("ReminderSet = true after the reminder was sent, want false")
	}
}

func refuseStatements(event, table string) []string {
	return []string{
		`CREATE FUNCTION refuse_reminder() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'refused'; END $$`,
		`CREATE TRIGGER refuse BEFORE ` + event + ` ON ` + table + ` FOR EACH ROW EXECUTE FUNCTION refuse_reminder()`,
	}
}

func (d voteDB) breakWith(t *testing.T, statements []string) {
	t.Helper()
	for _, stmt := range statements {
		if _, err := d.pool.Exec(t.Context(), stmt); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSetReminder_aFailedStepStoresNothing(t *testing.T) {
	t.Parallel()
	cases := map[string][]string{
		"reading the ballot":   {`ALTER TABLE votes RENAME COLUMN first_cast_at TO moved`},
		"storing the reminder": refuseStatements("INSERT OR UPDATE", "proposal_close_reminders"),
	}
	for name, statements := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := newVoteDB(t)
			p, v := d.openWithUsers(t)
			d.breakWith(t, statements)
			if _, err := d.setReminder(t, p.ID, v[0]); errs.CodeOf(err) != errs.CodeInternal {
				t.Fatalf("SetReminder err = %v, want internal", err)
			}
			if rows := d.reminderRows(t, p.ID, v[0]); len(rows) != 0 {
				t.Fatalf("rows = %+v, want none", rows)
			}
		})
	}
}

func TestClearReminder_aFailedStepKeepsTheReminder(t *testing.T) {
	t.Parallel()
	cases := map[string][]string{
		"finding the proposal":  {`ALTER TABLE proposals RENAME COLUMN id TO moved`},
		"deleting the reminder": refuseStatements("DELETE", "proposal_close_reminders"),
	}
	for name, statements := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := newVoteDB(t)
			p, v := d.openWithUsers(t)
			d.mustSet(t, p.ID, v[0])
			d.breakWith(t, statements)
			if _, err := d.clearReminder(t, p.ID, v[0]); errs.CodeOf(err) != errs.CodeInternal {
				t.Fatalf("ClearReminder err = %v, want internal", err)
			}
		})
	}
}

func TestCastVote_aRefusedReminderDeleteRollsTheBallotBack(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, v := d.openWithUsers(t)
	d.mustSet(t, p.ID, v[0])
	d.breakWith(t, refuseStatements("DELETE", "proposal_close_reminders"))
	if _, err := d.cast(t.Context(), d.handler(), p.ID, v[0], domain.ChoiceYes); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("CastVote err = %v, want internal", err)
	}
	if b := d.ballots(t, p.ID); len(b) != 0 {
		t.Fatalf("ballots = %v, want none after the rollback", b)
	}
	if rows := d.reminderRows(t, p.ID, v[0]); len(rows) != 1 {
		t.Fatalf("rows = %+v, want the reminder kept", rows)
	}
}

func userCtx(t *testing.T, user uuid.UUID) context.Context {
	t.Helper()
	return auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: user.String()})
}

func TestReminderRoutes_refuseAnonymousCallers(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	h := adapters.HTTP{Reminders: d.reminders()}
	if _, err := h.PutProposalReminder(
		t.Context(),
		api.PutProposalReminderRequestObject{Id: d.ids.NewV7()},
	); errs.CodeOf(
		err,
	) != errs.CodeUnauthorized {
		t.Errorf("PutProposalReminder without an actor err = %v, want unauthorized", err)
	}
	if _, err := h.DeleteProposalReminder(
		t.Context(),
		api.DeleteProposalReminderRequestObject{Id: d.ids.NewV7()},
	); errs.CodeOf(
		err,
	) != errs.CodeUnauthorized {
		t.Errorf("DeleteProposalReminder without an actor err = %v, want unauthorized", err)
	}
}

func TestReminderRoutes_unknownProposal(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	h := adapters.HTTP{Reminders: d.reminders()}
	ctx := userCtx(t, d.ids.NewV7())
	if _, err := h.PutProposalReminder(
		ctx,
		api.PutProposalReminderRequestObject{Id: d.ids.NewV7()},
	); errs.CodeOf(
		err,
	) != errs.CodeProposalNotFound {
		t.Errorf("PutProposalReminder err = %v, want proposal_not_found", err)
	}
	if _, err := h.DeleteProposalReminder(
		ctx,
		api.DeleteProposalReminderRequestObject{Id: d.ids.NewV7()},
	); errs.CodeOf(
		err,
	) != errs.CodeProposalNotFound {
		t.Errorf("DeleteProposalReminder err = %v, want proposal_not_found", err)
	}
}

func TestReminderRoutes_setThenClear(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	p, v := d.openWithUsers(t)
	h := adapters.HTTP{Reminders: d.reminders()}
	got, err := h.PutProposalReminder(userCtx(t, v[0]), api.PutProposalReminderRequestObject{Id: p.ID})
	set, ok := got.(api.PutProposalReminder200JSONResponse)
	if err != nil || !ok || !set.ReminderSet || set.DueAt == nil || set.ProposalId != p.ID {
		t.Fatalf("PutProposalReminder = %+v, %v, want a stored reminder", got, err)
	}
	cleared, err := h.DeleteProposalReminder(userCtx(t, v[0]), api.DeleteProposalReminderRequestObject{Id: p.ID})
	gone, ok := cleared.(api.DeleteProposalReminder200JSONResponse)
	if err != nil || !ok || gone.ReminderSet || gone.DueAt != nil {
		t.Fatalf("DeleteProposalReminder = %+v, %v, want reminder_set false", cleared, err)
	}
}
