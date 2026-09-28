package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestMain(m *testing.M) {
	testkit.Main(m, testkit.NoDB(), testkit.WithChild(main))
}

func TestMain_reportsGoStatementsOnlyOutsideCmdAndPlatform(t *testing.T) {
	t.Parallel()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	spawn := "package p\n\nfunc F() {\n\tgo func() {}()\n}\n"
	for rel, src := range map[string]string{
		"go.mod":                           "module github.com/monaco/monaco/apps/backend\n\ngo 1.24\n",
		"cmd/tool/main.go":                 "package main\n\nfunc main() {\n\tgo func() {}()\n}\n",
		"internal/platform/pool/p.go":      spawn,
		"internal/modules/treasury/p.go":   spawn,
		"internal/modules/ranking/idle.go": "package p\n\nfunc F() {}\n",
	} {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cmd := testkit.MainCommand(t, os.Environ(), "./...")
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	_ = cmd.Run()
	want := filepath.Join(dir, "internal/modules/treasury/p.go") +
		":4:2: bare go statement: use platform/concurrency or errgroup\n"
	if code := cmd.ProcessState.ExitCode(); code != 3 || stderr.String() != want || stdout.Len() != 0 {
		t.Fatalf("nogo = %d stdout=%q stderr=%q, want 3 and %q", code, stdout.String(), stderr.String(), want)
	}
}

func TestMain_reportsEveryTestMainThatIsNotOneTestkitMainCallInMainTest(t *testing.T) {
	t.Parallel()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	testMain := func(pkg, body string) string {
		return "package " + pkg + "\n\nimport (\n\t\"testing\"\n\n" +
			"\t\"github.com/monaco/monaco/apps/backend/internal/testkit\"\n)\n\n" +
			"var _ = testkit.Main\n\nfunc TestMain(m *testing.M) {\n" + body + "}\n"
	}
	for rel, src := range map[string]string{
		"go.mod": "module github.com/monaco/monaco/apps/backend\n\ngo 1.24\n",
		"internal/testkit/main.go": "package testkit\n\nimport \"testing\"\n\n" +
			"func Main(m *testing.M, _ ...int) { m.Run() }\n\nfunc Other(m *testing.M) { m.Run() }\n",
		"internal/good/main_test.go":     testMain("good", "\ttestkit.Main(m, 1)\n"),
		"internal/misplaced/x_test.go":   testMain("misplaced", "\ttestkit.Main(m)\n"),
		"internal/twostmts/main_test.go": testMain("twostmts", "\ttestkit.Main(m)\n\ttestkit.Main(m)\n"),
		"internal/ret/main_test.go":      testMain("ret", "\treturn\n"),
		"internal/notcall/main_test.go":  testMain("notcall", "\t_ = m\n"),
		"internal/noargs/main_test.go": "package noargs\n\nimport \"testing\"\n\nfunc run() {}\n\n" +
			"func TestMain(m *testing.M) {\n\trun()\n}\n",
		"internal/other/main_test.go": testMain("other", "\ttestkit.Other(m)\n"),
		"internal/notm/main_test.go":  testMain("notm", "\ttestkit.Main(nil)\n"),
		"internal/lit/main_test.go":   testMain("lit", "\tfunc(*testing.M) {}(m)\n"),
		"internal/helpers/main_test.go": "package helpers\n\nimport \"testing\"\n\ntype r struct{}\n\n" +
			"func (r) TestMain(m *testing.M) { m.Run() }\n\nfunc TestMain_x(t *testing.T) {}\n\nvar TestMainVar = 1\n",
	} {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cmd := testkit.MainCommand(t, os.Environ(), "./...")
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	_ = cmd.Run()
	body := "TestMain body must be exactly testkit.Main(m, opts...)"
	want := make([]string, 0, 8)
	for _, pkg := range []string{"lit", "notcall", "notm", "other", "ret", "twostmts"} {
		want = append(want, filepath.Join(dir, "internal", pkg, "main_test.go")+":11:1: "+body)
	}
	want = append(want,
		filepath.Join(dir, "internal/misplaced/x_test.go")+":11:1: TestMain belongs in main_test.go, one per package",
		filepath.Join(dir, "internal/noargs/main_test.go")+":7:1: "+body,
	)
	got := strings.Split(strings.TrimSpace(stderr.String()), "\n")
	slices.Sort(got)
	slices.Sort(want)
	if code := cmd.ProcessState.ExitCode(); code != 3 || !slices.Equal(got, want) {
		t.Fatalf("nogo = %d\n%s\nwant 3 and\n%s", code, strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for rel, src := range files {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func nogoStderr(t *testing.T, dir string, args ...string) (int, string) {
	t.Helper()
	cmd := testkit.MainCommand(t, os.Environ(), append(args, "./...")...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	_ = cmd.Run()
	if stdout.Len() != 0 {
		t.Fatalf("stdout %q", stdout.String())
	}
	return cmd.ProcessState.ExitCode(), stderr.String()
}

func TestWallclock_reportsStdlibTimeOnlyOutsideSynctestAndTheTestkitClock(t *testing.T) {
	t.Parallel()
	dir := writeTree(t, map[string]string{
		"go.mod": "module github.com/monaco/monaco/apps/backend\n\ngo 1.25\n",
		"internal/testkit/clock.go": "package testkit\n\nimport \"time\"\n\ntype Clock struct{}\n\n" +
			"func NewClock(time.Time) *Clock { return &Clock{} }\n\nfunc (c *Clock) Now() time.Time { return time.Time{} }\n",
		"internal/prod/p.go": "package prod\n\nimport \"time\"\n\nfunc F() { time.Sleep(time.Second) }\n",
		"internal/bubble/bubble_test.go": "package bubble\n\nimport (\n\t\"testing\"\n\t\"testing/synctest\"\n\t\"time\"\n)\n\n" +
			"func TestIn(t *testing.T) {\n\tsynctest.Test(t, func(t *testing.T) {\n\t\ttime.Sleep(time.Second)\n" +
			"\t\t_ = time.Now()\n\t\t_ = time.After(time.Second)\n\t\ttime.NewTicker(time.Second).Stop()\n" +
			"\t\tsynctest.Wait()\n\t\t_ = len(\"x\")\n\t\t_ = new(int)\n\t})\n}\n\nfunc TestOut(t *testing.T) {\n\ttime.Sleep(time.Millisecond)\n}\n",
		"internal/clocked/clock_test.go": "package clocked\n\nimport (\n\t\"testing\"\n\t\"time\"\n\n" +
			"\t\"github.com/monaco/monaco/apps/backend/internal/testkit\"\n)\n\n" +
			"func TestClock(t *testing.T) {\n\tc := testkit.NewClock(time.Time{})\n\t_ = c.Now()\n\t_ = time.Now()\n}\n\n" +
			"func helper() { time.Now() }\n\nfunc takes(c *testkit.Clock) { _ = time.Now(); _ = c }\n",
		"internal/plain/plain_test.go": "package plain\n\nimport (\n\t\"testing\"\n\t\"time\"\n)\n\nvar _ = time.Now()\n\n" +
			"func TestWall(t *testing.T) {\n\ttime.Sleep(0)\n\t_ = time.After(0)\n\ttime.NewTicker(time.Second).Stop()\n" +
			"\t_ = time.Now()\n\t_ = time.Since(time.Time{})\n\tstart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)\n" +
			"\t_ = start.After(start)\n}\n",
		"internal/dot/dot_test.go": "package dot\n\nimport (\n\t\"testing\"\n\t. \"time\"\n)\n\nfunc TestDot(t *testing.T) { Sleep(0) }\n",
		"internal/testkit/db_test.go": "package testkit_test\n\nimport (\n\t\"testing\"\n\t\"time\"\n)\n\n" +
			"func TestAllowed(t *testing.T) { time.Sleep(time.Second) }\n",
	})
	code, stderr := nogoStderr(t, dir)
	msg := "wall clock time.%s: use testing/synctest or the testkit clock"
	want := []string{
		filepath.Join(dir, "internal/bubble/bubble_test.go") + ":22:2: " + fmt.Sprintf(msg, "Sleep"),
		filepath.Join(dir, "internal/clocked/clock_test.go") + ":16:17: " + fmt.Sprintf(msg, "Now"),
		filepath.Join(dir, "internal/dot/dot_test.go") + ":8:30: " + fmt.Sprintf(msg, "Sleep"),
		filepath.Join(dir, "internal/plain/plain_test.go") + ":8:9: " + fmt.Sprintf(msg, "Now"),
		filepath.Join(dir, "internal/plain/plain_test.go") + ":11:2: " + fmt.Sprintf(msg, "Sleep"),
		filepath.Join(dir, "internal/plain/plain_test.go") + ":12:6: " + fmt.Sprintf(msg, "After"),
		filepath.Join(dir, "internal/plain/plain_test.go") + ":13:2: " + fmt.Sprintf(msg, "NewTicker"),
		filepath.Join(dir, "internal/plain/plain_test.go") + ":14:6: " + fmt.Sprintf(msg, "Now"),
	}
	got := strings.Split(strings.TrimSpace(stderr), "\n")
	slices.Sort(got)
	slices.Sort(want)
	if code != 3 || !slices.Equal(got, want) {
		t.Fatalf("nogo = %d\n%s\nwant 3 and\n%s", code, strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
