package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	codegen "github.com/monaco/monaco/apps/backend/internal/tools/gen"
)

const (
	errorCodeBegin = "# BEGIN GENERATED ErrorCode"
	errorCodeEnd   = "# END GENERATED ErrorCode"
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
	_, _ = fmt.Fprintln(stderr, "usage: monacoctl gen errors <spec/base.yaml> [<ErrorCodeCases.gen.swift>]")
	_, _ = fmt.Fprintln(stderr, "       monacoctl gen openapi <spec-dir> <openapi.yaml>")
	for _, g := range codegen.Generators() {
		_, _ = fmt.Fprintln(stderr, "       monacoctl "+g.Usage())
	}
	_, _ = fmt.Fprintln(stderr, "       monacoctl "+codegen.MigrationUsage())
	return 2
}

func gen(args []string, stdout, stderr io.Writer) int {
	if len(args) == 3 && args[0] == "openapi" {
		return runGenOpenAPI([3]string(args), stdout, stderr)
	}
	if len(args) < 2 || len(args) > 3 || args[0] != "errors" {
		return genUsage(stderr)
	}
	if err := writeErrorCodes(args[1], errs.All()); err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl: %v\n", err)
		return 1
	}
	if len(args) == 3 {
		if err := writeSwiftCases(args[2], errs.All()); err != nil {
			_, _ = fmt.Fprintf(stderr, "monacoctl: %v\n", err)
			return 1
		}
	}
	_, _ = fmt.Fprintf(stdout, "wrote %d error codes to %s\n", len(errs.All()), args[1])
	return 0
}

func writeErrorCodes(path string, codes []errs.Code) error {
	dir, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return errs.Wrap(err, errs.CodeInvalidInput, "monacoctl.writeErrorCodes")
	}
	defer func() { _ = dir.Close() }()
	spec, err := dir.ReadFile(filepath.Base(path))
	if err != nil {
		return errs.Wrap(err, errs.CodeInvalidInput, "monacoctl.writeErrorCodes")
	}
	out, err := spliceErrorCodes(string(spec), codes)
	if err != nil {
		return err
	}
	if err := dir.WriteFile(filepath.Base(path), []byte(out), 0o600); err != nil {
		return errs.Wrap(err, errs.CodeInternal, "monacoctl.writeErrorCodes")
	}
	return nil
}

func spliceErrorCodes(spec string, codes []errs.Code) (string, error) {
	lines := strings.Split(spec, "\n")
	begin, end := markerLine(lines, errorCodeBegin), markerLine(lines, errorCodeEnd)
	if begin < 0 || end < begin {
		return "", errs.New(errs.CodeInvalidInput, "monacoctl.spliceErrorCodes",
			slog.String("want", "one "+errorCodeBegin+" line followed by one "+errorCodeEnd+" line"))
	}
	indent := lines[begin][:len(lines[begin])-len(strings.TrimLeft(lines[begin], " "))]
	enum := make([]string, len(codes))
	for i, code := range codes {
		enum[i] = indent + "- " + string(code)
	}
	return strings.Join(slices.Concat(lines[:begin+1], enum, lines[end:]), "\n"), nil
}

func markerLine(lines []string, marker string) int {
	found := -1
	for i, line := range lines {
		if strings.TrimSpace(line) != marker {
			continue
		}
		if found >= 0 {
			return -1
		}
		found = i
	}
	return found
}

func runGenOpenAPI(args [3]string, stdout, stderr io.Writer) int {
	if err := genOpenAPI(args[1], args[2]); err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "wrote %s from %s\n", args[2], args[1])
	return 0
}
