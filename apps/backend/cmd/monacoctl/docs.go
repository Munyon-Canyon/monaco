package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

func docs(args []string, stdout, stderr io.Writer) int {
	switch {
	case slices.Equal(args, []string{"flows"}):
		return docsFlows(os.DirFS("../.."), flows.Markdown, stdout, stderr)
	default:
		_, _ = fmt.Fprintln(stderr, "usage: monacoctl docs flows")
		return 2
	}
}

func docsFlows(repo fs.FS, render func([]flows.Flow) string, stdout, stderr io.Writer) int {
	parsed, problems, err := readFlows(repo)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl docs flows: %v\n", err)
		return 1
	}
	for _, p := range problems {
		_, _ = fmt.Fprintln(stderr, p)
	}
	if len(problems) > 0 {
		return 1
	}
	_, _ = io.WriteString(stdout, render(parsed))
	return 0
}

func toolDocs(_ toolEnv) tool { return docs }
