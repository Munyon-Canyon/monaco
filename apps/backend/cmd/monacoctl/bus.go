package main

import (
	"context"
	"fmt"
	"io"

	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

func busTool(environ []string) tool {
	apply := func(cfg config.Config, args []string, stdout, stderr io.Writer) int {
		if len(args) != 0 {
			return busUsage(stderr)
		}
		ctx := context.Background()
		conn, err := bus.Connect(ctx, cfg.NATS, bus.ProcessMonacoctl)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "monacoctl: %v\n", err)
			return 1
		}
		defer conn.Close(ctx)
		changes, err := conn.Apply(ctx)
		for _, c := range changes {
			_, _ = io.WriteString(stdout, c.String())
		}
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "monacoctl: %v\n", err)
			return 1
		}
		return 0
	}
	return func(args []string, stdout, stderr io.Writer) int {
		return run(map[string]command{"apply": apply}, nil, environ, args, stdout, stderr)
	}
}

func busUsage(stderr io.Writer) int {
	_, _ = fmt.Fprintln(stderr, "usage: monacoctl bus apply")
	return 2
}
