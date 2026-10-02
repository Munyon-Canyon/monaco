package testkit

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/peterldowns/pgtestdb"
	"github.com/peterldowns/pgtestdb/migrators/common"

	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
)

const (
	testPort       = "54323"
	keepFailed     = 5
	staleAfter     = 10 * time.Minute
	maxNameLen     = 63
	poolMaxConns   = 4
	atlasSchema    = "atlas_schema_revisions"
	lockSpace      = 466
	instanceMarker = "_inst_"
)

var current atomic.Pointer[server]

type server struct {
	admin    *pgxpool.Pool
	base     pgtestdb.Config
	migrator atlasMigrator

	mu        sync.Mutex
	names     map[string]*queryCounter
	held      map[string]bool
	kept      atomic.Int32
	runPrefix string
	disk      diskUsage

	templateOnce sync.Once
	template     pgtestdb.Config
	holder       *pgx.Conn
}

type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type diskUsage func(context.Context) (used, total int64, err error)

const testDiskBytes int64 = 2 << 30

var errDiskTotal = errors.New("testkit: disk total is not positive")

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
	s := &server{
		admin: admin, base: base, migrator: migrator,
		names: map[string]*queryCounter{}, runPrefix: runPrefix(),
	}
	s.disk = s.containerDisk
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
	return s.dbFor(t)
}

func (s *server) dbFor(t *testing.T) *pgxpool.Pool {
	t.Helper()
	queries := s.claim(t.Name())
	tmpl := s.templateFor(t)
	inst := tmpl
	inst.Database = databaseName(s.runPrefix, t.Name())
	s.hold(inst.Database, true)
	t.Cleanup(func() { s.hold(inst.Database, false) })
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
	cfg.ConnConfig.Tracer = queries
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("testkit.DB: connect %s: %v", inst.Database, err)
	}
	t.Cleanup(func() {
		pool.Close()
		s.release(t.Name())
		kept, err := s.releaseDB(ctx, inst.Database, t.Failed())
		if err != nil {
			t.Errorf("testkit.DB: %v", err)
			return
		}
		if kept {
			t.Logf("testkit: kept %s for debugging at %s", inst.Database, redact(inst.URL()))
		}
	})
	return pool
}

func (s *server) claim(name string) *queryCounter {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.names[name] != nil {
		panic(fmt.Sprintf("testkit.DB: %s already has a database in this run; "+
			"call testkit.DB once per test and give every test a unique name", name))
	}
	c := &queryCounter{}
	s.names[name] = c
	return c
}

func (s *server) counter(name string) *queryCounter {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.names[name]
}

func (s *server) release(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.names, name)
}

func (s *server) hold(db string, live bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.held == nil {
		s.held = map[string]bool{}
	}
	if live {
		s.held[db] = true
		return
	}
	delete(s.held, db)
}

func (s *server) holds(db string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.held[db]
}

