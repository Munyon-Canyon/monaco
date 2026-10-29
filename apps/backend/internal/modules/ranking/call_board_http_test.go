package ranking_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/rankingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	callMonth = "month"
	callWeek  = "week"
	callBoard = "/v1/leaderboards/calls"
)

type cbRow struct {
	period     string
	rank       int
	name       string
	id         uuid.UUID
	right      int
	resolved   int
	voted      int
	eligible   int
	moveKind   string
	movement   int
	lastSymbol string
}

func cbID(name string) uuid.UUID {
	sum := sha256.Sum256([]byte("call-board/" + name))
	var id uuid.UUID
	copy(id[:], sum[:])
	id[6] = id[6]&0x0f | 0x70
	id[8] = id[8]&0x3f | 0x80
	return id
}

func (r cbRow) subject() uuid.UUID {
	if r.id != uuid.Nil {
		return r.id
	}
	return cbID(r.name)
}

func cbPeriodStart(period string) string {
	switch period {
	case callWeek:
		return "2026-09-28"
	case callMonth:
		return "2026-10-01"
	}
	return "1970-01-01"
}

func (r cbRow) entry(board string, run uuid.UUID, at time.Time) map[string]any {
	moveKind := r.moveKind
	if moveKind == "" {
		moveKind = "same"
	}
	kind, handle := "person", any(r.name)
	if board == "cabals" {
		kind, handle = "cabal", nil
	}
	entry := map[string]any{
		"board":               board,
		"period":              r.period,
		"period_start":        cbPeriodStart(r.period),
		"subject_id":          r.subject(),
		"subject_kind":        kind,
		"subject_name":        r.name,
		"subject_handle":      handle,
		"subject_picture_url": nil,
		"subject_created_at":  at.Add(-time.Duration(r.rank+1) * time.Hour).Format(time.RFC3339Nano),
		"rank":                nil,
		"right_count":         r.right,
		"resolved":            r.resolved,
		"voted":               r.voted,
		"eligible":            r.eligible,
		"rate_bps":            nil,
		"turnout_bps":         r.voted * 10_000 / max(r.eligible, 1),
		"baseline_rank":       nil,
		"movement":            r.movement,
		"movement_kind":       moveKind,
		"last_call":           nil,
		"run_id":              run,
		"computed_at":         at.Format(time.RFC3339Nano),
	}
	if r.rank > 0 {
		entry["rank"], entry["rate_bps"] = r.rank, r.right*10_000/r.resolved
	}
	if r.lastSymbol != "" {
		entry["last_call"] = map[string]any{
			"proposal_id": cbID("proposal/" + r.name), "kind": "buy", "symbol": r.lastSymbol, "right": true,
		}
	}
	return entry
}

func seedCallBoard(t *testing.T, s server, board string, rows []cbRow) {
	t.Helper()
	run := ids.Real{}.NewV7()
	at := s.clock.Now().UTC()
	wire := make([]map[string]any, len(rows))
	for i, r := range rows {
		wire[i] = r.entry(board, run, at)
	}
	raw, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	q := sqlc.New(s.pool)
	if err := q.InsertCallBoardEntries(t.Context(), raw); err != nil {
		t.Fatal(err)
	}
	if err := q.InsertCallBoardRun(t.Context(), sqlc.InsertCallBoardRunParams{
		RunID: run, StartedAt: at, FinishedAt: at,
	}); err != nil {
		t.Fatal(err)
	}
}

func newRun(t *testing.T, s server) {
	t.Helper()
	at := s.clock.Now().UTC().Add(time.Minute)
	exec(
		t,
		s.pool,
		`INSERT INTO call_board_runs (run_id, started_at, finished_at, rows_written) VALUES ($1, $2, $2, 0)`,
		ids.Real{}.NewV7(),
		at,
	)
}

