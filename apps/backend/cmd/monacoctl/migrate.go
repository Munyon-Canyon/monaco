package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/mod/modfile"

	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

const backendModule = "github.com/monaco/monaco/apps/backend"

var errNoModule = errors.New("cannot find module " + backendModule)

const renameLegacyRevisions = `UPDATE atlas_schema_revisions.atlas_schema_revisions AS r SET version = n.version
FROM (VALUES ('0001', '20260927140427'), ('0002', '20260927185159')) AS n(legacy, version)
WHERE r.version = n.legacy`

type atlas struct {
	dir, bin, versionFile string
	beforeDB              func(ctx context.Context, url string) error
}

func atlasAt(root string) atlas {
	return atlas{
		dir:         root,
		bin:         filepath.Join(root, "..", "..", ".bin", "atlas"),
		versionFile: filepath.Join(root, ".atlas-version"),
		beforeDB:    renameLegacy,
	}
}

func renameLegacy(ctx context.Context, url string) error {
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()
	_, err = conn.Exec(ctx, renameLegacyRevisions)
	var pg *pgconn.PgError
	if errors.As(err, &pg) && (pg.Code == "42P01" || pg.Code == "3F000") {
		return nil
	}
	if err != nil {
		return fmt.Errorf("rename legacy revisions: %w", err)
	}
	return nil
}

func moduleRoot(starts ...string) (string, error) {
	for _, start := range starts {
		for dir := start; ; dir = filepath.Dir(dir) {
			for _, candidate := range []string{dir, filepath.Join(dir, "apps", "backend")} {
				if isBackendModule(candidate) {
					return candidate, nil
				}
			}
			if filepath.Dir(dir) == dir {
				break
			}
		}
	}
	return "", fmt.Errorf("%w above %s", errNoModule, strings.Join(starts, " or "))
}

func isBackendModule(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	return err == nil && modfile.ModulePath(data) == backendModule
}

func locatedMigrateTool(environ []string, starts ...string) tool {
	root, err := moduleRoot(starts...)
	if err != nil {
		return func(_ []string, _, stderr io.Writer) int {
			_, _ = fmt.Fprintf(stderr, "monacoctl: %v\n", err)
			return 1
		}
	}
	return migrateTool(atlasAt(root), environ)
}

func migrateTool(a atlas, environ []string) tool {
	onDB := func(sub string) command {
		return func(cfg config.Config, args []string, stdout, stderr io.Writer) int {
			if len(args) != 0 {
				return migrateUsage(stderr)
			}
			if err := a.beforeDB(context.Background(), cfg.DB.URL); err != nil {
				_, _ = fmt.Fprintf(stderr, "monacoctl: %v\n", err)
				return 1
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
	cmd.Dir = a.dir
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
		_, _ = fmt.Fprintf(stderr, "monacoctl: %s is %s, want %q. run: just install\n",
			a.bin, reported, want)
		return false
	}
	return true
}

func toolMigrate(env toolEnv) tool {
	return locatedMigrateTool(env.environ, env.wd, filepath.Dir(env.exe))
}
