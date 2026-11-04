package adapters_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestCabalsOf_leavesOutABannedCabalWithNoShare(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{})
	active := testkit.NewCabal(t, pool, testkit.WithJoiner(user.ID))
	bannedNoShare := testkit.NewCabal(t, pool, testkit.WithJoiner(user.ID))
	bannedWithShare := testkit.NewCabal(t, pool, testkit.WithJoiner(user.ID))
	for _, c := range []testkit.SeededCabal{bannedNoShare, bannedWithShare} {
		if _, err := pool.Exec(
			t.Context(), `UPDATE cabals SET status = 'banned' WHERE id = $1`, c.ID.UUID(),
		); err != nil {
			t.Fatalf("ban cabal: %v", err)
		}
	}
	if _, err := pool.Exec(t.Context(), `INSERT INTO user_positions
		(user_id, cabal_id, share_units, contributed_micros, withdrawn_micros, updated_at)
		VALUES ($1, $2, 5, 5, 0, $3)`,
		user.ID.UUID(), bannedWithShare.ID.UUID(), clock.Real{}.Now().UTC(),
	); err != nil {
		t.Fatalf("seed position: %v", err)
	}
	got, err := adapters.NewQueries(pool).CabalsOf(t.Context(), user.ID)
	if err != nil {
		t.Fatalf("CabalsOf: %v", err)
	}
	want := []ids.CabalID{active.ID, bannedWithShare.ID}
	slices.SortFunc(got, func(a, b ids.CabalID) int { return strings.Compare(a.String(), b.String()) })
	slices.SortFunc(want, func(a, b ids.CabalID) int { return strings.Compare(a.String(), b.String()) })
	if !slices.Equal(got, want) {
		t.Fatalf("CabalsOf = %v, want the active cabal and the banned one the user holds shares in: %v", got, want)
	}
}
