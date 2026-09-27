package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

func flowsCmd(args []string, _, stderr io.Writer) int {
	if len(args) != 1 || args[0] != "check" {
		_, _ = fmt.Fprintln(stderr, "usage: monacoctl flows check")
		return 2
	}
	return flowsCheck(os.DirFS("../.."), stderr)
}

func flowsCheck(repo fs.FS, stderr io.Writer) int {
	const backendDir = "apps/backend"
	file, err := repo.Open(path.Join(backendDir, flows.File))
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl flows check: %v\n", err)
		return 1
	}
	defer func() { _ = file.Close() }()
	parsed, problems := flows.Parse(file)
	problems = append(problems, flows.CheckColumns(parsed, liveEnv(repo, backendDir))...)
	slices.SortStableFunc(problems, func(a, b flows.Problem) int { return a.Line - b.Line })
	for _, p := range problems {
		_, _ = fmt.Fprintln(stderr, p)
	}
	if len(problems) > 0 {
		return 1
	}
	return 0
}

func liveEnv(repo fs.FS, backendDir string) flows.Env {
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
	}
}
