package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	codegen "github.com/monaco/monaco/apps/backend/internal/tools/gen"
)

const (
	backendModule = "github.com/monaco/monaco/apps/backend"
	specDir       = "api/spec"
	apiDir        = "internal/platform/httpx/api"
)

type step struct {
	name string
	run  func() error
}

func steps(ctx context.Context) []step {
	return []step{
		{
			"golangci",
			func() error { return quiet(exec.CommandContext(ctx, "go", "run", "./scripts/gen-golangci", ".")) },
		},
		{
			"registry",
			func() error { return quiet(exec.CommandContext(ctx, "go", "run", "./scripts/gen-registry", ".")) },
		},
		{"sqlc", func() error { return codegen.GenerateSqlc(ctx, ".") }},
		{"errors", func() error {
			return genErrors("../../packages/mobile-core/Tests/MonacoAPITests/ErrorCodeCases.gen.swift")
		}},
		{"openapi", func() error { return genOpenAPI(specDir, "api/openapi.yaml") }},
		{"httpapi", func() error { return genAPIs(specDir, apiDir, backendModule+"/"+apiDir, errs.All()) }},
		{"flows", func() error { return genFlows("../..") }},
		{"docs", func() error { return genDocs("../../docs/reference") }},
		{"hash", func() error { return codegen.HashMigrations(ctx, ".") }},
	}
}

func quiet(cmd *exec.Cmd) error {
	if out, err := cmd.CombinedOutput(); err != nil {
		return errs.Wrap(fmt.Errorf("%s: %w\n%s", cmd, err, out), errs.CodeInternal, "gen.quiet")
	}
	return nil
}

func run(all []step, args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		return usage(all, stderr)
	}
	chosen := all
	if args[0] != "all" {
		chosen = nil
		for _, s := range all {
			if s.name == args[0] {
				chosen = []step{s}
			}
		}
		if chosen == nil {
			return usage(all, stderr)
		}
	}
	for _, s := range chosen {
		if err := s.run(); err != nil {
			_, _ = fmt.Fprintf(stderr, "gen %s: %v\n", s.name, err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "gen %s ok\n", s.name)
	}
	return 0
}

func usage(all []step, stderr io.Writer) int {
	names := make([]string, len(all))
	for i, s := range all {
		names[i] = s.name
	}
	_, _ = fmt.Fprintf(stderr, "usage: go run ./cmd/gen all|%s\n", strings.Join(names, "|"))
	return 2
}

func main() {
	os.Exit(run(steps(context.Background()), os.Args[1:], os.Stdout, os.Stderr))
}
