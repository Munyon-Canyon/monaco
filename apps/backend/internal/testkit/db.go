package testkit

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/peterldowns/pgtestdb"
	"github.com/peterldowns/pgtestdb/migrators/common"
	"go.uber.org/goleak"

	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

const (
	testPort       = "54323"
	keepFailed     = 5
	staleAfter     = time.Hour
	maxNameLen     = 63
	poolMaxConns   = 4
	atlasSchema    = "atlas_schema_revisions"
	instanceMarker = "_inst_"
)

var current atomic.Pointer[server]

type server struct {
	admin    *pgxpool.Pool
	base     pgtestdb.Config
	migrator atlasMigrator

	mu    sync.Mutex
	names map[string]bool
	kept  atomic.Int32

	templateOnce sync.Once
	template     pgtestdb.Config
}

func Main(m *testing.M) {
	s, err := open(context.Background(), config.TestDBURL(os.Environ()))
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "testkit.Main: %v\n", err)
		os.Exit(1)
	}
	current.Store(s)
	goleak.VerifyTestMain(runThenClose{m: m, s: s})
}

type runThenClose struct {
	m *testing.M
	s *server
}

func (r runThenClose) Run() int {
	code := r.m.Run()
	r.s.admin.Close()
	return code
}

func open(ctx context.Context, rawURL string) (*server, error) {
	base, err := parseTestURL(rawURL)
	if err != nil {
		return nil, err
	}
	migrator, err := findMigrator()
	if err != nil {
		return nil, err
	}
	admin, err := pgxpool.New(ctx, rawURL)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", redact(rawURL), err)
	}
	s := &server{admin: admin, base: base, migrator: migrator, names: map[string]bool{}}
	if _, err := s.dropStale(ctx, clock.Real{}.Now().Add(-staleAfter), "t_", "testdb_"); err != nil {
		admin.Close()
		return nil, err
	}
	return s, nil
}

func parseTestURL(rawURL string) (pgtestdb.Config, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return pgtestdb.Config{}, fmt.Errorf("parse TEST_DATABASE_URL: %w", err)
	}
	if u.Port() != testPort {
		return pgtestdb.Config{}, setupError(fmt.Sprintf(
			"TEST_DATABASE_URL %s is not monaco-postgres-test on port %s; tests never touch the dev database",
			redact(rawURL), testPort))
	}
	password, _ := u.User.Password()
	return pgtestdb.Config{
		DriverName: "pgx",
		Host:       u.Hostname(),
		Port:       u.Port(),
		User:       u.User.Username(),
		Password:   password,
		Database:   strings.TrimPrefix(u.Path, "/"),
		Options:    u.RawQuery,
	}, nil
}

func redact(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "<unparsable url>"
	}
	return u.Redacted()
}

func DB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	s := current.Load()
	if s == nil {
		t.Fatal("testkit.DB: call testkit.Main(m) from this package's TestMain")
	}
	s.claim(t.Name())
	tmpl := s.templateFor(t)
	inst := tmpl
	inst.Database = databaseName(t.Name())
	ctx := context.Background()
	create := fmt.Sprintf(`CREATE DATABASE %s TEMPLATE %s OWNER %s`,
		pgx.Identifier{inst.Database}.Sanitize(), pgx.Identifier{tmpl.Database}.Sanitize(),
		pgx.Identifier{tmpl.User}.Sanitize())
	if _, err := s.admin.Exec(ctx, create); err != nil {
		t.Fatalf("testkit.DB: clone %s: %v", tmpl.Database, err)
	}
	cfg, err := pgxpool.ParseConfig(inst.URL())
	if err != nil {
		t.Fatalf("testkit.DB: %v", err)
	}
	cfg.MaxConns = poolMaxConns
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("testkit.DB: connect %s: %v", inst.Database, err)
	}
	t.Cleanup(func() {
		pool.Close()
		s.release(t.Name())
		if t.Failed() && s.kept.Add(1) <= keepFailed {
			t.Logf("testkit: kept %s for debugging at %s", inst.Database, redact(inst.URL()))
			return
		}
		if err := s.drop(ctx, inst.Database); err != nil {
			t.Errorf("testkit.DB: %v", err)
		}
	})
	return pool
}

func (s *server) claim(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.names[name] {
		panic(fmt.Sprintf("testkit.DB: %s already has a database in this run; "+
			"call testkit.DB once per test and give every test a unique name", name))
	}
	s.names[name] = true
}

func (s *server) release(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.names, name)
}

