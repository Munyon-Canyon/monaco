package agents

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

func watchCmd(ctx context.Context, env *Env, args []string, stdout io.Writer) error {
	if len(args) != 0 {
		return usageError("watch")
	}
	rs, err := env.records()
	if err != nil {
		return err
	}
	idle, alive, running, err := env.watchLists(ctx, rs)
	if err != nil {
		return err
	}
	for _, r := range idle {
		_, _ = fmt.Fprintf(stdout, "idle: #%d %s\n", r.Ticket, r.Worktree)
	}
	for _, r := range alive {
		_, _ = fmt.Fprintf(stdout, "done but alive: #%d %s\n", r.Ticket, r.Worktree)
	}
	if running && !env.caffeinated(ctx) {
		_, _ = fmt.Fprintln(stdout, "watchdog: missing caffeinate")
	}
	if len(idle) > 0 || len(alive) > 0 {
		return exitError{code: 1}
	}
	return nil
}

func (env *Env) watchLists(ctx context.Context, rs []Record) ([]Record, []Record, bool, error) {
	var idle, alive []Record
	running := false
	for _, r := range rs {
		switch r.State {
		case Running:
			running = true
			last, err := env.lastActivity(ctx, r)
			if err != nil {
				return nil, nil, false, err
			}
			if env.Now().Sub(last) > idleAfter {
				idle = append(idle, r)
			}
		case Done:
			ok, err := env.alive(ctx, r.Worktree)
			if err != nil {
				return nil, nil, false, err
			}
			if ok {
				alive = append(alive, r)
			}
		case Exited:
		}
	}
	return idle, alive, running, nil
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
	prs, err := env.GitHub.PRs(ctx, "state=open")
	if err != nil {
		return time.Time{}, err
	}
	for _, pr := range prs {
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
	out, err := env.Run(ctx, "", "", "lsof", "-d", "cwd", "-Fn")
	if err != nil {
		return false, err
	}
	return strings.Contains(string(out), worktree), nil
}
