package agents

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	idleAfter = 20 * time.Minute
	agentType = "pstack:poteto-agent"
)

func issueNums() *regexp.Regexp { return regexp.MustCompile(`#(\d+)`) }

func closesRef() *regexp.Regexp { return regexp.MustCompile(`(?i)\bcloses\s+#(\d+)\b`) }

type dispatchIn struct {
	ticket int
	model  string
	dry    bool
	urgent bool
}

func dispatchCmd(ctx context.Context, env *Env, args []string, stdout io.Writer) error {
	in, err := parseDispatch(args)
	if err != nil {
		return err
	}
	if env.localConfig != "" {
		_, _ = fmt.Fprintf(stdout,
			"local config: %s (lanes=%d, check.slots=%d, dispatch.max_load=%d, tracking=%d, milestone=%s)\n",
			env.localConfig, env.Config.Lanes, env.Config.Slots, env.Config.MaxLoad,
			env.Config.Tracking, env.Config.Milestone)
	}
	if _, err := env.Run(ctx, env.Work, "", "git", "fetch", "origin", env.Config.FeatureBranch); err != nil {
		return err
	}
	if err := env.dispatchable(ctx, in, stdout); err != nil {
		return err
	}
	var risks strings.Builder
	if err := printForecast(ctx, env, &risks); err != nil {
		return err
	}
	tip, err := env.featureTip(ctx)
	if err != nil {
		return err
	}
	path := env.worktreePath(in.ticket)
	note, err := env.caffeinePlan(ctx, in.dry)
	if err != nil {
		return err
	}
	if err := env.logUrgent(ctx, in, stdout); err != nil {
		return err
	}
	if in.dry {
		_, _ = fmt.Fprintf(stdout, "dry-run: would add worktree %s at %s\n", path, tip)
		_, _ = fmt.Fprintf(stdout, "dry-run: would record #%d model %s state running\n", in.ticket, in.model)
		_, _ = fmt.Fprintf(stdout, "dry-run: would post the owner record on #%d\n%s\n", in.ticket, note)
		writeOwnerSpawn(stdout, in, path, tip, risks.String())
		return nil
	}
	if err := env.addWorktree(ctx, path, tip); err != nil {
		return err
	}
	rec := Record{
		Ticket: in.ticket, Model: in.model, Worktree: path, Base: tip,
		State: Running, Started: env.Now(), Changed: env.Now(),
	}
	if err := env.storeRecord(ctx, rec); err != nil {
		return err
	}
	writeOwnerSpawn(stdout, in, path, tip, risks.String())
	return nil
}

func writeOwnerSpawn(stdout io.Writer, in dispatchIn, path, tip, risks string) {
	writeSpawn(stdout, in.model, fmt.Sprintf(
		"ticket: %d\nworktree: %s\nparent: %s\nbrief: %s\norders: %s\n",
		in.ticket, path, tip, ownerBrief, standingOrders,
	))
	_, _ = io.WriteString(stdout, risks)
}

func writeSpawn(stdout io.Writer, model, prompt string) {
	_, _ = fmt.Fprintf(
		stdout, "spawn: Agent subagent_type=%s model=%s run_in_background=true, prompt:\n%s", agentType, model, prompt,
	)
}

func parseDispatch(args []string) (dispatchIn, error) {
	use := "dispatch <ticket> --model <name> [--dry-run] [--urgent]"
	var in dispatchIn
	args, in.dry = stripFlag(args, "--dry-run")
	args, in.urgent = stripFlag(args, "--urgent")
	if len(args) != 3 || args[1] != "--model" {
		return dispatchIn{}, usageError(use)
	}
	n, err := positiveInt(args[0], use)
	if err != nil {
		return dispatchIn{}, err
	}
	if args[2] != opus && args[2] != sonnet {
		return dispatchIn{}, detailErr(
			errs.CodeInvalidInput,
			"monacoctl.agents.dispatch",
			fmt.Sprintf("dispatch model must be %s or %s, got %q", opus, sonnet, args[2]),
		)
	}
	in.ticket, in.model = n, args[2]
	return in, nil
}

