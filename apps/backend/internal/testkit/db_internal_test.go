package testkit

import (
	"context"
	"crypto/rand"
	"errors"
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
	"github.com/jackc/pgx/v5/pgconn"

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
	name := databaseName("t_", "stale")
	ctx := context.Background()
	if _, err := s.admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := drop(ctx, s.admin, name); err != nil {
			t.Error(err)
		}
	})
	dropped, err := s.dropStale(ctx, time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), name)
	if err != nil || len(dropped) != 0 || !exists(t, s, name) {
		t.Fatalf("hour-old cutoff dropped %v, %v; the new database must stay", dropped, err)
	}
	dropped, err = s.dropStale(ctx, time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC), name)
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
	dropped, err := s.dropStale(context.Background(), time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC), stale, own)
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
	dropped, err := s.dropStale(ctx, time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC), held)
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
	dropped, err = s.dropStale(ctx, time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC), held)
	if err != nil || !slices.Equal(dropped, []string{held}) || exists(t, s, held) {
		t.Fatalf("after release dropStale dropped %v, %v; want %s gone", dropped, err, held)
	}
}

func TestDropStaleSkipsADatabaseWithOpenConnections(t *testing.T) {
	t.Parallel()
	s := serverWithOwnMigrations(t)
	ctx := context.Background()
	name := databaseName("t_", "open")
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
	dropped, err := s.dropStale(ctx, time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC), name)
	if err != nil || len(dropped) != 0 || !exists(t, s, name) {
		t.Fatalf("dropStale dropped %v, %v while a connection was open to %s", dropped, err, name)
	}
}

func runFixture(t *testing.T, run string) (string, error) {
	t.Helper()
	if testing.Short() {
		t.Skip("builds and runs another test binary; CI runs it without -short, outside the 10 s package budget")
	}
	cmd := exec.CommandContext(t.Context(), "go", "test", "-count=1", "-v", "-run", run, "./testdata/failing")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestMainWithNoDB_runsSetupThenTestsThenCleanupWithoutPostgres(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds and runs another test binary")
	}
	run := func(env ...string) (string, error) {
		cmd := exec.CommandContext(t.Context(), "go", "test", "-count=1", "-v", "./testdata/nodb")
		cmd.Env = append(os.Environ(), append(env, "TEST_DATABASE_URL=postgres://nowhere:1/none")...)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	out, err := run()
	order := regexp.MustCompile(`fixture: (\w+)`).FindAllStringSubmatch(out, -1)
	if err != nil || len(order) != 3 || order[0][1] != "setup" || order[1][1] != "test" || order[2][1] != "cleanup" {
		t.Fatalf("nodb fixture: err %v, want setup, test, cleanup in order:\n%s", err, out)
	}
	out, err = run("NODB_FAIL_SETUP=1")
	if err == nil || !strings.Contains(out, "testkit.Main: setup refused") || strings.Contains(out, "fixture: test") {
		t.Fatalf("failing setup: err %v, want exit before any test with the setup error:\n%s", err, out)
	}
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

func TestAtlasMigratorHash_changesWhenAMigrationIsRenamed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "0001_a.sql"), []byte("SELECT 1;"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := atlasMigrator{dir: dir}.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(dir, "0001_a.sql"), filepath.Join(dir, "20260101000000_a.sql")); err != nil {
		t.Fatal(err)
	}
	after, err := atlasMigrator{dir: dir}.Hash()
	if err != nil || after == before {
		t.Fatalf("hash after rename = %q, %v, want it to differ from %q", after, err, before)
	}
}

type countExec struct{ n int }

func (c *countExec) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	c.n++
	return pgconn.CommandTag{}, nil
}

func TestDropOwnedPanicsOutsideTheRunPrefix(t *testing.T) {
	t.Parallel()
	s := &server{runPrefix: "t_thisrun_"}
	execs := &countExec{}
	defer func() {
		msg := fmt.Sprint(recover())
		if !strings.Contains(msg, "DROP DATABASE monaco") ||
			!strings.Contains(msg, `run prefix "t_thisrun_"`) || execs.n != 0 {
			t.Fatalf("recovered %q after %d execs, want a cross-prefix panic and no DROP", msg, execs.n)
		}
	}()
	_ = s.dropOwned(context.Background(), execs, "monaco")
	t.Fatal("dropOwned returned, want a panic")
}

func createOwned(t *testing.T, s *server, name string) {
	t.Helper()
	ctx := context.Background()
	if _, err := s.admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if exists(t, s, name) {
			if err := drop(context.Background(), s.admin, name); err != nil {
				t.Error(err)
			}
		}
	})
}

func TestCleanKeptLeavesDatabasesAtHalfDisk(t *testing.T) {
	t.Parallel()
	prefix := "t_half_" + strings.ToLower(rand.Text()[:8]) + "_"
	s := &server{admin: current.Load().admin, runPrefix: prefix}
	name := prefix + "kept"
	createOwned(t, s, name)
	s.disk = func(context.Context) (int64, int64, error) { return 50, 100, nil }
	dropped, err := s.cleanKept(context.Background())
	if err != nil || len(dropped) != 0 || !exists(t, s, name) {
		t.Fatalf("at 50%% cleanKept dropped %v, %v; %s must stay", dropped, err, name)
	}
}

