package testkit_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func countRows(t *testing.T, query string, pool *pgxpool.Pool, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSeed_mapsThePlaceholdersToTheUsersTheTestMade(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	a, b := testkit.SeedUser(t, pool, testkit.UserOpts{}), testkit.SeedUser(t, pool, testkit.UserOpts{})
	seeded := testkit.Seed(t, pool, "two-cabals-ranked")
	if len(seeded) != 9 {
		t.Fatalf("seeded %d events, want 9", len(seeded))
	}
	if n := countRows(t, `SELECT count(*) FROM events WHERE payload::text LIKE '%00000000-0000-0000-0000-0000000000a%'
		OR actor_id LIKE '00000000-%'`, pool); n != 0 {
		t.Fatalf("%d events still name a placeholder", n)
	}
	if n := countRows(t, `SELECT count(*) FROM cabals WHERE creator_id = ANY($1)`, pool,
		[]uuid.UUID{a.ID.UUID(), b.ID.UUID()}); n != 2 {
		t.Fatalf("cabals created by the two users = %d, want 2", n)
	}
	if n := countRows(t, `SELECT count(*) FROM cabal_members WHERE user_id = ANY($1)`, pool,
		[]uuid.UUID{a.ID.UUID(), b.ID.UUID()}); n != 3 {
		t.Fatalf("memberships = %d, want 3", n)
	}
	if n := countRows(t, `SELECT count(*) FROM users`, pool); n != 2 {
		t.Fatalf("users = %d, want the two the test made", n)
	}
	if again := testkit.Seed(t, pool, "two-cabals-ranked"); len(again) != 0 {
		t.Fatalf("a second Seed applied %d events, want none", len(again))
	}
}

func TestSeedEvents_wantsAUserPerPlaceholder(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	raw, err := testkit.Scenario("two-cabals-ranked")
	if err != nil {
		t.Fatal(err)
	}
	if got := testkit.WantUsers(raw); got != 2 {
		t.Fatalf("WantUsers = %d, want 2", got)
	}
	one := testkit.SeedUser(t, pool, testkit.UserOpts{}).ID
	_, err = testkit.SeedEvents(t.Context(), pool, "two-cabals-ranked", raw, []ids.UserID{one})
	if !errors.Is(err, testkit.ErrSeed) || !strings.Contains(err.Error(), "want users: 2, have 1") {
		t.Fatalf("SeedEvents with one user = %v, want a seed error naming the two users it wants", err)
	}
}
