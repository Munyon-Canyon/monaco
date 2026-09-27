package main

import (
	"bytes"
	"testing"
	"testing/fstest"

	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

func repoWith(tsv string) fstest.MapFS {
	return fstest.MapFS{
		"apps/backend/flows.tsv":                          {Data: []byte(tsv)},
		"apps/backend/internal/modules/system/app/app.go": {Data: []byte("package app\n")},
		"docs/flows.md":                                   {Data: []byte("## Ping\n")},
	}
}

func TestFlowsCheck_headerOnlyFilePasses(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	if code := flowsCheck(repoWith(flows.Header+"\n"), &stderr); code != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestFlowsCheck_checksAgainstTheLiveEventsRegistryAndErrsTable(t *testing.T) {
	t.Parallel()
	row := "01\tPing\tsystem\tpoller:ping\tPing\tsystem.pinged;system.exploded\t\tok;Internal;NoSuchCode\tplanned\tdocs/flows.md#ping"
	var stderr bytes.Buffer
	code := flowsCheck(repoWith(flows.Header+"\n"+row+"\n"), &stderr)
	want := "flows.tsv:2: event system.exploded is not in the events registry\n" +
		"flows.tsv:2: outcome NoSuchCode is not an errs code name\n"
	if code != 1 || stderr.String() != want {
		t.Fatalf("code=%d stderr=\n%s\nwant\n%s", code, stderr.String(), want)
	}
}

func TestFlowsCheck_missingFileFails(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	if code := flowsCheck(fstest.MapFS{}, &stderr); code != 1 || stderr.Len() == 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}

func TestFlowsRejectsUnknownSubcommands(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"flows"}, {"flows", "lint"}} {
		var stdout, stderr bytes.Buffer
		if code := run(
			commands(), tools(), nil,
			args,
			&stdout,
			&stderr,
		); code != 2 ||
			stderr.String() != "usage: monacoctl flows check\n" {
			t.Fatalf("%q: code=%d stderr=%q", args, code, stderr.String())
		}
	}
}
