package main

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	testShardsUsage = "usage: monacoctl test-shards update [--out .test-shards.tsv] --from go-test.json [go-test.json]..."
	testShardsTable = ".test-shards.tsv"
)

func testShardsCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "update" {
		_, _ = fmt.Fprintln(stderr, testShardsUsage)
		return 2
	}
	fs := flag.NewFlagSet("test-shards update", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	from := fs.String("from", "", "")
	out := fs.String("out", testShardsTable, "")
	if fs.Parse(args[1:]) != nil || *from == "" {
		_, _ = fmt.Fprintln(stderr, testShardsUsage)
		return 2
	}
	seconds, err := packageSeconds(append([]string{*from}, fs.Args()...))
	if err == nil {
		err = os.WriteFile(*out, []byte(formatTestShards(seconds)), 0o600)
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl test-shards: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "wrote %d packages to %s\n", len(seconds), *out)
	return 0
}

func packageSeconds(files []string) (map[string]time.Duration, error) {
	seconds := map[string]time.Duration{}
	for _, name := range files {
		rep, err := readReportFile(name, time.Time{})
		if err != nil {
			return nil, err
		}
		for _, p := range rep.packages {
			seconds[p.name] = max(seconds[p.name], p.elapsed)
		}
	}
	if len(seconds) == 0 {
		return nil, errs.New(
			errs.CodeInvalidInput,
			"monacoctl.testShards",
			slog.String("reason", "no package results in the input"),
		)
	}
	return seconds, nil
}

func formatTestShards(seconds map[string]time.Duration) string {
	var b strings.Builder
	for _, pkg := range slices.Sorted(maps.Keys(seconds)) {
		_, _ = fmt.Fprintf(&b, "%s\t%.1f\n", pkg, seconds[pkg].Seconds())
	}
	return b.String()
}

func toolTestShards(_ toolEnv) tool { return testShardsCmd }
