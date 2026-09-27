package testkit

import (
	"context"
	"crypto/rand"
	"fmt"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

func TestParseTestURLRefusesAnyPortButTheTestContainer(t *testing.T) {
	t.Parallel()
	if _, err := parseTestURL(config.DefaultTestDBURL); err != nil {
		t.Fatalf("default test URL rejected: %v", err)
	}
	for _, raw := range []string{
		"postgres://monaco:secret@localhost:54322/monaco?sslmode=disable",
		"postgres://monaco:secret@localhost/monaco",
		"postgres://monaco:secret@db.example.supabase.co:5432/postgres",
	} {
		_, err := parseTestURL(raw)
		if err == nil || !strings.Contains(err.Error(), "not monaco-postgres-test on port 54323") {
			t.Fatalf("parseTestURL(%q) = %v, want a refusal", raw, err)
		}
		if strings.Contains(err.Error(), "secret") {
			t.Fatalf("refusal leaks the password: %v", err)
		}
	}
}

func exists(t *testing.T, s *server, name string) bool {
	t.Helper()
	var ok bool
	err := s.admin.QueryRow(context.Background(),
		"SELECT EXISTS (SELECT FROM pg_database WHERE datname = $1)", name).Scan(&ok)
	if err != nil {
		t.Fatal(err)
	}
	return ok
}

func TestDropStaleDropsOnlyDatabasesOlderThanTheCutoff(t *testing.T) {
	t.Parallel()
	s := current.Load()
	name := databaseName("stale")
	ctx := context.Background()
	if _, err := s.admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.drop(ctx, name); err != nil {
			t.Error(err)
		}
	})
	now := time.Now()
	dropped, err := s.dropStale(ctx, now.Add(-time.Hour), name)
	if err != nil || len(dropped) != 0 || !exists(t, s, name) {
		t.Fatalf("hour-old cutoff dropped %v, %v; the new database must stay", dropped, err)
	}
	dropped, err = s.dropStale(ctx, now.Add(time.Hour), name)
	if err != nil || !slices.Equal(dropped, []string{name}) || exists(t, s, name) {
		t.Fatalf("future cutoff dropped %v, %v; want only %s gone", dropped, err, name)
	}
}

func TestDropStaleDropsOldTemplatesButNotTheCurrentOne(t *testing.T) {
	t.Parallel()
	s := current.Load()
	DB(t)
	ctx := context.Background()
	stale := "testdb_tpl_" + strings.ToLower(rand.Text())
	for _, stmt := range []string{"CREATE DATABASE %s", "ALTER DATABASE %s IS_TEMPLATE true"} {
		if _, err := s.admin.Exec(ctx, fmt.Sprintf(stmt, pgx.Identifier{stale}.Sanitize())); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		if err := s.drop(ctx, stale); err != nil {
			t.Error(err)
		}
	})
	if got, err := s.currentTemplate(); err != nil || got != s.template.Database {
		t.Fatalf("currentTemplate() = %q, %v; pgtestdb built %q", got, err, s.template.Database)
	}
	dropped, err := s.dropStale(ctx, time.Now().Add(time.Hour), stale, s.template.Database)
	if err != nil || !slices.Equal(dropped, []string{stale}) {
		t.Fatalf("future cutoff dropped %v, %v; want only %s", dropped, err, stale)
	}
	if exists(t, s, stale) || !exists(t, s, s.template.Database) {
		t.Fatalf("after dropStale: stale template exists %v, current template exists %v",
			exists(t, s, stale), exists(t, s, s.template.Database))
	}
}

func runFixture(t *testing.T, run string) (string, error) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "go", "test", "-count=1", "-v", "-run", run, "./testdata/failing")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestMainFailsAPackageThatLeaksAGoroutine(t *testing.T) {
	t.Parallel()
	out, err := runFixture(t, "^TestLeaksAGoroutine$")
	if err == nil || !strings.Contains(out, "--- PASS: TestLeaksAGoroutine") ||
		!strings.Contains(out, "found unexpected goroutines") {
		t.Fatalf("leaking fixture: err %v\n%s", err, out)
	}
}

func TestDBKeepsOnlyTheFirstFiveFailedDatabases(t *testing.T) {
	t.Parallel()
	s := current.Load()
	out, err := runFixture(t, "^TestFail[1-6]$")
	if err == nil {
		t.Fatalf("failing fixture passed:\n%s", out)
	}
	used := regexp.MustCompile(`database=(t_\w+)`).FindAllStringSubmatch(out, -1)
	kept := regexp.MustCompile(`testkit: kept (t_\w+)`).FindAllStringSubmatch(out, -1)
	t.Cleanup(func() {
		for _, m := range kept {
			if err := s.drop(context.Background(), m[1]); err != nil {
				t.Error(err)
			}
		}
	})
	if len(used) != 6 || len(kept) != 5 {
		t.Fatalf("fixture used %d databases and kept %d, want 6 and 5:\n%s", len(used), len(kept), out)
	}
	for i, m := range used {
		if want := i < 5; exists(t, s, m[1]) != want {
			t.Fatalf("failed database %d %s exists = %v, want %v", i+1, m[1], !want, want)
		}
	}
}