func peopleMonth() []cbRow {
	rows := []cbRow{
		{rank: 1, name: "Christelle", right: 11, resolved: 12, voted: 12, eligible: 12, lastSymbol: "NVDA"},
		{rank: 2, name: "Logan", right: 10, resolved: 11, voted: 11, eligible: 12, moveKind: "up", movement: 1},
		{rank: 3, name: "Mia", right: 9, resolved: 10, voted: 10, eligible: 10},
		{rank: 4, name: "Jordan", right: 11, resolved: 13, voted: 13, eligible: 14},
		{rank: 5, name: "Kai5", right: 10, resolved: 12, voted: 12, eligible: 12},
		{rank: 6, name: "Sam", right: 9, resolved: 11, voted: 11, eligible: 12, moveKind: "down", movement: -2},
		{rank: 7, name: "Kai", right: 11, resolved: 14, voted: 14, eligible: 14},
		{rank: 8, name: "Andres", right: 7, resolved: 9, voted: 9, eligible: 11},
		{rank: 9, name: "Rosa", right: 6, resolved: 8, voted: 8, eligible: 9},
		{rank: 10, name: "Nora", right: 8, resolved: 11, voted: 11, eligible: 11},
		{rank: 11, name: "Diego", right: 7, resolved: 10, voted: 10, eligible: 10},
		{rank: 12, name: "Maya", right: 6, resolved: 9, voted: 9, eligible: 9},
	}
	for rank := 13; rank <= 24; rank++ {
		rows = append(rows, cbRow{
			rank:     rank,
			name:     fmt.Sprintf("Person %d", rank),
			right:    6 - (rank-13)/3,
			resolved: 10,
			voted:    10,
			eligible: 10,
		})
	}
	for i := range rows {
		rows[i].period = callMonth
	}
	return rows
}

type look struct {
	Rank int32
	Name string
	Pct  int32
	Mine bool
	You  bool
}

func lookAt(r *api.CallBoardRow) *look {
	if r == nil {
		return nil
	}
	out := look{Name: r.Subject.Name, Mine: r.IsMine, You: r.IsViewer}
	if r.Rank != nil {
		out.Rank = *r.Rank
	}
	if r.RatePct != nil {
		out.Pct = *r.RatePct
	}
	return &out
}

func looks(rows []api.CallBoardRow) []look {
	out := make([]look, len(rows))
	for i := range rows {
		out[i] = *lookAt(&rows[i])
	}
	return out
}

type gapLook struct {
	Versus  string
	Points  int32
	Calls   int32
	Perfect bool
}

func gapAt(g *api.CallBoardGap) *gapLook {
	if g == nil {
		return nil
	}
	out := gapLook{Versus: string(g.Versus), Points: g.Points, Calls: -1, Perfect: g.AboveIsPerfect}
	if g.CallsToPass != nil {
		out.Calls = *g.CallsToPass
	}
	return &out
}

func names(rows []api.CallBoardRow) []string {
	out := make([]string, len(rows))
	for i := range rows {
		out[i] = rows[i].Subject.Name
	}
	return out
}

func ranks(rows []api.CallBoardRow) []int32 {
	out := make([]int32, len(rows))
	for i := range rows {
		if rows[i].Rank != nil {
			out[i] = *rows[i].Rank
		}
	}
	return out
}

func same(t *testing.T, label string, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		g, _ := json.Marshal(got)
		w, _ := json.Marshal(want)
		t.Fatalf("%s = %s, want %s", label, g, w)
	}
}

func getCalls(t *testing.T, s server, query string, viewer ids.UserID) (api.CallBoard, int) {
	t.Helper()
	rec := s.get(t, callBoard+query, viewer)
	var page api.CallBoard
	if rec.Code == http.StatusOK {
		decode(t, rec, &page)
	}
	return page, rec.Code
}

func callsOK(t *testing.T, s server, query string, viewer ids.UserID) api.CallBoard {
	t.Helper()
	page, code := getCalls(t, s, query, viewer)
	same(t, "status", code, http.StatusOK)
	return page
}

func viewerOf(name string) ids.UserID { return ids.UserIDFrom(cbID(name)) }

func noNeighbours(page api.CallBoard) []any {
	return []any{page.Me, page.Above, page.Below, page.Gap}
}

