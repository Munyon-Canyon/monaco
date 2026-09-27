package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

type atlas struct {
	bin, versionFile string
}

func migrateTool(a atlas, environ []string) tool {
	onDB := func(sub string) command {
		return func(cfg config.Config, args []string, stdout, stderr io.Writer) int {
			if len(args) != 0 {
				return migrateUsage(stderr)
			}
			return a.run([]string{"migrate", sub, "--dir", "file://migrations", "--url", cfg.DB.URL}, stdout, stderr)
		}
	}
	lint := func(args []string, stdout, stderr io.Writer) int {
		if len(args) != 0 {
			return migrateUsage(stderr)
		}
		return a.run([]string{
			"migrate", "lint", "--dir", "file://migrations",
			"--dev-url", "docker://postgres/16/dev", "--latest", "1",
		}, stdout, stderr)
	}
	return func(args []string, stdout, stderr io.Writer) int {
		cmds := map[string]command{"apply": onDB("apply"), "status": onDB("status")}
		return run(cmds, map[string]tool{"lint": lint}, environ, args, stdout, stderr)
	}
}

func migrateUsage(stderr io.Writer) int {
	_, _ = fmt.Fprintln(stderr, "usage: monacoctl migrate apply|status|lint")
	return 2
}

func (a atlas) run(args []string, stdout, stderr io.Writer) int {
	if !a.pinned(stderr) {
		return 1
	}
	cmd := exec.CommandContext(context.Background(), a.bin, args...)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl: atlas %s: %v\n", strings.Join(args[:2], " "), err)
		return 1
	}
	return 0
}

func (a atlas) pinned(stderr io.Writer) bool {
	version, err := os.ReadFile(a.versionFile)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "monacoctl: read pinned atlas version: %v\n", err)
		return false
	}
	want := "atlas community version " + strings.TrimSpace(string(version))
	out, err := exec.CommandContext(context.Background(), a.bin, "version").Output()
	got, _, _ := bytes.Cut(out, []byte("\n"))
	if string(got) != want {
		reported := fmt.Sprintf("%q", got)
		if err != nil {
			reported += fmt.Sprintf(" (%v)", err)
		}
		_, _ = fmt.Fprintf(stderr, "monacoctl: %s is %s, want %q. Run scripts/install-atlas.sh from the repo root.\n",
			a.bin, reported, want)
		return false
	}
	return true
}
