package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	benchUsage = "usage: monacoctl bench db"
	benchPkg   = "./internal/testkit/testdata/bench"
)

type bench struct{ goBin string }

func (b bench) run(args []string, stdout, stderr io.Writer) int {
	if !slices.Equal(args, []string{"db"}) {
		_, _ = fmt.Fprintln(stderr, benchUsage)
		return 2
	}
	cmd := exec.CommandContext(
		context.Background(),
		b.goBin,
		"test",
		"-count=1",
		"-json",
		"-run",
		"^TestCloneBench$",
		benchPkg,
	)
	cmd.Stderr = stderr
	out, runErr := cmd.Output()
	line, err := benchReport(bytes.NewReader(out))
	if err != nil || runErr != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl bench db: %v %v\n%s", runErr, err, out)
		return 1
	}
	_, _ = fmt.Fprintln(stdout, line)
	return 0
}

type testEvent struct {
	Action string `json:"Action"`
	Test   string `json:"Test"`
	Output string `json:"Output"`
}

func benchReport(r io.Reader) (string, error) {
	line := regexp.MustCompile(`bench tests=(\d+) phase_ns=(\d+)`)
	dec := json.NewDecoder(r)
	passed, tests, phase := false, int64(0), int64(0)
	for dec.More() {
		var ev testEvent
		if err := dec.Decode(&ev); err != nil {
			return "", errs.Wrap(err, errs.CodeDecodeFailed, "monacoctl.benchReport")
		}
		passed = passed || ev.Test == "TestCloneBench" && ev.Action == "pass"
		if m := line.FindStringSubmatch(ev.Output); m != nil {
			var err error
			if tests, err = strconv.ParseInt(m[1], 10, 64); err == nil {
				phase, err = strconv.ParseInt(m[2], 10, 64)
			}
			if err != nil {
				return "", errs.Wrap(err, errs.CodeDecodeFailed, "monacoctl.benchReport")
			}
		}
	}
	if !passed || tests == 0 {
		return "", errs.New(errs.CodeInternal, "monacoctl.benchReport", slog.Bool("passed", passed))
	}
	d := time.Duration(phase)
	return fmt.Sprintf("bench db: %d parallel testkit.DB tests in %.2f s, %.1f ms per test amortized",
		tests, d.Seconds(), float64(d.Microseconds())/1000/float64(tests)), nil
}
