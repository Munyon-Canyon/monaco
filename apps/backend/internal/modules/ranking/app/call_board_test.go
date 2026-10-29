package app_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func boardNow() time.Time { return time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC) }

type boardCards struct {
	users  map[ids.UserID]identity.UserCard
	cabals []cabalport.CabalView
	err    error
	onlyOn string
}

func (c *boardCards) UsersByID(_ context.Context, in []ids.UserID) (map[ids.UserID]identity.UserCard, error) {
	if c.err != nil && c.onlyOn == "users" {
		return nil, c.err
	}
	out := map[ids.UserID]identity.UserCard{}
	for _, id := range in {
		if card, ok := c.users[id]; ok {
			out[id] = card
		}
	}
	return out, nil
}

func (c *boardCards) AllCabals(context.Context) ([]cabalport.CabalView, error) {
	if c.err != nil && c.onlyOn == "cabals" {
		return nil, c.err
	}
	return c.cabals, nil
}

func (*boardCards) MembersOf(context.Context, []ids.CabalID) (map[ids.CabalID][]cabalport.MemberView, error) {
	return map[ids.CabalID][]cabalport.MemberView{}, nil
}

func (c *boardCards) addUser(name string, n int) uuid.UUID {
	id := uuid.NewSHA1(uuid.Nil, []byte("board/user/"+name))
	if c.users == nil {
		c.users = map[ids.UserID]identity.UserCard{}
	}
	c.users[ids.UserIDFrom(id)] = identity.UserCard{
		ID: ids.UserIDFrom(id), Handle: name, DisplayName: name,
		CreatedAt: boardNow().Add(-time.Duration(100-n) * time.Hour),
	}
	return id
}

func (c *boardCards) addCabal(name string, status cabalport.Status) uuid.UUID {
	id := uuid.NewSHA1(uuid.Nil, []byte("board/cabal/"+name))
	c.cabals = append(c.cabals, cabalport.CabalView{
		ID: ids.CabalIDFrom(id), Name: name, Status: status, CreatedAt: boardNow().Add(-200 * time.Hour),
	})
	return id
}

type voteSeed struct {
	user  uuid.UUID
	voted bool
	right bool
}

type callSeed struct {
	proposal, cabal uuid.UUID
	openedAt        time.Time
	resolved        bool
	cabalRight      bool
	voters          []voteSeed
}

func (c callSeed) insert(t *testing.T, pool rig) {
	t.Helper()
	state, resolvedAt := "decided", any(nil)
	if c.resolved {
		state, resolvedAt = "resolved", c.openedAt.Add(25*time.Hour)
	}
	execAll(t, pool, `INSERT INTO calls (proposal_id, cabal_id, kind, symbol, opened_at, decided_at, outcome,
		resolve_at, resolved_at, state, updated_at)
		VALUES ($1, $2, 'buy', 'AAPLx', $3, $3, 'passed', $3, $4, $5, $3)`,
		c.proposal, c.cabal, c.openedAt, resolvedAt, state)
	var cabalRight any
	if c.resolved {
		cabalRight = c.cabalRight
	}
	execAll(t, pool, `INSERT INTO cabal_calls (proposal_id, cabal_id, side, is_right) VALUES ($1, $2, 'yes', $3)`,
		c.proposal, c.cabal, cabalRight)
	for _, v := range c.voters {
		var choice, right any
		if v.voted {
			choice = "yes"
		}
		if v.voted && c.resolved {
			right = v.right
		}
		execAll(t, pool, `INSERT INTO call_voters (proposal_id, user_id, cabal_id, choice, choice_at, is_right)
			VALUES ($1, $2, $3, $4, $5, $6)`, c.proposal, v.user, c.cabal, choice, c.openedAt, right)
	}
}

func execAll(t *testing.T, pool rig, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), sql, args...); err != nil {
		t.Fatalf("seed: %v\n%s", err, sql)
	}
}

type personSeed struct {
	name              string
	right, resolved   int
	openVotes, openOf int
}