func (env *Env) dispatchable(ctx context.Context, in dispatchIn, stdout io.Writer) error {
	if err := env.toolsInstalled(); err != nil {
		return err
	}
	if !in.urgent {
		if err := env.inBatch(in.ticket); err != nil {
			return err
		}
	}
	if err := env.blockersClear(ctx, in.ticket); err != nil {
		return err
	}
	if err := env.lanesOpen(ctx, stdout); err != nil {
		return err
	}
	return env.gates(ctx, in, stdout)
}

const queueWindow = 60 * time.Minute

type gate struct {
	name string
	run  func(context.Context) (string, error)
}

func (env *Env) gates(ctx context.Context, in dispatchIn, stdout io.Writer) error {
	if in.urgent {
		if in.dry {
			_, _ = io.WriteString(stdout, "dry-run: load gate and queue breaker bypassed by --urgent\n")
		}
		return nil
	}
	for _, g := range []gate{{"load", env.loadGate}, {"queue", env.queueGate}} {
		ok, err := g.run(ctx)
		switch {
		case err == nil && in.dry:
			_, _ = fmt.Fprintf(stdout, "dry-run: %s gate would pass: %s\n", g.name, ok)
		case err != nil && in.dry:
			_, _ = fmt.Fprintf(stdout, "dry-run: %s gate would refuse: %s\n", g.name, cliText(err))
		case err != nil:
			return err
		}
	}
	return nil
}

func (env *Env) loadGate(ctx context.Context) (string, error) {
	load := env.Load
	if load == nil {
		load = loadAverage
	}
	v, err := load(ctx, env.GOOS)
	if err != nil {
		return "", err
	}
	if v > float64(env.Config.MaxLoad) {
		return "", detailErr(errs.CodeInvalidInput, "monacoctl.agents.dispatch", fmt.Sprintf(
			"load1 %.1f is over max_load %d; wait or dispatch with --urgent", v, env.Config.MaxLoad))
	}
	return fmt.Sprintf("load1 %.1f, max_load %d", v, env.Config.MaxLoad), nil
}

func (env *Env) queueGate(ctx context.Context) (string, error) {
	data, err := env.watchData(ctx)
	if err != nil {
		return "", err
	}
	jobs, byJob := recentFailures(data.drafts, env.Now().Add(-queueWindow))
	for _, job := range jobs {
		if nums := byJob[job]; len(nums) >= 2 {
			slices.Sort(nums)
			refs := make([]string, len(nums))
			for i, n := range nums {
				refs[i] = "#" + strconv.Itoa(n)
			}
			return "", detailErr(errs.CodeInvalidInput, "monacoctl.agents.dispatch", fmt.Sprintf(
				"queue is failing on %s (drafts %s); fix the pipeline first, or dispatch the fix with --urgent",
				job, strings.Join(refs, ", ")))
		}
	}
	return "no job failed in two queue drafts in the last 60 minutes", nil
}

func recentFailures(drafts []queueDraft, since time.Time) ([]string, map[string][]int) {
	byJob := map[string][]int{}
	var jobs []string
	for _, d := range drafts {
		cs := d.commits()
		if d.State == "OPEN" || !strings.HasPrefix(d.HeadRefName, draftPrefix) || !d.UpdatedAt.After(since) ||
			len(cs) == 0 {
			continue
		}
		job := cs[len(cs)-1].failedJob().Name
		if job == "" {
			continue
		}
		if len(byJob[job]) == 0 {
			jobs = append(jobs, job)
		}
		byJob[job] = append(byJob[job], d.Number)
	}
	return jobs, byJob
}

func (env *Env) logUrgent(ctx context.Context, in dispatchIn, stdout io.Writer) error {
	switch {
	case !in.urgent:
		return nil
	case in.dry:
		_, _ = fmt.Fprintf(stdout, "dry-run: would log the urgent dispatch to #%d\n", env.Config.Tracking)
		return nil
	}
	body := fmt.Sprintf(
		"monacoctl agents dispatch --urgent: #%d dispatched outside batch.json at %s",
		in.ticket,
		env.Now().UTC().Format(time.RFC3339),
	)
	return env.writeComment(ctx, env.Config.Tracking, 0, false, body)
}

