package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
	if err != nil {
		return Run{}, false, err
	}
	if i := slices.IndexFunc(runs, func(x Run) bool { return x.HeadSHA == l.SHA }); i >= 0 {
		return runs[i], true, nil
	}
	return env.oldestRunContaining(ctx, l, runs)
}

func (env *Env) oldestRunContaining(ctx context.Context, l Landing, runs []Run) (Run, bool, error) {
	root := filepath.Dir(env.Common)
	sh := &shell{env: env}
	sh.run(ctx, root, "git", "fetch", "--quiet", "origin", env.Config.FeatureBranch)
	ordered := slices.SortedFunc(slices.Values(runs), func(a, b Run) int { return a.CreatedAt.Compare(b.CreatedAt) })
	for _, run := range ordered {
		base := sh.run(ctx, root, "git", "merge-base", l.SHA, run.HeadSHA)
		if sh.err != nil {
			return Run{}, false, sh.err
		}
		if base == l.SHA {
			return run, true, nil
		}
	}
	return Run{}, false, nil
}

func (env *Env) laterGreenRun(ctx context.Context, l Landing, related []string) (Run, bool, error) {
	runs, err := env.GitHub.DispatchedRuns(ctx, indicatorWorkflow, env.Config.FeatureBranch, l.At)
	if err != nil {
		return Run{}, false, err
	}
	var passed []Run
	for _, run := range runs {
		ok, err := env.passedJobs(ctx, run, related)
		if err != nil {
			return Run{}, false, err
		}
		if ok {
			passed = append(passed, run)
		}
	}
	if len(passed) == 0 {
		return Run{}, false, nil
	}
	return env.oldestRunContaining(ctx, l, passed)
}

func (env *Env) passedJobs(ctx context.Context, run Run, related []string) (bool, error) {
	if !run.done() || run.Conclusion == "success" {
		return run.done(), nil
	}
	jobs, err := env.GitHub.Jobs(ctx, run.ID)
	if err != nil {
		return false, err
	}
	for _, name := range related {
		if !slices.ContainsFunc(jobs, func(j Job) bool { return j.Name == name && j.Conclusion == "success" }) {
			return false, nil
		}
	}
	return true, nil
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
	jobs, err := env.failedJobList(ctx, run.ID)
	if err != nil {
		return landingCall{}, err
	}
	failed := jobNames(jobs)
	if len(failed) == 0 {
		return landingCall{lines: []string{fmt.Sprintf("staging run %s ended %s with no failed job; not reverting %s",
			run.URL, run.Conclusion, refs)}}, nil
	}
	failed = slices.DeleteFunc(failed, aggregateJob)
	if len(failed) == 0 {
		return landingCall{lines: []string{fmt.Sprintf("staging run %s failed only on the ci-ok aggregate "+
			"(a job was cancelled or the run was cut short); not reverting %s", run.URL, refs)}}, nil
	}
	related, err := env.relatedJobs(ctx, l, failed)
	if err != nil {
		return landingCall{}, err
	}
	if len(related) == 0 {
		return landingCall{
			lines: []string{fmt.Sprintf("staging run %s failed only on %s, which %s cannot affect; not reverting",
				run.URL, strings.Join(failed, ", "), refs)},
		}, nil
	}
	related, lines, err := env.dropAlreadyRed(ctx, l, run, related, jobs)
	if err != nil {
		return landingCall{}, err
	}
	if len(related) == 0 {
		return landingCall{lines: lines}, nil
	}
	if run.HeadSHA != "" && run.HeadSHA != l.SHA {
		return landingCall{lines: append(lines, fmt.Sprintf(
			"staging run %s covers %s and other landings; not reverting one of several", run.URL, refs))}, nil
	}
	return env.settleRerunOrRevert(ctx, r, l, run, related, lines)
}

