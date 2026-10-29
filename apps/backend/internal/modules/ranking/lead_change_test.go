package ranking_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/rankingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func leadStart() time.Time { return time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC) }

type leadPeople struct{ following []ids.UserID }

func leadUser(name string) ids.UserID {
	id := uuid.NewSHA1(uuid.Nil, []byte("lead/"+name))
	id[6] = id[6]&0x0f | 0x70
	id[8] = id[8]&0x3f | 0x80
	return ids.UserIDFrom(id)
}

func (leadPeople) UsersByID(_ context.Context, want []ids.UserID) (map[ids.UserID]identity.UserCard, error) {
	out := map[ids.UserID]identity.UserCard{}
	for _, name := range []string{"Logan", "Christelle"} {
		if id := leadUser(name); contains(want, id) {
			out[id] = identity.UserCard{ID: id, Handle: name, DisplayName: name}
		}
	}
	return out, nil
}

func contains(list []ids.UserID, id ids.UserID) bool {
	for _, v := range list {
		if v == id {
			return true
		}
	}
	return false
}

func (leadPeople) AllCabals(context.Context) ([]cabalport.CabalView, error) {
	return []cabalport.CabalView{}, nil
}

func (leadPeople) MembersOf(context.Context, []ids.CabalID) (map[ids.CabalID][]cabalport.MemberView, error) {
	return map[ids.CabalID][]cabalport.MemberView{}, nil
}

func (f leadPeople) FollowingIDs(context.Context, ids.UserID) ([]ids.UserID, error) {
	return f.following, nil
}

type leadFixture struct {
	d      deliverer
	m      *ranking.Module
	people leadPeople
	seq    int
}

func newLeadFixture(t *testing.T) *leadFixture {
	t.Helper()
	d := newDeliverer(t)
	d.clock.Set(leadStart())
	people := leadPeople{}
	deps := module.Deps{Pool: d.pool, UoW: d.uow, IDs: d.gen, Clock: d.clock, Bus: d.conn}
	m := ranking.New(deps, ranking.WithPorts(app.Ports{Users: people, Cabals: people}))
	return &leadFixture{d: d, m: m, people: people}
}

func (f *leadFixture) call(t *testing.T, who string, right bool, symbol string, at time.Time) {
	t.Helper()
	f.seq++
	proposal := uuid.NewSHA1(uuid.Nil, fmt.Appendf(nil, "lead/%s/%d", who, f.seq))
	cabal := uuid.NewSHA1(uuid.Nil, []byte("lead/cabal"))
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{
			`INSERT INTO calls (proposal_id, cabal_id, kind, symbol, opened_at, decided_at, outcome, resolve_at,
			resolved_at, state, updated_at) VALUES ($1, $2, 'buy', $3, $4, $4, 'passed', $4, $4, 'resolved', $4)`,
			[]any{proposal, cabal, symbol, at},
		},
		{
			`INSERT INTO call_voters (proposal_id, user_id, cabal_id, choice, choice_at, is_right)
			VALUES ($1, $2, $3, 'yes', $4, $5)`,
			[]any{proposal, leadUser(who).UUID(), cabal, at, right},
		},
	} {
		if _, err := f.d.pool.Exec(t.Context(), q.sql, q.args...); err != nil {
			t.Fatalf("seed call for %s: %v", who, err)
		}
	}
}

func (f *leadFixture) calls(t *testing.T, who string, right, wrong int, at time.Time) {
	t.Helper()
	for range right {
		f.call(t, who, true, "AAPL", at)
	}
	for range wrong {
		f.call(t, who, false, "AAPL", at)
	}
}

func (f *leadFixture) runCtx(t *testing.T) context.Context {
	t.Helper()
	return auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorSystem, ID: "poller.ranking.call_board"})
}

func (f *leadFixture) run(t *testing.T) {
	t.Helper()
	if _, err := f.m.RunCallBoard().
		Run(auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorSystem, ID: "poller.ranking.call_board"}), f.d.clock.Now()); err != nil {
		t.Fatalf("Run = %v", err)
	}
}

