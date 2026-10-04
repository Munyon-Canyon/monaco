package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
	codegen "github.com/monaco/monaco/apps/backend/internal/tools/gen"
)

func TestGen_otherArgsPrintUsage(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{nil, {"errors"}, {"openapi", "a", "b"}, {"flows"}, {"apis", "a", "b"}} {
		var stdout, stderr bytes.Buffer
		if code := toolGen(toolEnv{})(args, &stdout, &stderr); code != 2 ||
			!strings.HasPrefix(stderr.String(), "usage: monacoctl gen module <name>\n") ||
			!strings.Contains(stderr.String(), "\n       monacoctl gen migration <module> <name>") {
			t.Fatalf("gen %v = %d %q, want 2 and usage", args, code, stderr.String())
		}
	}
}

func TestGen_scaffoldsIntoTheWorkingDirAndPrintsWhatItWrote(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "apps", "backend")
	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gen := toolGen(toolEnv{wd: root})
	var stdout, stderr bytes.Buffer
	if code := gen([]string{"provider", "quotes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("gen provider = %d, stderr %q", code, stderr.String())
	}
	if !strings.HasPrefix(stdout.String(), "internal/providers/quotes/client.go\n") ||
		strings.Count(stdout.String(), "\n") != 6 {
		t.Fatalf("stdout = %q, want the six written files", stdout.String())
	}
	stdout.Reset()
	if code := gen([]string{"flow", "1"}, &stdout, &stderr); code != 1 || stdout.Len() != 0 ||
		!strings.HasPrefix(
			stderr.String(),
			"monacoctl: gen.planFlow",
		) || !strings.Contains(stderr.String(), flows.Dir) {
		t.Fatalf("gen flow without flow files = %d %q %q, want 1 and the error", code, stdout.String(), stderr.String())
	}
	stderr.Reset()
	if code := gen(
		[]string{"nope"},
		&stdout,
		&stderr,
	); code != 2 ||
		!strings.Contains(stderr.String(), "monacoctl gen flow <id>") {
		t.Fatalf("gen nope = %d %q, want 2 and usage", code, stderr.String())
	}
}

func TestGen_flowWithALetterSuffixWritesItsTestFileIntoTheModule(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	row := "01a\tSet handle\tidentity\tPOST /v1/me/handle\tSetHandle\t\t\tok;InvalidInput;crash:before-commit\tplanned\tdocs/x.md\n"
	for rel, body := range map[string]string{
		"apps/backend/go.mod":                              "module example.com/app\n",
		"apps/backend/internal/modules/identity/module.go": "package identity\n",
		flows.Dir + "/01a.tsv":                             flows.Header + "\n" + row,
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(repo, rel)), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, rel), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	dir, err := os.OpenRoot(filepath.Join(repo, "apps", "backend"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dir.Close() }()
	var stdout, stderr bytes.Buffer
	if code := toolGen(toolEnv{wd: dir.Name()})([]string{"flow", "01a"}, &stdout, &stderr); code != 0 {
		t.Fatalf("gen flow 01a = %d, stderr %q", code, stderr.String())
	}
	const written = "internal/modules/identity/flow01a_test.go"
	if stdout.String() != written+"\n" {
		t.Fatalf("stdout = %q, want %q", stdout.String(), written+"\n")
	}
	src, err := dir.ReadFile(written)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"TestFlow01a_SetHandle_OK", "TestFlow01a_SetHandle_InvalidInput", "TestFlow01a_SetHandle_CrashBeforeCommit",
	} {
		want := "func " + name + "(t *testing.T) {\n\tt.Parallel()\n\tt.Fatal(\"not implemented\")\n}"
		if !strings.Contains(string(src), want) {
			t.Errorf("%s lacks a failing %s:\n%s", written, name, src)
		}
	}
	if got := strings.Count(string(src), "func Test"); got != 3 {
		t.Errorf("%s has %d tests, want 3", written, got)
	}
}

func TestGen_migrationRoutesToTheMigratorAndReportsItsFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for rel, body := range map[string]string{
		"go.mod":                            "module example.com/app\n",
		"internal/modules/social/module.go": "package social\n",
		"migrations/atlas.sum":              "",
	} {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var stdout, stderr bytes.Buffer
	if code := toolGen(toolEnv{wd: root})([]string{"migration", "social"}, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "gen migration <module> <name>") {
		t.Fatalf("gen migration social = %d %q, want 1 and usage", code, stderr.String())
	}
	stderr.Reset()
	files := codegen.Migrator{
		Dir:    root,
		Now:    func() time.Time { return time.Date(2026, 10, 2, 13, 0, 0, 0, time.UTC) },
		Parent: func(context.Context) ([]string, error) { return nil, nil },
		Hash:   func(context.Context) error { return nil },
	}
	if code := migrate(files, []string{"social", "follows"}, &stdout, &stderr); code != 0 ||
		stdout.String() != "migrations/20261002130000_social_follows.sql\n" {
		t.Fatalf("migrate = %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
	if code := toolGen(toolEnv{wd: root})([]string{"nope"}, &stdout, &stderr); code != 2 ||
		!strings.Contains(stderr.String(), "gen migration --rebase") {
		t.Fatalf("usage lacks the migration line: %q", stderr.String())
	}
}