func wantNoNeighbours() []any {
	return []any{(*api.CallBoardRow)(nil), (*api.CallBoardRow)(nil), (*api.CallBoardRow)(nil), (*api.CallBoardGap)(nil)}
}

func TestCallBoard_peopleMonthServesTheCards(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	seedCallBoard(t, s, "people", peopleMonth())
	page := callsOK(t, s, "?board=people&period=month", viewerOf("Andres"))
	same(t, "board", []string{string(page.Board), string(page.Filter), string(page.Period.Kind)},
		[]string{"people", "all", "month"})
	same(t, "period", []string{page.Period.Start.Format(time.DateOnly), page.Period.End.Format(time.DateOnly)},
		[]string{"2026-10-01", "2026-10-31"})
	same(t, "computed_at", page.ComputedAt.Equal(s.clock.Now()), true)
	same(t, "total_ranked", page.TotalRanked, int32(24))
	same(t, "me", lookAt(page.Me), &look{Rank: 8, Name: "Andres", Pct: 78, You: true})
	same(
		t,
		"me record",
		[]int32{page.Me.Right, page.Me.Resolved, page.Me.Voted, page.Me.Eligible},
		[]int32{7, 9, 9, 11},
	)
	same(t, "above", lookAt(page.Above), &look{Rank: 7, Name: "Kai", Pct: 79})
	same(t, "below", lookAt(page.Below), &look{Rank: 9, Name: "Rosa", Pct: 75})
	same(t, "gap", gapAt(page.Gap), &gapLook{Versus: "above", Points: -1, Calls: 1})
	same(t, "climb_bps", page.ClimbBps, int32(7777))
	same(t, "unranked", []bool{page.UnrankedMe == nil, len(page.Unranked) == 0}, []bool{true, true})
}

func TestCallBoard_peopleMonthServesTheTopThreeAndTheWindow(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	seedCallBoard(t, s, "people", peopleMonth())
	page := callsOK(t, s, "?board=people&period=month", viewerOf("Andres"))
	same(t, "top", names(page.Top), []string{"Christelle", "Logan", "Mia"})
	same(t, "top[0].last_call", page.Top[0].LastCall.Symbol, "NVDA")
	same(t, "top[1].movement", []any{page.Top[1].Movement.Kind, page.Top[1].Movement.Places},
		[]any{api.Up, int32(1)})
	same(t, "around", names(page.Around.Rows), []string{"Sam", "Kai", "Andres", "Rosa", "Nora", "Diego", "Maya"})
	same(t, "around ranks", ranks(page.Around.Rows), []int32{6, 7, 8, 9, 10, 11, 12})
	same(t, "after_cursor", page.Around.AfterCursor != nil, true)
	same(t, "sam.movement", []any{page.Around.Rows[0].Movement.Kind, page.Around.Rows[0].Movement.Places},
		[]any{api.Down, int32(2)})
}

func TestCallBoard_theLeaderSeesTheOneBelow(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	seedCallBoard(t, s, "people", peopleMonth())
	page := callsOK(t, s, "", viewerOf("Christelle"))
	same(t, "me", lookAt(page.Me), &look{Rank: 1, Name: "Christelle", Pct: 92, You: true})
	same(t, "above", lookAt(page.Above), (*look)(nil))
	same(t, "below", lookAt(page.Below), &look{Rank: 2, Name: "Logan", Pct: 91})
	same(t, "gap", gapAt(page.Gap), &gapLook{Versus: "below", Points: 1, Calls: -1})
	same(t, "around ranks", ranks(page.Around.Rows), []int32{4, 5})
}

func TestCallBoard_aPerfectLeaderCannotBePassed(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	rows := peopleMonth()
	rows[0].right, rows[0].resolved = 12, 12
	seedCallBoard(t, s, "people", rows)
	page := callsOK(t, s, "", viewerOf("Logan"))
	same(t, "gap", gapAt(page.Gap), &gapLook{Versus: "above", Points: -9, Calls: -1, Perfect: true})
}