func seedPeople(
	t *testing.T, pool rig, cards *boardCards, cabal uuid.UUID, opened time.Time, people []personSeed,
) {
	t.Helper()
	most, openProposals := 0, 0
	for _, p := range people {
		most, openProposals = max(most, p.resolved), max(openProposals, p.openOf)
	}
	for i := range most + openProposals {
		call := callSeed{
			proposal: uuid.NewSHA1(uuid.Nil, fmt.Appendf(nil, "board/%s/proposal/%d", cabal, i)), cabal: cabal,
			openedAt: opened.Add(time.Duration(i) * time.Minute), resolved: i < most, cabalRight: true,
		}
		for n, p := range people {
			user := cards.addUser(p.name, n)
			switch {
			case i < most && i < p.resolved:
				call.voters = append(call.voters, voteSeed{user: user, voted: true, right: i < p.right})
			case i >= most && i-most < p.openOf:
				call.voters = append(call.voters, voteSeed{user: user, voted: i-most < p.openVotes})
			}
		}
		call.insert(t, pool)
	}
}

type entry struct {
	rank, rateBps                    *int
	right, resolved, voted, eligible int
	name, kind, periodStart          string
	movementKind                     string
	movement, baseline               *int
	lastSymbol                       *string
	lastRight                        *bool
}

func readEntries(t *testing.T, pool rig, board, period string) map[string]entry {
	t.Helper()
	rows, err := pool.Query(t.Context(), `SELECT subject_name, subject_kind, period_start::text, rank, rate_bps,
		right_count, resolved, voted, eligible, movement_kind, movement, baseline_rank,
		last_call->>'symbol', (last_call->>'right')::boolean
		FROM call_board_entries WHERE board = $1 AND period = $2`, board, period)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]entry{}
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.name, &e.kind, &e.periodStart, &e.rank, &e.rateBps, &e.right, &e.resolved, &e.voted,
			&e.eligible, &e.movementKind, &e.movement, &e.baseline, &e.lastSymbol, &e.lastRight); err != nil {
			t.Fatal(err)
		}
		out[e.name] = e
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func newBoardRun(t *testing.T, pool rig, cards *boardCards) app.RunCallBoard {
	t.Helper()
	return app.NewRunCallBoard(pool, pool.uow, testkit.NewIDs(6453), clock.Real{}, cards, cards)
}

func unmovedOnFirstRun(e entry) bool {
	want := "same"
	if e.rank == nil {
		want = "new"
	}
	return e.movementKind == want && e.lastSymbol == nil
}

func wantRank(t *testing.T, got map[string]entry, name string, rank, rate int) {
	t.Helper()
	e, ok := got[name]
	if !ok || e.rank == nil || *e.rank != rank || e.rateBps == nil || *e.rateBps != rate {
		t.Errorf("%s = %+v, want rank %d at rate_bps %d", name, e, rank, rate)
	}
}

func seedMonth(t *testing.T, pool rig, cards *boardCards) {
	t.Helper()
	cabal := cards.addCabal("Pilates", cabalport.StatusActive)
	people := make([]personSeed, 0, 24)
	people = append(people,
		personSeed{name: "Christelle", right: 12, resolved: 13}, personSeed{name: "Logan", right: 10, resolved: 11},
		personSeed{name: "Nia", right: 9, resolved: 10}, personSeed{name: "Omar", right: 8, resolved: 9},
		personSeed{name: "Pia", right: 7, resolved: 8}, personSeed{name: "Quin", right: 6, resolved: 7},
		personSeed{name: "Kai", right: 11, resolved: 14},
		personSeed{name: "Andres", right: 7, resolved: 9, openVotes: 2, openOf: 2},
	)
	for i := range 16 {
		people = append(people, personSeed{name: fmt.Sprintf("Rest%02d", i), right: 3, resolved: 5})
	}
	seedPeople(t, pool, cards, cabal, time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC), people)
}

func runBoard(t *testing.T, seed func(*testing.T, rig, *boardCards)) rig {
	t.Helper()
	pool, cards := newRig(t), &boardCards{}
	seed(t, pool, cards)
	if _, err := newBoardRun(t, pool, cards).Run(pollerContext(t), boardNow()); err != nil {
		t.Fatalf("Run = %v", err)
	}
	return pool
}

func TestRunCallBoard_monthFixtureRanksPeople(t *testing.T) {
	t.Parallel()
	pool := runBoard(t, seedMonth)

	month := readEntries(t, pool, "people", "month")
	wantRank(t, month, "Christelle", 1, 9230)
	wantRank(t, month, "Logan", 2, 9090)
	wantRank(t, month, "Kai", 7, 7857)
	wantRank(t, month, "Andres", 8, 7777)
	if a := month["Andres"]; a.voted != 11 || a.eligible != 11 || a.resolved != 9 || a.right != 7 {
		t.Errorf("Andres record = %+v, want 7 of 9 voted 11 of 11", a)
	}
	ranked := 0
	for name, e := range month {
		if e.rank != nil {
			ranked++
		}
		if e.kind != "person" || e.periodStart != "2026-10-01" || !unmovedOnFirstRun(e) {
			t.Errorf("entry %s = %+v, want a person from 2026-10-01 with unmoved ranks and no last call", name, e)
		}
	}
	if ranked != 24 {
		t.Errorf("ranked people = %d, want 24", ranked)
	}
}