func (env *Env) blockersClear(ctx context.Context, ticket int) error {
	is, err := env.GitHub.Issue(ctx, ticket)
	if err != nil {
		return err
	}
	reason, err := env.blockedReason(ctx, is.Body, nil)
	if err != nil {
		return err
	}
	if reason != "" {
		return detailErr(errs.CodeInvalidInput, "monacoctl.agents.dispatch", reason)
	}
	return nil
}

func (env *Env) blockerMerged(ctx context.Context, n int) error {
	is, err := env.GitHub.Issue(ctx, n)
	if err != nil {
		return err
	}
	if is.PullRequest != nil {
		return env.pullBlocker(ctx, n)
	}
	if is.State == "closed" && is.StateReason == "completed" {
		return nil
	}
	return env.issueBlocker(ctx, n)
}

func (env *Env) pullBlocker(ctx context.Context, n int) error {
	pr, err := env.GitHub.PR(ctx, n)
	if err != nil {
		return err
	}
	landed, err := env.landed(ctx, pr.closed())
	if err != nil {
		return err
	}
	if !landed {
		return detailErr(
			errs.CodeInvalidInput,
			"monacoctl.agents.dispatch",
			fmt.Sprintf("blocker #%d is not merged", n),
		)
	}
	return env.mergedIn(ctx, n, pr.landedSHA())
}

func (env *Env) issueBlocker(ctx context.Context, n int) error {
	closed, err := env.GitHub.PRs(ctx, "state=closed")
	if err != nil {
		return err
	}
	for _, pr := range closed {
		if pr.Base.Ref != env.Config.FeatureBranch || !closes(pr.Body, n) {
			continue
		}
		landed, err := env.landed(ctx, pr.closed())
		if err != nil {
			return err
		}
		if !landed {
			continue
		}
		ok, err := env.ancestor(ctx, pr.landedSHA())
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
	}
	return detailErr(
		errs.CodeInvalidInput,
		"monacoctl.agents.dispatch",
		fmt.Sprintf("blocker #%d is not merged into %s", n, env.Config.FeatureBranch),
	)
}

func (env *Env) mergedIn(ctx context.Context, n int, sha string) error {
	ok, err := env.ancestor(ctx, sha)
	if err != nil {
		return err
	}
	if !ok {
		return detailErr(
			errs.CodeInvalidInput,
			"monacoctl.agents.dispatch",
			fmt.Sprintf("blocker #%d is not in %s", n, env.Config.FeatureBranch),
		)
	}
	return nil
}

func closes(body string, n int) bool {
	for _, m := range closesRef().FindAllStringSubmatch(body, -1) {
		if m[1] == strconv.Itoa(n) {
			return true
		}
	}
	return false
}

func (env *Env) ancestor(ctx context.Context, sha string) (bool, error) {
	if _, err := env.Run(ctx, env.Work, "", "git", "cat-file", "-t", sha); err != nil {
		return false, err
	}
	_, err := env.Run(ctx, env.Work, "", "git", "merge-base", "--is-ancestor", sha, env.featureRef())
	return err == nil, nil
}

func (env *Env) featureRef() string {
	return "refs/remotes/origin/" + env.Config.FeatureBranch
}

