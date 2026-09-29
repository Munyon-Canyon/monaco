package main

import (
	"os"
	"path/filepath"
	"testing"
)

func symlinkCommittedScript(t *testing.T, script, bin string) {
	t.Helper()
	target, err := filepath.Abs(filepath.Join("testdata", script))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(bin), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, bin); err != nil {
		t.Fatal(err)
	}
}
