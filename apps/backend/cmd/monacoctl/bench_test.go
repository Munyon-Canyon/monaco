package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const benchStream = `{"Action":"run","Test":"TestCloneBench"}
{"Action":"output","Test":"TestCloneBench","Output":"    bench_test.go:31: bench tests=64 phase_ns=1024000000\n"}
{"Action":"pass","Test":"TestCloneBench/clones"}
{"Action":"pass","Test":"TestCloneBench","Elapsed":1.1}
`

func TestBenchReportPrintsAmortizedCost(t *testing.T) {
	t.Parallel()
	got, err := benchReport(strings.NewReader(benchStream))
	want := "bench db: 64 parallel testkit.DB tests in 1.02 s, 16.0 ms per test amortized"
	if err != nil || got != want {
		t.Fatalf("benchReport = %q, %v; want %q", got, err, want)
	}
}

func TestBenchReportRejectsAFailedOrIncompleteRun(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		in   string
		code errs.Code
	}{
		"failed":     {strings.Replace(benchStream, `"pass","Test":"TestCloneBench",`, `"fail","Test":"TestCloneBench",`, 1), errs.CodeInternal},
		"no timing":  {`{"Action":"pass","Test":"TestCloneBench"}`, errs.CodeInternal},
		"not json":   {"FAIL build failed\n", errs.CodeDecodeFailed},
		"huge count": {strings.Replace(benchStream, "tests=64", "tests=99999999999999999999", 1), errs.CodeDecodeFailed},
	} {
		if got, err := benchReport(strings.NewReader(tc.in)); errs.CodeOf(err) != tc.code || err == nil {
			t.Fatalf("%s: benchReport = %q, %v; want code %s", name, got, err, tc.code)
		}
	}
}

func TestBenchRejectsUnknownTargets(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{nil, {"nats"}, {"db", "extra"}} {
		var stdout, stderr bytes.Buffer
		if code := (bench{"go"}).run(args, &stdout, &stderr); code != 2 || stderr.String() != benchUsage+"\n" {
			t.Fatalf("bench %v = %d, stderr %q", args, code, stderr.String())
		}
	}
}

func fakeGo(t *testing.T, stdout, exitCode string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stream"), []byte(stdout), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "go")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" > " + filepath.Join(dir, "args") +
		"\ncat " + filepath.Join(dir, "stream") + "\necho go-err >&2\nexit " + exitCode + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return bin
}

func TestBenchRunsTheCloneBenchAndPrintsItsReport(t *testing.T) {
	t.Parallel()
	bin := fakeGo(t, benchStream, "0")
	var stdout, stderr bytes.Buffer
	code := bench{bin}.run([]string{"db"}, &stdout, &stderr)
	args, err := os.ReadFile(filepath.Join(filepath.Dir(bin), "args"))
	if err != nil {
		t.Fatal(err)
	}
	want := "bench db: 64 parallel testkit.DB tests in 1.02 s, 16.0 ms per test amortized\n"
	if code != 0 || stdout.String() != want || stderr.String() != "go-err\n" ||
		string(args) != "test -count=1 -json -run ^TestCloneBench$ "+benchPkg+"\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q args=%q", code, stdout.String(), stderr.String(), args)
	}
}

func TestBenchFailsWhenGoTestFails(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	code := bench{fakeGo(t, benchStream, "1")}.run([]string{"db"}, &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "monacoctl bench db: exit status 1 <nil>") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
