package agents

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const watchWorkers = 8

func watchCmd(ctx context.Context, env *Env, args []string, stdout io.Writer) error {
	once, every, err := watchArgs(args)
	if err != nil {
		return err
	}
	if !once {
		return env.watchStream(ctx, every, stdout)
	}
	defer env.traceWatch(os.Stderr)()
	rs, err := env.records()
	if err != nil {
		return err
	}
	armed, landed := env.landArmedOnce(ctx, rs)
	data, err := env.watchData(ctx)
	if err != nil {
		return err
	}
	if rs, err = env.rereadRecords(rs, landed); err != nil {
		return err
	}
	if err := env.unqueueEjected(ctx, rs, stdout); err != nil {
		return err
	}
	lines, flagged, err := env.ownerLines(ctx, rs)
	if err != nil {
		return err
	}
	lines = append(lines, armed...)
	failed, err := env.failures(ctx, rs, data)
	if err != nil {
		return err
	}
	stuck := append(stuckOnGraphiteBase(data.prs, rs, env.Config.QueueLabel), env.silentStalls(ctx, data.prs, rs)...)
	for _, line := range append(lines, stuck...) {
		_, _ = fmt.Fprintln(stdout, line)
	}
	env.writeFailures(ctx, failed, stdout)
	if flagged+len(stuck)+len(failed) > 0 {
		return errs.New(errs.CodeForbidden, "monacoctl.agents.watch")
	}
	return nil
}

func (env *Env) landArmedOnce(ctx context.Context, rs []Record) ([]string, [][]string) {
	var lines []string
	landed := env.landEachArmed(ctx, rs, map[int64]int{})
	for i, r := range rs {
		if line, stale := r.staleLine(); stale {
			lines = append(lines, line)
		}
		lines = append(lines, landed[i]...)
	}
	return lines, landed
}

func (env *Env) ownerLines(ctx context.Context, rs []Record) ([]string, int, error) {
	idle, alive, running, err := env.watchLists(ctx, rs)
	if err != nil {
		return nil, 0, err
	}
	var lines []string
	for _, r := range idle {
		lines = append(lines, fmt.Sprintf("idle: #%d %s", r.Ticket, r.Worktree))
	}
	for _, r := range alive {
		lines = append(lines, fmt.Sprintf("done but alive: #%d %s", r.Ticket, r.Worktree))
	}
	if running && env.caffeineOnPath() && !env.caffeinated(ctx) {
		lines = append(lines, "watchdog: missing caffeinate")
	}
	return lines, len(idle) + len(alive), nil
}

func (env *Env) warmWatchCaches(ctx context.Context, rs []Record) error {
	if slices.ContainsFunc(rs, func(r Record) bool { return r.State == Running }) {
		open, err := env.GitHub.PRs(ctx, "state=open")
		if err != nil {
			return err
		}
		env.openPRs = open
	}
	if slices.ContainsFunc(rs, func(r Record) bool { return r.State == Done }) {
		out, err := env.Run(ctx, "", "", "lsof", "-d", "cwd", "-Fn")
		if err != nil {
			return err
		}
		env.cwds = out
	}
	return nil
}

func (env *Env) watchLists(ctx context.Context, rs []Record) ([]Record, []Record, bool, error) {
	flagged := make([]bool, len(rs))
	running := false
	if err := env.warmWatchCaches(ctx, rs); err != nil {
		return nil, nil, false, err
	}
	defer func() { env.cwds, env.openPRs = nil, nil }()
	var g errgroup.Group
	g.SetLimit(watchWorkers)
	for i, r := range rs {
		switch r.State {
		case Running:
			running = true
			g.Go(func() error {
				stale, err := env.idle(ctx, r)
				flagged[i] = stale
				return err
			})
		case Done:
			g.Go(func() error {
				ok, err := env.alive(ctx, r.Worktree)
				flagged[i] = ok
				return err
			})
		case Exited:
		}
	}
	if err := g.Wait(); err != nil {
		return nil, nil, false, fmt.Errorf("read owner activity: %w", err)
	}
	var idle, alive []Record
	for i, r := range rs {
		switch {
		case !flagged[i]:
		case r.State == Running:
			idle = append(idle, r)
		default:
			alive = append(alive, r)
		}
	}
	return idle, alive, running, nil
}

func (env *Env) idle(ctx context.Context, r Record) (bool, error) {
	if _, err := os.Stat(r.Worktree); errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	last, err := env.lastActivity(ctx, r)
	if err != nil {
		return false, err
	}
	return env.Now().Sub(last) > idleAfter, nil
}

func (env *Env) lastActivity(ctx context.Context, r Record) (time.Time, error) {
	latest := r.Started
	if t, err := env.commitTime(ctx, r.Worktree); err != nil {
		return time.Time{}, err
	} else if t.After(latest) {
		latest = t
	}
	is, err := env.GitHub.Issue(ctx, r.Ticket)
	if err != nil {
		return time.Time{}, err
	}
	if is.UpdatedAt.After(latest) {
		latest = is.UpdatedAt
	}
	open, err := env.openIssuePRs(ctx)
	if err != nil {
		return time.Time{}, err
	}
	for _, pr := range open {
		if strings.Contains(pr.Body, "#"+strconv.Itoa(r.Ticket)) && pr.UpdatedAt.After(latest) {
			latest = pr.UpdatedAt
		}
	}
	if t, ok, err := env.pushTime(ctx, r.Worktree); err != nil {
		return time.Time{}, err
	} else if ok && t.After(latest) {
		latest = t
	}
	return latest, nil
}

func (env *Env) openIssuePRs(ctx context.Context) ([]PR, error) {
	if env.openPRs != nil {
		return env.openPRs, nil
	}
	return env.GitHub.PRs(ctx, "state=open")
}

func (env *Env) commitTime(ctx context.Context, worktree string) (time.Time, error) {
	out, err := env.Run(ctx, worktree, "", "git", "log", "-1", "--format=%ct")
	if err != nil {
		return time.Time{}, err
	}
	return unixTime(string(out))
}

func (env *Env) pushTime(ctx context.Context, worktree string) (time.Time, bool, error) {
	out, err := env.Run(ctx, worktree, "", "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return time.Time{}, false, err
	}
	branch := strings.TrimSpace(string(out))
	if branch == "" || branch == "HEAD" {
		return time.Time{}, false, nil
	}
	out, err = env.Run(ctx, worktree, "", "git", "log", "-1", "--format=%ct", "origin/"+branch)
	if err != nil {
		return time.Time{}, false, skipPush(err)
	}
	t, err := unixTime(string(out))
	return t, err == nil, err
}

func skipPush(error) error { return nil }

func unixTime(raw string) (time.Time, error) {
	n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("commit time: %w", err)
	}
	return time.Unix(n, 0).UTC(), nil
}

func (env *Env) alive(ctx context.Context, worktree string) (bool, error) {
	out := env.cwds
	if out == nil {
		var err error
		if out, err = env.Run(ctx, "", "", "lsof", "-d", "cwd", "-Fn"); err != nil {
			return false, err
		}
	}
	return strings.Contains(string(out), worktree), nil
}
