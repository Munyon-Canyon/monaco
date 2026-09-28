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
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	testReportUsage = "usage: monacoctl test-report --from go-test.json [--start unix-seconds] [--ci]"
	slowestShown    = 10
	packageBudget   = 10 * time.Second
	runBudget       = 60 * time.Second
)

type budget struct {
	pkg, run time.Duration
}

type timing struct {
	name    string
	elapsed time.Duration
}

type report struct {
	tests, packages []timing
	run             time.Duration
}

func testReportCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("test-report", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	from := fs.String("from", "", "")
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
	file, err := os.Open(*from)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl test-report: %v\n", err)
		return 1
	}
	defer func() { _ = file.Close() }()
	rep, err := readReport(file, began)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl test-report: %v\n", err)
		return 1
	}
	b := budget{pkg: packageBudget, run: runBudget}
	if *ci {
		b.run = 0
	}
	rep.write(stdout, b)
	over := rep.overBudget(b)
	for _, o := range over {
		_, _ = fmt.Fprintf(stderr, "monacoctl test-report: %s\n", o)
	}
	if len(over) > 0 {
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

func readReport(r io.Reader, start time.Time) (report, error) {
	var rep report
	var last time.Time
	reader := bufio.NewReader(r)
	for {
		line, err := reader.ReadBytes('\n')
		var ev reportEvent
		if json.Unmarshal(line, &ev) == nil && !ev.Time.IsZero() {
			if start.IsZero() {
				start = ev.Time
			}
			last = ev.Time
			rep.add(ev)
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return report{}, errs.Wrap(err, errs.CodeInternal, "monacoctl.readReport")
		}
	}
	rep.run = last.Sub(start)
	slowestFirst := func(a, b timing) int { return cmp.Compare(b.elapsed, a.elapsed) }
	slices.SortStableFunc(rep.tests, slowestFirst)
	slices.SortStableFunc(rep.packages, slowestFirst)
	return rep, nil
}

func (r *report) add(ev reportEvent) {
	t, ok := ev.timing()
	switch {
	case !ok || strings.Contains(ev.Test, "/"):
	case ev.Test == "":
		r.packages = append(r.packages, t)
	default:
		r.tests = append(r.tests, t)
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
	if b.run == 0 {
		_, _ = fmt.Fprintf(w, "run: %.1fs (not gated in CI; the %.0fs budget is for a laptop), %.0fs per package\n",
			r.run.Seconds(), runBudget.Seconds(), b.pkg.Seconds())
		return
	}
	_, _ = fmt.Fprintf(
		w,
		"run: %.1fs (budget %.0fs, %.0fs per package)\n",
		r.run.Seconds(),
		b.run.Seconds(),
		b.pkg.Seconds(),
	)
}

func (r report) overBudget(b budget) []string {
	var over []string
	for _, p := range r.packages {
		if p.elapsed > b.pkg {
			over = append(
				over,
				fmt.Sprintf(
					"package %s took %.2fs, over the %.0fs per-package budget",
					p.name,
					p.elapsed.Seconds(),
					b.pkg.Seconds(),
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
