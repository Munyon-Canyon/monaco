package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

func flowFS(tsv string) fstest.MapFS {
	repo := fstest.MapFS{}
	header, rows, _ := strings.Cut(tsv, "\n")
	for row := range strings.Lines(rows) {
		id, _, _ := strings.Cut(row, "\t")
		repo[flows.Dir+"/"+strings.TrimSpace(id)+".tsv"] = &fstest.MapFile{Data: []byte(header + "\n" + row)}
	}
	return repo
}

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
