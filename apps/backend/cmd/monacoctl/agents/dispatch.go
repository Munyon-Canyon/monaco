package agents

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const idleAfter = 20 * time.Minute

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
	if _, err := env.Run(ctx, env.Work, "", "git", "fetch", "origin", env.Config.FeatureBranch); err != nil {
		return err
	}
	if err := env.dispatchable(ctx, in); err != nil {
		return err
	}
	if err := printForecast(ctx, env, stdout); err != nil {
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
		_, _ = fmt.Fprintf(stdout, "dry-run: would record #%d model %s state running\n%s\n", in.ticket, in.model, note)
		return nil
	}
	if err := env.addWorktree(ctx, path, tip); err != nil {
		return err
	}
	return env.saveRecord(Record{
		Ticket: in.ticket, Model: in.model, Worktree: path, Base: tip,
		State: Running, Started: env.Now(), Changed: env.Now(),
	})
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
	if args[2] == "" || args[2] == "fable" {
		return dispatchIn{}, detailErr(
			errs.CodeInvalidInput,
			"monacoctl.agents.dispatch",
			"dispatch model must be set and must not be fable",
		)
	}
	in.ticket, in.model = n, args[2]
	return in, nil
}

func (env *Env) dispatchable(ctx context.Context, in dispatchIn) error {
	if !in.urgent {
		if err := env.inBatch(in.ticket); err != nil {
			return err
		}
	}
	if err := env.blockersClear(ctx, in.ticket); err != nil {
		return err
	}
	return env.lanesOpen()
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
	return env.writeStatus(ctx, 0, false, body)
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
	if pr.MergedAt == nil {
		return detailErr(
			errs.CodeInvalidInput,
			"monacoctl.agents.dispatch",
			fmt.Sprintf("blocker #%d is not merged", n),
		)
	}
	return env.mergedIn(ctx, n, pr.MergeCommitSHA)
}

func (env *Env) issueBlocker(ctx context.Context, n int) error {
	closed, err := env.GitHub.PRs(ctx, "state=closed")
	if err != nil {
		return err
	}
	for _, pr := range closed {
		if pr.MergedAt == nil || pr.Base.Ref != env.Config.FeatureBranch || !closes(pr.Body, n) {
			continue
		}
		ok, err := env.ancestor(ctx, pr.MergeCommitSHA)
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

func (env *Env) lanesOpen() error {
	rs, err := env.records()
	if err != nil {
		return err
	}
	n := 0
	for _, r := range rs {
		if r.State != Exited {
			n++
		}
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
	return nil
}

func (env *Env) caffeinePlan(ctx context.Context, dry bool) (string, error) {
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
	pid := os.Getpid()
	for range 32 {
		out, err := env.Run(ctx, "", "", "ps", "-o", "ppid=,comm=", "-p", strconv.Itoa(pid))
		if err != nil {
			return 0, err
		}
		fields := strings.Fields(string(out))
		if len(fields) < 2 {
			return 0, detailErr(errs.CodeInvalidInput, "monacoctl.agents.dispatch", "watchdog: missing assertion")
		}
		if strings.Contains(fields[1], "claude") {
			return pid, nil
		}
		ppid, err := strconv.Atoi(fields[0])
		if err != nil || ppid <= 1 {
			return 0, detailErr(errs.CodeInvalidInput, "monacoctl.agents.dispatch", "watchdog: missing assertion")
		}
		pid = ppid
	}
	return 0, detailErr(errs.CodeInvalidInput, "monacoctl.agents.dispatch", "watchdog: missing assertion")
}
