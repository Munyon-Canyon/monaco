package agents

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	migrationsDir  = "apps/backend/migrations/"
	migrationStamp = "20060102150405"
)

func (env *Env) addedMigrations(ctx context.Context, prs []stackPR) ([][]string, error) {
	out := make([][]string, len(prs))
	for i, p := range prs {
		files, err := env.GitHub.Files(ctx, p.Number)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			if f.Status == "added" && strings.HasPrefix(f.Filename, migrationsDir) &&
				strings.HasSuffix(f.Filename, ".sql") {
				out[i] = append(out[i], f.Filename)
			}
		}
	}
	return out, nil
}

func addsAny(added [][]string) bool {
	return slices.ContainsFunc(added, func(m []string) bool { return len(m) > 0 })
}

func (env *Env) migrationWait(ctx context.Context, stack []stackPR) (flowsWait, error) {
	mine, err := env.addedMigrations(ctx, stack)
	if err != nil {
		return flowsWait{}, err
	}
	return env.waitBehind(ctx, stack, mine)
}

func (env *Env) waitBehind(ctx context.Context, stack []stackPR, mine [][]string) (flowsWait, error) {
	if !addsAny(mine) {
		return flowsWait{}, nil
	}
	queued, err := env.labeledStacks(ctx, numbers(stack))
	if err != nil {
		return flowsWait{}, err
	}
	for _, q := range queued {
		theirs, err := env.addedMigrations(ctx, q.prs)
		if err != nil {
			return flowsWait{}, err
		}
		if addsAny(theirs) {
			return flowsWait{waiting: fmt.Sprintf("queued stack #%d, which also adds a migration", q.top.Number)}, nil
		}
	}
	return flowsWait{}, nil
}

func (env *Env) migrationStep(ctx context.Context, r Record, dir string, stack []stackPR) ([]string, bool) {
	lines, handled, err := env.restackMigrations(ctx, dir, stack)
	if err != nil {
		return env.disarm(ctx, r, stack[len(stack)-1].Number, "migration restack: "+err.Error()), true
	}
	return lines, handled
}

func (env *Env) restackMigrations(ctx context.Context, dir string, stack []stackPR) ([]string, bool, error) {
	mine, err := env.addedMigrations(ctx, stack)
	if err != nil {
		return nil, false, err
	}
	wait, err := env.waitBehind(ctx, stack, mine)
	if err != nil || wait.waiting != "" {
		return nil, err == nil, err
	}
	if !addsAny(mine) {
		return nil, false, nil
	}
	top := stack[len(stack)-1]
	trunk := "origin/" + env.Config.FeatureBranch
	if _, err := env.git(ctx, "fetch", "--no-tags", "origin", env.Config.FeatureBranch); err != nil {
		return nil, false, err
	}
	if _, err := env.git(ctx, "fetch", "--no-tags", "origin", top.Head); err != nil {
		return nil, false, err
	}
	base, err := env.git(ctx, "merge-base", trunk, top.HeadOID)
	if err != nil {
		return nil, false, err
	}
	moved, err := env.git(ctx, "diff", "--name-only", strings.TrimSpace(base)+".."+trunk, "--", migrationsDir)
	if err != nil || strings.TrimSpace(moved) == "" {
		return nil, false, err
	}
	if err := env.regenMigrations(ctx, dir, stack, mine, trunk); err != nil {
		return nil, false, err
	}
	line := "armed stack #%d restacked over staging's migrations and resubmitted; waiting for stage 1"
	return []string{fmt.Sprintf(line, top.Number)}, true, nil
}

type restackError string

func (e restackError) Error() string { return string(e) }

func (env *Env) regenMigrations(ctx context.Context, dir string, stack []stackPR, mine [][]string, trunk string) error {
	root := filepath.Dir(env.Common)
	top := stack[len(stack)-1]
	if dir == root {
		return restackError(fmt.Sprintf("no worktree holds %s and the record's worktree is not on this machine; "+
			"refusing to restack in the primary checkout", top.Head))
	}
	for _, step := range [][]string{{"gt", "sync", "--no-interactive", "--no-restack"}, {"bash", "scripts/restack-regen.sh"}} {
		if _, err := env.Run(ctx, dir, "", step[0], step[1:]...); err != nil {
			return err
		}
	}
	names, err := env.git(ctx, "ls-tree", "--name-only", trunk, migrationsDir)
	if err != nil {
		return err
	}
	newest := ""
	for _, n := range strings.Fields(names) {
		if strings.HasSuffix(n, ".sql") {
			newest = max(newest, path.Base(n))
		}
	}
	for i, p := range stack {
		wd := cmp.Or(env.checkout(ctx, root, p.Head), dir)
		if newest, err = env.restackBranch(ctx, wd, p, mine[i], newest); err != nil {
			return err
		}
	}
	submit := cmp.Or(env.checkout(ctx, root, top.Head), dir)
	_, err = env.Run(ctx, submit, "", "gt", "submit", "--stack", "--no-interactive", "--draft")
	return err
}

func (env *Env) restackBranch(
	ctx context.Context,
	wd string,
	p stackPR,
	files []string,
	newest string,
) (string, error) {
	if _, err := env.Run(ctx, wd, "", "gt", "checkout", "--no-interactive", p.Head); err != nil {
		return "", err
	}
	newest, err := env.renumber(ctx, wd, files, newest)
	if err != nil {
		return "", err
	}
	dirty, err := env.Run(ctx, wd, "", "git", "status", "--porcelain")
	if err != nil {
		return "", err
	}
	if left := strings.TrimSpace(string(dirty)); left != "" {
		return "", restackError(fmt.Sprintf("%s left uncommitted files in %s:\n%s", p.Head, wd, left))
	}
	stage0 := []string{"run", "./cmd/monacoctl", "agents", "check"}
	_, err = env.Run(ctx, path.Join(wd, "apps/backend"), "", "go", stage0...)
	return newest, err
}

func (env *Env) renumber(ctx context.Context, dir string, files []string, newest string) (string, error) {
	renamed := false
	for _, file := range files {
		if path.Base(file)[:len(migrationStamp)] > newest[:len(migrationStamp)] {
			continue
		}
		at, err := time.Parse(migrationStamp, newest[:len(migrationStamp)])
		if err != nil {
			return "", fmt.Errorf("migration %s: %w", newest, err)
		}
		newest = at.Add(time.Second).Format(migrationStamp) + path.Base(file)[len(migrationStamp):]
		if _, err := env.Run(ctx, dir, "", "git", "mv", file, path.Join(path.Dir(file), newest)); err != nil {
			return "", err
		}
		renamed = true
	}
	if !renamed {
		return newest, nil
	}
	backend := path.Join(dir, "apps/backend")
	toolchain, err := goToolchain(backend)
	if err != nil {
		return "", err
	}
	if _, err := env.Run(ctx, backend, "", "env", "GOTOOLCHAIN="+toolchain, "go", "generate", "./..."); err != nil {
		return "", err
	}
	if _, err := env.Run(ctx, dir, "", "gt", "modify", "--all", "--no-interactive"); err != nil {
		return "", err
	}
	return newest, nil
}

func goToolchain(backend string) (string, error) {
	mod, err := os.ReadFile(path.Join(backend, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("read go.mod: %w", err)
	}
	for _, line := range strings.Split(string(mod), "\n") {
		if f := strings.Fields(line); len(f) >= 2 && f[0] == "toolchain" {
			return f[1], nil
		}
	}
	return "", detailErr(errs.CodeInvalidInput, "monacoctl.agents.toolchain", backend+"/go.mod has no toolchain line")
}
