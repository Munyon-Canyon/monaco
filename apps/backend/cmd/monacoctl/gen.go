package main

import (
	"context"
	"fmt"
	"io"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	codegen "github.com/monaco/monaco/apps/backend/internal/tools/gen"
)

func toolGen(env toolEnv) tool {
	return func(args []string, stdout, stderr io.Writer) int {
		if len(args) > 0 && args[0] == "migration" {
			return migrate(codegen.NewMigrator(env.wd), args[1:], stdout, stderr)
		}
		if len(args) > 0 {
			if g, ok := codegen.Find(args[0]); ok {
				return scaffold(g, env.wd, args[1:], stdout, stderr)
			}
		}
		return gen(args, stdout, stderr)
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
	_, _ = fmt.Fprintln(stderr, "usage: monacoctl gen errors <ErrorCodeCases.gen.swift>")
	_, _ = fmt.Fprintln(stderr, "       monacoctl gen openapi <spec-dir> <openapi.yaml>")
	_, _ = fmt.Fprintln(stderr, "       monacoctl gen flows")
	for _, g := range codegen.Generators() {
		_, _ = fmt.Fprintln(stderr, "       monacoctl "+g.Usage())
	}
	_, _ = fmt.Fprintln(stderr, "       monacoctl "+codegen.MigrationUsage())
	return 2
}

func gen(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "flows" {
		return runGenFlows("../..", stdout, stderr)
	}
	if len(args) == 3 && args[0] == "openapi" {
		return runGenOpenAPI([3]string(args), stdout, stderr)
	}
	if len(args) != 2 || args[0] != "errors" {
		return genUsage(stderr)
	}
	if err := writeSwiftCases(args[1], errs.All()); err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "wrote %d error codes to %s\n", len(errs.All()), args[1])
	return 0
}

func runGenOpenAPI(args [3]string, stdout, stderr io.Writer) int {
	if err := genOpenAPI(args[1], args[2]); err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "wrote %s from %s\n", args[2], args[1])
	return 0
}
