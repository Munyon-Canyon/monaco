package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"slices"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

const (
	backendDir = "apps/backend"
	flowsUsage = "usage: monacoctl flows check [--from go-test.json]"
)

func flowsCmd(args []string, _, stderr io.Writer) int {
	var tests io.Reader
	switch {
	case slices.Equal(args, []string{"check"}):
		if info, err := os.Stdin.Stat(); err == nil && info.Mode()&os.ModeCharDevice == 0 {
			tests = os.Stdin
		}
	case len(args) == 3 && args[0] == "check" && args[1] == "--from":
		file, err := os.Open(args[2])
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "monacoctl flows check: %v\n", err)
			return 1
		}
		defer func() { _ = file.Close() }()
		tests = file
	default:
		_, _ = fmt.Fprintln(stderr, flowsUsage)
		return 2
	}
	return flowsCheck(liveEnv(os.DirFS("../.."), gitFresh(context.Background(), ".")), tests, stderr)
}

func flowsCheck(env flows.Env, tests io.Reader, stderr io.Writer) int {
	parsed, problems, err := readFlows(env.Repo)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl flows check: %v\n", err)
		return 1
	}
	results := flows.TestResults{}
	if tests != nil {
		if results, err = flows.ReadTestResults(tests); err != nil {
			_, _ = fmt.Fprintf(stderr, "monacoctl flows check: %v\n", err)
			return 1
		}
	}
	problems = append(problems, flows.CheckColumns(parsed, env)...)
	problems = append(problems, flows.CheckTests(parsed, results)...)
	problems = append(problems, flows.CheckEvidence(parsed, env)...)
	slices.SortStableFunc(problems, func(a, b flows.Problem) int { return a.Line - b.Line })
	for _, p := range problems {
		_, _ = fmt.Fprintln(stderr, p)
	}
	if len(problems) > 0 {
		return 1
	}
	return 0
}

func readFlows(repo fs.FS) ([]flows.Flow, []flows.Problem, error) {
	file, err := repo.Open(path.Join(backendDir, flows.File))
	if err != nil {
		return nil, nil, errs.Wrap(err, errs.CodeInternal, "monacoctl.readFlows")
	}
	defer func() { _ = file.Close() }()
	parsed, problems := flows.Parse(file)
	return parsed, problems, nil
}

func liveEnv(repo fs.FS, fresh flows.Fresh) flows.Env {
	catalog := events.Catalog()
	eventTypes := make([]string, 0, len(catalog))
	for _, e := range catalog {
		eventTypes = append(eventTypes, string(e.Type))
	}
	codes := errs.All()
	codeNames := make([]string, 0, len(codes))
	for _, c := range codes {
		codeNames = append(codeNames, errs.Name(c))
	}
	return flows.Env{
		Repo:        repo,
		BackendDir:  backendDir,
		Events:      func(_ flows.Flow, v string) bool { return slices.Contains(eventTypes, v) },
		Codes:       func(_ flows.Flow, v string) bool { return slices.Contains(codeNames, v) },
		Triggers:    flows.Unchecked,
		Commands:    flows.Unchecked,
		Consumers:   flows.Unchecked,
		Faultpoints: flows.Unchecked,
		Fresh:       fresh,
	}
}

func gitFresh(ctx context.Context, dir string) flows.Fresh {
	return func(module, sha string) (bool, error) { return isFresh(ctx, dir, module, sha) }
}

func isFresh(ctx context.Context, dir, module, sha string) (bool, error) {
	const op = "monacoctl.gitFresh"
	git := func(args ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = dir
		return cmd
	}
	out, err := git("log", "-1", "--format=%H", "--", path.Join("internal/modules", module)).Output()
	if err != nil {
		return false, errs.Wrap(err, errs.CodeInternal, op)
	}
	head := strings.TrimSpace(string(out))
	if head == "" {
		return true, nil
	}
	err = git("merge-base", "--is-ancestor", head, sha).Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return true, nil
	case errors.As(err, &exit) && exit.ExitCode() == 1:
		return false, nil
	default:
		return false, errs.Wrap(err, errs.CodeInternal, op)
	}
}
