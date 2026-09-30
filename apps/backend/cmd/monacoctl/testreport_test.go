package main

import (
	"bytes"
	"fmt"
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
	rep.write(&out, budget{warn: 10 * time.Second, fail: 20 * time.Second, run: 90 * time.Second})
	want := `slowest tests:
   3.00s  m/a TestSlow
   1.25s  m/b TestBroken
   0.01s  m/a TestFast
packages:
  11.00s  m/b
   4.20s  m/a
   0.00s  m/c
run: 15.0s (budget 90s), packages warn at 10s, fail at 20s
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
	rep.write(&out, budget{fail: time.Second, run: time.Second})
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
		{"within", budget{fail: 11 * time.Second, run: 15 * time.Second}, nil},
		{"package over", budget{fail: 4 * time.Second, run: time.Minute}, []string{
			"package m/b took 11.00s, over the 4s per-package budget",
			"package m/a took 4.20s, over the 4s per-package budget",
		}},
		{"run over", budget{fail: time.Minute, run: 14 * time.Second}, []string{"run took 15.0s, over the 14s budget"}},
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
	seventy := strconv.FormatInt(time.Date(2026, 9, 27, 9, 59, 10, 0, time.UTC).Unix(), 10)
	fast := filepath.Join(t.TempDir(), "fast.json")
	if err := os.WriteFile(
		fast,
		[]byte(`{"Time":"2026-09-27T10:00:00Z","Action":"pass","Package":"m/a","Elapsed":1}`+"\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	warnB := "monacoctl test-report: package m/b took 11.00s, over the 10s per-package budget (fails at 20s)\n"
	failedB := "failed: m/b TestBroken\nfailed: m/b\n"
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
		{"missing file", []string{"--from", file + ".gone"}, 1, "", "monacoctl test-report: monacoctl.readReport: internal: open " + file + ".gone: no such file or directory\n"},
		{"directory", []string{"--from", filepath.Dir(file)}, 1, "", "monacoctl test-report: monacoctl.readReport: internal: read " + filepath.Dir(file) + ": is a directory\n"},
		{"package in the warning band", []string{"--from", file}, 1, "run: 15.0s (budget 90s), packages warn at 10s, fail at 20s\n" + warnB, failedB},
		{"run of 70s within the laptop budget", []string{"--start", seventy, "--from", file}, 1, "run: 70.0s (budget 90s), packages warn at 10s, fail at 20s\n" + warnB, failedB},
		{"run over budget", []string{"--start", early, "--from", file}, 1, "run: 120.0s", "monacoctl test-report: run took 120.0s, over the 90s budget\n" + failedB},
		{"run not gated in CI", []string{"--start", early, "--from", file, "--ci"}, 1, "run: 120.0s (not gated in CI; the 90s budget is for a laptop), packages warn at 10s, fail at 20s\n::warning::" + warnB, failedB},
	} {
		var stdout, stderr bytes.Buffer
		code := testReportCmd(tc.args, &stdout, &stderr)
		if code != tc.code || !strings.Contains(stdout.String(), tc.stdoutHas) || stderr.String() != tc.stderr {
			t.Fatalf("%s: code=%d stdout=%q stderr=%q", tc.name, code, stdout.String(), stderr.String())
		}
	}
}

func TestTestReportWarnsPastTenSecondsAndFailsPastTwentyOnTheLaptopAndInCI(t *testing.T) {
	t.Parallel()
	warning := func(elapsed string) string {
		return "monacoctl test-report: package m/p took " + elapsed + ".00s, over the 10s per-package budget (fails at 20s)\n"
	}
	fail := "monacoctl test-report: package m/p took 21.00s, over the 20s per-package budget\n"
	for _, tc := range []struct {
		elapsed string
		ci      bool
		code    int
		warning string
		stderr  string
	}{
		{"9", false, 0, "", ""},
		{"12", false, 0, warning("12"), ""},
		{"19", false, 0, warning("19"), ""},
		{"21", false, 1, "", fail},
		{"9", true, 0, "", ""},
		{"12", true, 0, "::warning::" + warning("12"), ""},
		{"19", true, 0, "::warning::" + warning("19"), ""},
		{"21", true, 1, "", fail},
	} {
		t.Run(fmt.Sprintf("ci=%v %ss", tc.ci, tc.elapsed), func(t *testing.T) {
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
			warned := strings.Contains(stdout.String(), "(fails at 20s)")
			annotated := strings.Contains(stdout.String(), "::warning::")
			if code != tc.code || warned != (tc.warning != "") || annotated != (tc.ci && warned) ||
				!strings.HasSuffix(stdout.String(), tc.warning) || stderr.String() != tc.stderr {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			t.Logf("exit %d\nstdout:\n%sstderr:\n%s", code, stdout.String(), stderr.String())
		})
	}
}

func TestTestReportFailsAfterTheBudgetReportWithOneLinePerFailedTest(t *testing.T) {
	t.Parallel()
	stream := fmt.Sprintf(`{"ImportPath":"m/b [m/b.test]","Action":"build-fail"}
{"Time":%[1]q,"Action":"fail","Package":"m/b","Elapsed":0,"FailedBuild":"m/b [m/b.test]"}
{"Time":%[1]q,"Action":"fail","Package":"m/s","Test":"TestZ/case","Elapsed":0}
{"Time":%[1]q,"Action":"fail","Package":"m/s","Test":"TestZ","Elapsed":0}
{"Time":%[1]q,"Action":"fail","Package":"m/s","Elapsed":0.2}
{"Time":%[1]q,"Action":"fail","Package":"m/x","Elapsed":0.5}
{"Time":%[1]q,"Action":"pass","Package":"m/ok","Test":"TestOK","Elapsed":0}
{"Time":%[1]q,"Action":"skip","Package":"m/ok","Test":"TestSkipped","Elapsed":0}
{"Time":%[1]q,"Action":"pass","Package":"m/ok","Elapsed":0.7}
{"Time":%[1]q,"Action":"fail","Package":"m/s","Test":"TestZ","Elapsed":0}
{"Time":%[1]q,"Action":"fail","Package":"m/s","Elapsed":0.3}
`, "2026-09-27T10:00:00Z")
	file := filepath.Join(t.TempDir(), "go-test.json")
	if err := os.WriteFile(file, []byte(stream), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := testReportCmd([]string{"--from", file}, &stdout, &stderr)
	want := "failed: m/b\nfailed: m/s TestZ/case\nfailed: m/s TestZ\nfailed: m/s\nfailed: m/x\n"
	if code != 1 || stderr.String() != want || !strings.Contains(stdout.String(), "   0.70s  m/ok\n") {
		t.Fatalf("code=%d stdout=%q stderr=%q; want 1, the report, and one failed line per failed test", code,
			stdout.String(), stderr.String())
	}
}

func TestTestReportPrintsBudgetExemptTimesWithoutGatingThemAndStillFailsTheirTests(t *testing.T) {
	t.Parallel()
	write := func(name string, lines ...string) string {
		file := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(file, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return file
	}
	event := func(at, action, pkg, test, elapsed string) string {
		return fmt.Sprintf(`{"Time":"2026-09-27T10:%s:00Z","Action":%q,"Package":%q,"Test":%q,"Elapsed":%s}`,
			at, action, pkg, test, elapsed)
	}
	fast := write("short.json", event("00", "pass", "m/p", "", "1"), event("01", "pass", "m/q", "", "2"))
	slowP := write("short.json", event("00", "pass", "m/p", "", "21"), event("01", "pass", "m/q", "", "2"))
	full := write("full.json", event("02", "pass", "m/p", "", "45"), event("09", "pass", "m/gen", "", "60"))
	failing := write("full.json", event("02", "fail", "m/p", "TestLong", "40"), event("02", "fail", "m/p", "", "45"))
	exemptBlock := "packages exempt from the budget:\n  60.00s  m/gen\n  45.00s  m/p\n" +
		"run: 60.0s (budget 90s), packages warn at 10s, fail at 20s\n"
	for _, tc := range []struct {
		name, from, exempt string
		code               int
		stdoutHas, stderr  string
	}{
		{"exempt times print, gate nothing and leave the run alone", fast, full, 0, exemptBlock, ""},
		{
			"the from stream keeps its budget for a package the exempt stream also ran", slowP, full, 1, exemptBlock,
			"monacoctl test-report: package m/p took 21.00s, over the 20s per-package budget\n",
		},
		{
			"an exempt stream's failed test fails the report", fast, failing, 1, "packages exempt from the budget:\n  45.00s  m/p\n",
			"failed: m/p TestLong\nfailed: m/p\n",
		},
		{
			"missing exempt stream", fast, full + ".gone", 1, "",
			"monacoctl test-report: monacoctl.readReport: internal: open " + full + ".gone: no such file or directory\n",
		},
	} {
		var stdout, stderr bytes.Buffer
		code := testReportCmd([]string{"--from", tc.from, "--budget-exempt", tc.exempt}, &stdout, &stderr)
		if code != tc.code || !strings.Contains(stdout.String(), tc.stdoutHas) || stderr.String() != tc.stderr ||
			strings.Contains(stdout.String(), "m/gen took") || strings.Contains(stdout.String(), "m/p took 45") {
			t.Fatalf("%s: code=%d stdout=%q stderr=%q", tc.name, code, stdout.String(), stderr.String())
		}
	}
}