func (f *leadFixture) baseline(t *testing.T) {
	t.Helper()
	f.calls(t, "Logan", 5, 0, leadStart().Add(-48*time.Hour))
	f.calls(t, "Christelle", 4, 1, leadStart().Add(-48*time.Hour))
	f.run(t)
	if n := f.changes(t, "month"); n != 0 {
		t.Fatalf("lead changes after the first run = %d, want none on an empty board", n)
	}
	f.d.clock.Advance(2 * time.Hour)
}

func (f *leadFixture) changes(t *testing.T, period string) int {
	t.Helper()
	var n int
	err := f.d.pool.QueryRow(t.Context(), `SELECT count(*) FROM call_board_lead_changes WHERE period = $1`, period).
		Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

type storedChange struct {
	leader, passed uuid.UUID
	places         int
	prior          *int
	call           *string
}

func (f *leadFixture) stored(t *testing.T) storedChange {
	t.Helper()
	var s storedChange
	err := f.d.pool.QueryRow(t.Context(), `SELECT leader_id, passed_id, movement_places, leader_prior_rank, call->>'symbol'
		FROM call_board_lead_changes WHERE period = 'month'`).
		Scan(&s.leader, &s.passed, &s.places, &s.prior, &s.call)
	if err != nil {
		t.Fatalf("read lead change: %v", err)
	}
	return s
}

func (f *leadFixture) board() app.ReadCallBoard {
	reads := sqlc.New(f.d.pool)
	return app.ReadCallBoard{
		Runs: reads, Ranks: reads, Subjects: reads, Leads: reads, Users: f.people, Follows: f.people, Clock: f.d.clock,
	}
}

func (f *leadFixture) read(t *testing.T, friends bool) *app.CallLeadChange {
	t.Helper()
	view, err := f.board().Run(t.Context(), app.CallBoardQuery{
		Viewer: leadUser("Logan"), Board: domain.BoardPeople, Period: domain.PeriodMonth, Friends: friends,
	})
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	return view.LeadChange
}

func TestLeadChange_aRightCallThatPassesTheLeaderIsStoredAndServed(t *testing.T) {
	t.Parallel()
	f := newLeadFixture(t)
	f.baseline(t)
	now := f.d.clock.Now()
	f.call(t, "Christelle", true, "NVDA", now.Add(-time.Hour))
	f.call(t, "Christelle", true, "META", now.Add(-30*time.Minute))
	f.call(t, "Logan", false, "AAPL", now.Add(-30*time.Minute))
	f.run(t)

	got := f.stored(t)
	if got.leader != leadUser("Christelle").UUID() || got.passed != leadUser("Logan").UUID() || got.places != 1 ||
		got.prior == nil || *got.prior != 2 || got.call == nil || *got.call != "META" {
		t.Fatalf("stored = %+v, want Christelle past Logan from #2 on META", got)
	}
	wantServedChristelle(t, f.read(t, false))
}

func TestLeadChange_theFormerLeadersMissLeavesNoCall(t *testing.T) {
	t.Parallel()
	f := newLeadFixture(t)
	f.baseline(t)
	now := f.d.clock.Now()
	f.call(t, "Logan", false, "AAPL", now.Add(-time.Hour))
	f.call(t, "Logan", false, "AAPL", now.Add(-30*time.Minute))
	f.run(t)

	got := f.stored(t)
	if got.leader != leadUser("Christelle").UUID() || got.call != nil || got.places != 1 {
		t.Fatalf("stored = %+v, want Christelle leading with no call", got)
	}
	if served := f.read(t, false); served == nil || served.Call != nil {
		t.Fatalf("served = %+v, want a lead change without a call", served)
	}
}

func TestLeadChange_aLatestCallThatMissedLeavesNoCall(t *testing.T) {
	t.Parallel()
	f := newLeadFixture(t)
	f.baseline(t)
	now := f.d.clock.Now()
	f.call(t, "Logan", false, "AAPL", now.Add(-time.Hour))
	f.call(t, "Logan", false, "AAPL", now.Add(-time.Hour))
	f.call(t, "Logan", false, "AAPL", now.Add(-time.Hour))
	f.call(t, "Christelle", true, "META", now.Add(-time.Hour))
	f.call(t, "Christelle", false, "TSLA", now.Add(-30*time.Minute))
	f.run(t)

	if got := f.stored(t); got.leader != leadUser("Christelle").UUID() || got.call != nil {
		t.Fatalf("stored = %+v, want Christelle leading with no call", got)
	}
}

func TestLeadChange_laterRunsKeepTheStoredMovementAndWriteNoSecondChange(t *testing.T) {
	t.Parallel()
	f := newLeadFixture(t)
	f.baseline(t)
	now := f.d.clock.Now()
	f.call(t, "Christelle", true, "META", now.Add(-time.Hour))
	f.call(t, "Christelle", true, "META", now.Add(-30*time.Minute))
	f.call(t, "Logan", false, "AAPL", now.Add(-30*time.Minute))
	f.run(t)

	f.d.clock.Advance(time.Minute)
	f.run(t)
	f.d.clock.Advance(24 * time.Hour)
	f.run(t)
	if n := f.changes(t, "month"); n != 1 {
		t.Fatalf("lead changes after unchanged runs = %d, want 1", n)
	}
	if got := f.stored(t); got.places != 1 {
		t.Fatalf("movement_places = %d after later runs, want 1", got.places)
	}
}

func TestLeadChange_isServedForADayThenDrops(t *testing.T) {
	t.Parallel()
	f := newLeadFixture(t)
	f.baseline(t)
	now := f.d.clock.Now()
	f.call(t, "Christelle", true, "META", now.Add(-time.Hour))
	f.call(t, "Christelle", true, "META", now.Add(-30*time.Minute))
	f.call(t, "Logan", false, "AAPL", now.Add(-30*time.Minute))
	f.run(t)

	f.d.clock.Advance(24*time.Hour - time.Second)
	if f.read(t, false) == nil {
		t.Fatal("lead change gone just inside 24 hours, want it served")
	}
	f.d.clock.Advance(time.Second)
	if got := f.read(t, false); got != nil {
		t.Fatalf("lead change = %+v at 24 hours, want null", got)
	}
}

func TestLeadChange_friendsSeeItOnlyWhenBothAreFriends(t *testing.T) {
	t.Parallel()
	f := newLeadFixture(t)
	f.baseline(t)
	now := f.d.clock.Now()
	f.call(t, "Christelle", true, "META", now.Add(-time.Hour))
	f.call(t, "Christelle", true, "META", now.Add(-30*time.Minute))
	f.call(t, "Logan", false, "AAPL", now.Add(-30*time.Minute))
	f.run(t)

	if got := f.read(t, true); got != nil {
		t.Fatalf("friends lead change = %+v with no friends, want null", got)
	}
	f.people.following = []ids.UserID{leadUser("Christelle")}
	if got := f.read(t, true); got == nil || got.Leader.Name != "Christelle" {
		t.Fatalf("friends lead change = %+v, want Christelle", got)
	}
}

func TestLeadChange_aMonthRolloverWritesNone(t *testing.T) {
	t.Parallel()
	f := newLeadFixture(t)
	f.baseline(t)
	f.d.clock.Set(time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC))
	f.calls(t, "Christelle", 5, 0, time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC))
	f.run(t)
	if n := f.changes(t, "month") + f.changes(t, "week"); n != 0 {
		t.Fatalf("lead changes across a rollover = %d, want none", n)
	}
}