func (s *server) templateFor(t *testing.T) pgtestdb.Config {
	t.Helper()
	s.templateOnce.Do(func() {
		inst := pgtestdb.Custom(t, s.base, s.migrator)
		name, _, ok := strings.Cut(inst.Database, instanceMarker)
		if !ok {
			t.Fatalf("testkit.DB: pgtestdb instance %s has no template prefix", inst.Database)
		}
		if err := s.drop(context.Background(), inst.Database); err != nil {
			t.Fatalf("testkit.DB: %v", err)
		}
		s.template = *inst
		s.template.Database = name
	})
	if s.template.Database == "" {
		t.Fatal("testkit.DB: the template database failed to build earlier in this run")
	}
	return s.template
}

func (s *server) currentTemplate() (string, error) {
	hash, err := s.migrator.Hash()
	if err != nil {
		return "", err
	}
	role := pgtestdb.DefaultRole()
	return "testdb_tpl_" + common.NewRecursiveHash(
		common.Field("Username", role.Username),
		common.Field("Password", role.Password),
		common.Field("Capabilities", role.Capabilities),
		common.Field("MigratorHash", hash),
	).String(), nil
}

func (s *server) drop(ctx context.Context, name string) error {
	if _, err := s.admin.Exec(
		ctx,
		`UPDATE pg_database SET datistemplate = false WHERE datname = $1`,
		name,
	); err != nil {
		return fmt.Errorf("unmark template %s: %w", name, err)
	}
	stmt := fmt.Sprintf(`DROP DATABASE IF EXISTS %s WITH (FORCE)`, pgx.Identifier{name}.Sanitize())
	if _, err := s.admin.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("drop %s: %w", name, err)
	}
	return nil
}

func (s *server) dropStale(ctx context.Context, cutoff time.Time, prefixes ...string) ([]string, error) {
	keep, err := s.currentTemplate()
	if err != nil {
		return nil, err
	}
	rows, err := s.admin.Query(ctx, `
		SELECT datname FROM pg_database
		WHERE datname <> $3
		  AND EXISTS (SELECT 1 FROM unnest($1::text[]) AS p WHERE starts_with(datname, p))
		  AND (pg_stat_file('base/' || oid || '/PG_VERSION')).modification < $2`, prefixes, cutoff, keep)
	if err != nil {
		return nil, fmt.Errorf("list stale test databases: %w", err)
	}
	stale, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("list stale test databases: %w", err)
	}
	for _, name := range stale {
		if err := s.drop(ctx, name); err != nil {
			return nil, err
		}
	}
	return stale, nil
}

func databaseName(testName string) string {
	suffix := "_" + strings.ToLower(rand.Text()[:8])
	slug := strings.Trim(regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(strings.ToLower(testName), "_"), "_")
	slug = strings.TrimRight(slug[:min(len(slug), maxNameLen-len("t_")-len(suffix))], "_")
	return "t_" + slug + suffix
}

func Reset(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	var tables string
	err := db.QueryRow(ctx, `
		SELECT string_agg(format('%I.%I', schemaname, tablename), ', ')
		FROM pg_tables
		WHERE schemaname NOT IN ('pg_catalog', 'information_schema', $1)`, atlasSchema).Scan(&tables)
	if err != nil {
		t.Fatalf("testkit.Reset: list tables: %v", err)
	}
	if _, err := db.Exec(ctx, "TRUNCATE "+tables+" RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("testkit.Reset: %v", err)
	}
}

type setupError string

func (e setupError) Error() string { return string(e) }

type atlasMigrator struct {
	bin, dir string
}

func findMigrator() (atlasMigrator, error) {
	dir, err := os.Getwd()
	if err != nil {
		return atlasMigrator{}, fmt.Errorf("find apps/backend: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "atlas.hcl")); err == nil {
			return atlasMigrator{
				bin: filepath.Join(dir, "..", "..", ".bin", "atlas"),
				dir: filepath.Join(dir, "migrations"),
			}, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return atlasMigrator{}, setupError("no atlas.hcl above the test's working directory")
		}
		dir = parent
	}
}

func (a atlasMigrator) Hash() (string, error) {
	h, err := common.HashDir(a.dir)
	if err != nil {
		return "", fmt.Errorf("hash %s: %w", a.dir, err)
	}
	return h, nil
}

func (a atlasMigrator) Migrate(ctx context.Context, _ *sql.DB, conf pgtestdb.Config) error {
	cmd := exec.CommandContext(ctx, a.bin, "migrate", "apply", "--dir", "file://"+a.dir, "--url", conf.URL())
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s migrate apply (run scripts/install-atlas.sh from the repo root if it is missing): %w\n%s",
			a.bin, err, out)
	}
	return nil
}
