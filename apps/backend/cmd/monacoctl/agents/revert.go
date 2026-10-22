package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	runWithin     = 10 * time.Minute
	runGiveUpAt   = 30 * time.Minute
	revertBodyRel = ".build/revert-body.md"
)

var errNoPR = errors.New("no revert PR number")

func (env *Env) landingLines(ctx context.Context, rs []Record) []string {
	var lines []string
	for _, r := range rs {
		for _, l := range r.Landed {
			if !l.Checked {
				lines = append(lines, env.followLanding(ctx, r, l)...)
			}
		}
	}
	return lines
}

func (env *Env) followLanding(ctx context.Context, r Record, l Landing) []string {
	lines, mark := env.judgeLanding(ctx, r, l)
	if mark == nil {
		return lines
	}
	err := env.updateRecord(ctx, r.Ticket, func(rec *Record) {
		for i := range rec.Landed {
			if rec.Landed[i].SHA == l.SHA {
				mark(&rec.Landed[i])
			}
		}
		rec.Changed = env.Now()
	})
	if err != nil {
		lines = append(lines, watchErr(fmt.Sprintf("landing %s: ", sha12(l.SHA)), err))
	}
	return lines
}

func (env *Env) judgeLanding(ctx context.Context, r Record, l Landing) ([]string, func(*Landing)) {
	sha, refs := sha12(l.SHA), prRefs(l.PRs)
	run, found, err := env.stagingRun(ctx, l)
	if err != nil {
		return []string{watchErr(fmt.Sprintf("landing %s: ", sha), err)}, nil
	}
	if !found {
		return env.noLandingRun(ctx, l)
	}
	var mark func(*Landing)
	if l.RunID == 0 {
		mark = func(x *Landing) { x.RunID, x.RunURL = run.ID, run.URL }
	}
	if !run.done() {
		return nil, mark
	}
	call := landingCall{lines: []string{fmt.Sprintf("staging run for %s passed (%s)", sha, refs)}}
	if run.Conclusion != "success" {
		call.lines = []string{fmt.Sprintf("staging run %s ended %s; not reverting %s", run.URL, run.Conclusion, refs)}
	}
	if run.broken() || run.Conclusion == "timed_out" {
		var rerr error
		call, rerr = env.relatedFailure(ctx, r, l, run)
		if rerr != nil {
			call.lines = append(call.lines, watchErr(fmt.Sprintf("revert %s: ", refs), rerr))
		}
	}
	if call.wait {
		return call.lines, nil
	}
	return call.lines, func(x *Landing) {
		x.RunID, x.RunURL = run.ID, run.URL
		if call.rerun {
			x.Reran = true
			return
		}
		x.Checked, x.Revert = true, call.revert
	}
}

type landingCall struct {
	revert int
	rerun  bool
	wait   bool
	lines  []string
}

func (env *Env) stagingRun(ctx context.Context, l Landing) (Run, bool, error) {
	if l.RunID != 0 {
		run, err := env.GitHub.Run(ctx, l.RunID)
		return run, err == nil, err
	}
	runs, err := env.GitHub.DispatchedRuns(ctx, indicatorWorkflow, env.Config.FeatureBranch, l.At)
	i := slices.IndexFunc(runs, func(x Run) bool { return x.HeadSHA == l.SHA })
	if err != nil || i < 0 {
		return Run{}, false, err
	}
	return runs[i], true, nil
}

func (env *Env) noLandingRun(ctx context.Context, l Landing) ([]string, func(*Landing)) {
	sha, age := sha12(l.SHA), env.Now().Sub(l.At)
	switch {
	case age >= runGiveUpAt:
		line := fmt.Sprintf(
			"no staging run for %s after 30m; dispatch it: gh workflow run staging.yml -f sha=%s",
			sha,
			l.SHA,
		)
		return []string{line}, func(x *Landing) { x.Checked = true }
	case age > runWithin && !l.Redispatched:
		inputs := map[string]string{"sha": l.SHA}
		if err := env.GitHub.DispatchWorkflow(ctx, indicatorWorkflow, env.Config.FeatureBranch, inputs); err != nil {
			return []string{watchErr(fmt.Sprintf("dispatch staging run for %s: ", sha), err)}, nil
		}
		return []string{
			fmt.Sprintf("no staging run for %s; dispatched one", sha),
		}, func(x *Landing) { x.Redispatched = true }
	}
	return nil, nil
}