func TestCallBoard_aViewerOnThePodiumGetsNeighboursFromTheTop(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	seedCallBoard(t, s, "people", peopleMonth())
	page := callsOK(t, s, "", viewerOf("Jordan"))
	same(t, "above", lookAt(page.Above), &look{Rank: 3, Name: "Mia", Pct: 90})
	same(t, "below", lookAt(page.Below), &look{Rank: 5, Name: "Kai5", Pct: 83})
	same(t, "around ranks", ranks(page.Around.Rows), []int32{4, 5, 6, 7, 8})
}

func pagedBoard() []cbRow {
	rows := []cbRow{{period: callMonth, name: "Newcomer", right: 1, resolved: 1, voted: 1, eligible: 1}}
	for rank := 1; rank <= 60; rank++ {
		rows = append(rows, cbRow{
			period:   callMonth,
			rank:     rank,
			name:     fmt.Sprintf("P%02d", rank),
			right:    9,
			resolved: 10,
			voted:    10,
			eligible: 10,
		})
	}
	return rows
}

func TestCallBoard_pagingContinuesWithoutRepeatingOrSkipping(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	seedCallBoard(t, s, "people", pagedBoard())
	viewer := viewerOf("Newcomer")
	page := callsOK(t, s, "", viewer)
	same(t, "first around", ranks(page.Around.Rows), []int32{4, 5, 6, 7, 8, 9, 10})
	var seen []int32
	for cursor := page.Around.AfterCursor; cursor != nil; {
		next := callsOK(t, s, "?cursor="+*cursor, viewer)
		same(
			t,
			"only around",
			[]bool{len(next.Top) == 0, next.Me == nil, next.UnrankedMe == nil, len(next.Unranked) == 0},
			[]bool{true, true, true, true},
		)
		seen = append(seen, ranks(next.Around.Rows)...)
		cursor = next.Around.AfterCursor
	}
	want := make([]int32, 0, 50)
	for rank := int32(11); rank <= 60; rank++ {
		want = append(want, rank)
	}
	same(t, "paged ranks", seen, want)
	same(t, "page size", len(callsOK(t, s, "?cursor="+*page.Around.AfterCursor, viewer).Around.Rows), 20)
}

func TestCallBoard_aStaleCursorIsTheCursorStaleCode(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	seedCallBoard(t, s, "people", peopleMonth())
	page := callsOK(t, s, "", viewerOf("Andres"))
	newRun(t, s)
	rec := s.get(t, callBoard+"?cursor="+*page.Around.AfterCursor, viewerOf("Andres"))
	same(t, "status and code", []any{rec.Code, problemOf(t, rec)}, []any{http.StatusConflict, apibase.CursorStale})
}

func TestCallBoard_friendsReRanksTheViewerAndTheirFollows(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	seedCallBoard(t, s, "people", peopleMonth())
	me := cbID("Andres")
	for _, name := range []string{"Christelle", "Logan", "Mia", "Jordan", "Kai", "Rosa", "Nora", "Diego", "Maya"} {
		follow(t, s.pool, me, cbID(name))
	}
	page := callsOK(t, s, "?filter=friends", ids.UserIDFrom(me))
	same(t, "filter and total", []any{page.Filter, page.TotalRanked}, []any{api.CallBoardFilterFriends, int32(10)})
	same(t, "top", names(page.Top), []string{"Christelle", "Logan", "Mia"})
	same(t, "me", lookAt(page.Me), &look{Rank: 6, Name: "Andres", Pct: 78, You: true})
	same(t, "above", lookAt(page.Above), &look{Rank: 5, Name: "Kai", Pct: 79})
	same(t, "around", names(page.Around.Rows), []string{"Jordan", "Kai", "Andres", "Rosa", "Nora", "Diego", "Maya"})
	same(t, "around ranks", ranks(page.Around.Rows), []int32{4, 5, 6, 7, 8, 9, 10})
	same(t, "after_cursor", page.Around.AfterCursor == nil, true)
	same(t, "movement", page.Top[1].Movement.Kind, api.Up)
}

