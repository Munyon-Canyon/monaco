package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
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
	listing := "usage: monacoctl <command> [args]\n  apply\n  lint\n  status\n"
	for _, tc := range []struct {
		args []string
		want string
	}{
		{nil, listing},
		{[]string{"down"}, "monacoctl: unknown command \"down\"\n" + listing},
		{[]string{"apply", "extra"}, "usage: monacoctl migrate apply|status|lint\n"},
		{[]string{"lint", "extra"}, "usage: monacoctl migrate apply|status|lint\n"},
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

func writeFile(t *testing.T, repo, rel, body string, mode os.FileMode) {
	t.Helper()
	root, err := os.OpenRoot(repo)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if err := root.MkdirAll(filepath.Dir(rel), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile(rel, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}

func fakeRepo(t *testing.T) (repo, backend string) {
	t.Helper()
	repo = t.TempDir()
	backend = filepath.Join(repo, "apps", "backend")
	writeFile(t, repo, "apps/backend/go.mod", "module "+backendModule+"\n\ngo 1.25.0\n", 0o600)
	writeFile(t, repo, "scripts/go.mod", "module github.com/monaco/monaco/scripts\n", 0o600)
	writeFile(t, repo, "apps/backend/internal/db/x.go", "package db\n", 0o600)
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
	writeFile(t, repo, "apps/backend/.atlas-version", string(pinned), 0o600)
	writeFile(t, repo, ".bin/atlas",
		"#!/bin/sh\nif [ \"$1\" = version ]; then echo 'atlas community version v1.3.0'; exit 0; fi\npwd -P\n", 0o700)

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
