package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const coverModule = "github.com/monaco/monaco/apps/backend"

func coverDir(t *testing.T, profiles map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":        "// generated\nmodule " + coverModule + "\n\ngo 1.25.0\n",
		coverageExclude: "*.gen.go\ninternal/platform/db/sqlc/\ninternal/testkit/\n",
	}
	for name, body := range profiles {
		files[name] = body
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const unitProfile = `mode: atomic
github.com/monaco/monaco/apps/backend/internal/a/a.go:10.2,12.3 2 0
github.com/monaco/monaco/apps/backend/internal/a/a.go:3.1,4.2 1 5
github.com/monaco/monaco/apps/backend/internal/a/a.go:20.2,20.9 1 0
github.com/monaco/monaco/apps/backend/internal/a/api.gen.go:1.1,2.2 7 0
github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc/q.go:1.1,2.2 3 0
github.com/monaco/monaco/apps/backend/internal/testkit/db.go:1.1,2.2 4 0
github.com/monaco/monaco/apps/backend/cmd/api/main.go:5.2,6.3 2 0
`

const e2eProfile = `mode: set
github.com/monaco/monaco/apps/backend/internal/a/a.go:20.2,20.9 1 1
github.com/monaco/monaco/apps/backend/cmd/api/main.go:5.2,6.3 2 0
`

func TestCheckCoverageNamesEachUncoveredBlockAndMergesProfiles(t *testing.T) {
	t.Parallel()
	dir := coverDir(t, map[string]string{"unit.out": unitProfile, "e2e.out": e2eProfile})
	var out bytes.Buffer
	missed, err := checkCoverage(
		dir,
		[]string{filepath.Join(dir, "unit.out"), filepath.Join(dir, "e2e.out")},
		nil,
		&out,
	)
	want := "cmd/api/main.go:5-6: 2 statements not covered\n" +
		"internal/a/a.go:10-12: 2 statements not covered\n" +
		"coverage: 33.33% of 6 statements\n"
	if err != nil || missed != 4 || out.String() != want {
		t.Fatalf("missed=%d err=%v out:\n%s\nwant:\n%s", missed, err, out.String(), want)
	}
}

func TestCheckCoverageKeepsABlockCoveredWhicheverProfileHitIt(t *testing.T) {
	t.Parallel()
	dir := coverDir(t, map[string]string{"unit.out": unitProfile, "e2e.out": e2eProfile})
	var out bytes.Buffer
	if _, err := checkCoverage(
		dir,
		[]string{filepath.Join(dir, "e2e.out"), filepath.Join(dir, "unit.out")},
		nil,
		&out,
	); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "a.go:20") {
		t.Fatalf("a.go:20 was hit by the e2e profile read first, but reported:\n%s", out.String())
	}
}

func TestCheckCoverageIsCleanAtFullCoverage(t *testing.T) {
	t.Parallel()
	dir := coverDir(t, map[string]string{"c.out": "mode: set\n\n" + coverModule + "/internal/a/a.go:1.1,2.2 3 1\n"})
	var out bytes.Buffer
	missed, err := checkCoverage(dir, []string{filepath.Join(dir, "c.out")}, nil, &out)
	if err != nil || missed != 0 || out.String() != "coverage: 100.00% of 3 statements\n" {
		t.Fatalf("missed=%d err=%v out=%q", missed, err, out.String())
	}
	out.Reset()
	empty := coverDir(t, map[string]string{"c.out": "mode: set\n"})
	if missed, err := checkCoverage(
		empty,
		[]string{filepath.Join(empty, "c.out")},
		nil,
		&out,
	); err != nil ||
		missed != 0 ||
		out.String() != "coverage: 100.00% of 0 statements\n" {
		t.Fatalf("empty profile: missed=%d err=%v out=%q", missed, err, out.String())
	}
}

