package verify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func schemaStack(t *testing.T, docker, atlas string) (*Stack, string) {
	t.Helper()
	url := testkit.DB(t).Config().ConnString()
	cache := t.TempDir()
	return &Stack{DBURL: url, container: "pg", opts: Options{
		Dir: backendDir(t), Atlas: atlas, Docker: Docker(docker), SchemaCache: cache,
	}}, cache
}

func TestApplySchema_dumpsTheMigratedSchemaThenRestoresItInsteadOfMigrating(t *testing.T) {
	t.Parallel()
	docker := writeScript(
		t,
		"docker",
		`printf '%s\n' '\restrict k' 'CREATE TABLE verify_schema_probe (id int);' '\unrestrict k'`,
	)
	s, cache := schemaStack(t, docker, "true")
	if err := s.applySchema(t.Context()); err != nil {
		t.Fatalf("first applySchema: %v", err)
	}
	dumps, _ := filepath.Glob(filepath.Join(cache, "*.sql"))
	if len(dumps) != 1 {
		t.Fatalf("cache holds %v, want one dump", dumps)
	}
	body, _ := os.ReadFile(dumps[0])
	if strings.Contains(string(body), `\restrict`) || !strings.Contains(string(body), "verify_schema_probe") {
		t.Fatalf("dump = %q, want the SQL without psql meta-commands", body)
	}
	s.opts.Atlas = "/nonexistent/atlas"
	if err := s.applySchema(t.Context()); err != nil {
		t.Fatalf("cached applySchema: %v", err)
	}
	conn, err := pgx.Connect(t.Context(), s.DBURL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(t.Context()) }()
	var n int
	if err := conn.QueryRow(t.Context(), "select count(*) from verify_schema_probe").Scan(&n); err != nil {
		t.Fatalf("restored table: %v", err)
	}
}

func TestApplySchema_migratesWithoutCachingOutsideDocker(t *testing.T) {
	t.Parallel()
	s, cache := schemaStack(t, "/nonexistent/docker", "true")
	s.container = ""
	if err := s.applySchema(t.Context()); err != nil {
		t.Fatalf("applySchema: %v", err)
	}
	if dumps, _ := filepath.Glob(filepath.Join(cache, "*.sql")); len(dumps) != 0 {
		t.Fatalf("cache holds %v, want nothing without a container to dump", dumps)
	}
}

func TestApplySchema_reportsEachFailure(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		edit func(*Stack)
		want string
	}{
		{"no atlas.sum", func(s *Stack) { s.opts.Dir = t.TempDir() }, "schema cache key"},
		{"atlas", func(s *Stack) { s.opts.Atlas = "/nonexistent/atlas" }, "/nonexistent/atlas migrate apply"},
		{"pg_dump", func(s *Stack) {
			s.opts.Docker = Docker(writeScript(t, "docker", `echo "no such container" >&2; exit 1`))
		}, "no such container"},
		{"cache dir", func(s *Stack) { s.opts.SchemaCache = filepath.Join(writeScript(t, "file", "true"), "sub") }, "not a directory"},
		{"restore", func(s *Stack) {
			path, _ := schemaDumpPath(s.opts.SchemaCache, filepath.Join(s.opts.Dir, "migrations"))
			if err := writeAtomic(path, "not sql"); err != nil {
				t.Fatal(err)
			}
		}, "restore cached schema"},
		{"unreachable", func(s *Stack) {
			path, _ := schemaDumpPath(s.opts.SchemaCache, filepath.Join(s.opts.Dir, "migrations"))
			if err := writeAtomic(path, "select 1"); err != nil {
				t.Fatal(err)
			}
			s.DBURL = "postgres://monaco:monaco@127.0.0.1:1/monaco?sslmode=disable&connect_timeout=1"
		}, "restore cached schema"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, _ := schemaStack(t, writeScript(t, "docker", "echo 'select 1;'"), "true")
			tc.edit(s)
			if err := s.applySchema(t.Context()); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("applySchema = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestSchemaDumpPath_changesWithTheMigrations(t *testing.T) {
	t.Parallel()
	a, b := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(a, "atlas.sum"), []byte("h1:a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b, "atlas.sum"), []byte("h1:b"), 0o600); err != nil {
		t.Fatal(err)
	}
	pa, _ := schemaDumpPath("/cache", a)
	pb, _ := schemaDumpPath("/cache", b)
	if pa == pb || filepath.Dir(pa) != "/cache" {
		t.Fatalf("paths %q and %q, want distinct dumps under /cache", pa, pb)
	}
	if p, err := schemaDumpPath("", a); p != "" || err != nil {
		t.Fatalf("no cache dir = %q, %v, want no dump", p, err)
	}
}

func TestSchemaCacheDir_isUnderTheUserCache(t *testing.T) {
	if got := SchemaCacheDir(); !strings.HasSuffix(got, filepath.Join("monaco", "verify-schema")) {
		t.Fatalf("SchemaCacheDir = %q", got)
	}
	t.Setenv("HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	if got := SchemaCacheDir(); got != "" {
		t.Fatalf("SchemaCacheDir without HOME = %q, want none", got)
	}
}