func (env *Env) settleRerunOrRevert(
	ctx context.Context, r Record, l Landing, run Run, related, lines []string,
) (landingCall, error) {
	refs := prRefs(l.PRs)
	later, settled, err := env.laterGreenRun(ctx, l, related)
	if err != nil {
		return landingCall{}, err
	}
	if settled {
		return landingCall{lines: append(lines, fmt.Sprintf(
			"staging run %s failed, but %s at %s contains %s and passed %s; not reverting",
			run.URL, later.URL, sha12(later.HeadSHA), refs, strings.Join(related, ", ")))}, nil
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

func (env *Env) relatedJobs(ctx context.Context, l Landing, failed []string) ([]string, error) {
	changed, err := env.landingFiles(ctx, l)
	if err != nil {
		return nil, err
	}
	if docsOnly(changed) {
		return slices.DeleteFunc(slices.Clone(failed), func(name string) bool { return !docsJob(name) }), nil
	}
	if mobileOnly(changed) {
		return slices.DeleteFunc(slices.Clone(failed), backendJob), nil
	}
	if touchesMobile(changed) {
		return failed, nil
	}
	return slices.DeleteFunc(slices.Clone(failed), unrelatedJob), nil
}

func mobileOnly(changed []string) bool {
	roots := []string{"apps/mobile/", "packages/mobile-core/", "docs/"}
	return len(changed) > 0 && !slices.ContainsFunc(changed, func(path string) bool {
		inRoot := slices.ContainsFunc(roots, func(root string) bool { return strings.HasPrefix(path, root) })
		return !inRoot && !strings.HasSuffix(path, ".md")
	})
}

func docsOnly(changed []string) bool {
	return len(changed) > 0 && !slices.ContainsFunc(changed, func(path string) bool {
		return !strings.HasPrefix(path, "docs/") && !strings.HasSuffix(path, ".md")
	})
}

func docsJob(name string) bool {
	name = strings.ToLower(strings.TrimPrefix(name, "staging / "))
	return strings.Contains(name, "docs") || strings.Contains(name, "mkdocs")
}

func jobNames(jobs []Job) []string {
	names := make([]string, 0, len(jobs))
	for _, j := range jobs {
		names = append(names, j.Name)
	}
	return names
}

func (env *Env) failedJobList(ctx context.Context, runID int64) ([]Job, error) {
	jobs, err := env.GitHub.Jobs(ctx, runID)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(jobs, func(j Job) bool {
		return !slices.Contains([]string{"failure", "timed_out", "cancelled"}, j.Conclusion)
	}), nil
}

var failingNameRE = regexp.MustCompile(
	`FAIL flow (\S+ \S+)|--- FAIL: (\S+)|Test Case '-\[(\S+ \S+)\]' failed|✘ Test (\S+) failed`,
)

func failingNames(log []byte) map[string]bool {
	names := map[string]bool{}
	for _, m := range failingNameRE.FindAllSubmatch(log, -1) {
		for _, g := range m[1:] {
			if len(g) > 0 {
				names[string(g)] = true
			}
		}
	}
	return names
}

func (env *Env) newFailures(ctx context.Context, before, after Job) []string {
	if before.ID == 0 || after.ID == 0 {
		return nil
	}
	old, err := env.GitHub.JobLog(ctx, before.ID)
	if err != nil {
		return nil
	}
	fresh, err := env.GitHub.JobLog(ctx, after.ID)
	if err != nil {
		return nil
	}
	seen := failingNames(old)
	if len(seen) == 0 {
		return nil
	}
	var added []string
	for name := range failingNames(fresh) {
		if !seen[name] {
			added = append(added, name)
		}
	}
	slices.Sort(added)
	return added
}

func (env *Env) landingsSince(ctx context.Context, prior, sha string) int {
	sh := &shell{env: env}
	out := sh.run(ctx, filepath.Dir(env.Common), "git", "rev-list", "--count", "--first-parent", prior+".."+sha)
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if sh.err != nil || err != nil {
		return 0
	}
	return n
}

func (env *Env) dropAlreadyRed(
	ctx context.Context, l Landing, run Run, related []string, now []Job,
) ([]string, []string, error) {
	prior, found, err := env.GitHub.LatestStagingRunBefore(ctx, indicatorWorkflow, env.Config.FeatureBranch,
		run.CreatedAt, l.SHA)
	if err != nil || !found || prior.Conclusion == "success" {
		return related, nil, err
	}
	red, err := env.failedJobList(ctx, prior.ID)
	if err != nil {
		return nil, nil, err
	}
	byName := func(jobs []Job, name string) (Job, bool) {
		i := slices.IndexFunc(jobs, func(j Job) bool { return j.Name == name })
		if i < 0 {
			return Job{}, false
		}
		return jobs[i], true
	}
	short := prior.HeadSHA[:min(8, len(prior.HeadSHA))]
	gap := env.landingsSince(ctx, prior.HeadSHA, l.SHA) > len(l.PRs)
	var already, lines []string
	remaining := slices.DeleteFunc(slices.Clone(related), func(name string) bool {
		before, ok := byName(red, name)
		if !ok || aggregateJob(name) {
			return false
		}
		after, _ := byName(now, name)
		if added := env.newFailures(ctx, before, after); len(added) > 0 {
			if gap {
				lines = append(lines, fmt.Sprintf("staging run %s failed on %s with new failures, "+
					"but other landings sit between %s and %s; not reverting for them",
					run.URL, name, short, prRefs(l.PRs)))
				return true
			}
			lines = append(lines, fmt.Sprintf("staging run %s failed on %s, red on %s too, but with new failures: %s",
				run.URL, name, short, strings.Join(added, ", ")))
			return false
		}
		already = append(already, name)
		return true
	})
	if len(already) == 0 {
		return remaining, lines, nil
	}
	return remaining, append(lines, fmt.Sprintf(
		"staging run %s failed on %s, already red on %s before %s landed; not reverting for them",
		run.URL,
		strings.Join(already, ", "),
		short,
		prRefs(l.PRs),
	)), nil
}

func aggregateJob(name string) bool {
	return strings.TrimPrefix(name, "staging / ") == "ci-ok"
}

func unrelatedJob(name string) bool {
	name = strings.TrimPrefix(name, "staging / ")
	return slices.ContainsFunc([]string{"ios", "mobile-core", "Flake (changed Swift"}, func(prefix string) bool {
		return len(name) >= len(prefix) && strings.EqualFold(name[:len(prefix)], prefix)
	})
}

func backendJob(name string) bool {
	name = strings.TrimPrefix(name, "staging / ")
	return slices.ContainsFunc([]string{
		"Backend tests", "E2E", "Lint (golangci-lint)", "Lint (nogo, comments, OpenAPI, migrations, vuln)",
	}, func(prefix string) bool {
		return len(name) >= len(prefix) && strings.EqualFold(name[:len(prefix)], prefix)
	})
}

func squashRevList(ctx context.Context, sh *shell, dir string, l Landing) []string {
	return strings.Fields(sh.run(ctx, dir, "git", "rev-list", "--first-parent", "-n", strconv.Itoa(len(l.PRs)), l.SHA))
}

func (env *Env) landingFiles(ctx context.Context, l Landing) ([]string, error) {
	root := filepath.Dir(env.Common)
	sh := &shell{env: env}
	sh.run(ctx, root, "git", "fetch", "--quiet", "origin", env.Config.FeatureBranch)
	first := ""
	for _, sha := range squashRevList(ctx, sh, root, l) {
		first = sha
	}
	changed := strings.Fields(sh.run(ctx, root, "git", "diff", "--name-only", first+"^", l.SHA))
	return changed, sh.err
}

func touchesMobile(changed []string) bool {
	return slices.ContainsFunc(changed, func(path string) bool {
		return strings.HasPrefix(path, "apps/mobile/") || strings.HasPrefix(path, "packages/mobile-core/")
	})
}

func (env *Env) revertLanding(ctx context.Context, r Record, l Landing, run Run, jobs []string) (int, []string, error) {
	root, branch := filepath.Dir(env.Common), fmt.Sprintf("revert-%d", l.Top)
	dir, refs := filepath.Join(root, ".worktrees", branch), prRefs(l.PRs)
	if _, statErr := os.Stat(dir); l.RevertTried || statErr == nil {
		return 0, []string{fmt.Sprintf("revert of %s not retried: it failed stage 0 at %s; root: fix by hand",
			refs, r.Changed.Format(time.RFC3339))}, nil
	}
	if err := env.markRevertTried(ctx, r.Ticket, l, run); err != nil {
		return 0, nil, err
	}
	sh := &shell{env: env}
	if err := env.prepareRevert(ctx, sh, l, root, dir, branch); err != nil {
		return 0, nil, err
	}
	if out, err := env.revertStage0(ctx, dir); err != nil {
		env.dropRevert(ctx, root, dir, branch)
		return 0, []string{fmt.Sprintf("revert of %s failed stage 0 (%s): %s; not retried, root: fix by hand",
			refs, run.URL, tail(out, err))}, nil
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
	return pr, env.landRevert(ctx, pr, refs, run, jobs, root, dir, branch), nil
}

func (env *Env) landRevert(
	ctx context.Context, pr int, refs string, run Run, jobs []string, root, dir, branch string,
) []string {
	var landing strings.Builder
	landed := env.landStack(ctx, pr, &landing)
	lines := []string{
		fmt.Sprintf("reverted %s with #%d: staging run %s failed on %s", refs, pr, run.URL, strings.Join(jobs, ", ")),
	}
	if landed == nil && env.revertLanded(ctx, pr) {
		env.dropRevert(ctx, root, dir, branch)
	} else {
		lines = append(lines,
			fmt.Sprintf("revert worktree %s stays until #%d lands; agents done is not involved", dir, pr))
	}
	for line := range strings.Lines(landing.String()) {
		lines = append(lines, strings.TrimSuffix(line, "\n"))
	}
	if landed != nil {
		lines = append(lines, watchErr(fmt.Sprintf("land revert #%d: ", pr), landed))
	}
	return lines
}

func (env *Env) prepareRevert(ctx context.Context, sh *shell, l Landing, root, dir, branch string) error {
	trunk := env.Config.FeatureBranch
	sh.run(ctx, root, "git", "fetch", "--quiet", "origin", trunk)
	sh.run(ctx, root, "git", "worktree", "add", "-b", branch, dir, "origin/"+trunk)
	revert := append([]string{"revert", "--no-edit"}, squashRevList(ctx, sh, dir, l)...)
	sh.run(ctx, dir, "git", revert...)
	sh.run(ctx, dir, "gt", "track", branch, "--parent", trunk, "--no-interactive")
	if sh.err != nil {
		return fmt.Errorf("revert setup in %s: %w", dir, sh.err)
	}
	env.copyTools(ctx, root, dir)
	return nil
}

func (env *Env) revertStage0(ctx context.Context, dir string) ([]byte, error) {
	check := []string{"run", "./cmd/monacoctl", "agents", "check"}
	return env.Run(ctx, filepath.Join(dir, "apps", "backend"), "", "go", check...)
}

func (env *Env) markRevertTried(ctx context.Context, ticket int, l Landing, run Run) error {
	err := env.updateRecord(ctx, ticket, func(rec *Record) {
		for i := range rec.Landed {
			if rec.Landed[i].SHA == l.SHA {
				rec.Landed[i].Checked, rec.Landed[i].RevertTried = true, true
				rec.Landed[i].RunID, rec.Landed[i].RunURL = run.ID, run.URL
			}
		}
		rec.Changed = env.Now()
	})
	if err != nil {
		return fmt.Errorf("record the revert attempt: %w", err)
	}
	return nil
}

func (env *Env) revertLanded(ctx context.Context, pr int) bool {
	p, err := env.GitHub.PR(ctx, pr)
	if err != nil {
		return false
	}
	ok, err := env.landed(ctx, p.closed())
	return err == nil && ok
}

func (env *Env) dropRevert(ctx context.Context, root, dir, branch string) {
	bg := context.WithoutCancel(ctx)
	_, _ = env.Run(bg, root, "", "git", "worktree", "remove", "--force", dir)
	_, _ = env.Run(bg, root, "", "git", "branch", "-D", branch)
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