func TestCheckCoverageRejectsBrokenInputs(t *testing.T) {
	t.Parallel()
	good := coverModule + "/a.go:1.1,2.2 1 1"
	for _, tc := range []struct {
		name  string
		setup func(dir string) error
		code  errs.Code
	}{
		{"no go.mod", func(dir string) error { return os.Remove(filepath.Join(dir, "go.mod")) }, errs.CodeInternal},
		{"no module line", func(dir string) error { return os.WriteFile(filepath.Join(dir, "go.mod"), []byte("go 1.25\n"), 0o600) }, errs.CodeInternal},
		{"no exclude file", func(dir string) error { return os.Remove(filepath.Join(dir, coverageExclude)) }, errs.CodeInternal},
		{"no profile", func(dir string) error { return os.Remove(filepath.Join(dir, "c.out")) }, errs.CodeInternal},
		{"profile is a directory", func(dir string) error {
			if err := os.Remove(filepath.Join(dir, "c.out")); err != nil {
				return err
			}
			return os.Mkdir(filepath.Join(dir, "c.out"), 0o700)
		}, errs.CodeInternal},
		{"no counts", writeProfile(coverModule + "/a.go:1.1,2.2"), errs.CodeDecodeFailed},
		{"no span", writeProfile(coverModule + "/a.go 1 1"), errs.CodeDecodeFailed},
		{"one-sided span", writeProfile(coverModule + "/a.go:1.1 1 1"), errs.CodeDecodeFailed},
		{"no count", writeProfile(coverModule + "/a.go:1.1,2.2 1"), errs.CodeDecodeFailed},
		{"bad start", writeProfile(coverModule + "/a.go:x.1,2.2 1 1"), errs.CodeDecodeFailed},
		{"bad end", writeProfile(coverModule + "/a.go:1.1,y.2 1 1"), errs.CodeDecodeFailed},
		{"bad stmts", writeProfile(coverModule + "/a.go:1.1,2.2 z 1"), errs.CodeDecodeFailed},
		{"bad count", writeProfile(coverModule + "/a.go:1.1,2.2 1 z"), errs.CodeDecodeFailed},
		{"line too long", writeProfile(good + strings.Repeat(" ", 70_000)), errs.CodeInternal},
	} {
		dir := coverDir(t, map[string]string{"c.out": "mode: set\n" + good + "\n"})
		if err := tc.setup(dir); err != nil {
			t.Fatal(err)
		}
		_, err := checkCoverage(dir, []string{filepath.Join(dir, "c.out")}, nil, &bytes.Buffer{})
		if err == nil || errs.CodeOf(err) != tc.code {
			t.Fatalf("%s: err = %v, want code %s", tc.name, err, tc.code)
		}
	}
}

func writeProfile(line string) func(dir string) error {
	return func(dir string) error {
		return os.WriteFile(filepath.Join(dir, "c.out"), []byte("mode: set\n"+line+"\n"), 0o600)
	}
}

func TestExcludedMatchesBaseNameGlobsAndDirectoryPrefixes(t *testing.T) {
	t.Parallel()
	patterns := []string{"*.gen.go", "internal/testkit/", "[", "internal/*.go"}
	for rel, want := range map[string]bool{
		"internal/platform/httpx/api/api.gen.go": true,
		"internal/testkit/fakes/fakes.go":        true,
		"internal/testkitx/a.go":                 false,
		"internal/platform/a.go":                 false,
		"internal/a.go":                          false,
	} {
		if got := excluded(rel, patterns); got != want {
			t.Fatalf("excluded(%q) = %v, want %v", rel, got, want)
		}
	}
}

