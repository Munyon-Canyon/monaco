package main

import (
	"io"

	"github.com/monaco/monaco/apps/backend/internal/platform/lint/comments"
)

func toolLint(_ toolEnv) tool {
	return func(args []string, stdout, stderr io.Writer) int {
		return run(nil, map[string]tool{"comments": comments.Run}, nil, args, stdout, stderr)
	}
}