func TestRunCallBoard_writesEveryPeriodAndTheCabalBoard(t *testing.T) {
	t.Parallel()
	pool := runBoard(t, seedMonth)

	if got := readEntries(t, pool, "people", "week"); len(got) != 0 {
		t.Errorf("week board = %v, want empty: the calls opened before this week", got)
	}
	month, all := readEntries(t, pool, "people", "month"), readEntries(t, pool, "people", "all")
	if all["Christelle"].periodStart != "1970-01-01" || len(all) != len(month) {
		t.Errorf(
			"all board = %d rows from %s, want %d from 1970-01-01",
			len(all),
			all["Christelle"].periodStart,
			len(month),
		)
	}
	cabals := readEntries(t, pool, "cabals", "month")
	if p := cabals["Pilates"]; len(cabals) != 1 || p.rank == nil || *p.rank != 1 || p.kind != "cabal" {
		t.Errorf("cabals month = %+v, want Pilates alone at #1", cabals)
	}
}

func TestRunCallBoard_leavesOutBannedAndDeletedSubjects(t *testing.T) {
	t.Parallel()
	pool := runBoard(t, func(t *testing.T, pool rig, cards *boardCards) {
		t.Helper()
		seedMonth(t, pool, cards)
		opened := time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)
		banned := cards.addCabal("Banned", cabalport.StatusBanned)
		cabal := cards.cabals[0].ID.UUID()
		fan, ghost := cards.addUser("Banned fan", 98), cards.addUser("Ghost", 99)
		cards.users[ids.UserIDFrom(fan)] = identity.UserCard{AccountStatus: "banned"}
		cards.users[ids.UserIDFrom(ghost)] = identity.UserCard{Deleted: true}
		for i, member := range []uuid.UUID{fan, ghost} {
			for n := range 5 {
				callSeed{
					proposal: uuid.NewSHA1(
						uuid.Nil,
						fmt.Appendf(nil, "extra/%d/%d", i, n),
					),
					cabal:    cabal,
					openedAt: opened,
					resolved: true,
					voters:   []voteSeed{{user: member, voted: true, right: true}},
				}.insert(t, pool)
			}
		}
		for n := range 5 {
			callSeed{
				proposal: uuid.NewSHA1(uuid.Nil, fmt.Appendf(nil, "banned/%d", n)), cabal: banned, openedAt: opened,
				resolved: true, cabalRight: true,
			}.insert(t, pool)
		}
	})

	for _, board := range []string{"people", "cabals"} {
		for name := range readEntries(t, pool, board, "month") {
			if name == "Banned fan" || name == "Ghost" || name == "Banned" {
				t.Errorf("%s month board lists %s, want banned and deleted subjects left out", board, name)
			}
		}
	}
	if got := readEntries(t, pool, "people", "month"); len(got) != 24 {
		t.Errorf("month people rows = %d, want the 24 from the fixture", len(got))
	}
}

func seedWeek(t *testing.T, pool rig, cards *boardCards) {
	t.Helper()
	cabal := cards.addCabal("Pilates", cabalport.StatusActive)
	seedPeople(t, pool, cards, cabal, time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC), []personSeed{
		{name: "Ada", right: 5, resolved: 5, openVotes: 2, openOf: 2},
		{name: "Bo", right: 5, resolved: 5, openVotes: 2, openOf: 2},
		{name: "Cy", right: 5, resolved: 5, openVotes: 2, openOf: 2},
		{name: "Jordan", right: 4, resolved: 5, openVotes: 0, openOf: 2},
		{name: "Kai", right: 4, resolved: 5, openVotes: 1, openOf: 2},
		{name: "Andres", right: 3, resolved: 3, openOf: 2},
		{name: "Rosa", right: 2, resolved: 2, openOf: 2},
	})
}

