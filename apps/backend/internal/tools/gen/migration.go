package gen

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
)

const (
	migrationsDir   = "migrations"
	migrationBase   = "origin/staging"
	prefixLayout    = "20060102150405"
	migrationUsage  = "gen migration <module> <name> | gen migration --rebase"
	migrationRebase = "--rebase"
)

var (
	migrationFile = regexp.MustCompile(`^(\d{14})_[a-z0-9_]+\.sql$`)
	migrationName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
)

func MigrationUsage() string { return migrationUsage }

type Migrator struct {
	Dir    string
	Now    func() time.Time
	Parent func(ctx context.Context) ([]string, error)
	Hash   func(ctx context.Context) error
}

type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

func NewMigrator(dir string, run Runner) Migrator {
	return Migrator{Dir: dir, Now: clock.Real{}.Now, Parent: gitParentMigrations(run), Hash: atlasHash(dir)}
}

func (m Migrator) Run(ctx context.Context, args []string) ([]string, error) {
	const op = "gen.Migrator.Run"
	rebase := len(args) == 1 && args[0] == migrationRebase
	if !rebase && (len(args) != 2 || strings.HasPrefix(args[0], "-")) {
		return nil, invalid(op, "usage: %s", migrationUsage)
	}
	root, err := os.OpenRoot(m.Dir)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInvalidInput, op)
	}
	defer func() { _ = root.Close() }()
	if rebase {
		return m.rebase(ctx, root)
	}
	return m.create(ctx, root, args[0], args[1])
}

func (m Migrator) create(ctx context.Context, root *os.Root, module, name string) ([]string, error) {
	if err := requireModule(root, module, name, migrationName); err != nil {
		return nil, err
	}
	names, err := migrationNames(root)
	if err != nil {
		return nil, err
	}
	rel := filepath.Join(migrationsDir, NextPrefix(names, m.Now())+"_"+module+"_"+name+".sql")
	if err := writeFile(root, rel, ""); err != nil {
		return nil, err
	}
	return []string{rel}, m.Hash(ctx)
}

func (m Migrator) rebase(ctx context.Context, root *os.Root) ([]string, error) {
	onDisk, err := migrationNames(root)
	if err != nil {
		return nil, err
	}
	parent, err := m.Parent(ctx)
	if err != nil {
		return nil, err
	}
	const op = "gen.Migrator.rebase"
	var touched []string
	for _, r := range RebasePlan(onDisk, parent, m.Now()) {
		from, to := filepath.Join(migrationsDir, r[0]), filepath.Join(migrationsDir, r[1])
		if _, err := root.Lstat(to); !errors.Is(err, fs.ErrNotExist) {
			return touched, errs.Wrap(cmp.Or(err, fs.ErrExist), errs.CodeInternal, op, slog.String("file", to))
		}
		if err := root.Rename(from, to); err != nil {
			return touched, errs.Wrap(err, errs.CodeInternal, op, slog.String("file", from))
		}
		touched = append(touched, to)
	}
	return touched, m.Hash(ctx)
}

func NextPrefix(names []string, now time.Time) string {
	next := now.UTC().Truncate(time.Second)
	for _, name := range names {
		m := migrationFile.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		if t, err := time.Parse(prefixLayout, m[1]); err == nil && !next.After(t) {
			next = t.Add(time.Second)
		}
	}
	return next.Format(prefixLayout)
}

func RebasePlan(onDisk, parent []string, now time.Time) [][2]string {
	var own []string
	for _, name := range slices.Sorted(slices.Values(onDisk)) {
		if migrationFile.MatchString(name) && !slices.Contains(parent, name) {
			own = append(own, name)
		}
	}
	newest := ""
	for _, name := range parent {
		if m := migrationFile.FindStringSubmatch(name); m != nil {
			newest = max(newest, m[1])
		}
	}
	if len(own) == 0 || own[0][:14] > newest {
		return nil
	}
	taken := slices.Concat(parent, onDisk)
	renames := make([][2]string, len(own))
	for i, name := range own {
		to := NextPrefix(taken, now) + name[14:]
		taken = append(taken, to)
		renames[i] = [2]string{name, to}
	}
	return renames
}

type MigrationCommit struct {
	SHA    string
	Added  []string
	Before []string
}

func MigrationOrder(commits []MigrationCommit) []string {
	var lines []string
	for _, c := range commits {
		newest := ""
		for _, name := range c.Before {
			if newest == "" || migrationVersion(name) > migrationVersion(newest) {
				newest = name
			}
		}
		for _, file := range c.Added {
			if newest != "" && migrationVersion(file) <= migrationVersion(newest) {
				lines = append(lines, fmt.Sprintf(
					"%s adds %s at or below %s; run just gen migration --rebase on that branch", c.SHA, file, newest))
			}
		}
	}
	return lines
}

func migrationVersion(name string) string {
	version, _, _ := strings.Cut(name, "_")
	return version
}

func migrationNames(root *os.Root) ([]string, error) {
	entries, err := fs.ReadDir(root.FS(), migrationsDir)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInvalidInput, "gen.migrationNames")
	}
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

func gitParentMigrations(run Runner) func(context.Context) ([]string, error) {
	return func(ctx context.Context) ([]string, error) {
		const op = "gen.gitParentMigrations"
		out, err := run(ctx, "gt", "parent", "--no-interactive")
		parent, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
		if err != nil || parent == "" {
			return nil, invalid(op, "gt parent found no Graphite parent: "+
				"finish the restack, check out the branch and rerun gen migration --rebase")
		}
		if parent == "staging" {
			parent = migrationBase
		}
		out, err = run(ctx, "git", "ls-tree", "--name-only", parent, migrationsDir+"/")
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeInternal, op)
		}
		var names []string
		for line := range strings.Lines(string(out)) {
			if name := filepath.Base(strings.TrimSpace(line)); strings.HasSuffix(name, ".sql") {
				names = append(names, name)
			}
		}
		return names, nil
	}
}

func atlasHash(dir string) func(context.Context) error {
	return func(ctx context.Context) error {
		bin := "atlas"
		pinned := filepath.Join(dir, "..", "..", ".bin", "atlas")
		if _, err := os.Stat(pinned); err == nil {
			bin = pinned
		}
		cmd := exec.CommandContext(ctx, bin, "migrate", "hash", "--dir", "file://"+migrationsDir)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			return errs.Wrap(problem(fmt.Sprintf("%s: %v\n%s", cmd, err, out)), errs.CodeInternal, "gen.atlasHash")
		}
		return nil
	}
}
