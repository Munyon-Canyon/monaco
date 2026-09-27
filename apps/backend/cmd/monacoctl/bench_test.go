package main

import (
	"bytes"
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
		if code := benchCmd(args, &stdout, &stderr); code != 2 || stderr.String() != benchUsage+"\n" {
			t.Fatalf("bench %v = %d, stderr %q", args, code, stderr.String())
		}
	}
}
