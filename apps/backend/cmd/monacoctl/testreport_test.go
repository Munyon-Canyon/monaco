package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const reportStream = `{"Time":"2026-09-27T10:00:05Z","Action":"start","Package":"m/a"}
{"ImportPath":"m/b","Action":"build-output","Output":"compiling\n"}
not json
{"Time":"2026-09-27T10:00:06Z","Action":"pass","Package":"m/a","Test":"TestFast","Elapsed":0.01}
{"Time":"2026-09-27T10:00:07Z","Action":"pass","Package":"m/a","Test":"TestSlow/sub","Elapsed":2.5}
{"Time":"2026-09-27T10:00:07Z","Action":"pass","Package":"m/a","Test":"TestSlow","Elapsed":3}
{"Time":"2026-09-27T10:00:08Z","Action":"fail","Package":"m/b","Test":"TestBroken","Elapsed":1.25}
{"Time":"2026-09-27T10:00:09Z","Action":"pass","Package":"m/a","Elapsed":4.2}
{"Time":"2026-09-27T10:00:20Z","Action":"fail","Package":"m/b","Elapsed":11}
{"Time":"2026-09-27T10:00:20Z","Action":"skip","Package":"m/c","Elapsed":0}
`

func TestReadReportRanksTopLevelTestsAndPackagesByElapsed(t *testing.T) {
	t.Parallel()
	rep, err := readReport(strings.NewReader(reportStream), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	rep.write(&out, budget{pkg: 10 * time.Second, run: 60 * time.Second})
	want := `slowest tests:
   3.00s  m/a TestSlow
   1.25s  m/b TestBroken
   0.01s  m/a TestFast
packages:
  11.00s  m/b
   4.20s  m/a
   0.00s  m/c
run: 15.0s (budget 60s, 10s per package)
`
	if out.String() != want {
		t.Fatalf("report:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestReadReportMeasuresTheRunFromTheGivenStart(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 9, 27, 9, 59, 30, 0, time.UTC)
	rep, err := readReport(strings.NewReader(reportStream), start)
	if err != nil || rep.run != 50*time.Second {
		t.Fatalf("run = %v, %v; want 50s from the start flag to the last event", rep.run, err)
	}
}

func TestReadReportCountsAMissingElapsedAsZeroAndFailsOnAReadError(t *testing.T) {
	t.Parallel()
	rep, err := readReport(
		strings.NewReader(`{"Time":"2026-09-27T10:00:00Z","Action":"pass","Package":"m"}`),
		time.Time{},
	)
	if err != nil || len(rep.packages) != 1 || rep.packages[0].elapsed != 0 {
		t.Fatalf("report = %+v, %v; want one package at 0s", rep, err)
	}
	if _, err := readReport(iotest.ErrReader(io.ErrUnexpectedEOF), time.Time{}); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("read error = %v, want an internal error", err)
	}
}

func TestReportWritesOnlyTheTenSlowestTests(t *testing.T) {
	t.Parallel()
	var in strings.Builder
	for i := range 12 {
		in.WriteString(
			`{"Time":"2026-09-27T10:00:00Z","Action":"pass","Package":"m","Test":"T` + string(
				rune('a'+i),
			) + `","Elapsed":` + string(
				rune('1'+i%9),
			) + "}\n",
		)
	}
	rep, err := readReport(strings.NewReader(in.String()), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	rep.write(&out, budget{pkg: time.Second, run: time.Second})
	if got := strings.Count(out.String(), "  m T"); got != 10 {
		t.Fatalf("listed %d tests, want 10:\n%s", got, out.String())
	}
}

func TestReportOverBudgetNamesEachSlowPackageAndTheRun(t *testing.T) {
	t.Parallel()
	rep, err := readReport(strings.NewReader(reportStream), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		b    budget
		want []string
	}{
		{"within", budget{pkg: 11 * time.Second, run: 15 * time.Second}, nil},
		{"package over", budget{pkg: 4 * time.Second, run: time.Minute}, []string{
			"package m/b took 11.00s, over the 4s per-package budget",
			"package m/a took 4.20s, over the 4s per-package budget",
		}},
		{"run over", budget{pkg: time.Minute, run: 14 * time.Second}, []string{"run took 15.0s, over the 14s budget"}},
	} {
		got := rep.overBudget(tc.b)
		if strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
			t.Fatalf("%s: overBudget = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestTestReportCommand(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "go-test.json")
	if err := os.WriteFile(file, []byte(reportStream), 0o600); err != nil {
		t.Fatal(err)
	}
	early := strconv.FormatInt(time.Date(2026, 9, 27, 9, 58, 20, 0, time.UTC).Unix(), 10)
	fast := filepath.Join(t.TempDir(), "fast.json")
	if err := os.WriteFile(
		fast,
		[]byte(`{"Time":"2026-09-27T10:00:00Z","Action":"pass","Package":"m/a","Elapsed":1}`+"\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	slowB := "monacoctl test-report: package m/b took 11.00s, over the 10s per-package budget\n"
	for _, tc := range []struct {
		name      string
		args      []string
		code      int
		stdoutHas string
		stderr    string
	}{
		{"within budget", []string{"--from", fast}, 0, "   1.00s  m/a\nrun: 0.0s", ""},
		{"usage", nil, 2, "", testReportUsage + "\n"},
		{"unknown flag", []string{"--from", file, "--bogus"}, 2, "", testReportUsage + "\n"},
		{"bad start", []string{"--from", file, "--start", "soon"}, 2, "", testReportUsage + "\n"},
		{"missing file", []string{"--from", file + ".gone"}, 1, "", "monacoctl test-report: open " + file + ".gone: no such file or directory\n"},
		{"directory", []string{"--from", filepath.Dir(file)}, 1, "", "monacoctl test-report: monacoctl.readReport: internal: read " + filepath.Dir(file) + ": is a directory\n"},
		{"package over budget", []string{"--from", file}, 1, "run: 15.0s", slowB},
		{"run over budget", []string{"--start", early, "--from", file}, 1, "run: 120.0s", slowB + "monacoctl test-report: run took 120.0s, over the 60s budget\n"},
		{"run not gated in CI", []string{"--start", early, "--from", file, "--ci"}, 0, "run: 120.0s (not gated in CI; the 60s budget is for a laptop), 15s per package\n", ""},
	} {
		var stdout, stderr bytes.Buffer
		code := testReportCmd(tc.args, &stdout, &stderr)
		if code != tc.code || !strings.Contains(stdout.String(), tc.stdoutHas) || stderr.String() != tc.stderr {
			t.Fatalf("%s: code=%d stdout=%q stderr=%q", tc.name, code, stdout.String(), stderr.String())
		}
	}
}

func TestTestReportCIWarnsFromTenSecondsAndFailsPastFifteen(t *testing.T) {
	t.Parallel()
	warn := "::warning::monacoctl test-report: package m/p took 12.00s, over the 10s per-package budget (CI fails at 15s)\n"
	for _, tc := range []struct {
		name    string
		elapsed string
		ci      bool
		code    int
		warning string
		stderr  string
	}{
		{"ci 9s is quiet", "9", true, 0, "", ""},
		{"ci 12s warns", "12", true, 0, warn, ""},
		{"ci 16s fails", "16", true, 1, "", "monacoctl test-report: package m/p took 16.00s, over the 15s per-package budget\n"},
		{"local 12s fails", "12", false, 1, "", "monacoctl test-report: package m/p took 12.00s, over the 10s per-package budget\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			file := filepath.Join(t.TempDir(), "go-test.json")
			event := `{"Time":"2026-09-27T10:00:00Z","Action":"pass","Package":"m/p","Elapsed":` + tc.elapsed + "}\n"
			if err := os.WriteFile(file, []byte(event), 0o600); err != nil {
				t.Fatal(err)
			}
			args := []string{"--from", file}
			if tc.ci {
				args = append(args, "--ci")
			}
			var stdout, stderr bytes.Buffer
			code := testReportCmd(args, &stdout, &stderr)
			warned := strings.Contains(stdout.String(), "::warning::")
			if code != tc.code || warned != (tc.warning != "") || !strings.HasSuffix(stdout.String(), tc.warning) ||
				stderr.String() != tc.stderr {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			t.Logf("exit %d\nstdout:\n%sstderr:\n%s", code, stdout.String(), stderr.String())
		})
	}
}
