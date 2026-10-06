package sqlc_test

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	seededMessages = 10_000
	seenIndex      = "chat_seen_cabal_last_seen_idx"
)

func seededChat(t *testing.T) (*pgxpool.Pool, testkit.SeededCabal, time.Time) {
	t.Helper()
	pool := testkit.DB(t)
	cabal := testkit.NewCabal(t, pool, testkit.WithMembers(3))
	now := clock.Real{}.Now().UTC()
	if _, err := pool.Exec(t.Context(), `INSERT INTO cabal_messages (id, cabal_id, author_id, body, created_at)
		SELECT gen_random_uuid(), $1, $2, 'gm', $3::timestamptz - n * interval '1 second'
		FROM generate_series(1, $4::int) AS n`,
		cabal.ID.UUID(), cabal.Members[0].ID.UUID(), now, seededMessages); err != nil {
		t.Fatal(err)
	}
	for _, m := range cabal.Members {
		if _, err := pool.Exec(t.Context(),
			`INSERT INTO chat_seen (cabal_id, user_id, last_seen_at) VALUES ($1, $2, $3)`,
			cabal.ID.UUID(), m.ID.UUID(), now); err != nil {
			t.Fatal(err)
		}
	}
	for _, table := range []string{"cabal_messages", "chat_seen"} {
		if _, err := pool.Exec(t.Context(), `VACUUM (ANALYZE) `+table); err != nil {
			t.Fatal(err)
		}
	}
	return pool, cabal, now
}

func plan(t *testing.T, pool *pgxpool.Pool, query string, args ...any) string {
	t.Helper()
	conn, err := pool.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err := conn.Exec(t.Context(), `SET enable_seqscan = off`); err != nil {
		t.Fatal(err)
	}
	rows, err := conn.Query(t.Context(), "EXPLAIN "+query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		out.WriteString(line + "\n")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestSeenQueries_useIndexesOnTenThousandMessages(t *testing.T) {
	t.Parallel()
	pool, cabal, now := seededChat(t)
	tests := []struct {
		name, query string
		args        []any
	}{
		{"newest seen count", sqlc.NewestSeenCountSQL, []any{cabal.ID.UUID()}},
		{"seen by list", sqlc.ListSeenBySQL, []any{cabal.ID.UUID(), cabal.Members[0].ID.UUID(), now}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := plan(t, pool, tt.query, tt.args...)
			if strings.Contains(got, "Seq Scan") || !strings.Contains(got, seenIndex) {
				t.Fatalf("plan =\n%s\nwant %s and no sequential scan", got, seenIndex)
			}
		})
	}
}