func cabalNames() []string {
	return []string{
		"Pilates",
		"Tech Titans",
		"AC Capital",
		"Degens",
		"Quant Kids",
		"Moonshots",
		"Value Club",
		"Macro Crew",
	}
}

func cabalRows(mine map[string]uuid.UUID, ranked bool) []cbRow {
	rights := []int{13, 8, 7, 6, 6, 6, 5, 5}
	resolved := []int{14, 9, 8, 7, 8, 9, 8, 9}
	rows := make([]cbRow, len(rights))
	next := 0
	for i, name := range cabalNames() {
		rows[i] = cbRow{
			period:   callMonth,
			name:     name,
			id:       mine[name],
			right:    rights[i],
			resolved: resolved[i],
			voted:    5,
			eligible: 5,
		}
		if ranked || mine[name] == uuid.Nil {
			next++
			rows[i].rank = next
			continue
		}
		rows[i].right, rows[i].resolved = 1, 1+i
	}
	return rows
}

func mineIDs(t *testing.T, s server, viewer testkit.SeededUser, names ...string) map[string]uuid.UUID {
	t.Helper()
	out := map[string]uuid.UUID{}
	for _, name := range names {
		out[name] = testkit.NewCabal(t, s.pool, testkit.WithName(name), testkit.WithCreator(viewer.ID)).ID.UUID()
	}
	return out
}

func TestCallBoard_cabalsServesYourBestAndRepeatsYoursBelowThePodium(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	viewer := testkit.SeedUser(t, s.pool, testkit.UserOpts{})
	mine := mineIDs(t, s, viewer, "Tech Titans", "AC Capital", "Degens")
	seedCallBoard(t, s, "cabals", cabalRows(mine, true))
	page := callsOK(t, s, "?board=cabals", viewer.ID)
	same(t, "board and total", []any{page.Board, page.TotalRanked}, []any{api.CallBoardBoardCabals, int32(8)})
	same(t, "top", names(page.Top), []string{"Pilates", "Tech Titans", "AC Capital"})
	same(t, "me", lookAt(page.Me), &look{Rank: 2, Name: "Tech Titans", Pct: 89, Mine: true})
	same(t, "me kind", page.Me.Subject.Kind, api.Cabal)
	same(t, "above", lookAt(page.Above), &look{Rank: 1, Name: "Pilates", Pct: 93})
	same(t, "gap", gapAt(page.Gap), &gapLook{Versus: "above", Points: -4, Calls: 5})
	same(t, "around", looks(page.Around.Rows), []look{
		{Rank: 2, Name: "Tech Titans", Pct: 89, Mine: true},
		{Rank: 3, Name: "AC Capital", Pct: 88, Mine: true},
		{Rank: 4, Name: "Degens", Pct: 86, Mine: true},
		{Rank: 5, Name: "Quant Kids", Pct: 75},
		{Rank: 6, Name: "Moonshots", Pct: 67},
	})
}

func TestCallBoard_cabalsReachesYourLowestCabalWithinTenRanks(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	viewer := testkit.SeedUser(t, s.pool, testkit.UserOpts{})
	mine := mineIDs(t, s, viewer, "Pilates", "Macro Crew")
	seedCallBoard(t, s, "cabals", cabalRows(mine, true))
	page := callsOK(t, s, "?board=cabals", viewer.ID)
	same(t, "me", lookAt(page.Me), &look{Rank: 1, Name: "Pilates", Pct: 93, Mine: true})
	same(t, "above", lookAt(page.Above), (*look)(nil))
	same(t, "below", lookAt(page.Below), &look{Rank: 2, Name: "Tech Titans", Pct: 89})
	same(t, "gap", gapAt(page.Gap), &gapLook{Versus: "below", Points: 4, Calls: -1})
	same(t, "around ranks", ranks(page.Around.Rows), []int32{1, 2, 3, 4, 5, 6, 7, 8})
}

