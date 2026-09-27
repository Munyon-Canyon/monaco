package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestMain(m *testing.M) {
	testkit.RunMain(m, main)
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
