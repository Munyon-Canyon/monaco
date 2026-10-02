package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func event(action, pkg, test, elapsed string) string {
	line := `{"Time":"2026-09-27T10:00:00Z","Action":"` + action + `","Package":"` + pkg + `"`
	if test != "" {
		line += `,"Test":"` + test + `"`
	}
	return line + `,"Elapsed":` + elapsed + "}\n"
}

func TestTestShardsUpdateWritesTheSlowestRunPerPackageSortedByName(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	short := writeTestFile(t, dir, "short.json",
		event("pass", "m/b", "", "2.25")+event("pass", "m/a", "TestX", "9")+
			event("pass", "m/a", "", "1.04")+event("skip", "m/c", "", "0"))
	full := writeTestFile(t, dir, "full.json", event("fail", "m/a", "", "30"))
	out := filepath.Join(dir, "table.tsv")
	var stdout, stderr bytes.Buffer
	code := testShardsCmd([]string{"update", "--out", out, "--from", short, full}, &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 || stdout.String() != "wrote 3 packages to "+out+"\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	got, err := os.ReadFile(filepath.Clean(out))
	if err != nil {
		t.Fatal(err)
	}
	if want := "m/a\t30.0\nm/b\t2.2\nm/c\t0.0\n"; string(got) != want {
		t.Fatalf("table = %q, want %q", got, want)
	}
}

func TestTestShardsUpdateRejectsBadInput(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	empty := writeTestFile(t, dir, "empty.json", "not json\n")
	ok := writeTestFile(t, dir, "ok.json", event("pass", "m/a", "", "1"))
	gone := filepath.Join(dir, "gone.json")
	for _, tc := range []struct {
		name   string
		args   []string
		code   int
		stderr string
	}{
		{"no subcommand", nil, 2, testShardsUsage + "\n"},
		{"unknown subcommand", []string{"show"}, 2, testShardsUsage + "\n"},
		{"no from", []string{"update"}, 2, testShardsUsage + "\n"},
		{"unknown flag", []string{"update", "--bogus"}, 2, testShardsUsage + "\n"},
		{"missing file", []string{"update", "--from", gone}, 1, "monacoctl test-shards: monacoctl.readReport: internal: open " + gone + ": no such file or directory"},
		{"no packages", []string{"update", "--from", empty}, 1, "monacoctl test-shards: monacoctl.testShards: invalid_input"},
		{"unwritable table", []string{"update", "--out", dir, "--from", ok}, 1, "monacoctl test-shards: "},
	} {
		var stdout, stderr bytes.Buffer
		code := testShardsCmd(tc.args, &stdout, &stderr)
		if code != tc.code || stdout.Len() != 0 || !strings.HasPrefix(stderr.String(), tc.stderr) {
			t.Fatalf("%s: code=%d stdout=%q stderr=%q", tc.name, code, stdout.String(), stderr.String())
		}
	}
}
