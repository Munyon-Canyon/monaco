package agents

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	attestContext  = "stage0/ok"
	ciOkEvery      = 20 * time.Second
	ciOkFor        = 10 * time.Minute
	mergeableEvery = 2 * time.Second
	mergeableFor   = time.Minute
	queuedLookback = 3 * time.Hour
	stage2For      = 5 * time.Minute
	tailLines      = 20
	watchedPaths   = "apps/backend packages/flows"
)

func sha12(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

func tail(out []byte, err error) string {
	text := strings.TrimSpace(string(out))
	if err != nil {
		text = strings.TrimSpace(text + "\n" + err.Error())
	}
	lines := strings.Split(text, "\n")
	return strings.Join(lines[max(0, len(lines)-tailLines):], "\n")
}

type shell struct {
	env *Env
	err error
}

func (s *shell) run(ctx context.Context, dir, name string, args ...string) string {
	if s.err != nil {
		return ""
	}
	var out []byte
	out, s.err = s.env.Run(ctx, dir, "", name, args...)
	return strings.TrimSpace(string(out))
}

func (env *Env) treeOfHead(ctx context.Context, dir, sha string) (string, error) {
	s := &shell{env: env}
	tree := s.run(ctx, dir, "git", "rev-parse", sha+"^{tree}")
	if s.err != nil {
		s.err = nil
		s.run(ctx, dir, "git", "fetch", "--quiet", "origin", sha)
		tree = s.run(ctx, dir, "git", "rev-parse", sha+"^{tree}")
	}
	if s.err != nil {
		return "", landFailed(s.err)
	}
	return tree, nil
}

func (env *Env) attestation(ctx context.Context, dir string, p stackPR) (found, valid bool, err error) {
	statuses, err := env.GitHub.Statuses(ctx, p.HeadOID)
	if err != nil {
		return false, false, err
	}
	var newest GHStatus
	for _, s := range statuses {
		if s.Context == attestContext && (!found || s.CreatedAt.After(newest.CreatedAt)) {
			newest, found = s, true
		}
	}
	if !found || newest.State != "success" {
		return found, false, nil
	}
	tree, err := env.treeOfHead(ctx, dir, p.HeadOID)
	return true, newest.Description == "tree "+tree, err
}

func (env *Env) attested(ctx context.Context, dir string, stack []stackPR) (bool, error) {
	top := stack[len(stack)-1]
	found, valid, err := env.attestation(ctx, dir, top)
	if err != nil || !found {
		return false, err
	}
	ok := map[int]bool{top.Number: valid}
	for _, p := range stack[:len(stack)-1] {
		if _, ok[p.Number], err = env.attestation(ctx, dir, p); err != nil {
			return false, err
		}
	}
	for _, p := range stack {
		if !ok[p.Number] {
			return false, landErr(fmt.Sprintf("#%d has no stage 0 attestation for head %s; "+
				"run monacoctl agents check and scripts/pr-body.sh on it, then land-stack again",
				p.Number, sha12(p.HeadOID)))
		}
	}
	return true, nil
}

type stage2 struct {
	env    *Env
	cancel context.CancelFunc
	done   chan error
	once   sync.Once
	err    error
	note   string
	ran    bool
	start  time.Time
}

func (s *stage2) join() error {
	s.once.Do(func() { s.err = <-s.done })
	return s.err
}

func (s *stage2) stop() {
	s.cancel()
	_ = s.join()
}

func (s *stage2) wait(stdout io.Writer) error {
	if err := s.join(); err != nil {
		return err
	}
	line := s.note
	if s.ran {
		line += fmt.Sprintf(": ok in %ds", int(s.env.Now().Sub(s.start).Seconds()))
	}
	_, _ = fmt.Fprintln(stdout, line)
	return nil
}

func (env *Env) startStage2(ctx context.Context, dir string, top stackPR) (*stage2, error) {
	trunk := "origin/" + env.Config.FeatureBranch
	sh := &shell{env: env}
	base := sh.run(ctx, dir, "git", "merge-base", trunk, top.HeadOID)
	diff := append([]string{"diff", "--name-only", base, trunk, "--"}, strings.Fields(watchedPaths)...)
	moved := strings.Fields(sh.run(ctx, dir, "git", diff...))
	if sh.err != nil {
		return nil, landFailed(sh.err)
	}
	runCtx, cancel := context.WithTimeout(ctx, stage2For)
	s := &stage2{env: env, cancel: cancel, done: make(chan error, 1), start: env.Now(), ran: len(moved) > 0}
	if !s.ran {
		s.note = fmt.Sprintf("trunk unchanged under apps/backend and packages/flows since %s; "+
			"stage 0's ready row stands", sha12(base))
		s.done <- nil
		return s, nil
	}
	s.note = fmt.Sprintf("stage 2 on the merged tree (trunk moved in %d files since %s)", len(moved), sha12(base))
	go func() { s.done <- env.stage2On(runCtx, dir, top) }()
	return s, nil
}

func (env *Env) stage2On(ctx context.Context, dir string, top stackPR) error {
	trunk := "origin/" + env.Config.FeatureBranch
	root := filepath.Dir(env.Common)
	wt := filepath.Join(root, ".worktrees", fmt.Sprintf(".land-%d", top.Number))
	sh := &shell{env: env}
	tree, _, _ := strings.Cut(sh.run(ctx, dir, "git", "merge-tree", "--write-tree", trunk, top.HeadOID), "\n")
	msg := fmt.Sprintf("land-stack #%d: stage 2 on the merged tree", top.Number)
	commit := sh.run(ctx, dir, "git", "commit-tree", tree, "-p", trunk, "-m", msg)
	defer func() {
		bg := context.WithoutCancel(ctx)
		_, _ = env.Run(bg, dir, "", "git", "worktree", "remove", "--force", "--force", wt)
		_, _ = env.Run(bg, dir, "", "git", "worktree", "prune")
	}()
	sh.run(ctx, dir, "git", "worktree", "add", "--detach", wt, commit)
	if sh.err != nil {
		return landFailed(sh.err)
	}
	env.copyTools(ctx, root, wt)
	if out, err := env.Run(ctx, wt, "", "scripts/ci/ready.sh"); err != nil {
		return landErr(fmt.Sprintf("stage 2 on the merged tree failed: %s. "+
			"Restack with gt restack, run monacoctl agents check, then land-stack again", tail(out, err)))
	}
	return nil
}

func (env *Env) copyTools(ctx context.Context, root, wt string) {
	src := filepath.Join(root, ".bin")
	if _, err := os.Stat(src); err == nil {
		_, _ = env.Run(ctx, root, "", "cp", "-R", src, filepath.Join(wt, ".bin"))
	}
}

func (env *Env) landDirect(ctx context.Context, rec Record, dir string, stack []stackPR, stdout io.Writer) error {
	nums := numbers(stack)
	top := stack[len(stack)-1]
	_, _ = fmt.Fprintf(stdout, "landing %s on stage 0\n", prRefs(nums))
	if err := env.mergeable(ctx, dir, stack[0], top.Number); err != nil {
		return err
	}
	s2, err := env.startStage2(ctx, dir, top)
	if err != nil {
		return err
	}
	defer s2.stop()
	pending, err := env.waitCIOk(ctx, nums)
	if err != nil {
		return err
	}
	if pending != nil {
		s2.stop()
		_, _ = fmt.Fprintf(stdout, "ci-ok still pending after 10m; armed #%d for the queue path\n", top.Number)
		return env.arm(ctx, rec, stack, waitingOn(pending), stdout)
	}
	if err := s2.wait(stdout); err != nil {
		return err
	}
	landed, sha, err := env.squashBottomUp(ctx, stack, stdout)
	if len(landed) > 0 {
		err = errors.Join(err, env.recordLanding(ctx, rec, stack, landed, sha, stdout))
	}
	return err
}

func (env *Env) waitCIOk(ctx context.Context, nums []int) ([]stackPR, error) {
	deadline := env.Now().Add(ciOkFor)
	for {
		fresh, err := env.stackPulls(ctx, nums)
		if err != nil {
			return nil, err
		}
		if b := blocker(fresh); b != "" {
			return nil, landErr(b)
		}
		if len(waitingOn(fresh)) == 0 {
			return nil, nil
		}
		if !env.Now().Before(deadline) {
			return fresh, nil
		}
		<-env.After(ciOkEvery)
	}
}

func (env *Env) squashConflict(ctx context.Context, dir string, stack []stackPR) (string, error) {
	if len(stack) < 2 {
		return "", nil
	}
	trunk := env.Config.FeatureBranch
	prev := "origin/" + trunk
	lower := 0
	for i, p := range stack {
		out, err := env.Run(
			ctx,
			dir,
			"",
			"git",
			"merge-tree",
			"--write-tree",
			"--name-only",
			"--no-messages",
			prev,
			p.HeadOID,
		)
		files, err := mergeConflict(out, err)
		if err != nil {
			return "", landFailed(err)
		}
		if len(files) > 0 {
			if i == 0 {
				return "", nil
			}
			return fmt.Sprintf("#%d would conflict with %s after #%d squashes (%s)",
				p.Number, trunk, lower, strings.Join(files, ", ")), nil
		}
		if i == len(stack)-1 {
			break
		}
		tree, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
		commit, err := env.Run(
			ctx,
			dir,
			"",
			"git",
			"commit-tree",
			tree,
			"-p",
			prev,
			"-m",
			"land-stack: simulated squash of #"+strconv.Itoa(p.Number),
		)
		if err != nil {
			return "", landFailed(err)
		}
		prev, lower = strings.TrimSpace(string(commit)), p.Number
	}
	return "", nil
}

func (env *Env) reopenClosed(ctx context.Context, stack []stackPR, stdout io.Writer) error {
	for _, p := range stack {
		got, err := env.GitHub.PR(ctx, p.Number)
		if err != nil {
			return landErr(apiMessage(err))
		}
		if got.State != "closed" || got.MergedAt != nil {
			continue
		}
		if err := env.GitHub.Reopen(ctx, p.Number); err != nil {
			return landErr(apiMessage(err))
		}
		_, _ = fmt.Fprintf(stdout, "#%d was closed by GitHub when its base branch went away; reopened\n", p.Number)
	}
	return nil
}

func (env *Env) squashBottomUp(ctx context.Context, stack []stackPR, stdout io.Writer) ([]stackPR, string, error) {
	trunk := env.Config.FeatureBranch
	top := stack[len(stack)-1].Number
	if err := env.reopenClosed(ctx, stack, stdout); err != nil {
		return nil, "", err
	}
	notLanded := func(err error, i int) error {
		if i == 0 {
			return err
		}
		return landErr(fmt.Sprintf("%s\n#%d is on %s; #%d is not: fix and run land-stack %d again",
			cliText(err), stack[i-1].Number, trunk, stack[i].Number, top))
	}
	var landed []stackPR
	var sha string
	for i, p := range stack {
		got, err := env.squash(ctx, stack, i, trunk)
		if err != nil {
			return landed, sha, notLanded(err, i)
		}
		sha = got
		landed = append(landed, p)
		_, _ = fmt.Fprintf(stdout, "#%d landed as %s\n", p.Number, sha12(sha))
		if i+1 < len(stack) {
			if err := env.retarget(ctx, stack[i+1], p, trunk); err != nil {
				return landed, sha, notLanded(err, i+1)
			}
			stack[i+1].Base = trunk
		}
		if err := env.GitHub.DeleteBranch(ctx, p.Head); err != nil {
			_, _ = fmt.Fprintf(stdout, "#%d: branch %s was not deleted: %s\n", p.Number, p.Head, apiMessage(err))
		}
	}
	return landed, sha, nil
}

func (env *Env) squash(ctx context.Context, stack []stackPR, i int, trunk string) (string, error) {
	p := stack[i]
	if p.Base != trunk && i == 0 {
		return "", landErr(
			fmt.Sprintf("#%d is the bottom of its stack but its base %s is not %s", p.Number, p.Base, trunk),
		)
	}
	if p.Base != trunk {
		return "", landErr(
			fmt.Sprintf("#%d's base %s is not %s after #%d landed", p.Number, p.Base, trunk, stack[i-1].Number),
		)
	}
	title := fmt.Sprintf("%s (#%d)", p.Title, p.Number)
	sha, err := env.GitHub.Squash(ctx, p.Number, p.HeadOID, title, p.Body)
	if err != nil {
		return "", landErr(fmt.Sprintf("#%d did not merge: %s", p.Number, apiMessage(err)))
	}
	return sha, nil
}

func (env *Env) retarget(ctx context.Context, p, prev stackPR, trunk string) error {
	if err := env.GitHub.Retarget(ctx, p.Number, trunk); err != nil {
		return landErr(apiMessage(err))
	}
	deadline := env.Now().Add(mergeableFor)
	for {
		ok, err := env.GitHub.Mergeable(ctx, p.Number)
		switch {
		case err != nil:
			return landErr(apiMessage(err))
		case ok != nil && *ok:
			return nil
		case ok != nil:
			return landErr(
				fmt.Sprintf("#%d is not mergeable onto %s after #%d landed; restack and run land-stack again",
					p.Number, trunk, prev.Number),
			)
		case !env.Now().Before(deadline):
			return landErr(fmt.Sprintf("#%d mergeability onto %s is still unknown after %s; run land-stack again",
				p.Number, trunk, mergeableFor))
		}
		<-env.After(mergeableEvery)
	}
}

func (env *Env) recordLanding(
	ctx context.Context, rec Record, stack, landed []stackPR, sha string, stdout io.Writer,
) error {
	nums := numbers(landed)
	top := nums[len(nums)-1]
	now := env.Now()
	err := env.updateRecord(ctx, rec.Ticket, func(r *Record) {
		r.Landed = append(r.Landed, Landing{Top: top, PRs: nums, SHA: sha, At: now})
		r.Armed = r.Armed.without(top)
		if len(landed) == len(stack) {
			detail := fmt.Sprintf("stack #%d landed on stage 0 (%s) as %s", top, prRefs(nums), sha12(sha))
			r.Settled = &Settlement{Top: top, PRs: nums, Outcome: outcomeLanded, Detail: detail, At: now}
		}
		r.Changed = now
	})
	if err != nil {
		return err
	}
	queued, err := env.queuedDispatchedRun(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(stdout, "watch error: list queued staging runs: %s\n", cliText(err))
	}
	if queued {
		_, _ = fmt.Fprintf(stdout, "staging run for %s will share the next run (a dispatched run is still queued)\n",
			sha12(sha))
		return nil
	}
	err = env.GitHub.DispatchWorkflow(ctx, indicatorWorkflow, env.Config.FeatureBranch, map[string]string{"sha": sha})
	if err != nil {
		_, _ = fmt.Fprintf(stdout, "watch error: dispatch staging run: %s\n", cliText(err))
		return nil
	}
	_, _ = fmt.Fprintf(stdout, "dispatched staging run for %s; agents watch reverts %s if it fails\n",
		sha12(sha), prRefs(nums))
	return nil
}

func (env *Env) queuedDispatchedRun(ctx context.Context) (bool, error) {
	runs, err := env.GitHub.DispatchedRuns(ctx, indicatorWorkflow, env.Config.FeatureBranch,
		env.Now().Add(-queuedLookback))
	return slices.ContainsFunc(runs, func(r Run) bool { return r.Status == "queued" }), err
}
