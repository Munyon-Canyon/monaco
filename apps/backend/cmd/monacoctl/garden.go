package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/tools/garden"
)

const (
	gardenUsage  = "usage: monacoctl garden report [--skip-mutation]"
	gardenReport = "garden-report.md"
)

func toolGarden(env toolEnv) tool {
	return func(args []string, stdout, stderr io.Writer) int {
		return gardenCmd(env.wd, args, stdout, stderr)
	}
}

func gardenCmd(wd string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("garden report", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	skipMutation := fs.Bool("skip-mutation", false, "")
	if len(args) == 0 || args[0] != "report" || fs.Parse(args[1:]) != nil || fs.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, gardenUsage)
		return 2
	}
	cfg := garden.Config{
		ModuleDir:    wd,
		GolangciLint: "golangci-lint",
		Sqlc:         localBin(wd, "sqlc"),
		TempDir:      os.TempDir(),
		Exec:         gardenExec,
	}
	if !*skipMutation {
		cfg.Mutants = func(ctx context.Context) ([]garden.Finding, error) { return gardenMutants(ctx, stdout) }
	}
	report, err := garden.Run(context.Background(), cfg)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl garden: %v\n", err)
		return 1
	}
	if err := os.WriteFile(filepath.Join(wd, gardenReport), []byte(report.Markdown()), 0o600); err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl garden: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "wrote %s\n", gardenReport)
	for _, s := range report.Failed() {
		_, _ = fmt.Fprintf(stderr, "monacoctl garden: %s: %v\n", s.Kind, s.Err)
	}
	if len(report.Failed()) > 0 {
		return 1
	}
	return 0
}

func localBin(wd, name string) string {
	pinned := filepath.Join(wd, "..", "..", ".bin", name)
	if info, err := os.Stat(pinned); err == nil && !info.IsDir() {
		return pinned
	}
	return name
}

func gardenExec(ctx context.Context, dir, name string, args ...string) (garden.Result, error) {
	out, err := runCommand(ctx, dir, nil, name, args...)
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return garden.Result{Stdout: out, Stderr: err.Error(), Code: exit.ExitCode()}, nil
	}
	return garden.Result{Stdout: out}, err
}

func gardenMutants(ctx context.Context, progress io.Writer) ([]garden.Finding, error) {
	survivors, err := defaultMutationEnv().run(ctx, mutationArgs{all: true}, progress)
	if err != nil {
		return nil, err
	}
	findings := make([]garden.Finding, 0, len(survivors))
	for _, key := range survivors {
		findings = append(findings, mutantFinding(key))
	}
	return findings, nil
}

func mutantFinding(key string) garden.Finding {
	pos, mutator, _ := strings.Cut(key, " ")
	file, rest, _ := strings.Cut(pos, ":")
	lineText, _, _ := strings.Cut(rest, ":")
	line, _ := strconv.Atoi(lineText)
	return garden.Finding{
		Rule: mutator,
		File: file,
		Line: line,
		Text: "survived; kill it with a test or list it in " + mutantsAllow + " with a reason",
	}
}