func (env *Env) relatedFailure(ctx context.Context, r Record, l Landing, run Run) (landingCall, error) {
	refs := prRefs(l.PRs)
	failed, err := env.failedJobs(ctx, run.ID)
	if err != nil {
		return landingCall{}, err
	}
	if len(failed) == 0 {
		return landingCall{lines: []string{fmt.Sprintf("staging run %s ended %s with no failed job; not reverting %s",
			run.URL, run.Conclusion, refs)}}, nil
	}
	related := failed
	mobile, err := env.touchesMobile(ctx, l)
	if err != nil {
		return landingCall{}, err
	}
	if !mobile {
		related = slices.DeleteFunc(slices.Clone(failed), unrelatedJob)
	}
	if len(related) == 0 {
		return landingCall{
			lines: []string{fmt.Sprintf("staging run %s failed only on %s, which %s cannot affect; not reverting",
				run.URL, strings.Join(failed, ", "), refs)},
		}, nil
	}
	var lines []string
	related, redLine, err := env.dropAlreadyRed(ctx, l, run, related)
	if err != nil {
		return landingCall{}, err
	}
	if redLine != "" {
		lines = append(lines, redLine)
	}
	if len(related) == 0 {
		return landingCall{lines: lines}, nil
	}
	if !l.Reran {
		if err := env.GitHub.RerunFailed(ctx, run.ID); err != nil {
			return landingCall{wait: true, lines: append(lines, watchErr("rerun "+run.URL+": ", err))}, nil
		}
		lines = append(
			lines,
			fmt.Sprintf("staging run %s failed on %s; reran the failed jobs once before deciding on %s",
				run.URL, strings.Join(related, ", "), refs),
		)
		return landingCall{rerun: true, lines: lines}, nil
	}
	pr, more, err := env.revertLanding(ctx, r, l, run, related)
	return landingCall{revert: pr, lines: append(lines, more...)}, err
}

func (env *Env) failedJobs(ctx context.Context, runID int64) ([]string, error) {
	jobs, err := env.GitHub.Jobs(ctx, runID)
	if err != nil {
		return nil, err
	}
	var failed []string
	for _, j := range jobs {
		if slices.Contains([]string{"failure", "timed_out", "cancelled"}, j.Conclusion) {
			failed = append(failed, j.Name)
		}
	}
	return failed, nil
}

func (env *Env) dropAlreadyRed(ctx context.Context, l Landing, run Run, related []string) ([]string, string, error) {
	prior, found, err := env.GitHub.LatestStagingRunBefore(ctx, indicatorWorkflow, env.Config.FeatureBranch,
		run.CreatedAt, l.SHA)
	if err != nil || !found || prior.Conclusion == "success" {
		return related, "", err
	}
	red, err := env.failedJobs(ctx, prior.ID)
	if err != nil {
		return nil, "", err
	}
	var already []string
	remaining := slices.DeleteFunc(slices.Clone(related), func(name string) bool {
		if slices.Contains(red, name) {
			already = append(already, name)
			return true
		}
		return false
	})
	if len(already) == 0 {
		return related, "", nil
	}
	return remaining, fmt.Sprintf(
		"staging run %s failed on %s, already red on %s before %s landed; not reverting for them",
		run.URL,
		strings.Join(already, ", "),
		prior.HeadSHA[:min(8, len(prior.HeadSHA))],
		prRefs(l.PRs),
	), nil
}

func unrelatedJob(name string) bool {
	name = strings.TrimPrefix(name, "staging / ")
	return slices.ContainsFunc([]string{"ios", "mobile-core", "Flake (changed Swift"}, func(prefix string) bool {
		return len(name) >= len(prefix) && strings.EqualFold(name[:len(prefix)], prefix)
	})
}

func squashRevList(ctx context.Context, sh *shell, dir string, l Landing) []string {
	return strings.Fields(sh.run(ctx, dir, "git", "rev-list", "--first-parent", "-n", strconv.Itoa(len(l.PRs)), l.SHA))
}

func (env *Env) touchesMobile(ctx context.Context, l Landing) (bool, error) {
	root := filepath.Dir(env.Common)
	sh := &shell{env: env}
	sh.run(ctx, root, "git", "fetch", "--quiet", "origin", env.Config.FeatureBranch)
	first := ""
	for _, sha := range squashRevList(ctx, sh, root, l) {
		first = sha
	}
	changed := strings.Fields(sh.run(ctx, root, "git", "diff", "--name-only", first+"^", l.SHA))
	mobile := slices.ContainsFunc(changed, func(path string) bool {
		return strings.HasPrefix(path, "apps/mobile/") || strings.HasPrefix(path, "packages/mobile-core/")
	})
	return mobile, sh.err
}

