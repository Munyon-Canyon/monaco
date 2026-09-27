package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeAtlas struct {
	bin, versionFile, argsFile string
}

func newFakeAtlas(t *testing.T, reportedVersion, exitCode string) fakeAtlas {
	t.Helper()
	dir := t.TempDir()
	f := fakeAtlas{
		bin:         filepath.Join(dir, "atlas"),
		versionFile: filepath.Join(dir, ".atlas-version"),
		argsFile:    filepath.Join(dir, "args"),
	}
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = version ]; then echo '" + reportedVersion + "'; echo https://example; exit 0; fi\n" +
		"printf '%s\\n' \"$@\" > " + f.argsFile + "\necho atlas-out\necho atlas-err >&2\nexit " + exitCode + "\n"
	if err := os.WriteFile(f.bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.versionFile, []byte("v1.3.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f fakeAtlas) ran(t *testing.T) string {
	t.Helper()
	args, err := os.ReadFile(f.argsFile)
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(args)
}

func validMigrateEnviron() []string {
	return []string{
		"MONACO_ENV=test",
		"DATABASE_URL=postgres://monaco@localhost:54323/fresh",
		"NATS_URL=nats://localhost:4222",
	}
}

func TestMigrate_runsThePinnedAtlasWithTheSubcommandArguments(t *testing.T) {
	t.Parallel()
	url := "postgres://monaco@localhost:54323/fresh"
	for sub, tc := range map[string]struct {
		environ []string
		want    []string
	}{
		"apply":  {validMigrateEnviron(), []string{"migrate", "apply", "--dir", "file://migrations", "--url", url}},
		"status": {validMigrateEnviron(), []string{"migrate", "status", "--dir", "file://migrations", "--url", url}},
		"lint without config": {nil, []string{
			"migrate", "lint", "--dir", "file://migrations",
			"--dev-url", "docker://postgres/16/dev", "--latest", "1",
		}},
	} {
		t.Run(sub, func(t *testing.T) {
			t.Parallel()
			f := newFakeAtlas(t, "atlas community version v1.3.0", "0")

			var stdout, stderr bytes.Buffer
			code := migrateTool(atlas{f.bin, f.versionFile}, tc.environ)(
				[]string{strings.Fields(sub)[0]}, &stdout, &stderr)

			got := f.ran(t)
			if code != 0 || got != strings.Join(tc.want, "\n")+"\n" ||
				stdout.String() != "atlas-out\n" || stderr.String() != "atlas-err\n" {
				t.Fatalf("code=%d args=%q stdout=%q stderr=%q", code, got, stdout.String(), stderr.String())
			}
		})
	}
}

func TestMigrate_applyAndStatusFailBeforeAtlasWhenConfigIsMissing(t *testing.T) {
	t.Parallel()
	for _, sub := range []string{"apply", "status"} {
		f := newFakeAtlas(t, "atlas community version v1.3.0", "0")

		var stdout, stderr bytes.Buffer
		code := migrateTool(atlas{f.bin, f.versionFile}, nil)([]string{sub}, &stdout, &stderr)

		want := "monacoctl: config.Load: invalid_input: missing MONACO_ENV, DATABASE_URL, NATS_URL\n"
		if code != 1 || f.ran(t) != "" || stderr.String() != want {
			t.Fatalf("%s: code=%d ran=%q stderr=%q", sub, code, f.ran(t), stderr.String())
		}
	}
}

func TestMigrate_failsWhenAtlasFails(t *testing.T) {
	t.Parallel()
	f := newFakeAtlas(t, "atlas community version v1.3.0", "3")

	var stdout, stderr bytes.Buffer
	code := migrateTool(atlas{f.bin, f.versionFile}, validMigrateEnviron())([]string{"apply"}, &stdout, &stderr)

	want := "atlas-err\nmonacoctl: atlas migrate apply: exit status 3\n"
	if code != 1 || stderr.String() != want {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestMigrate_refusesAnAtlasThatIsNotThePinnedCommunityBuild(t *testing.T) {
	t.Parallel()
	for _, reported := range []string{"atlas version v1.3.4-1fc284b-canary", "atlas community version v1.2.0"} {
		f := newFakeAtlas(t, reported, "0")

		var stdout, stderr bytes.Buffer
		code := migrateTool(atlas{f.bin, f.versionFile}, nil)([]string{"lint"}, &stdout, &stderr)

		want := "monacoctl: " + f.bin + " is \"" + reported + "\", want \"atlas community version v1.3.0\". " +
			"Run scripts/install-atlas.sh from the repo root.\n"
		if code != 1 || f.ran(t) != "" || stderr.String() != want {
			t.Fatalf("reported %q: code=%d ran=%q stderr=%q", reported, code, f.ran(t), stderr.String())
		}
	}
}

func TestMigrate_explainsTheFixWhenThePinnedAtlasIsMissing(t *testing.T) {
	t.Parallel()
	f := newFakeAtlas(t, "atlas community version v1.3.0", "0")
	missing := filepath.Join(t.TempDir(), "atlas")

	var stdout, stderr bytes.Buffer
	code := migrateTool(atlas{missing, f.versionFile}, validMigrateEnviron())([]string{"status"}, &stdout, &stderr)

	want := "monacoctl: " + missing + " is \"\", want \"atlas community version v1.3.0\". " +
		"Run scripts/install-atlas.sh from the repo root.\n"
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
		f := newFakeAtlas(t, "atlas community version v1.3.0", "0")

		var stdout, stderr bytes.Buffer
		code := migrateTool(atlas{f.bin, f.versionFile}, validMigrateEnviron())(tc.args, &stdout, &stderr)

		if code != 2 || f.ran(t) != "" || stderr.String() != tc.want {
			t.Fatalf("args %q: code=%d stderr=%q", tc.args, code, stderr.String())
		}
	}
}
