package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func revisions(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(t.Context(),
		`SELECT version FROM atlas_schema_revisions.atlas_schema_revisions ORDER BY version`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestRenameLegacy_movesSequenceRevisionsToTheirTimestampNamesOnce(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	want := revisions(t, pool)
	if len(want) < 2 || want[0] != "20260927140427" || want[1] != "20260927185159" {
		t.Fatalf("migrated revisions = %q, want the two legacy migrations first", want)
	}
	if _, err := pool.Exec(t.Context(), `UPDATE atlas_schema_revisions.atlas_schema_revisions
		SET version = CASE version WHEN '20260927140427' THEN '0001' ELSE '0002' END
		WHERE version IN ('20260927140427', '20260927185159')`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := renameLegacy(t.Context(), pool.Config().ConnString()); err != nil {
			t.Fatal(err)
		}
	}
	if got := revisions(t, pool); !slices.Equal(got, want) {
		t.Fatalf("revisions = %q, want %q", got, want)
	}
}

func TestRenameLegacy_leavesAnUnmigratedDatabaseAlone(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	if _, err := pool.Exec(t.Context(), `DROP SCHEMA atlas_schema_revisions CASCADE`); err != nil {
		t.Fatal(err)
	}
	if err := renameLegacy(t.Context(), pool.Config().ConnString()); err != nil {
		t.Fatalf("renameLegacy on a database with no revision table = %v, want nil", err)
	}
}

func TestRenameLegacy_reportsARevisionTableItCannotRewrite(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	if _, err := pool.Exec(t.Context(), `DROP TABLE atlas_schema_revisions.atlas_schema_revisions;
		CREATE TABLE atlas_schema_revisions.atlas_schema_revisions (id int)`); err != nil {
		t.Fatal(err)
	}
	err := renameLegacy(t.Context(), pool.Config().ConnString())
	if err == nil || !strings.HasPrefix(err.Error(), "rename legacy revisions: ") {
		t.Fatalf("renameLegacy on a foreign revision table = %v, want a rename error", err)
	}
}

func TestRenameLegacy_reportsAnUnreachableDatabase(t *testing.T) {
	t.Parallel()
	err := renameLegacy(t.Context(), "postgres://nobody@127.0.0.1:1/none?connect_timeout=1")
	if err == nil || !strings.HasPrefix(err.Error(), "connect: ") {
		t.Fatalf("renameLegacy = %v, want a connect error", err)
	}
}
