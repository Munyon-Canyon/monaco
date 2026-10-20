package ranking_test

import (
	"net/http"
	"net/url"
	"strconv"
	"testing"

	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/rankingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func viewRows(board string) []boardRow {
	type spec struct {
		name    string
		bps     int64
		pnl     int64
		join    string
		members int
	}
	specs := []spec{
		{"delta", 900, 300, "open", 12},
		{"Alpha", 800, 700, "request", 4},
		{"charlie", 700, 100, "open", 6},
		{"Bravo", 600, 500, "request", 20},
		{"echo", 500, 900, "open", 2},
	}
	rows := make([]boardRow, 0, len(specs))
	for i, sp := range specs {
		bps, join := sp.bps, sp.join
		rows = append(rows, boardRow{
			board: board, rank: i + 1, subject: ids.Real{}.NewV7(), bps: &bps, value: int64(1000 * (i + 1)),
			name: sp.name, pnl: sp.pnl, join: &join, members: sp.members,
		})
	}
	return rows
}

func namesOf(page api.LeaderboardPage) []string {
	out := make([]string, 0, len(page.Rows))
	for _, row := range page.Rows {
		out = append(out, row.Subject.Name)
	}
	return out
}

func walk(t *testing.T, s server, path string, viewer ids.UserID, limit int) api.LeaderboardPage {
	t.Helper()
	var all api.LeaderboardPage
	cursor := ""
	for range 20 {
		query := path + "&limit=" + strconv.Itoa(limit)
		if cursor != "" {
			query += "&cursor=" + url.QueryEscape(cursor)
		}
		rec := s.get(t, query, viewer)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d %s", query, rec.Code, rec.Body)
		}
		page := pageOf(t, rec)
		all.Rows = append(all.Rows, page.Rows...)
		all.Me = page.Me
		if page.NextCursor == nil {
			return all
		}
		cursor = *page.NextCursor
	}
	t.Fatalf("GET %s never ended", path)
	return all
}

func TestBoards_CabalsSortOrdersTheWholeBoardAndPagesStably(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	seedBoard(t, s.pool, s.clock.Now().UTC(), viewRows("cabals"))
	viewer := ids.UserIDFrom(ids.Real{}.NewV7())
	for query, want := range map[string][]string{
		"range=ALL":       {"delta", "Alpha", "charlie", "Bravo", "echo"},
		"sort=return_pct": {"delta", "Alpha", "charlie", "Bravo", "echo"},
		"sort=pnl":        {"echo", "Alpha", "Bravo", "delta", "charlie"},
		"sort=name":       {"Alpha", "Bravo", "charlie", "delta", "echo"},
	} {
		path := "/v1/leaderboards/cabals?" + query
		for _, limit := range []int{50, 2, 1} {
			page := walk(t, s, path, viewer, limit)
			if got := namesOf(page); !equal(got, want) {
				t.Errorf("GET %s limit %d = %v, want %v", path, limit, got, want)
			}
			for i, row := range page.Rows {
				if row.Rank != int32(i+1) {
					t.Errorf("GET %s limit %d rank[%d] = %d", path, limit, i, row.Rank)
				}
			}
		}
	}
}

func TestBoards_CabalsMembershipAndMinMembersFilter(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	seedBoard(t, s.pool, s.clock.Now().UTC(), viewRows("cabals"))
	viewer := ids.UserIDFrom(ids.Real{}.NewV7())
	for query, want := range map[string][]string{
		"membership=all":                         {"delta", "Alpha", "charlie", "Bravo", "echo"},
		"membership=accepting_requests":          {"delta", "Alpha", "charlie", "Bravo", "echo"},
		"membership=open":                        {"delta", "charlie", "echo"},
		"membership=approval":                    {"Alpha", "Bravo"},
		"min_members=5":                          {"delta", "charlie", "Bravo"},
		"min_members=10":                         {"delta", "Bravo"},
		"min_members=21":                         {},
		"membership=open&min_members=5":          {"delta", "charlie"},
		"membership=approval&sort=name":          {"Alpha", "Bravo"},
		"membership=open&min_members=5&sort=pnl": {"delta", "charlie"},
	} {
		page := walk(t, s, "/v1/leaderboards/cabals?"+query, viewer, 2)
		if got := namesOf(page); !equal(got, want) {
			t.Errorf("GET ?%s = %v, want %v", query, got, want)
		}
		for i, row := range page.Rows {
			if row.Rank != int32(i+1) {
				t.Errorf("GET ?%s rank[%d] = %d, want %d", query, i, row.Rank, i+1)
			}
		}
	}
}

func TestBoards_ViewParamsRejectUnknownValues(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	viewer := ids.UserIDFrom(ids.Real{}.NewV7())
	for _, path := range []string{
		"/v1/leaderboards/cabals?sort=size",
		"/v1/leaderboards/cabals?membership=closed",
		"/v1/leaderboards/cabals?min_members=-1",
		"/v1/leaderboards/cabals?min_members=10001",
		"/v1/leaderboards/people?sort=size",
	} {
		rec := s.get(t, path, viewer)
		if rec.Code != http.StatusBadRequest || problemOf(t, rec) != apibase.InvalidInput {
			t.Errorf("GET %s = %d %s, want 400 invalid_input", path, rec.Code, rec.Body)
		}
	}
}

func TestBoards_PeopleSortReranksRowsAndMe(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	rows := viewRows("people")
	seedBoard(t, s.pool, s.clock.Now().UTC(), rows)
	viewer := ids.UserIDFrom(rows[2].subject)
	for query, want := range map[string]struct {
		names []string
		me    int32
	}{
		"?sort=pnl":         {[]string{"echo", "Alpha", "Bravo", "delta", "charlie"}, 5},
		"?sort=name":        {[]string{"Alpha", "Bravo", "charlie", "delta", "echo"}, 3},
		"?sort=return_pct":  {[]string{"delta", "Alpha", "charlie", "Bravo", "echo"}, 3},
		"?sort=pnl&limit=2": {[]string{"echo", "Alpha"}, 5},
	} {
		rec := s.get(t, "/v1/leaderboards/people"+query, viewer)
		page := pageOf(t, rec)
		if rec.Code != http.StatusOK || !equal(namesOf(page), want.names) || page.Me == nil || page.Me.Rank != want.me {
			t.Errorf("GET people%s = %d %s, want %v me %d", query, rec.Code, rec.Body, want.names, want.me)
		}
	}
}

func TestBoards_FriendsSortRanksTheFriendsDensely(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	rows := viewRows("people")
	seedBoard(t, s.pool, s.clock.Now().UTC(), rows)
	viewer := rows[2].subject
	follow(t, s.pool, viewer, rows[0].subject)
	follow(t, s.pool, viewer, rows[4].subject)
	rec := s.get(t, friendsPath+"&sort=pnl", ids.UserIDFrom(viewer))
	page := pageOf(t, rec)
	want := []string{"echo", "delta", "charlie"}
	if rec.Code != http.StatusOK || !equal(namesOf(page), want) || !equal(ranksOf(page), []int32{1, 2, 3}) ||
		page.Me == nil || page.Me.Rank != 3 {
		t.Fatalf("GET friends sort=pnl = %d %s, want %v", rec.Code, rec.Body, want)
	}
	rec = s.get(t, friendsPath+"&sort=name", ids.UserIDFrom(viewer))
	if got, want := namesOf(pageOf(t, rec)), []string{"charlie", "delta", "echo"}; !equal(got, want) {
		t.Fatalf("GET friends sort=name = %v, want %v", got, want)
	}
}
