package main

import (
	"context"
	"fmt"
	"io"

	codegen "github.com/monaco/monaco/apps/backend/internal/tools/gen"
)

func toolGen(env toolEnv) tool {
	return func(args []string, stdout, stderr io.Writer) int {
		if len(args) > 0 && args[0] == "migration" {
			return migrate(codegen.NewMigrator(env.wd, runIn(env.wd)), args[1:], stdout, stderr)
		}
		if len(args) > 0 {
			if g, ok := codegen.Find(args[0]); ok {
				return scaffold(g, env.wd, args[1:], stdout, stderr)
			}
		}
		return genUsage(stderr)
	}
}

func scaffold(g codegen.Generator, root string, args []string, stdout, stderr io.Writer) int {
	touched, err := g.Run(context.Background(), root, args)
	for _, rel := range touched {
		_, _ = fmt.Fprintln(stdout, rel)
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl: %v\n", err)
		return 1
	}
	return 0
}

func migrate(m codegen.Migrator, args []string, stdout, stderr io.Writer) int {
	touched, err := m.Run(context.Background(), args)
	for _, rel := range touched {
		_, _ = fmt.Fprintln(stdout, rel)
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl: %v\n", err)
		return 1
	}
	return 0
}

func genUsage(stderr io.Writer) int {
	for i, g := range codegen.Generators() {
		lead := "       "
		if i == 0 {
			lead = "usage: "
		}
		_, _ = fmt.Fprintln(stderr, lead+"monacoctl "+g.Usage())
	}
	_, _ = fmt.Fprintln(stderr, "       monacoctl "+codegen.MigrationUsage())
	return 2
}
