package testkit

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
		if err := drop(ctx, s.admin, name); err != nil {
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

func createTemplate(t *testing.T, s *server, name string) {
	t.Helper()
	ctx := context.Background()
	for _, stmt := range []string{"CREATE DATABASE %s", "ALTER DATABASE %s IS_TEMPLATE true"} {
		if _, err := s.admin.Exec(ctx, fmt.Sprintf(stmt, pgx.Identifier{name}.Sanitize())); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		if err := drop(ctx, s.admin, name); err != nil {
			t.Error(err)
		}
	})
}

func uniqueTemplateName() string {
	return "testdb_tpl_testkitfixture_" + strings.ToLower(rand.Text())
}

func serverWithOwnMigrations(t *testing.T) *server {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "0001.sql"), []byte("-- "+rand.Text()), 0o600); err != nil {
		t.Fatal(err)
	}
	return &server{admin: current.Load().admin, migrator: atlasMigrator{dir: dir}}
}

func TestDropStaleDropsOldTemplatesButNotTheCurrentOne(t *testing.T) {
	t.Parallel()
	s := serverWithOwnMigrations(t)
	own, err := s.currentTemplate()
	if err != nil {
		t.Fatal(err)
	}
	stale := uniqueTemplateName()
	createTemplate(t, s, own)
	createTemplate(t, s, stale)
	dropped, err := s.dropStale(context.Background(), time.Now().Add(time.Hour), stale, own)
	if err != nil || !slices.Equal(dropped, []string{stale}) || exists(t, s, stale) || !exists(t, s, own) {
		t.Fatalf("dropStale dropped %v, %v; want only %s gone and %s kept", dropped, err, stale, own)
	}
}

func TestDropStaleSkipsATemplateAnotherRunHolds(t *testing.T) {
	t.Parallel()
	s := serverWithOwnMigrations(t)
	held := uniqueTemplateName()
	createTemplate(t, s, held)
	other := &server{admin: s.admin}
	ctx := context.Background()
	if err := other.holdTemplate(ctx, held); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.holder.Close(ctx) }()
	dropped, err := s.dropStale(ctx, time.Now().Add(time.Hour), held)
	if err != nil || len(dropped) != 0 || !exists(t, s, held) {
		t.Fatalf("dropStale dropped %v, %v while another run held %s", dropped, err, held)
	}
	if _, err := other.holder.Exec(
		ctx,
		`SELECT pg_advisory_unlock_shared($1::int4, hashtext($2))`,
		lockSpace,
		held,
	); err != nil {
		t.Fatal(err)
	}
	dropped, err = s.dropStale(ctx, time.Now().Add(time.Hour), held)
	if err != nil || !slices.Equal(dropped, []string{held}) || exists(t, s, held) {
		t.Fatalf("after release dropStale dropped %v, %v; want %s gone", dropped, err, held)
	}
}

func TestDropStaleSkipsADatabaseWithOpenConnections(t *testing.T) {
	t.Parallel()
	s := serverWithOwnMigrations(t)
	ctx := context.Background()
	name := databaseName("open")
	if _, err := s.admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := drop(ctx, s.admin, name); err != nil {
			t.Error(err)
		}
	})
	cfg := s.admin.Config().ConnConfig.Copy()
	cfg.Database = name
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	dropped, err := s.dropStale(ctx, time.Now().Add(time.Hour), name)
	if err != nil || len(dropped) != 0 || !exists(t, s, name) {
		t.Fatalf("dropStale dropped %v, %v while a connection was open to %s", dropped, err, name)
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
			if err := drop(context.Background(), s.admin, m[1]); err != nil {
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