func (env *Env) revertLanding(ctx context.Context, r Record, l Landing, run Run, jobs []string) (int, []string, error) {
	root, trunk, branch := filepath.Dir(env.Common), env.Config.FeatureBranch, fmt.Sprintf("revert-%d", l.Top)
	dir, refs := filepath.Join(root, ".worktrees", branch), prRefs(l.PRs)
	sh := &shell{env: env}
	sh.run(ctx, root, "git", "fetch", "--quiet", "origin", trunk)
	sh.run(ctx, root, "git", "worktree", "add", "-b", branch, dir, "origin/"+trunk)
	revert := append([]string{"revert", "--no-edit"}, squashRevList(ctx, sh, dir, l)...)
	sh.run(ctx, dir, "git", revert...)
	sh.run(ctx, dir, "gt", "track", branch, "--parent", trunk, "--no-interactive")
	if sh.err != nil {
		return 0, nil, fmt.Errorf("revert setup in %s: %w", dir, sh.err)
	}
	env.copyTools(ctx, root, dir)
	check := []string{"run", "./cmd/monacoctl", "agents", "check"}
	if out, err := env.Run(ctx, filepath.Join(dir, "apps", "backend"), "", "go", check...); err != nil {
		return 0, []string{fmt.Sprintf("revert of %s failed stage 0 in %s: %s; fix it there by hand",
			refs, dir, tail(out, err))}, nil
	}
	sh.run(ctx, dir, "gt", "submit", "--stack", "--no-interactive", "--draft")
	view := sh.run(ctx, dir, "gh", "pr", "view", "--json", "number")
	if sh.err != nil {
		return 0, nil, fmt.Errorf("open the revert PR from %s: %w", dir, sh.err)
	}
	var opened struct {
		Number int `json:"number"`
	}
	if err := json.Unmarshal([]byte(view), &opened); err != nil || opened.Number == 0 {
		return 0, nil, fmt.Errorf("%w in %q", errNoPR, view)
	}
	pr := opened.Number
	if err := env.setRevertBody(ctx, r, l, run, jobs, dir, pr); err != nil {
		return pr, nil, err
	}
	var landing strings.Builder
	landed := env.landStack(ctx, pr, &landing)
	lines := []string{
		fmt.Sprintf("reverted %s with #%d: staging run %s failed on %s", refs, pr, run.URL, strings.Join(jobs, ", ")),
		fmt.Sprintf("revert worktree %s stays until #%d lands; agents done is not involved", dir, pr),
	}
	for line := range strings.Lines(landing.String()) {
		lines = append(lines, strings.TrimSuffix(line, "\n"))
	}
	if landed != nil {
		lines = append(lines, watchErr(fmt.Sprintf("land revert #%d: ", pr), landed))
	}
	return pr, lines, nil
}

func (env *Env) setRevertBody(
	ctx context.Context, r Record, l Landing, run Run, jobs []string, dir string, pr int,
) error {
	top, err := env.GitHub.PR(ctx, l.Top)
	if err != nil {
		return fmt.Errorf("read #%d: %w", l.Top, err)
	}
	body := filepath.Join(dir, revertBodyRel)
	err = os.MkdirAll(filepath.Dir(body), 0o750)
	if err == nil {
		err = os.WriteFile(body, []byte(revertBody(r.Ticket, l, run, strings.Join(jobs, ", "))), 0o600)
	}
	if err != nil {
		return fmt.Errorf("write revert body: %w", err)
	}
	title := fmt.Sprintf("Revert %s after staging run failed on %s", top.Title, jobs[0])
	sh := &shell{env: env}
	sh.run(ctx, dir, "scripts/pr-body.sh", strconv.Itoa(pr), title, body)
	if sh.err != nil {
		return fmt.Errorf("pr-body in %s: %w", dir, sh.err)
	}
	return nil
}

func revertBody(ticket int, l Landing, run Run, jobs string) string {
	refs := prRefs(l.PRs)
	return fmt.Sprintf(`## TLDR

Reverts %[1]s (%[2]s): staging run %[3]s failed on %[4]s after they landed on stage 0.

## Why

Part of #%[5]d. The post-merge staging run %[3]s failed on a job the landed change can affect, so the watch reverts it to keep staging green.

## What changed

git revert of the squash commits of %[1]s, newest first.

## Proof

monacoctl agents check passed on this branch before it was pushed.

## What came up

Nothing.

## Reviewer focus

Reland after fixing %[4]s.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
`, refs, sha12(l.SHA), run.URL, jobs, ticket)
}
