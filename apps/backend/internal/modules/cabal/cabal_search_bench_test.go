package cabal_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func BenchmarkSearchCabals_tenThousand(b *testing.B) {
	pool := testkit.DB(b)
	user := testkit.SeedUser(b, pool, testkit.UserOpts{})
	seedSearchPerfCabals(b, pool, user.ID.UUID())
	q := sqlc.New(pool)
	params := sqlc.SearchCabalsParams{ActorID: user.ID.UUID(), Query: "abc", PageSize: 21}
	b.ResetTimer()
	for b.Loop() {
		rows, err := q.SearchCabals(b.Context(), params)
		if err != nil || len(rows) != 14 {
			b.Fatalf("SearchCabals = %d rows, %v, want 14", len(rows), err)
		}
	}
}
