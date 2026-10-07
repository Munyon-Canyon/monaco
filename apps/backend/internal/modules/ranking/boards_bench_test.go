package ranking_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func BenchmarkBoards_peoplePage(b *testing.B) {
	pool := testkit.DB(b)
	seedBoard(b, pool, clock.Real{}.Now().UTC(), boardRows("people", 10000))
	vacuumBoard(b, pool)
	q := sqlc.New(pool)
	params := sqlc.BoardPageParams{Board: "people", Range: "ALL", AfterRank: 1000, RowLimit: 21}
	b.ResetTimer()
	for b.Loop() {
		rows, err := q.BoardPage(b.Context(), params)
		if err != nil || len(rows) != 21 {
			b.Fatalf("BoardPage = %d rows, %v, want 21", len(rows), err)
		}
	}
}
