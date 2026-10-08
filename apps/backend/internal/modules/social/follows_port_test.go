package social_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (f fixture) port() app.Follows {
	deps := module.Deps{Pool: f.pool, UoW: db.New(f.pool, f.gen, f.clock), IDs: f.gen, Clock: f.clock}
	return social.New(deps, social.WithUsers(f.users)).Follows()
}

func wantCounts(t *testing.T, port social.FollowsPort, user ids.UserID, followers, following int) {
	t.Helper()
	gotFollowers, gotFollowing, err := port.Counts(t.Context(), user)
	if err != nil || gotFollowers != followers || gotFollowing != following {
		t.Fatalf("counts = %d followers, %d following, err = %v; want %d and %d",
			gotFollowers, gotFollowing, err, followers, following)
	}
}

func TestFollowsPort_Counts(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	port := f.port()
	wantCounts(t, port, f.bob, 0, 0)
	if err := f.follows(t, domain.SourceProfile); err != nil {
		t.Fatal(err)
	}
	wantCounts(t, port, f.bob, 1, 0)
	wantCounts(t, port, f.alice, 0, 1)
	if err := f.unfollows(t); err != nil {
		t.Fatal(err)
	}
	wantCounts(t, port, f.bob, 0, 0)
	if err := f.follows(t, domain.SourceProfile); err != nil {
		t.Fatal(err)
	}
	wantCounts(t, port, f.bob, 1, 0)
	f.insertFollow(t, f.banned, f.bob)
	wantCounts(t, port, f.bob, 2, 0)
}

func TestFollowsPort_FollowedByMeAndFollowingIDs(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	port := f.port()
	if err := f.follows(t, domain.SourceProfile); err != nil {
		t.Fatal(err)
	}
	if followed, err := port.FollowedByMe(t.Context(), f.alice, f.bob); err != nil || !followed {
		t.Fatalf("alice follows bob = %v, err = %v", followed, err)
	}
	if followed, err := port.FollowedByMe(t.Context(), f.bob, f.alice); err != nil || followed {
		t.Fatalf("bob follows alice = %v, err = %v", followed, err)
	}
	got, err := port.FollowingIDs(t.Context(), f.alice)
	if err != nil || len(got) != 1 || got[0] != f.bob {
		t.Fatalf("following = %v, err = %v", got, err)
	}
	if err := f.unfollows(t); err != nil {
		t.Fatal(err)
	}
	if got, err := port.FollowingIDs(t.Context(), f.alice); err != nil || len(got) != 0 {
		t.Fatalf("following after unfollow = %v, err = %v", got, err)
	}
}

func TestFollowsPort_FollowedAmong(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	port := f.port()
	if err := f.follows(t, domain.SourceProfile); err != nil {
		t.Fatal(err)
	}
	got, err := port.FollowedAmong(t.Context(), f.alice, []ids.UserID{f.bob, f.banned})
	if err != nil || len(got) != 1 || !got[f.bob] || got[f.banned] {
		t.Fatalf("followed among = %v, err = %v, want only bob", got, err)
	}
	if err := f.unfollows(t); err != nil {
		t.Fatal(err)
	}
	if got, err := port.FollowedAmong(t.Context(), f.alice, []ids.UserID{f.bob}); err != nil || len(got) != 0 {
		t.Fatalf("followed among after unfollow = %v, err = %v", got, err)
	}
	if got, err := port.FollowedAmong(t.Context(), f.alice, nil); err != nil || len(got) != 0 {
		t.Fatalf("followed among empty = %v, err = %v", got, err)
	}
}

func TestFollowsPort_failures(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	port := app.NewFollows(f.pool)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := port.Counts(ctx, f.alice); err == nil {
		t.Fatal("counts on a cancelled context succeeded")
	}
	if _, err := port.FollowedByMe(ctx, f.alice, f.bob); err == nil {
		t.Fatal("followed lookup on a cancelled context succeeded")
	}
	if _, err := port.FollowedAmong(ctx, f.alice, []ids.UserID{f.bob}); err == nil {
		t.Fatal("followed among on a cancelled context succeeded")
	}
	if _, err := port.FollowingIDs(ctx, f.alice); err == nil {
		t.Fatal("following ids on a cancelled context succeeded")
	}
}

func TestFollowCounts_isTheNarrowIdentityView(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.insertFollow(t, f.alice, f.bob)
	counts := social.New(module.Deps{Pool: f.pool}, social.WithUsers(f.users)).FollowCounts()
	followers, following, err := counts.Counts(t.Context(), f.bob)
	if err != nil || followers != 1 || following != 0 {
		t.Fatalf("counts = %d, %d, err = %v", followers, following, err)
	}
	if followed, err := counts.FollowedByMe(t.Context(), f.alice, f.bob); err != nil || !followed {
		t.Fatalf("followed = %v, err = %v", followed, err)
	}
}

func TestFollowGraph_isTheNarrowRankingView(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.insertFollow(t, f.alice, f.bob)
	graph := social.New(module.Deps{Pool: f.pool}, social.WithUsers(f.users)).FollowGraph()
	got, err := graph.FollowingIDs(t.Context(), f.alice)
	if err != nil || len(got) != 1 || got[0] != f.bob {
		t.Fatalf("following = %v, err = %v", got, err)
	}
}

func seedFollowers(tb testing.TB, pool *pgxpool.Pool, target ids.UserID, count int) {
	tb.Helper()
	_, err := pool.Exec(tb.Context(), `INSERT INTO follows (id, follower_id, followee_id, created_at)
		SELECT gen_random_uuid(), gen_random_uuid(), $1, now() - n * interval '1 second'
		FROM generate_series(1, $2::int) AS n`, target.UUID(), count)
	if err != nil {
		tb.Fatal(err)
	}
	if _, err := pool.Exec(tb.Context(), `VACUUM (ANALYZE) follows`); err != nil {
		tb.Fatal(err)
	}
}

func TestFollowsPort_countsUseAnIndexOnlyScan(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	seedFollowers(t, f.pool, f.alice, 2000)
	conn, err := f.pool.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err := conn.Exec(t.Context(), `SET enable_seqscan = off; SET enable_bitmapscan = off`); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = conn.Exec(t.Context(), `RESET enable_seqscan; RESET enable_bitmapscan`) }()
	rows, err := conn.Query(t.Context(), `EXPLAIN SELECT
  (SELECT count(*) FROM follows WHERE followee_id = $1::uuid AND deleted_at IS NULL)::int,
  (SELECT count(*) FROM follows WHERE follower_id = $1::uuid AND deleted_at IS NULL)::int`, f.alice.UUID())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(line + "\n")
	}
	if err := rows.Err(); err != nil || strings.Count(plan.String(), "Index Only Scan") != 2 {
		t.Fatalf("plan = %q, %v; want two index only scans", plan.String(), err)
	}
}

func BenchmarkCountFollowers(b *testing.B) {
	pool := testkit.DB(b)
	target := ids.NewUserID(testkit.NewIDs(587))
	seedFollowers(b, pool, target, 100_000)
	port := app.NewFollows(pool)
	b.ResetTimer()
	for b.Loop() {
		if followers, _, err := port.Counts(b.Context(), target); err != nil || followers != 100_000 {
			b.Fatalf("followers = %d, err = %v", followers, err)
		}
	}
}
