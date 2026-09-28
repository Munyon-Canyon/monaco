package main

import (
	"bytes"
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
