package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
	codegen "github.com/monaco/monaco/apps/backend/internal/tools/gen"
)

func fakeAtlas(name string) atlas {
	dir, _ := filepath.Abs(filepath.Join("testdata", "atlas"))
	return atlas{
		bin:         filepath.Join(dir, name),
		versionFile: filepath.Join(dir, "pinned-version"),
		beforeDB:    func(context.Context, string) error { return nil },
	}
}

func validMigrateEnviron() []string {
	return []string{
		"MONACO_ENV=test",
		"DATABASE_URL=postgres://monaco@localhost:54323/fresh",
		"NATS_URL=nats://localhost:4222",
	}
}

func lines(args ...string) string {
	return strings.Join(args, "\n") + "\n"
}

func TestMigrate_runsThePinnedAtlasWithTheSubcommandArguments(t *testing.T) {
	t.Parallel()
	url := "postgres://monaco@localhost:54323/fresh"
	for name, tc := range map[string]struct {
		sub     string
		environ []string
		want    string
	}{
		"apply":  {"apply", validMigrateEnviron(), lines("migrate", "apply", "--dir", "file://migrations", "--url", url)},
		"status": {"status", validMigrateEnviron(), lines("migrate", "status", "--dir", "file://migrations", "--url", url)},
		"lint without config": {"lint", nil, lines(
			"migrate", "lint", "--dir", "file://migrations", "--dev-url", "docker://postgres/16/dev", "--latest", "1")},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			code := migrateTool(fakeAtlas("pinned"), tc.environ)([]string{tc.sub}, &stdout, &stderr)

			if code != 0 || stdout.String() != tc.want || stderr.String() != "atlas-err\n" {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
		})
	}
}

func TestMigrate_applyAndStatusFailBeforeAtlasWhenConfigIsMissing(t *testing.T) {
	t.Parallel()
	for _, sub := range []string{"apply", "status"} {
		var stdout, stderr bytes.Buffer
		code := migrateTool(fakeAtlas("pinned"), nil)([]string{sub}, &stdout, &stderr)

		want := "monacoctl: config.Load: invalid_input: missing MONACO_ENV, DATABASE_URL, NATS_URL\n"
		if code != 1 || stdout.Len() != 0 || stderr.String() != want {
			t.Fatalf("%s: code=%d stdout=%q stderr=%q", sub, code, stdout.String(), stderr.String())
		}
	}
}

