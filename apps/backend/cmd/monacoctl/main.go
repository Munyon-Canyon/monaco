package main

import (
	"fmt"
	"io"
	"maps"
	"os"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

//go:generate go run ../../scripts/gen-depguard ../..

type command func(cfg config.Config, args []string, stdout, stderr io.Writer) int

func commands() map[string]command {
	return map[string]command{}
}

func main() {
	os.Exit(run(commands(), os.Environ(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(cmds map[string]command, environ, args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		if cmd, ok := cmds[args[0]]; ok {
			cfg, err := config.Load(environ)
			if err != nil {
				_, _ = fmt.Fprintf(stderr, "monacoctl: %v\n", err)
				return 1
			}
			return cmd(cfg, args[1:], stdout, stderr)
		}
		_, _ = fmt.Fprintf(stderr, "monacoctl: unknown command %q\n", args[0])
	}
	_, _ = fmt.Fprintln(stderr, "usage: monacoctl <command> [args]")
	for _, name := range slices.Sorted(maps.Keys(cmds)) {
		_, _ = fmt.Fprintf(stderr, "  %s\n", name)
	}
	return 2
}