func TestCoverageCommand(t *testing.T) {
	t.Parallel()
	dir := coverDir(
		t,
		map[string]string{
			"unit.out": unitProfile,
			"full.out": "mode: set\n" + coverModule + "/a.go:1.1,2.2 1 1\n",
			"mixed.out": "mode: set\n" + coverModule + "/internal/b/b.go:1.1,2.2 2 1\n" +
				coverModule + "/cmd/api/main.go:5.2,6.3 2 0\n",
		},
	)
	for _, tc := range []struct {
		name   string
		args   []string
		code   int
		stdout string
		stderr string
	}{
		{"usage", nil, 2, "", coverageUsage + "\n"},
		{"positional", []string{"--profile", "x", "y"}, 2, "", coverageUsage + "\n"},
		{"full", []string{"--profile", filepath.Join(dir, "full.out")}, 0, "coverage: 100.00% of 1 statements\n", ""},
		{
			"gaps",
			[]string{"--profile", filepath.Join(dir, "unit.out")},
			1, "a.go:20-20: 1 statements not covered\ncoverage: 16.67% of 6 statements\n",
			"monacoctl coverage: 5 statements uncovered, the gate is 100%\n",
		},
		{
			"only a changed file with gaps",
			[]string{"--profile", filepath.Join(dir, "unit.out"), "--only", "internal/a/a.go"},
			1,
			"internal/a/a.go:10-12: 2 statements not covered\n" +
				"internal/a/a.go:20-20: 1 statements not covered\ncoverage: 25.00% of 4 statements\n",
			"monacoctl coverage: 3 statements uncovered, the gate is 100%\n",
		},
		{
			"only a covered file beside an uncovered one",
			[]string{"--profile", filepath.Join(dir, "mixed.out"), "--only", "internal/b/b.go"},
			0, "coverage: 100.00% of 2 statements\n", "",
		},
		{
			"only excluded files",
			[]string{
				"--profile", filepath.Join(dir, "unit.out"),
				"--only", "internal/a/api.gen.go", "--only", "internal/testkit/db.go",
			},
			0, "coverage: 100.00% of 0 statements\n", "",
		},
		{
			"missing profile",
			[]string{"--profile", filepath.Join(dir, "gone.out")},
			1, "",
			"monacoctl coverage: monacoctl.readProfile: internal: open " + filepath.Join(dir, "gone.out") + ": no such file or directory\n",
		},
		{
			"covdir merged",
			[]string{"--profile", filepath.Join(dir, "full.out"), "--covdir", filepath.Join(dir, "e2e")},
			1,
			"cmd/api/main.go:7-8: 1 statements not covered\ncoverage: 66.67% of 3 statements\n",
			"monacoctl coverage: 1 statements uncovered, the gate is 100%\n",
		},
		{
			"covdata fails",
			[]string{"--profile", filepath.Join(dir, "full.out"), "--covdir", filepath.Join(dir, "missing")},
			1, "",
			"monacoctl coverage: monacoctl.covdataText: internal: exit status 1: covdata: missing input\n",
		},
	} {
		var stdout, stderr bytes.Buffer
		code := fakeGoEnv(t, dir).run(tc.args, &stdout, &stderr)
		if code != tc.code || !strings.HasSuffix(stdout.String(), tc.stdout) || stderr.String() != tc.stderr {
			t.Fatalf("%s: code=%d stdout=%q stderr=%q", tc.name, code, stdout.String(), stderr.String())
		}
	}
}

func fakeGoEnv(t *testing.T, dir string) coverageEnv {
	t.Helper()
	goBin, err := filepath.Abs(filepath.Join("testdata", "go-covdata"))
	if err != nil {
		t.Fatal(err)
	}
	return coverageEnv{moduleDir: dir, goBin: goBin, tmpDir: t.TempDir()}
}

func TestCoverageReportsATempDirItCannotWrite(t *testing.T) {
	t.Parallel()
	dir := coverDir(t, map[string]string{"full.out": "mode: set\n"})
	env := fakeGoEnv(t, dir)
	env.tmpDir = filepath.Join(env.tmpDir, "missing")
	var stdout, stderr bytes.Buffer
	code := env.run([]string{"--profile", filepath.Join(dir, "full.out"), "--covdir", dir}, &stdout, &stderr)
	if code != 1 || !strings.HasPrefix(stderr.String(), "monacoctl coverage: monacoctl.covdataText: internal: open ") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestCoverageMergesACoverBinarysGOCOVERDIR(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds a -cover binary; CI runs it without -short, outside the 10 s package budget")
	}
	tmp := t.TempDir()
	bin, covdir := filepath.Join(tmp, "covered"), filepath.Join(tmp, "covdata")
	if err := os.Mkdir(covdir, 0o700); err != nil {
		t.Fatal(err)
	}
	build := exec.CommandContext(context.Background(), "go", "build", "-cover", "-o", bin, "./testdata/covered")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build -cover: %v\n%s", err, out)
	}
	run := exec.CommandContext(context.Background(), bin)
	run.Env = append(os.Environ(), "GOCOVERDIR="+covdir)
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("covered binary: %v\n%s", err, out)
	}
	dir := coverDir(t, map[string]string{"unit.out": "mode: set\n"})
	var stdout, stderr bytes.Buffer
	code := coverageEnv{moduleDir: dir, goBin: "go", tmpDir: t.TempDir()}.run(
		[]string{"--profile", filepath.Join(dir, "unit.out"), "--covdir", covdir},
		&stdout,
		&stderr,
	)
	prefix, suffix := "cmd/monacoctl/testdata/covered/main.go:", "-8: 1 statements not covered\ncoverage: 50.00% of 2 statements\n"
	if code != 1 || !strings.HasPrefix(stdout.String(), prefix) || !strings.HasSuffix(stdout.String(), suffix) {
		t.Fatalf(
			"code=%d stdout=%q stderr=%q, want the unreached os.Exit named",
			code,
			stdout.String(),
			stderr.String(),
		)
	}
}