func TestMigrate_failsWhenAtlasFails(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	code := migrateTool(fakeAtlas("failing"), validMigrateEnviron())([]string{"apply"}, &stdout, &stderr)

	want := "atlas-err\nmonacoctl: atlas migrate apply: exit status 3\n"
	if code != 1 || stderr.String() != want {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestMigrate_applyAndStatusStopBeforeAtlasWhenTheLegacyRenameFails(t *testing.T) {
	t.Parallel()
	a := fakeAtlas("pinned")
	var got []string
	a.beforeDB = func(_ context.Context, url string) error {
		got = append(got, url)
		return errors.New("rename legacy revisions: boom")
	}
	for _, sub := range []string{"apply", "status"} {
		var stdout, stderr bytes.Buffer
		code := migrateTool(a, validMigrateEnviron())([]string{sub}, &stdout, &stderr)

		if code != 1 || stdout.Len() != 0 || stderr.String() != "monacoctl: rename legacy revisions: boom\n" {
			t.Fatalf("%s: code=%d stdout=%q stderr=%q", sub, code, stdout.String(), stderr.String())
		}
	}
	if want := "postgres://monaco@localhost:54323/fresh"; len(got) != 2 || got[0] != want || got[1] != want {
		t.Fatalf("beforeDB urls = %q, want the config URL twice", got)
	}
}

func TestMigrate_refusesAnAtlasThatIsNotThePinnedCommunityBuild(t *testing.T) {
	t.Parallel()
	for name, reported := range map[string]string{
		"official": "atlas version v1.3.4-1fc284b-canary",
		"older":    "atlas community version v1.2.0",
	} {
		a := fakeAtlas(name)
		var stdout, stderr bytes.Buffer
		code := migrateTool(a, nil)([]string{"lint"}, &stdout, &stderr)

		want := "monacoctl: " + a.bin + " is \"" + reported + "\", want \"atlas community version v1.3.0\". " +
			"run: just install\n"
		if code != 1 || stdout.Len() != 0 || stderr.String() != want {
			t.Fatalf("%s: code=%d stdout=%q stderr=%q", name, code, stdout.String(), stderr.String())
		}
	}
}

func TestMigrate_explainsTheFixWhenThePinnedAtlasIsMissing(t *testing.T) {
	t.Parallel()
	a := fakeAtlas("pinned")
	a.bin = filepath.Join(t.TempDir(), "atlas")

	var stdout, stderr bytes.Buffer
	code := migrateTool(a, validMigrateEnviron())([]string{"status"}, &stdout, &stderr)

	want := "monacoctl: " + a.bin + " is \"\" (fork/exec " + a.bin + ": no such file or directory), " +
		"want \"atlas community version v1.3.0\". run: just install\n"
	if code != 1 || stderr.String() != want {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestMigrate_rejectsUnknownSubcommandOrExtraArgsWithoutRunningAtlas(t *testing.T) {
	t.Parallel()
	listing := "usage: monacoctl <command> [args]\n  apply\n  lint\n  order\n  status\n"
	for _, tc := range []struct {
		args []string
		want string
	}{
		{nil, listing},
		{[]string{"down"}, "monacoctl: unknown command \"down\"\n" + listing},
		{[]string{"apply", "extra"}, "usage: monacoctl migrate apply|status|lint|order\n"},
		{[]string{"lint", "extra"}, "usage: monacoctl migrate apply|status|lint|order\n"},
		{[]string{"order", "extra"}, "usage: monacoctl migrate apply|status|lint|order\n"},
	} {
		var stdout, stderr bytes.Buffer
		code := migrateTool(fakeAtlas("pinned"), validMigrateEnviron())(tc.args, &stdout, &stderr)

		if code != 2 || stdout.Len() != 0 || stderr.String() != tc.want {
			t.Fatalf("args %q: code=%d stdout=%q stderr=%q", tc.args, code, stdout.String(), stderr.String())
		}
	}
}

func TestMigrate_explainsAnUnreadablePinnedVersion(t *testing.T) {
	t.Parallel()
	a := fakeAtlas("pinned")
	a.versionFile = filepath.Join(t.TempDir(), "missing")
	var stdout, stderr bytes.Buffer
	code := migrateTool(a, nil)([]string{"lint"}, &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 || !strings.HasPrefix(stderr.String(), "monacoctl: read pinned atlas version: ") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func writeFile(t *testing.T, repo, rel, body string) {
	t.Helper()
	root, err := os.OpenRoot(repo)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if err := root.MkdirAll(filepath.Dir(rel), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile(rel, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func fakeRepo(t *testing.T) (repo, backend string) {
	t.Helper()
	repo = t.TempDir()
	backend = filepath.Join(repo, "apps", "backend")
	writeFile(t, repo, "apps/backend/go.mod", "module "+backendModule+"\n\ngo 1.25.0\n")
	writeFile(t, repo, "scripts/go.mod", "module github.com/monaco/monaco/scripts\n")
	writeFile(t, repo, "apps/backend/internal/db/x.go", "package db\n")
	return repo, backend
}

func TestModuleRoot_findsTheBackendModuleFromAnyDirectoryInTheRepo(t *testing.T) {
	t.Parallel()
	repo, backend := fakeRepo(t)
	for name, start := range map[string]string{
		"module dir":         backend,
		"package subdir":     filepath.Join(backend, "internal", "db"),
		"repo root":          repo,
		"other module":       filepath.Join(repo, "scripts"),
		"bin next to module": filepath.Join(repo, "bin"),
	} {
		got, err := moduleRoot(start)
		if err != nil || got != backend {
			t.Errorf("%s: moduleRoot(%s) = %q, %v, want %q", name, start, got, err, backend)
		}
	}
}

func TestModuleRoot_triesEachStartInOrder(t *testing.T) {
	t.Parallel()
	repo, backend := fakeRepo(t)
	outside := t.TempDir()
	got, err := moduleRoot(outside, filepath.Join(repo, "bin"))
	if err != nil || got != backend {
		t.Fatalf("backendRoot = %q, %v, want the module found from the second start %q", got, err, backend)
	}
}

func TestModuleRoot_namesTheStartsWhenNoneIsInsideTheRepo(t *testing.T) {
	t.Parallel()
	outside := t.TempDir()
	_, err := moduleRoot(outside)
	want := "cannot find module " + backendModule + " above " + outside
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
}

func TestMigrate_runsAtlasFromTheModuleRootWithTheRepoPinnedBinary(t *testing.T) {
	t.Parallel()
	repo, backend := fakeRepo(t)
	pinned, err := os.ReadFile(filepath.Join("testdata", "atlas", "pinned-version"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, repo, "apps/backend/.atlas-version", string(pinned))
	symlinkCommittedScript(t, filepath.Join("atlas", "pwd"), filepath.Join(repo, ".bin", "atlas"))

	environ := []string{"MONACO_ENV=test", "DATABASE_URL=" + testkit.DB(t).Config().ConnString(), "NATS_URL=nats://x"}
	var stdout, stderr bytes.Buffer
	code := migrateTool(atlasAt(backend), environ)([]string{"apply"}, &stdout, &stderr)

	resolved, _ := filepath.EvalSymlinks(backend)
	if code != 0 || strings.TrimSpace(stdout.String()) != resolved {
		t.Fatalf("code=%d stdout=%q stderr=%q, want atlas run in %s", code, stdout.String(), stderr.String(), resolved)
	}
}

func TestMigrate_failsAndNamesTheSearchWhenNoBackendModuleIsFound(t *testing.T) {
	t.Parallel()
	outside := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := locatedMigrateTool(validMigrateEnviron(), outside)([]string{"apply"}, &stdout, &stderr)

	want := "monacoctl: cannot find module " + backendModule + " above " + outside + "\n"
	if code != 1 || stdout.Len() != 0 || stderr.String() != want {
		t.Fatalf("code=%d stdout=%q stderr=%q, want %q", code, stdout.String(), stderr.String(), want)
	}
}

func TestRunIn_runsInTheDirAndNamesTheCommandOnFailure(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, err := runIn(dir)(t.Context(), "pwd", "-P")
	if resolved, _ := filepath.EvalSymlinks(dir); err != nil || strings.TrimSpace(string(out)) != resolved {
		t.Fatalf("pwd = %q, %v; want %s", out, err, resolved)
	}
	_, err = runIn(dir)(t.Context(), "git", "ls-tree", "nope")
	if err == nil || !strings.Contains(err.Error(), "git ls-tree nope") {
		t.Fatalf("err = %v, want it to name the command", err)
	}
}

func orderRepo(t *testing.T) (repo, backend string) {
	t.Helper()
	repo = t.TempDir()
	backend = filepath.Join(repo, "apps", "backend")
	writeMigration(t, backend, "20261001000000_a.sql")
	writeMigration(t, backend, "20261003000000_s.sql")
	git(t, repo, "init", "-q")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "staging")
	return repo, backend
}

func writeMigration(t *testing.T, backend, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(backend, "migrations"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backend, "migrations", name), []byte("select 1;"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func migrateOrder(backend string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := migrateTool(atlas{dir: backend}, nil)([]string{"order"}, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestMigrateOrder_failsBelowOriginStagingAndPassesAfterRebase(t *testing.T) {
	t.Parallel()
	repo, backend := orderRepo(t)
	git(t, repo, "update-ref", "refs/remotes/origin/staging", "HEAD")
	git(t, repo, "switch", "-q", "-c", "two")
	writeMigration(t, backend, "20261002000000_two.sql")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "two")

	code, stdout, stderr := migrateOrder(backend)
	want := " adds 20261002000000_two.sql at or below 20261003000000_s.sql; run just gen migration --rebase on that branch\n"
	if code != 1 || !strings.HasSuffix(stdout, want) || stderr != "" {
		t.Fatalf("before rebase: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}

	m := codegen.Migrator{
		Dir: backend,
		Now: func() time.Time { return time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC) },
		Parent: func(context.Context) ([]string, error) {
			return []string{"20261001000000_a.sql", "20261003000000_s.sql"}, nil
		},
		Hash: func(context.Context) error { return nil },
	}
	if _, err := m.Run(t.Context(), []string{"--rebase"}); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-q", "--amend", "--no-edit")
	if code, stdout, stderr := migrateOrder(backend); code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("after rebase: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestMigrateOrder_explainsAMissingOriginStaging(t *testing.T) {
	t.Parallel()
	_, backend := orderRepo(t)
	if code, stdout, stderr := migrateOrder(backend); code != 1 || stdout != "" ||
		stderr != "monacoctl: origin/staging is missing; fetch it\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestMigrationCommits_stopsOnTheFirstGitFailure(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	for _, failing := range []string{"rev-list", "diff-tree", "ls-tree"} {
		git := func(_ context.Context, _ string, args ...string) ([]byte, error) {
			if args[0] == failing {
				return nil, boom
			}
			return []byte("abc\n"), nil
		}
		var stdout, stderr bytes.Buffer
		if code := migrationOrder(
			t.Context(),
			git,
			&stdout,
			&stderr,
		); code != 1 ||
			!strings.Contains(stderr.String(), "boom") {
			t.Fatalf("%s failing: code=%d stderr=%q", failing, code, stderr.String())
		}
	}
}