func wantServedChristelle(t *testing.T, served *app.CallLeadChange) {
	t.Helper()
	if served == nil {
		t.Fatal("served = nil, want Christelle's lead change")
	}
	call := served.Call
	if call == nil {
		t.Fatal("served call = nil, want Buy META")
	}
	if served.Leader.Name != "Christelle" || served.Passed.Name != "Logan" ||
		served.LeaderRatePct != 86 || served.MovementPlaces != 1 || call.Symbol != "META" || call.Kind != "buy" {
		t.Fatalf("served = %+v, want Christelle at 86%% past Logan on Buy META", served)
	}
}

func (f *leadFixture) christelleTakesTheLead(t *testing.T) {
	t.Helper()
	f.baseline(t)
	now := f.d.clock.Now()
	f.call(t, "Christelle", true, "NVDA", now.Add(-time.Hour))
	f.call(t, "Christelle", true, "META", now.Add(-30*time.Minute))
	f.call(t, "Logan", false, "AAPL", now.Add(-30*time.Minute))
}

func (f *leadFixture) exec(t *testing.T, sql string) {
	t.Helper()
	if _, err := f.d.pool.Exec(t.Context(), sql); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func TestLeadChange_theHandlerWiresTheCrownCard(t *testing.T) {
	t.Parallel()
	f := newLeadFixture(t)
	f.christelleTakesTheLead(t)
	f.run(t)
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: leadUser("Logan").String()})
	resp, err := adapters.HTTP{CallBoard: f.board()}.GetCallLeaderboard(ctx, api.GetCallLeaderboardRequestObject{})
	if err != nil {
		t.Fatalf("GetCallLeaderboard = %v", err)
	}
	got := api.CallBoard(resp.(api.GetCallLeaderboard200JSONResponse)).LeadChange
	if got == nil || got.Leader.Name != "Christelle" || got.Passed.Name != "Logan" || got.MovementPlaces != 1 ||
		got.LeaderRatePct != 86 || got.Call == nil || got.Call.Symbol != "META" {
		t.Fatalf("lead_change = %+v, want Christelle past Logan on META", got)
	}
}