func TestCleanKeptReportsDiskErrorsWithoutDropping(t *testing.T) {
	t.Parallel()
	prefix := "t_derr_" + strings.ToLower(rand.Text()[:8]) + "_"
	s := &server{admin: current.Load().admin, runPrefix: prefix}
	name := prefix + "kept"
	createOwned(t, s, name)
	s.disk = func(context.Context) (int64, int64, error) { return 0, 0, errors.New("statfs failed") }
	_, err := s.cleanKept(context.Background())
	if err == nil || !strings.Contains(err.Error(), "statfs failed") || !exists(t, s, name) {
		t.Fatalf("disk error = %v, exists %v; want the error and %s kept", err, exists(t, s, name), name)
	}
	s.disk = func(context.Context) (int64, int64, error) { return 1, 0, nil }
	_, err = s.cleanKept(context.Background())
	if err == nil || !strings.Contains(err.Error(), prefix) || !exists(t, s, name) {
		t.Fatalf("zero total = %v; want an error naming %s and the database kept", err, prefix)
	}
}

func TestCleanKeptDropsUnusedDatabasesPastHalfAndLeavesAHeldOne(t *testing.T) {
	t.Parallel()
	prefix := "t_fullc_" + strings.ToLower(rand.Text()[:8]) + "_"
	s := &server{admin: current.Load().admin, runPrefix: prefix}
	name, held := prefix+"kept", prefix+"held"
	createOwned(t, s, name)
	createOwned(t, s, held)
	ctx := context.Background()
	cfg := s.admin.Config().ConnConfig.Copy()
	cfg.Database = held
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	s.disk = func(context.Context) (int64, int64, error) { return 51, 100, nil }
	dropped, err := s.cleanKept(ctx)
	if err != nil || !slices.Equal(dropped, []string{name}) || exists(t, s, name) || !exists(t, s, held) {
		t.Fatalf("past half, cleanKept dropped %v, %v; want only %s gone and %s held", dropped, err, name, held)
	}
}

func TestCleanKeptPanicsWhenTheRunPrefixIsEmpty(t *testing.T) {
	t.Parallel()
	s := &server{disk: func(context.Context) (int64, int64, error) { return 51, 100, nil }}
	defer func() {
		msg := fmt.Sprint(recover())
		if !strings.Contains(msg, `empty run prefix ""`) {
			t.Fatalf("recovered %q, want the empty-prefix panic", msg)
		}
	}()
	_, _ = s.cleanKept(context.Background())
	t.Fatal("cleanKept returned, want a panic")
}

func TestReleaseDBKeepsAFailureUntilDiskPassesHalf(t *testing.T) {
	t.Parallel()
	admin := current.Load().admin
	prefix := "t_rel_" + strings.ToLower(rand.Text()[:8]) + "_"
	s := &server{admin: admin, runPrefix: prefix, disk: func(context.Context) (int64, int64, error) {
		return 1, 100, nil
	}}
	ctx := context.Background()
	name := prefix + "db"
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if exists(t, s, name) {
			if err := drop(context.Background(), admin, name); err != nil {
				t.Error(err)
			}
		}
	})
	kept, err := s.releaseDB(ctx, name, true)
	if err != nil || !kept || !exists(t, s, name) {
		t.Fatalf("releaseDB kept %v, %v; database exists %v", kept, err, exists(t, s, name))
	}
	s.kept.Store(keepFailed)
	kept, err = s.releaseDB(ctx, name, true)
	if err != nil || kept || exists(t, s, name) {
		t.Fatalf("after %d keeps, releaseDB kept %v, %v; exists %v", keepFailed, kept, err, exists(t, s, name))
	}
}

func TestReleaseDBDropsAPassingTest(t *testing.T) {
	t.Parallel()
	prefix := "t_pass_" + strings.ToLower(rand.Text()[:8]) + "_"
	s := &server{admin: current.Load().admin, runPrefix: prefix, disk: func(context.Context) (int64, int64, error) {
		return 1, 100, nil
	}}
	name := prefix + "db"
	createOwned(t, s, name)
	kept, err := s.releaseDB(context.Background(), name, false)
	if err != nil || kept || exists(t, s, name) {
		t.Fatalf("passing releaseDB kept %v, %v; exists %v, want a drop", kept, err, exists(t, s, name))
	}
}

func TestReleaseDBDropsWhenDiskPassesHalf(t *testing.T) {
	t.Parallel()
	admin := current.Load().admin
	prefix := "t_full_" + strings.ToLower(rand.Text()[:8]) + "_"
	s := &server{admin: admin, runPrefix: prefix, disk: func(context.Context) (int64, int64, error) {
		return 80, 100, nil
	}}
	ctx := context.Background()
	name := prefix + "db"
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if exists(t, s, name) {
			if err := drop(context.Background(), admin, name); err != nil {
				t.Error(err)
			}
		}
	})
	kept, err := s.releaseDB(ctx, name, true)
	if err != nil || kept || exists(t, s, name) {
		t.Fatalf("full disk releaseDB kept %v, %v; exists %v, want the database dropped", kept, err, exists(t, s, name))
	}
}

func TestDiskOverNilUsageIsNotFull(t *testing.T) {
	t.Parallel()
	over, err := (&server{}).diskOver(context.Background())
	if err != nil || over {
		t.Fatalf("nil disk usage = %v, %v, want not full", over, err)
	}
}

func TestAtlasMigratorHash_reportsAnUnreadableDirectoryOrMigration(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "0001_dir.sql"), 0o750); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{dir, filepath.Join(dir, "missing")} {
		if _, err := (atlasMigrator{dir: d}).Hash(); err == nil {
			t.Fatalf("Hash(%s) = nil error, want one", d)
		}
	}
}
