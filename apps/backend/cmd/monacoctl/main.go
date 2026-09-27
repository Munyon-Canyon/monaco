package main

import (
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/lint/comments"
)

//go:generate go run ../../scripts/gen-depguard ../..
//go:generate go run . gen errors ../../api/openapi.yaml

type command func(cfg config.Config, args []string, stdout, stderr io.Writer) int

type tool func(args []string, stdout, stderr io.Writer) int

func commands() map[string]command {
	return map[string]command{"dev": devCmd}
}

func tools(environ []string) map[string]tool {
	gremlinsBin, _ := filepath.Abs("../../.bin/gremlins")
	wd, _ := os.Getwd()
	exe, _ := os.Executable()
	return map[string]tool{
		"bench":    bench{"go"}.run,
		"bus":      busTool(environ),
		"coverage": coverageEnv{moduleDir: ".", goBin: "go", tmpDir: os.TempDir()}.run,
		"docs":     docs,
		"flows":    flowsCmd,
		"gen":      gen,
		"migrate":  locatedMigrateTool(environ, wd, filepath.Dir(exe)),
		"mutation": mutationTool(
			mutationEnv{
				moduleDir: ".",
				goBin:     "go",
				gitBin:    "git",
				gremlins:  gremlinsBin,
				tmpDir:    os.TempDir(),
				exec:      runCommand,
			},
		),
		"test-report": testReportCmd,
		"lint": func(args []string, stdout, stderr io.Writer) int {
			return run(nil, map[string]tool{"comments": comments.Run}, nil, args, stdout, stderr)
		},
	}
}

func main() {
	environ := os.Environ()
	os.Exit(run(commands(), tools(environ), environ, os.Args[1:], os.Stdout, os.Stderr))
}

func run(cmds map[string]command, tls map[string]tool, environ, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return usage(cmds, tls, stderr)
	}
	if t, ok := tls[args[0]]; ok {
		return t(args[1:], stdout, stderr)
	}
	cmd, ok := cmds[args[0]]
	if !ok {
		_, _ = fmt.Fprintf(stderr, "monacoctl: unknown command %q\n", args[0])
		return usage(cmds, tls, stderr)
	}
	cfg, err := config.Load(environ)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl: %v\n", err)
		return 1
	}
	return cmd(cfg, args[1:], stdout, stderr)
}

func usage(cmds map[string]command, tls map[string]tool, stderr io.Writer) int {
	_, _ = fmt.Fprintln(stderr, "usage: monacoctl <command> [args]")
	names := slices.Concat(slices.Collect(maps.Keys(cmds)), slices.Collect(maps.Keys(tls)))
	for _, name := range slices.Sorted(slices.Values(names)) {
		_, _ = fmt.Fprintf(stderr, "  %s\n", name)
	}
	return 2
}
