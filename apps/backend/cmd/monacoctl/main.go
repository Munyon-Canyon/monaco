package main

import (
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
)

//go:generate go run ../../scripts/gen-depguard ../..

type command func(args []string, stdout, stderr io.Writer) int

func commands() map[string]command {
	return map[string]command{}
}

func main() {
	os.Exit(run(commands(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(cmds map[string]command, args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		if cmd, ok := cmds[args[0]]; ok {
			return cmd(args[1:], stdout, stderr)
		}
		_, _ = fmt.Fprintf(stderr, "monacoctl: unknown command %q\n", args[0])
	}
	_, _ = fmt.Fprintln(stderr, "usage: monacoctl <command> [args]")
	for _, name := range slices.Sorted(maps.Keys(cmds)) {
		_, _ = fmt.Fprintf(stderr, "  %s\n", name)
	}
	return 2
}
