package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func fakeAtlas(name string) atlas {
	return atlas{
		bin:         filepath.Join("testdata", "atlas", name),
		versionFile: filepath.Join("testdata", "atlas", "pinned-version"),
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
			"Run scripts/install-atlas.sh from the repo root.\n"
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
		"want \"atlas community version v1.3.0\". Run scripts/install-atlas.sh from the repo root.\n"
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
