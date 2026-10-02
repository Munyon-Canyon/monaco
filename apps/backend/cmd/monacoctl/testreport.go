package main

import (
	"bufio"
	"cmp"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	testReportUsage = "usage: monacoctl test-report --from go-test.json [--budget-exempt go-test.json] [--start unix-seconds] [--ci]"
	slowestShown    = 10
	packageWarn     = 10 * time.Second
	packageFail     = 40 * time.Second
	runBudget       = 90 * time.Second
)

type budget struct {
	warn, fail, run time.Duration
}

type timing struct {
	name    string
	elapsed time.Duration
}

type report struct {
	tests, packages, exempt []timing
	failed                  []string
	run                     time.Duration
}

func testReportCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("test-report", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	from := fs.String("from", "", "")
	exempt := fs.String("budget-exempt", "", "")
	start := fs.String("start", "", "")
	ci := fs.Bool("ci", false, "")
	if fs.Parse(args) != nil || *from == "" || fs.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, testReportUsage)
		return 2
	}
	var began time.Time
	if *start != "" {
		secs, err := strconv.ParseInt(*start, 10, 64)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, testReportUsage)
			return 2
		}
		began = time.Unix(secs, 0)
	}
	rep, err := readReportFile(*from, began)
	if err == nil && *exempt != "" {
		err = rep.addExempt(*exempt)
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl test-report: %v\n", err)
		return 1
	}
	b := budget{warn: packageWarn, fail: packageFail, run: runBudget}
	prefix := ""
	if *ci {
		b.run = 0
		prefix = "::warning::"
	}
	rep.write(stdout, b)
	return rep.gate(b, prefix, stdout, stderr)
}

func (r report) gate(b budget, prefix string, stdout, stderr io.Writer) int {
	for _, w := range r.warnings(b) {
		_, _ = fmt.Fprintf(stdout, "%smonacoctl test-report: %s\n", prefix, w)
	}
	over := r.overBudget(b)
	for _, o := range over {
		_, _ = fmt.Fprintf(stderr, "monacoctl test-report: %s\n", o)
	}
	for _, name := range r.failed {
		_, _ = fmt.Fprintf(stderr, "failed: %s\n", name)
	}
	if len(over) > 0 || len(r.failed) > 0 {
		return 1
	}
	return 0
}

type reportEvent struct {
	Time    time.Time   `json:"Time"`
	Action  string      `json:"Action"`
	Package string      `json:"Package"`
	Test    string      `json:"Test"`
	Elapsed json.Number `json:"Elapsed"`
}

func readReportFile(name string, start time.Time) (report, error) {
	file, err := os.Open(filepath.Clean(name))
	if err != nil {
		return report{}, errs.Wrap(err, errs.CodeInternal, "monacoctl.readReport")
	}
	defer func() { _ = file.Close() }()
	return readReport(file, start)
}

func (r *report) addExempt(name string) error {
	ex, err := readReportFile(name, time.Time{})
	if err != nil {
		return err
	}
	r.exempt = ex.packages
	for _, test := range ex.failed {
		r.fail(test)
	}
	return nil
}

type span struct{ first, last time.Time }

func (s *span) add(t time.Time) {
	if s.first.IsZero() || t.Before(s.first) {
		s.first = t
	}
	if t.After(s.last) {
		s.last = t
	}
}

func (s span) length(start time.Time) time.Duration {
	if start.IsZero() {
		start = s.first
	}
	return s.last.Sub(start)
}

func readReport(r io.Reader, start time.Time) (report, error) {
	var rep report
	var seen span
	reader := bufio.NewReader(r)
	for {
		line, err := reader.ReadBytes('\n')
		var ev reportEvent
		if json.Unmarshal(line, &ev) == nil && !ev.Time.IsZero() {
			seen.add(ev.Time)
			rep.add(ev)
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return report{}, errs.Wrap(err, errs.CodeInternal, "monacoctl.readReport")
		}
	}
	rep.run = seen.length(start)
	slowestFirst := func(a, b timing) int { return cmp.Compare(b.elapsed, a.elapsed) }
	slices.SortStableFunc(rep.tests, slowestFirst)
	slices.SortStableFunc(rep.packages, slowestFirst)
	return rep, nil
}

func (r *report) add(ev reportEvent) {
	t, ok := ev.timing()
	if ev.Action == "fail" {
		r.fail(t.name)
	}
	switch {
	case !ok || strings.Contains(ev.Test, "/"):
	case ev.Test == "":
		r.packages = append(r.packages, t)
	default:
		r.tests = append(r.tests, t)
	}
}

func (r *report) fail(test string) {
	if !slices.Contains(r.failed, test) {
		r.failed = append(r.failed, test)
	}
}

func (ev reportEvent) timing() (timing, bool) {
	if ev.Action != "pass" && ev.Action != "fail" && ev.Action != "skip" {
		return timing{}, false
	}
	d, _ := time.ParseDuration(ev.Elapsed.String() + "s")
	name := ev.Package
	if ev.Test != "" {
		name += " " + ev.Test
	}
	return timing{name: name, elapsed: d}, true
}

func (r report) write(w io.Writer, b budget) {
	_, _ = fmt.Fprintln(w, "slowest tests:")
	for _, t := range r.tests[:min(slowestShown, len(r.tests))] {
		_, _ = fmt.Fprintf(w, "%7.2fs  %s\n", t.elapsed.Seconds(), t.name)
	}
	_, _ = fmt.Fprintln(w, "packages:")
	for _, p := range r.packages {
		_, _ = fmt.Fprintf(w, "%7.2fs  %s\n", p.elapsed.Seconds(), p.name)
	}
	if len(r.exempt) > 0 {
		_, _ = fmt.Fprintln(w, "packages exempt from the budget:")
	}
	for _, p := range r.exempt {
		_, _ = fmt.Fprintf(w, "%7.2fs  %s\n", p.elapsed.Seconds(), p.name)
	}
	perPackage := fmt.Sprintf("packages warn at %.0fs, fail at %.0fs", b.warn.Seconds(), b.fail.Seconds())
	if b.run == 0 {
		_, _ = fmt.Fprintf(w, "run: %.1fs (not gated in CI; the %.0fs budget is for a laptop), %s\n",
			r.run.Seconds(), runBudget.Seconds(), perPackage)
		return
	}
	_, _ = fmt.Fprintf(w, "run: %.1fs (budget %.0fs), %s\n", r.run.Seconds(), b.run.Seconds(), perPackage)
}

func (r report) warnings(b budget) []string {
	var warn []string
	for _, p := range r.packages {
		if p.elapsed > b.warn && p.elapsed <= b.fail {
			warn = append(warn, fmt.Sprintf(
				"package %s took %.2fs, over the %.0fs per-package budget (fails at %.0fs)",
				p.name, p.elapsed.Seconds(), b.warn.Seconds(), b.fail.Seconds(),
			))
		}
	}
	return warn
}

func (r report) overBudget(b budget) []string {
	var over []string
	for _, p := range r.packages {
		if p.elapsed > b.fail {
			over = append(
				over,
				fmt.Sprintf(
					"package %s took %.2fs, over the %.0fs per-package budget",
					p.name,
					p.elapsed.Seconds(),
					b.fail.Seconds(),
				),
			)
		}
	}
	if b.run > 0 && r.run > b.run {
		over = append(over, fmt.Sprintf("run took %.1fs, over the %.0fs budget", r.run.Seconds(), b.run.Seconds()))
	}
	return over
}

func toolTestReport(_ toolEnv) tool { return testReportCmd }
