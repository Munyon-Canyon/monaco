package db_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/migrations"
)

func dbConfig(pool *pgxpool.Pool) config.DB {
	return config.DB{URL: pool.Config().ConnString(), MaxConns: 2}
}

func codedError(t *testing.T, err error, code errs.Code) []slog.Attr {
	t.Helper()
	var coded *errs.Error
	if !errors.As(err, &coded) || coded.Code != code {
		t.Fatalf("err = %v, want code %s", err, code)
	}
	return coded.Attrs
}

func attr(attrs []slog.Attr, key string) string {
	for _, a := range attrs {
		if a.Key == key {
			return a.Value.String()
		}
	}
	return "<missing>"
}

func TestOpen_returnsAPingedPoolSizedFromConfig(t *testing.T) {
	t.Parallel()
	pool, err := db.Open(t.Context(), dbConfig(testkit.DB(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var one int
	if err := pool.QueryRow(t.Context(), `SELECT 1`).Scan(&one); err != nil || one != 1 {
		t.Fatalf("SELECT 1 = %d, %v", one, err)
	}
	if pool.Config().MaxConns != 2 {
		t.Fatalf("MaxConns = %d, want 2", pool.Config().MaxConns)
	}
}

func TestOpen_acceptsADatabaseAheadOfTheBinary(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO atlas_schema_revisions.atlas_schema_revisions
		  (version, description, type, applied, total, executed_at, execution_time, hash, operator_version)
		VALUES ('9999', 'future', 2, 1, 1, now(), 0, 'h', 'test')`); err != nil {
		t.Fatal(err)
	}
	opened, err := db.Open(t.Context(), dbConfig(pool))
	if err != nil {
		t.Fatal(err)
	}
	opened.Close()
}

func TestOpen_failsWhenTheDatabaseIsBehindTheBinary(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	if _, err := pool.Exec(t.Context(), `DELETE FROM atlas_schema_revisions.atlas_schema_revisions`); err != nil {
		t.Fatal(err)
	}
	want := migrations.Latest()
	opened, err := db.Open(t.Context(), dbConfig(pool))
	if opened != nil {
		t.Fatal("Open returned a pool for a database behind the binary")
	}
	attrs := codedError(t, err, errs.CodeInternal)
	if have, got := attr(attrs, "have"), attr(attrs, "want"); have != "" || got != want {
		t.Fatalf("attrs have=%q want=%q, expected have=\"\" want=%q", have, got, want)
	}
}

func TestOpen_failsWhenTheRevisionTableIsMissing(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	if _, err := pool.Exec(t.Context(), `DROP SCHEMA atlas_schema_revisions CASCADE`); err != nil {
		t.Fatal(err)
	}
	_, err := db.Open(t.Context(), dbConfig(pool))
	codedError(t, err, errs.CodeInternal)
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != "42P01" {
		t.Fatalf("err = %v, want to wrap undefined_table", err)
	}
}

func TestOpen_rejectsBadConfig(t *testing.T) {
	t.Parallel()
	ok := testkit.DB(t).Config().ConnString()
	for name, cfg := range map[string]config.DB{
		"unparsable url": {URL: "postgres://user:pass@host:notaport/db", MaxConns: 2},
		"zero pool size": {URL: ok, MaxConns: 0},
		"port 1 refuses": {URL: "postgres://monaco@127.0.0.1:1/monaco?sslmode=disable&connect_timeout=2", MaxConns: 2},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pool, err := db.Open(t.Context(), cfg)
			if pool != nil {
				t.Fatal("Open returned a pool")
			}
			want := errs.CodeInternal
			if name == "port 1 refuses" {
				want = errs.CodeDBUnavailable
			}
			codedError(t, err, want)
		})
	}
}

func TestOpen_stopsAtACancelledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := db.Open(ctx, dbConfig(testkit.DB(t)))
	codedError(t, err, errs.CodeDBUnavailable)
}
