package ranking_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const leaderboardEntriesForSubject = `
SELECT range, value_micros
FROM leaderboard_entries
WHERE subject_id = $1
ORDER BY range
`

func TestRankingTables_roundTrip(t *testing.T) {
	t.Parallel()
	db := testkit.DB(t)
	q := sqlc.New(db)
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	cabal := ids.CabalIDFrom(ids.Real{}.NewV7())
	run := ids.Real{}.NewV7()
	insertRankingTrigger(t, q, cabal, now)
	replaceRankingEntries(t, q, cabal, now, 200, "ALL")
	replaceRankingEntries(t, q, cabal, now, 99, "DAY")
	replaceRankingEntries(t, q, cabal, now, 300, "ALL")
	assertReplacedRankingEntries(t, db, cabal, 300)
	insertCabalValue(t, q, cabal, now)
	insertLeaderboardRun(t, q, run, now)
	assertLatestRun(t, q, run)
	assertLatestCabalValue(t, q, cabal)
	deleteRankingTrigger(t, q, now)
}

func insertRankingTrigger(t *testing.T, q *sqlc.Queries, cabal ids.CabalID, now time.Time) {
	t.Helper()
	if err := q.InsertRankingTrigger(t.Context(), sqlc.InsertRankingTriggerParams{
		CabalID: cabal.UUID(), Reason: "cabal.funded", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	oldest, err := q.OldestRankingTrigger(t.Context())
	if err != nil || !oldest.Equal(now) {
		t.Fatalf("OldestRankingTrigger = %v, %v, want %v", oldest, err, now)
	}
}

func replaceRankingEntries(
	t *testing.T, q *sqlc.Queries, cabal ids.CabalID, now time.Time, value int64, boardRange string,
) {
	t.Helper()
	rows, err := json.Marshal([]map[string]any{{
		"board": "cabals", "range": boardRange, "rank": 1, "subject_id": cabal.String(), "subject_name": "Cabal",
		"subject_created_at": now.Format(time.RFC3339Nano), "value_micros": value, "pnl_micros": 10,
		"return_bps": 500, "prices_as_of": now.Format(time.RFC3339Nano), "computed_at": now.Format(time.RFC3339Nano),
		"flags": []string{"stale_prices"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if boardRange == "ALL" {
		if err := q.DeleteAllLeaderboardEntries(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if err := q.InsertLeaderboardEntries(t.Context(), rows); err != nil {
		t.Fatal(err)
	}
}

func assertReplacedRankingEntries(t *testing.T, db *pgxpool.Pool, cabal ids.CabalID, allValue int64) {
	t.Helper()
	rows, err := db.Query(t.Context(), leaderboardEntriesForSubject, cabal.UUID())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	values := map[string]int64{}
	for rows.Next() {
		var boardRange string
		var value int64
		if err := rows.Scan(&boardRange, &value); err != nil {
			t.Fatal(err)
		}
		values[boardRange] = value
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values["ALL"] != allValue {
		t.Fatalf("leaderboard entries = %#v, want only ALL=%d after a replacement drops every range", values, allValue)
	}
}

func insertCabalValue(t *testing.T, q *sqlc.Queries, cabal ids.CabalID, now time.Time) {
	t.Helper()
	if err := q.InsertCabalValueSnapshot(t.Context(), sqlc.InsertCabalValueSnapshotParams{
		CabalID: cabal.UUID(), At: now, ValueMicros: 200, NavPerShareMicros: 2, TotalShares: 100,
	}); err != nil {
		t.Fatal(err)
	}
}

func insertLeaderboardRun(t *testing.T, q *sqlc.Queries, run uuid.UUID, now time.Time) {
	t.Helper()
	if err := q.InsertLeaderboardRun(t.Context(), sqlc.InsertLeaderboardRunParams{
		RunID: run, AsOf: now, PricesAsOf: now, StartedAt: now, FinishedAt: now, RowsWritten: 1,
	}); err != nil {
		t.Fatal(err)
	}
}

func assertLatestRun(t *testing.T, q *sqlc.Queries, run uuid.UUID) {
	t.Helper()
	latest, err := q.LastLeaderboardRun(t.Context())
	if err != nil || latest.RunID != run || latest.RowsWritten != 1 {
		t.Fatalf("LastLeaderboardRun = %+v, %v", latest, err)
	}
}

func assertLatestCabalValue(t *testing.T, q *sqlc.Queries, cabal ids.CabalID) {
	t.Helper()
	values, err := q.LatestCabalValues(t.Context())
	if err != nil || len(values) != 1 || values[0].CabalID != cabal.UUID() || values[0].ValueMicros != 200 {
		t.Fatalf("LatestCabalValues = %+v, %v", values, err)
	}
}

func deleteRankingTrigger(t *testing.T, q *sqlc.Queries, now time.Time) {
	t.Helper()
	if err := q.DeleteRankingTriggersThrough(t.Context(), now); err != nil {
		t.Fatal(err)
	}
	if _, err := q.OldestRankingTrigger(t.Context()); err == nil {
		t.Fatal("OldestRankingTrigger succeeded after deletion")
	}
}