func (env *Env) featureTip(ctx context.Context) (string, error) {
	out, err := env.Run(ctx, env.Work, "", "git", "rev-parse", "--verify", "--quiet", env.featureRef())
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (env *Env) lanesOpen(ctx context.Context, stdout io.Writer) error {
	rs, err := env.records()
	if err != nil {
		return err
	}
	n := 0
	for _, r := range rs {
		if r.State == Exited {
			continue
		}
		if !worktreeHere(r) {
			_, _ = fmt.Fprintf(stdout, "not counted: #%d (worktree on another machine)\n", r.Ticket)
			continue
		}
		is, err := env.GitHub.Issue(ctx, r.Ticket)
		if err != nil {
			return err
		}
		if is.State == "closed" {
			_, _ = fmt.Fprintf(stdout, "not counted: #%d (ticket closed)\n", r.Ticket)
			continue
		}
		n++
	}
	if n >= env.Config.Lanes {
		return detailErr(
			errs.CodeInvalidInput,
			"monacoctl.agents.dispatch",
			fmt.Sprintf("%d owners are not exited; lane cap is %d", n, env.Config.Lanes),
		)
	}
	return nil
}

func (env *Env) worktreePath(ticket int) string {
	return filepath.Join(filepath.Dir(env.Common), ".worktrees", strconv.Itoa(ticket))
}

func (env *Env) addWorktree(ctx context.Context, path, tip string) error {
	if _, err := env.Run(ctx, env.Work, "", "git", "worktree", "add", "--detach", path, tip); err != nil {
		return err
	}
	for _, tool := range pinnedTools() {
		src := env.cloneTool(tool)
		info, err := os.Stat(src)
		if err != nil {
			return fmt.Errorf("stat %s: %w", src, err)
		}
		if err := linkOrCopy(os.Link, src, filepath.Join(path, ".bin", tool), info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

func pinnedTools() []string { return []string{"atlas", "sqlc"} }

func (env *Env) cloneTool(tool string) string {
	return filepath.Join(filepath.Dir(env.Common), ".bin", tool)
}

func (env *Env) toolsInstalled() error {
	for _, tool := range pinnedTools() {
		if src := env.cloneTool(tool); !isFile(src) {
			return detailErr(
				errs.CodeInvalidInput, "monacoctl.agents.dispatch", "dispatch: "+src+" is missing; run just install",
			)
		}
	}
	return nil
}

func linkOrCopy(link func(oldname, newname string) error, src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return fmt.Errorf("make %s: %w", filepath.Dir(dst), err)
	}
	if link(src, dst) == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	_, err = io.Copy(out, in)
	if err = errors.Join(err, out.Close()); err != nil {
		return fmt.Errorf("copy %s to %s: %w", src, dst, err)
	}
	return nil
}

func (env *Env) caffeineOnPath() bool {
	look := exec.LookPath
	if env.LookPath != nil {
		look = env.LookPath
	}
	_, err := look("caffeinate")
	return err == nil
}

func (env *Env) caffeinePlan(ctx context.Context, dry bool) (string, error) {
	if !env.caffeineOnPath() {
		return "caffeinate not found; the host must stay awake on its own", nil
	}
	if env.caffeinated(ctx) {
		return "dry-run: caffeinate already running", nil
	}
	pid, err := env.claudePID(ctx)
	if err != nil {
		return "", err
	}
	note := fmt.Sprintf("caffeinate -dimsu -w %d", pid)
	if dry {
		return "dry-run: would start " + note, nil
	}
	if err := env.Start("caffeinate", "-dimsu", "-w", strconv.Itoa(pid)); err != nil {
		return "", err
	}
	return "started " + note, nil
}

func (env *Env) caffeinated(ctx context.Context) bool {
	_, err := env.Run(ctx, "", "", "pgrep", "-x", "caffeinate")
	return err == nil
}

func (env *Env) claudePID(ctx context.Context) (int, error) {
	start := os.Getpid()
	pid := start
	for range 32 {
		out, err := env.Run(ctx, "", "", "ps", "-o", "ppid=,comm=", "-p", strconv.Itoa(pid))
		if err != nil {
			return 0, err
		}
		ppidField, command, _ := strings.Cut(strings.TrimSpace(string(out)), " ")
		if strings.Contains(filepath.Base(command), "claude") {
			return pid, nil
		}
		ppid, err := strconv.Atoi(ppidField)
		if err != nil || ppid <= 1 {
			break
		}
		pid = ppid
	}
	return 0, detailErr(
		errs.CodeInvalidInput,
		"monacoctl.agents.dispatch",
		fmt.Sprintf("watchdog: no claude process above pid %d", start),
	)
}