func TestCallBoard_cabalsUnrankedPointsAtYourCabalWithTheMostResolvedCalls(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	viewer := testkit.SeedUser(t, s.pool, testkit.UserOpts{})
	mine := mineIDs(t, s, viewer, "Tech Titans", "Degens")
	seedCallBoard(t, s, "cabals", cabalRows(mine, false))
	page := callsOK(t, s, "?board=cabals", viewer.ID)
	same(t, "me, above, below, gap", noNeighbours(page), wantNoNeighbours())
	card := page.UnrankedMe
	same(t, "unranked_me", []any{card.Subject.Name, card.Subject.Id, card.Resolved, card.Needed, card.PendingSymbols},
		[]any{"Degens", mine["Degens"], int32(4), int32(1), []string{}})
	same(t, "climb_bps", page.ClimbBps, int32(8000))
	same(t, "unranked", names(page.Unranked), []string{"Degens", "Tech Titans"})
	same(t, "around ranks", ranks(page.Around.Rows), []int32{1, 2, 3, 4, 5, 6})
}

func seedPendingVote(t *testing.T, s server, user uuid.UUID, symbol string, openedAt time.Time) {
	t.Helper()
	proposal, cabal := cbID("pending/"+symbol), cbID("cabal/pending")
	exec(t, s.pool, `INSERT INTO calls (proposal_id, cabal_id, kind, symbol, opened_at, state, updated_at)
		VALUES ($1, $2, 'buy', $3, $4, 'open', $4)`, proposal, cabal, symbol, openedAt)
	exec(t, s.pool, `INSERT INTO call_voters (proposal_id, user_id, cabal_id, choice, choice_at)
		VALUES ($1, $2, $3, 'yes', $4)`, proposal, user, cabal, openedAt)
}

func weekBoard() []cbRow {
	return []cbRow{
		{period: callWeek, rank: 1, name: "Mia", right: 6, resolved: 6, voted: 6, eligible: 6},
		{period: callWeek, rank: 2, name: "Logan", right: 6, resolved: 7, voted: 7, eligible: 7},
		{period: callWeek, rank: 3, name: "Christelle", right: 5, resolved: 6, voted: 6, eligible: 6},
		{period: callWeek, rank: 4, name: "Kai", right: 4, resolved: 5, voted: 6, eligible: 7},
		{period: callWeek, rank: 5, name: "Jordan", right: 4, resolved: 5, voted: 5, eligible: 7},
		{period: callWeek, name: "Andres", right: 2, resolved: 3, voted: 5, eligible: 6},
		{period: callWeek, name: "Rosa", right: 1, resolved: 2, voted: 3, eligible: 4},
		{period: callWeek, name: "Quiet", right: 0, resolved: 0, voted: 0, eligible: 4},
	}
}

func TestCallBoard_aViewerUnderFiveResolvedVotesGetsTheNotRankedCard(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	seedCallBoard(t, s, "people", weekBoard())
	me := cbID("Andres")
	for _, name := range []string{"Rosa", "Quiet", "Kai"} {
		follow(t, s.pool, me, cbID(name))
	}
	opened := s.clock.Now().UTC().Add(-2 * time.Hour)
	seedPendingVote(t, s, me, "SPACEX", opened)
	seedPendingVote(t, s, me, "AMZN", opened.Add(time.Hour))
	page := callsOK(t, s, "?period=week", ids.UserIDFrom(me))
	same(t, "no standing", noNeighbours(page), wantNoNeighbours())
	same(
		t,
		"period",
		[]any{page.TotalRanked, page.Period.Start.Format(time.DateOnly), page.Period.End.Format(time.DateOnly)},
		[]any{int32(5), "2026-09-28", "2026-10-04"},
	)
	card := page.UnrankedMe
	same(
		t,
		"unranked_me",
		[]any{
			card.Subject.Id,
			card.Subject.Name,
			card.Resolved,
			card.Needed,
			card.Voted,
			card.Eligible,
			card.PendingSymbols,
		},
		[]any{me, "Andres", int32(3), int32(2), int32(5), int32(6), []string{"SPACEX", "AMZN"}},
	)
	same(t, "climb_bps", page.ClimbBps, int32(6000))
	same(t, "around", names(page.Around.Rows), []string{"Kai", "Jordan"})
	same(t, "unranked", looks(page.Unranked), []look{{Name: "Andres", You: true}, {Name: "Rosa"}})
	same(t, "needed", []int32{*page.Unranked[0].Needed, *page.Unranked[1].Needed}, []int32{2, 3})
}