func (s *server) templateFor(t *testing.T) pgtestdb.Config {
	t.Helper()
	s.templateOnce.Do(func() {
		ctx := context.Background()
		name, err := s.currentTemplate()
		if err != nil {
			t.Fatalf("testkit.DB: %v", err)
		}
		if err := s.holdTemplate(ctx, name); err != nil {
			t.Fatalf("testkit.DB: %v", err)
		}
		inst := pgtestdb.Custom(t, s.base, s.migrator)
		if built, _, _ := strings.Cut(inst.Database, instanceMarker); built != name {
			t.Fatalf("testkit.DB: pgtestdb built template %s, testkit holds %s", built, name)
		}
		if err := drop(ctx, s.admin, inst.Database); err != nil {
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

func (s *server) holdTemplate(ctx context.Context, name string) error {
	conn, err := s.admin.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("hold template %s: %w", name, err)
	}
	s.holder = conn.Hijack()
	if _, err := s.holder.Exec(
		ctx,
		`SELECT pg_advisory_lock_shared($1::int4, hashtext($2))`,
		lockSpace,
		name,
	); err != nil {
		return fmt.Errorf("hold template %s: %w", name, err)
	}
	return nil
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

func drop(ctx context.Context, q execer, name string) error {
	if _, err := q.Exec(
		ctx,
		`UPDATE pg_database SET datistemplate = false WHERE datname = $1`,
		name,
	); err != nil {
		return fmt.Errorf("unmark template %s: %w", name, err)
	}
	stmt := fmt.Sprintf(`DROP DATABASE IF EXISTS %s WITH (FORCE)`, pgx.Identifier{name}.Sanitize())
	if _, err := q.Exec(ctx, stmt); err != nil {
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
		  AND (pg_stat_file('base/' || oid || '/PG_VERSION', true)).modification < $2`, prefixes, cutoff, keep)
	if err != nil {
		return nil, fmt.Errorf("list stale test databases: %w", err)
	}
	stale, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("list stale test databases: %w", err)
	}
	conn, err := s.admin.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("drop stale test databases: %w", err)
	}
	defer conn.Release()
	var dropped []string
	for _, name := range stale {
		ok, err := dropIfUnused(ctx, conn, name)
		if err != nil {
			return nil, err
		}
		if ok {
			dropped = append(dropped, name)
		}
	}
	return dropped, nil
}

func dropIfUnused(ctx context.Context, conn *pgxpool.Conn, name string) (bool, error) {
	var locked bool
	err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1::int4, hashtext($2))`, lockSpace, name).Scan(&locked)
	if err != nil {
		return false, fmt.Errorf("lock %s: %w", name, err)
	}
	if !locked {
		return false, nil
	}
	defer func() {
		_, _ = conn.Exec(ctx, `SELECT pg_advisory_unlock($1::int4, hashtext($2))`, lockSpace, name)
	}()
	var busy bool
	err = conn.QueryRow(ctx, `SELECT EXISTS (SELECT FROM pg_stat_activity WHERE datname = $1)`, name).Scan(&busy)
	if err != nil {
		return false, fmt.Errorf("check connections to %s: %w", name, err)
	}
	if busy {
		return false, nil
	}
	return true, drop(ctx, conn, name)
}

func runPrefix() string {
	return "t_" + strings.ToLower(rand.Text()[:8]) + "_"
}

func (s *server) containerDisk(ctx context.Context) (int64, int64, error) {
	var used int64
	err := s.admin.QueryRow(ctx,
		`SELECT coalesce(sum(pg_database_size(oid)), 0)::bigint FROM pg_database`).Scan(&used)
	if err != nil {
		return 0, 0, fmt.Errorf("testkit: disk usage under run prefix %s: %w", s.runPrefix, err)
	}
	return used, testDiskBytes, nil
}

func (s *server) diskOver(ctx context.Context) (bool, error) {
	if s.disk == nil {
		return false, nil
	}
	used, total, err := s.disk(ctx)
	if err != nil {
		return false, fmt.Errorf("testkit: disk usage under run prefix %s: %w", s.runPrefix, err)
	}
	if total <= 0 {
		return false, fmt.Errorf("testkit: disk total %d under run prefix %s: %w", total, s.runPrefix, errDiskTotal)
	}
	return used > total/2, nil
}

func (s *server) guardPrefix(name string) {
	if s.runPrefix == "" || !strings.HasPrefix(name, s.runPrefix) {
		panic(fmt.Sprintf("testkit: DROP DATABASE %s is outside run prefix %q", name, s.runPrefix))
	}
}

func (s *server) dropOwned(ctx context.Context, q execer, name string) error {
	s.guardPrefix(name)
	return drop(ctx, q, name)
}

func (s *server) releaseDB(ctx context.Context, name string, failed bool) (bool, error) {
	over, err := s.diskOver(ctx)
	if err != nil {
		return false, err
	}
	if over {
		if _, err := s.cleanKept(ctx); err != nil {
			return false, err
		}
		return false, s.dropOwned(ctx, s.admin, name)
	}
	if failed && s.kept.Add(1) <= keepFailed {
		return true, nil
	}
	return false, s.dropOwned(ctx, s.admin, name)
}

func (s *server) cleanKept(ctx context.Context) ([]string, error) {
	over, err := s.diskOver(ctx)
	if err != nil || !over {
		return nil, err
	}
	if s.runPrefix == "" {
		panic(`testkit: DROP DATABASE cleaner has an empty run prefix ""`)
	}
	rows, err := s.admin.Query(ctx, `
		SELECT datname FROM pg_database
		WHERE starts_with(datname, $1)
		ORDER BY datname`, s.runPrefix)
	if err != nil {
		return nil, fmt.Errorf("testkit: list kept databases under %s: %w", s.runPrefix, err)
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("testkit: list kept databases under %s: %w", s.runPrefix, err)
	}
	conn, err := s.admin.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("testkit: drop kept databases under %s: %w", s.runPrefix, err)
	}
	defer conn.Release()
	var dropped []string
	for _, name := range names {
		s.guardPrefix(name)
		if s.holds(name) {
			continue
		}
		ok, err := dropIfUnused(ctx, conn, name)
		if err != nil {
			return nil, err
		}
		if ok {
			dropped = append(dropped, name)
		}
	}
	return dropped, nil
}

func databaseName(prefix, testName string) string {
	suffix := "_" + strings.ToLower(rand.Text()[:8])
	slug := strings.Trim(regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(strings.ToLower(testName), "_"), "_")
	room := maxNameLen - len(prefix) - len(suffix)
	if room < 0 {
		room = 0
	}
	slug = strings.TrimRight(slug[:min(len(slug), room)], "_")
	return prefix + slug + suffix
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
	entries, err := os.ReadDir(a.dir)
	if err != nil {
		return "", fmt.Errorf("hash %s: %w", a.dir, err)
	}
	h := common.NewRecursiveHash()
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		contents, err := os.ReadFile(filepath.Join(a.dir, e.Name()))
		if err != nil {
			return "", fmt.Errorf("hash %s: %w", a.dir, err)
		}
		h.Add([]byte(e.Name()))
		h.Add(contents)
	}
	return h.String(), nil
}

func (a atlasMigrator) Migrate(ctx context.Context, _ *sql.DB, conf pgtestdb.Config) error {
	cmd := exec.CommandContext(ctx, a.bin, "migrate", "apply", "--dir", "file://"+a.dir, "--url", conf.URL())
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s migrate apply (run scripts/install-atlas.sh from the repo root if it is missing): %w\n%s",
			a.bin, err, out)
	}
	return nil
}
