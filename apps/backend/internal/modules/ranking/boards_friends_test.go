package ranking_test

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/rankingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const friendsPath = "/v1/leaderboards/people?filter=friends"

func follow(t *testing.T, pool *pgxpool.Pool, follower, followee uuid.UUID) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO follows (id, follower_id, followee_id) VALUES ($1, $2, $3)`,
		ids.Real{}.NewV7(), follower, followee); err != nil {
		t.Fatal(err)
	}
}

func unfollow(t *testing.T, pool *pgxpool.Pool, follower, followee uuid.UUID) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		`UPDATE follows SET deleted_at = now() WHERE follower_id = $1 AND followee_id = $2`,
		follower, followee); err != nil {
		t.Fatal(err)
	}
}

func subjectsOf(page api.LeaderboardPage) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(page.Rows))
	for _, row := range page.Rows {
		out = append(out, row.Subject.Id)
	}
	return out
}

func ranksOf(page api.LeaderboardPage) []int32 {
	out := make([]int32, 0, len(page.Rows))
	for _, row := range page.Rows {
		out = append(out, row.Rank)
	}
	return out
}

func equal[T comparable](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func friendsFixture(t *testing.T) (server, []boardRow, ids.UserID) {
	t.Helper()
	s := newServer(t)
	rows := boardRows("people", 8)
	seedBoard(t, s.pool, s.clock.Now().UTC(), rows)
	viewer := rows[5].subject
	follow(t, s.pool, viewer, rows[1].subject)
	follow(t, s.pool, viewer, rows[3].subject)
	follow(t, s.pool, viewer, rows[7].subject)
	follow(t, s.pool, rows[0].subject, viewer)
	return s, rows, ids.UserIDFrom(viewer)
}

func TestBoards_FriendsFilter_OnlyFollowedPlusMe(t *testing.T) {
	t.Parallel()
	s, rows, viewer := friendsFixture(t)
	rec := s.get(t, friendsPath, viewer)
	page := pageOf(t, rec)
	want := []uuid.UUID{rows[1].subject, rows[3].subject, rows[5].subject, rows[7].subject}
	if rec.Code != http.StatusOK || page.Board != "people" || !equal(subjectsOf(page), want) {
		t.Fatalf("GET = %d %s, want subjects %v", rec.Code, rec.Body, want)
	}
	if page.Me == nil || page.Me.Subject.Id != viewer.UUID() || page.Rows[0].Subject.Kind != api.User {
		t.Fatalf("me = %+v", page.Me)
	}
}

func TestBoards_FriendsFilter_RenumbersRanks(t *testing.T) {
	t.Parallel()
	s, rows, viewer := friendsFixture(t)
	first := pageOf(t, s.get(t, friendsPath+"&limit=3", viewer))
	cursor := domain.EncodeCursor(3)
	if !equal(ranksOf(first), []int32{1, 2, 3}) || first.NextCursor == nil || *first.NextCursor != cursor {
		t.Fatalf("first page ranks = %v, cursor %v", ranksOf(first), first.NextCursor)
	}
	if first.Me == nil || first.Me.Rank != 3 || first.Me.Subject.Id != rows[5].subject {
		t.Fatalf("first page me = %+v, want the friends rank 3", first.Me)
	}
	second := pageOf(t, s.get(t, friendsPath+"&limit=3&cursor="+cursor, viewer))
	if !equal(ranksOf(second), []int32{4}) || second.NextCursor != nil || second.Rows[0].Subject.Id != rows[7].subject {
		t.Fatalf("second page = %+v", second)
	}
	if second.Me == nil || second.Me.Rank != 3 {
		t.Fatalf("second page me = %+v, want the friends rank 3", second.Me)
	}
}

func TestBoards_FilterAllKeepsTheGlobalRanks(t *testing.T) {
	t.Parallel()
	s, _, viewer := friendsFixture(t)
	global := pageOf(t, s.get(t, "/v1/leaderboards/people?filter=all", viewer))
	if global.Me == nil || global.Me.Rank != 6 || len(global.Rows) != 8 {
		t.Fatalf("filter=all me = %+v, want the global rank 6", global.Me)
	}
}

func assertOwnRowOnly(t *testing.T, page api.LeaderboardPage, subject uuid.UUID) {
	t.Helper()
	if len(page.Rows) != 1 || page.Rows[0].Rank != 1 || page.Rows[0].Subject.Id != subject ||
		page.Me == nil || page.Me.Rank != 1 || page.NextCursor != nil {
		t.Fatalf("ranked viewer page = %+v", page)
	}
}

func TestBoards_FriendsFilter_FollowsNobody(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	rows := boardRows("people", 3)
	run := seedBoard(t, s.pool, s.clock.Now().UTC(), rows)
	assertOwnRowOnly(t, pageOf(t, s.get(t, friendsPath, ids.UserIDFrom(rows[2].subject))), rows[2].subject)
	rec := s.get(t, friendsPath, ids.UserIDFrom(ids.Real{}.NewV7()))
	page := pageOf(t, rec)
	if rec.Code != http.StatusOK || page.RunId == nil || *page.RunId != run || len(page.Rows) != 0 {
		t.Fatalf("unranked viewer GET = %d %s", rec.Code, rec.Body)
	}
	if page.Me != nil || page.NextCursor != nil {
		t.Fatalf("unranked viewer page = %+v, want no me and no cursor", page)
	}
}

func TestBoards_FriendsFilter_Unfollowed(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	rows := boardRows("people", 3)
	seedBoard(t, s.pool, s.clock.Now().UTC(), rows)
	viewer := ids.UserIDFrom(rows[2].subject)
	follow(t, s.pool, viewer.UUID(), rows[0].subject)
	both := []uuid.UUID{rows[0].subject, rows[2].subject}
	if page := pageOf(t, s.get(t, friendsPath, viewer)); !equal(subjectsOf(page), both) {
		t.Fatalf("subjects after following = %v", subjectsOf(page))
	}
	unfollow(t, s.pool, viewer.UUID(), rows[0].subject)
	if page := pageOf(t, s.get(t, friendsPath, viewer)); !equal(subjectsOf(page), []uuid.UUID{rows[2].subject}) {
		t.Fatalf("subjects after unfollowing = %v, want the viewer only", subjectsOf(page))
	}
}

type downFollows struct{ testkit.Faults }

func (f *downFollows) FollowingIDs(context.Context, ids.UserID) ([]ids.UserID, error) {
	return nil, f.Check("FollowingIDs")
}

func TestBoards_FriendsFilter_SocialDown(t *testing.T) {
	t.Parallel()
	down := &downFollows{}
	down.Fail("FollowingIDs", errs.New(errs.CodeUpstreamUnavailable, "social.FollowsPort.FollowingIDs"))
	s := newServerWith(t, down, false)
	rows := boardRows("people", 2)
	seedBoard(t, s.pool, s.clock.Now().UTC(), rows)
	viewer := ids.UserIDFrom(rows[0].subject)
	rec := s.get(t, friendsPath, viewer)
	var problem apibase.Problem
	decode(t, rec, &problem)
	unavailable := rec.Code == http.StatusServiceUnavailable && problem.Code == apibase.UpstreamUnavailable
	if !unavailable || problem.TraceId == "" {
		t.Fatalf("GET = %d %s, want 503 upstream_unavailable with a trace_id", rec.Code, rec.Body)
	}
	if rec = s.get(t, "/v1/leaderboards/people", viewer); rec.Code != http.StatusOK {
		t.Fatalf("filter=all GET = %d %s, want it to ignore social", rec.Code, rec.Body)
	}
}

func TestBoards_FriendsFilter_UnwiredSocialIsUnavailable(t *testing.T) {
	t.Parallel()
	s := newServerWith(t, nil, false)
	rows := boardRows("people", 1)
	seedBoard(t, s.pool, s.clock.Now().UTC(), rows)
	rec := s.get(t, friendsPath, ids.UserIDFrom(rows[0].subject))
	if rec.Code != http.StatusServiceUnavailable || problemOf(t, rec) != apibase.UpstreamUnavailable {
		t.Fatalf("GET = %d %s, want 503 upstream_unavailable", rec.Code, rec.Body)
	}
}

func TestBoards_FriendsFilter_UnknownFilterIsInvalidInput(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	viewer := ids.UserIDFrom(ids.Real{}.NewV7())
	for _, query := range []string{"filter=enemies", "filter=", "filter=FRIENDS"} {
		rec := s.get(t, "/v1/leaderboards/people?"+query, viewer)
		if rec.Code != http.StatusBadRequest || problemOf(t, rec) != apibase.InvalidInput {
			t.Errorf("GET ?%s = %d %s, want 400 invalid_input", query, rec.Code, rec.Body)
		}
	}
}

func TestBoards_FriendsFilter_AHitIssuesTheFollowsQueryAndOneBoardQuery(t *testing.T) {
	t.Parallel()
	s, _, viewer := friendsFixture(t)
	testkit.AssertQueries(t, "friends board miss", func() { s.get(t, friendsPath, viewer) })
	testkit.AssertQueries(t, "friends board repeat", func() { s.get(t, friendsPath, viewer) })
}

func TestBoards_FilterAllIsByteIdenticalToTheGolden(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	at := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	g := testkit.NewIDs(9)
	handle := "ada"
	rows := make([]boardRow, 0, 3)
	for i := 1; i <= 3; i++ {
		bps := int64(400 - 100*i)
		rows = append(rows, boardRow{
			board: "people", rank: i, subject: g.NewV7(), bps: &bps, value: int64(1000 * i), handle: &handle,
		})
	}
	seedBoardRun(t, s.pool, at, g.NewV7(), rows)
	viewer := ids.UserIDFrom(rows[1].subject)
	want, err := os.ReadFile("testdata/people_all.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	utc, local := at.Format(time.RFC3339), at.In(time.Local).Format(time.RFC3339)
	for _, path := range []string{"/v1/leaderboards/people", "/v1/leaderboards/people?filter=all"} {
		got := strings.ReplaceAll(s.get(t, path, viewer).Body.String(), local, utc)
		if got != string(want) {
			t.Errorf("GET %s =\n%s\nwant\n%s", path, got, want)
		}
	}
}