func TestLeadChange_nobodyRankedWritesNone(t *testing.T) {
	t.Parallel()
	f := newLeadFixture(t)
	f.baseline(t)
	f.exec(t, `DELETE FROM call_voters`)
	f.call(t, "Logan", true, "AAPL", f.d.clock.Now().Add(-time.Hour))
	f.run(t)
	if n := f.changes(t, "month"); n != 0 {
		t.Fatalf("lead changes with nobody ranked = %d, want none", n)
	}
}

func TestLeadChange_runFailsWhenTheCauseCannotBeRead(t *testing.T) {
	t.Parallel()
	f := newLeadFixture(t)
	f.christelleTakesTheLead(t)
	f.exec(t, `ALTER TABLE calls RENAME COLUMN updated_at TO updated_at_x`)
	if _, err := f.m.RunCallBoard().Run(f.runCtx(t), f.d.clock.Now()); err == nil {
		t.Fatal("Run = nil, want the cause lookup error")
	}
}

func TestLeadChange_runFailsWhenTheChangeCannotBeStored(t *testing.T) {
	t.Parallel()
	f := newLeadFixture(t)
	f.christelleTakesTheLead(t)
	f.exec(t, `ALTER TABLE call_board_lead_changes RENAME TO lead_changes_x`)
	if _, err := f.m.RunCallBoard().Run(f.runCtx(t), f.d.clock.Now()); err == nil {
		t.Fatal("Run = nil, want the insert error")
	}
}

func TestLeadChange_readFailsWhenTheChangesCannotBeRead(t *testing.T) {
	t.Parallel()
	f := newLeadFixture(t)
	f.christelleTakesTheLead(t)
	f.run(t)
	f.exec(t, `ALTER TABLE call_board_lead_changes RENAME TO lead_changes_x`)
	_, err := f.board().Run(t.Context(), app.CallBoardQuery{
		Viewer: leadUser("Logan"), Board: domain.BoardPeople, Period: domain.PeriodMonth,
	})
	if err == nil {
		t.Fatal("read = nil error, want the lookup error")
	}
}

func TestLeadChange_readWithoutTheSourceServesNone(t *testing.T) {
	t.Parallel()
	f := newLeadFixture(t)
	f.christelleTakesTheLead(t)
	f.run(t)
	board := f.board()
	board.Leads = nil
	view, err := board.Run(t.Context(), app.CallBoardQuery{
		Viewer: leadUser("Logan"), Board: domain.BoardPeople, Period: domain.PeriodMonth,
	})
	if err != nil || view.LeadChange != nil {
		t.Fatalf("read = %+v, %v; want no lead change", view.LeadChange, err)
	}
}