func TestRunCallBoard_weekFixtureBreaksTheTieOnTurnoutAndHidesFewResolved(t *testing.T) {
	t.Parallel()
	pool := runBoard(t, seedWeek)

	week := readEntries(t, pool, "people", "week")
	wantRank(t, week, "Kai", 4, 8000)
	wantRank(t, week, "Jordan", 5, 8000)
	if k, j := week["Kai"], week["Jordan"]; k.voted != 6 || k.eligible != 7 || j.voted != 5 || j.eligible != 7 {
		t.Errorf("Kai %d of %d, Jordan %d of %d, want 6 of 7 and 5 of 7", k.voted, k.eligible, j.voted, j.eligible)
	}
	for name, resolved := range map[string]int{"Andres": 3, "Rosa": 2} {
		if e := week[name]; e.rank != nil || e.rateBps != nil || e.resolved != resolved {
			t.Errorf("%s = %+v, want unranked with %d resolved", name, e, resolved)
		}
	}
	if week["Kai"].periodStart != "2026-10-26" {
		t.Errorf("week starts %s, want the Monday 2026-10-26", week["Kai"].periodStart)
	}
}

func boardSignature(t *testing.T, pool rig) string {
	t.Helper()
	var got string
	err := pool.QueryRow(t.Context(), `SELECT coalesce(string_agg(board || period || subject_id::text || coalesce(rank::text, '-')
		|| coalesce(rate_bps::text, '-'), ',' ORDER BY board, period, subject_id), '') FROM call_board_entries`).
		Scan(&got)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestRunCallBoard_twoRunsOnUnchangedInputsWriteIdenticalRanks(t *testing.T) {
	t.Parallel()
	pool, cards := newRig(t), &boardCards{}
	cabal := cards.addCabal("Pilates", cabalport.StatusActive)
	people := make([]personSeed, 0, 8)
	for i := range 8 {
		people = append(people, personSeed{name: fmt.Sprintf("Tied%d", i), right: 4, resolved: 5})
	}
	seedPeople(t, pool, cards, cabal, time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC), people)
	run := newBoardRun(t, pool, cards)

	first, err := run.Run(pollerContext(t), boardNow())
	if err != nil {
		t.Fatalf("first Run = %v", err)
	}
	before := boardSignature(t, pool)
	second, err := run.Run(pollerContext(t), boardNow())
	if err != nil {
		t.Fatalf("second Run = %v", err)
	}
	if after := boardSignature(t, pool); after != before || first != second || before == "" {
		t.Fatalf("second run wrote %d rows %q, want the first run's %d rows %q", second, after, first, before)
	}
	var runs, written int
	err = pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM call_board_runs),
		(SELECT count(*) FROM events WHERE type = 'ranking.call_board_written')`).Scan(&runs, &written)
	if err != nil || runs != 2 || written != 2 {
		t.Fatalf("runs %d, call_board_written events %d, err %v; want 2 and 2", runs, written, err)
	}
}

func TestRunCallBoard_keepsEachJobsTriggersToItself(t *testing.T) {
	t.Parallel()
	pool, cards := newRig(t), &boardCards{}
	old := boardNow().Add(-time.Minute)
	execAll(t, pool, `INSERT INTO call_board_triggers (reason, created_at) VALUES ('call_resolved', $1)`, old)
	execAll(t, pool, `INSERT INTO ranking_triggers (cabal_id, reason, created_at) VALUES ($1, 'trade_confirmed', $2)`,
		uuid.NewSHA1(uuid.Nil, []byte("board/valuation-cabal")), old)

	if _, err := newBoardRun(t, pool, cards).Run(pollerContext(t), boardNow()); err != nil {
		t.Fatalf("Run = %v", err)
	}
	var board, valuation int
	err := pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM call_board_triggers),
		(SELECT count(*) FROM ranking_triggers)`).Scan(&board, &valuation)
	if err != nil || board != 0 || valuation != 1 {
		t.Fatalf("after the call board run: call_board_triggers %d, ranking_triggers %d, err %v; want 0 and 1",
			board, valuation, err)
	}

	execAll(t, pool, `INSERT INTO call_board_triggers (reason, created_at) VALUES ('vote_cast', $1)`, old)
	if err := sqlc.New(pool).DeleteRankingTriggersThrough(t.Context(), boardNow()); err != nil {
		t.Fatal(err)
	}
	err = pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM call_board_triggers),
		(SELECT count(*) FROM ranking_triggers)`).Scan(&board, &valuation)
	if err != nil || board != 1 || valuation != 0 {
		t.Fatalf("after the valuation run: call_board_triggers %d, ranking_triggers %d, err %v; want 1 and 0",
			board, valuation, err)
	}
}

var errInjected = errs.New(errs.CodeInternal, "test")

func TestRunCallBoard_failsWhenAnInputOrCardReadFails(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		cards     func(*boardCards)
		dropTable string
	}{
		"the people inputs": {dropTable: "call_voters"},
		"the cabal inputs":  {dropTable: "cabal_calls"},
		"the people cards":  {cards: func(c *boardCards) { c.err, c.onlyOn = errInjected, "users" }},
		"the cabal cards":   {cards: func(c *boardCards) { c.err, c.onlyOn = errInjected, "cabals" }},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pool, cards := newRig(t), &boardCards{}
			seedMonth(t, pool, cards)
			if tc.cards != nil {
				tc.cards(cards)
			}
			if tc.dropTable != "" {
				execAll(t, pool, "ALTER TABLE "+tc.dropTable+" RENAME TO "+tc.dropTable+"_gone")
			}
			if _, err := newBoardRun(t, pool, cards).Run(pollerContext(t), boardNow()); err == nil {
				t.Fatal("Run error = nil, want the read to fail")
			}
		})
	}
}

func TestRunCallBoard_failsWhenATableItWritesIsGone(t *testing.T) {
	t.Parallel()
	for _, table := range []string{"call_board_entries", "call_board_runs", "call_board_triggers"} {
		t.Run(table, func(t *testing.T) {
			t.Parallel()
			pool, cards := newRig(t), &boardCards{}
			seedMonth(t, pool, cards)
			execAll(t, pool, "ALTER TABLE "+table+" RENAME TO "+table+"_gone")
			if _, err := newBoardRun(t, pool, cards).Run(pollerContext(t), boardNow()); err == nil {
				t.Fatal("Run error = nil, want the write to fail")
			}
		})
	}
}

func TestRunCallBoard_failsWhenTheOldEntriesCannotBeDeleted(t *testing.T) {
	t.Parallel()
	pool, cards := newRig(t), &boardCards{}
	seedMonth(t, pool, cards)
	execAll(t, pool,
		`CREATE FUNCTION refuse_delete() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'no'; END $$`)
	execAll(t, pool,
		`CREATE TRIGGER refuse BEFORE DELETE ON call_board_entries FOR EACH STATEMENT EXECUTE FUNCTION refuse_delete()`)
	if _, err := newBoardRun(t, pool, cards).Run(pollerContext(t), boardNow()); err == nil {
		t.Fatal("Run error = nil, want the delete of the old entries to fail")
	}
}

func runBoardAt(t *testing.T, run app.RunCallBoard, at time.Time) {
	t.Helper()
	if _, err := run.Run(pollerContext(t), at); err != nil {
		t.Fatalf("Run at %s = %v", at, err)
	}
}

func seedMovementMonth(t *testing.T, pool rig, cards *boardCards) uuid.UUID {
	t.Helper()
	cabal := cards.addCabal("Pilates", cabalport.StatusActive)
	seedPeople(t, pool, cards, cabal, time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC), []personSeed{
		{name: "Cy", right: 9, resolved: 10},
		{name: "Ada", right: 8, resolved: 10},
		{name: "Bo", right: 7, resolved: 9},
		{name: "Di", right: 5, resolved: 10},
		{name: "Eli", right: 4, resolved: 4},
	})
	return cabal
}

type moved struct {
	rank, movement int
	kind           string
	right          bool
}

func wantMoved(t *testing.T, got map[string]entry, name string, want moved) {
	t.Helper()
	e := got[name]
	if e.rank == nil || *e.rank != want.rank || e.movement == nil || *e.movement != want.movement ||
		e.movementKind != want.kind || e.lastRight == nil || *e.lastRight != want.right ||
		e.lastSymbol == nil || *e.lastSymbol != "AAPLx" {
		t.Errorf("%s = %+v, want rank %d, %s %d, last call AAPLx right=%t", name, e, want.rank, want.kind,
			want.movement, want.right)
	}
}

func TestRunCallBoard_movementIsTheRankAtTheDaysFirstRunMinusTheRankNow(t *testing.T) {
	t.Parallel()
	pool, cards := newRig(t), &boardCards{}
	cabal := seedMovementMonth(t, pool, cards)
	run := newBoardRun(t, pool, cards)
	day := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)

	runBoardAt(t, run, day.Add(10*time.Minute))
	for name, e := range readEntries(t, pool, "people", "month") {
		if !unmovedOnFirstRun(e) {
			t.Errorf("first run: %s = %+v, want unmoved ranks and no last call", name, e)
		}
	}

	callSeed{
		proposal: uuid.NewSHA1(uuid.Nil, []byte("movement/late")), cabal: cabal, openedAt: day.Add(-19 * time.Hour),
		resolved: true, cabalRight: true, voters: []voteSeed{
			{user: boardUser("Cy").UUID(), voted: true, right: true},
			{user: boardUser("Ada").UUID(), voted: true, right: false},
			{user: boardUser("Bo").UUID(), voted: true, right: true},
			{user: boardUser("Di").UUID(), voted: true, right: true},
			{user: boardUser("Eli").UUID(), voted: true, right: false},
		},
	}.insert(t, pool)
	runBoardAt(t, run, day.Add(12*time.Hour))

	got := readEntries(t, pool, "people", "month")
	wantMoved(t, got, "Cy", moved{1, 0, "same", true})
	wantMoved(t, got, "Bo", moved{2, 1, "up", true})
	wantMoved(t, got, "Ada", moved{4, -2, "down", false})
	wantMoved(t, got, "Di", moved{5, -1, "down", true})
	if e := got["Eli"]; e.rank == nil || *e.rank != 3 || e.movementKind != "new" || e.baseline != nil ||
		e.movement != nil || e.lastRight == nil || *e.lastRight {
		t.Errorf("Eli = %+v, want #3, new, no baseline and a missed last call", e)
	}
}

func TestRunCallBoard_everyMonthRowIsNewOnTheFirstDayOfTheMonth(t *testing.T) {
	t.Parallel()
	pool, cards := newRig(t), &boardCards{}
	cabal := cards.addCabal("Pilates", cabalport.StatusActive)
	voters := make([]voteSeed, 0, 6)
	for _, name := range []string{"Ada", "Bo", "Cy", "Di", "Eli", "Fay"} {
		voters = append(voters, voteSeed{user: cards.addUser(name, len(voters)), voted: true})
	}
	callSeed{
		proposal: uuid.NewSHA1(uuid.Nil, []byte("first-day")), cabal: cabal,
		openedAt: time.Date(2026, 11, 1, 0, 5, 0, 0, time.UTC), voters: voters,
	}.insert(t, pool)
	run := newBoardRun(t, pool, cards)

	runBoardAt(t, run, time.Date(2026, 11, 1, 0, 20, 0, 0, time.UTC))

	month := readEntries(t, pool, "people", "month")
	if len(month) != 6 {
		t.Fatalf("month rows = %d, want 6", len(month))
	}
	for name, e := range month {
		if e.movementKind != "new" || e.movement != nil || e.baseline != nil || e.rank != nil {
			t.Errorf("%s = %+v, want an unranked new row", name, e)
		}
	}
}

func TestRunCallBoard_aNewDayTakesItsOwnBaselineAndOldOnesAgeOut(t *testing.T) {
	t.Parallel()
	pool, cards := newRig(t), &boardCards{}
	seedMovementMonth(t, pool, cards)
	run := newBoardRun(t, pool, cards)
	execAll(t, pool, `INSERT INTO call_board_baselines (board, period, period_start, day, subject_id, rank)
		VALUES ('people', 'month', '2026-08-01', '2026-09-17', $1, 1), ('people', 'month', '2026-09-01', '2026-09-19', $1, 1)`,
		boardUser("Cy").UUID())

	runBoardAt(t, run, time.Date(2026, 10, 28, 0, 10, 0, 0, time.UTC))
	runBoardAt(t, run, time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC))
	runBoardAt(t, run, time.Date(2026, 10, 29, 0, 10, 0, 0, time.UTC))

	rows, err := pool.Query(t.Context(), `SELECT day::text, count(*) FROM call_board_baselines
		WHERE board = 'people' AND period = 'month' GROUP BY day ORDER BY day`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var days []string
	for rows.Next() {
		var day string
		var n int
		if err := rows.Scan(&day, &n); err != nil {
			t.Fatal(err)
		}
		days = append(days, fmt.Sprintf("%s:%d", day, n))
	}
	if want := []string{"2026-09-19:1", "2026-10-28:4", "2026-10-29:4"}; fmt.Sprint(days) != fmt.Sprint(want) {
		t.Fatalf("baselines per day = %v, want %v: one per ranked person, none older than 40 days", days, want)
	}
}