func TestCallBoard_aViewerWithNoRowStillGetsTheNotRankedCard(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	viewer := testkit.SeedUser(t, s.pool, testkit.UserOpts{Handle: "newbie"})
	seedCallBoard(t, s, "people", peopleMonth())
	page := callsOK(t, s, "", viewer.ID)
	card := page.UnrankedMe
	same(t, "unranked_me", []any{card.Subject.Id, card.Subject.Name, card.Resolved, card.Needed},
		[]any{viewer.ID.UUID(), "newbie", int32(0), int32(5)})
	same(t, "me, climb, unranked", []any{page.Me, page.ClimbBps, len(page.Unranked)},
		[]any{(*api.CallBoardRow)(nil), int32(0), 0})
}

func TestCallBoard_theTopThreeIsReadOncePerRun(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	seedCallBoard(t, s, "people", peopleMonth())
	viewer := viewerOf("Andres")
	callsOK(t, s, "", viewer)
	exec(t, s.pool, `UPDATE call_board_entries SET subject_name = 'Renamed' WHERE rank = 1`)
	same(t, "while the run is unchanged", callsOK(t, s, "", viewer).Top[0].Subject.Name, "Christelle")
	newRun(t, s)
	same(t, "after a new run", callsOK(t, s, "", viewer).Top[0].Subject.Name, "Renamed")
}

func TestCallBoard_beforeTheFirstRunTheScreenIsEmpty(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	page := callsOK(t, s, "?period=all", viewerOf("Andres"))
	same(t, "counts", []any{page.TotalRanked, len(page.Top), len(page.Around.Rows), len(page.Unranked)},
		[]any{int32(0), 0, 0, 0})
	same(t, "me and dates", []any{page.Me, page.Period.Start, page.Period.End},
		[]any{(*api.CallBoardRow)(nil), page.Period.Start, page.Period.End})
	same(t, "all time has no dates", page.Period.Start == nil && page.Period.End == nil, true)
}

func TestCallBoard_theHandlerNeedsAUserCaller(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	_, err := adapters.HTTP{}.GetCallLeaderboard(ctx, api.GetCallLeaderboardRequestObject{})
	same(t, "no actor", errs.CodeOf(err), errs.CodeUnauthorized)
	system := auth.WithActor(ctx, auth.Actor{Kind: auth.ActorSystem, ID: "poller.test"})
	_, err = adapters.HTTP{}.GetCallLeaderboard(system, api.GetCallLeaderboardRequestObject{})
	same(t, "a system actor", errs.CodeOf(err), errs.CodeForbidden)
}

func TestCallBoard_theHeadCacheReturnsALoadFailure(t *testing.T) {
	t.Parallel()
	cache := adapters.NewCallBoardHeadCache(1)
	_, err := cache.Load(t.Context(), app.CallBoardHeadKey{}, func(context.Context) (app.CallBoardHead, error) {
		return app.CallBoardHead{}, errs.New(errs.CodeInternal, "test.load")
	})
	same(t, "load failure", errs.CodeOf(err), errs.CodeInternal)
}

func TestCallBoard_rejectsWhatItCannotServe(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	viewer := viewerOf("Andres")
	for _, query := range []string{
		"?board=teams", "?period=year", "?filter=mutuals", "?board=cabals&filter=friends", "?cursor=!!", "?cursor=MQ",
	} {
		rec := s.get(t, callBoard+query, viewer)
		same(t, "GET "+query, []any{rec.Code, problemOf(t, rec)}, []any{http.StatusBadRequest, apibase.InvalidInput})
	}
	same(t, "GET without a token", s.get(t, callBoard, ids.UserID{}).Code, http.StatusUnauthorized)
}
