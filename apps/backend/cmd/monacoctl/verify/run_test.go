package verify

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		args []string
		want Target
		err  string
	}{
		{nil, Target{}, ""},
		{[]string{"all"}, Target{}, ""},
		{[]string{"flow", "00", "--outcome", "InvalidInput"}, Target{Flow: "00", Outcome: "InvalidInput"}, ""},
		{[]string{"all", "--crash-at", "after-publish"}, Target{CrashAt: "after-publish"}, ""},
		{[]string{"flow"}, Target{}, "bad arguments: flow needs a value"},
		{[]string{"--crash-at", "after-lunch"}, Target{}, "bad arguments: --crash-at after-lunch is not a faultpoint"},
		{[]string{"everything"}, Target{}, `bad arguments: unknown argument "everything"`},
	} {
		got, err := ParseArgs(tc.args)
		if tc.err != "" {
			if err == nil || err.Error() != tc.err {
				t.Errorf("ParseArgs(%q) = %v, want %q", tc.args, err, tc.err)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("ParseArgs(%q) = %+v, %v, want %+v", tc.args, got, err, tc.want)
		}
	}
}

func fakeGo(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return writeScript(t, "go", `echo "$@" > "$(dirname "$0")/args"
while [ $# -gt 0 ]; do [ "$1" = "-o" ] && out="$2"; shift; done
for b in api worker fakes; do printf '#!/bin/sh\nexec "%s" "$@"\n' "`+exe+`" > "$out$b"; chmod +x "$out$b"; done
`)
}

func testConfig(t *testing.T, mode string) (Config, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	o := testOptions(t, mode)
	dir := t.TempDir()
	if err := os.Symlink(filepath.Join(o.Dir, "migrations"), filepath.Join(dir, "migrations")); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	return Config{
		Dir: dir, Environ: o.Environ, Go: fakeGo(t), Atlas: o.Atlas,
		Budget: o.Budget, Postgres: o.Postgres, Stdout: &stdout, Stderr: &stderr,
	}, &stdout, &stderr
}

func TestRun_buildsTheBinariesBringsTheStackUpAndTearsItDown(t *testing.T) {
	t.Parallel()
	cfg, stdout, stderr := testConfig(t, "ok")
	if code := Run(t.Context(), cfg, Target{CrashAt: "after-publish"}); code != 0 {
		t.Fatalf("Run = %d\n%s", code, stderr)
	}
	if !strings.Contains(stdout.String(), "healthy: api http://127.0.0.1:") {
		t.Fatalf("stdout = %q", stdout)
	}
	args, err := os.ReadFile(filepath.Join(filepath.Dir(cfg.Go), "args"))
	if err != nil || !strings.HasPrefix(string(args), "build -cover -tags faultpoints -o ") ||
		!strings.Contains(string(args), "./cmd/api ./cmd/worker ./cmd/fakes") {
		t.Fatalf("go args = %q, %v", args, err)
	}
	if _, err := os.Stat(filepath.Join(cfg.Dir, ".verify", "cover")); err != nil {
		t.Fatalf("coverage dir: %v", err)
	}
}

func TestRun_failsWithTheReasonAndExitOne(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		edit func(t *testing.T, cfg *Config)
		want string
	}{
		{"tempdir", func(_ *testing.T, cfg *Config) { cfg.TempDir = "/nonexistent" }, "binaries dir"},
		{
			"build", func(t *testing.T, cfg *Config) {
				t.Helper()
				cfg.Go = writeScript(t, "go", "echo broken; exit 2")
			},
			"go build: exit status 2\nbroken",
		},
		{"image", func(t *testing.T, cfg *Config) {
			t.Helper()
			cfg.Postgres = nil
			cfg.Docker, _ = fakeDocker(t, "exit 0", "exit 0", "exit 1")
		}, "pull failed"},
		{"cover", func(t *testing.T, cfg *Config) {
			t.Helper()
			if err := os.WriteFile(filepath.Join(cfg.Dir, ".verify"), nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}, "coverage dir"},
		{"stack", func(_ *testing.T, cfg *Config) { cfg.Atlas = "/nonexistent/atlas" }, "migrate apply"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg, _, stderr := testConfig(t, "ok")
			tc.edit(t, &cfg)
			if code := Run(t.Context(), cfg, Target{}); code != 1 || !strings.Contains(stderr.String(), tc.want) {
				t.Fatalf("Run = %d, stderr %q, want 1 and %q", code, stderr, tc.want)
			}
		})
	}
}
