package agents

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

func conflictsCmd(ctx context.Context, env *Env, args []string, stdout io.Writer) error {
	n, err := prArg(args, "conflicts <pr>")
	if err != nil {
		return err
	}
	pr, err := env.GitHub.PR(ctx, n)
	if err != nil {
		return err
	}
	behind, files, err := env.mergeTree(ctx, pr)
	if err != nil {
		return err
	}
	ticket, ok := pr.Ticket()
	rec := Record{}
	if ok {
		rec, err = env.localRecord(ticket)
		if err != nil && errs.CodeOf(err) != errs.CodeNotFound {
			return err
		}
	}
	return writeRebase(stdout, pr, rec, behind, files)
}

func (env *Env) mergeTree(ctx context.Context, pr PR) (bool, []string, error) {
	base := "origin/" + pr.Base.Ref
	if _, err := env.Run(ctx, env.Work, "", "git", "rev-parse", "--verify", "--quiet", base); err != nil {
		return false, nil, err
	}
	if _, err := env.Run(ctx, env.Work, "", "git", "rev-parse", "--verify", "--quiet", pr.Head.SHA); err != nil {
		return false, nil, err
	}
	_, anc := env.Run(ctx, env.Work, "", "git", "merge-base", "--is-ancestor", base, pr.Head.SHA)
	out, err := env.Run(
		ctx, env.Work, "", "git", "merge-tree",
		"--write-tree", "--name-only", "--no-messages", base, pr.Head.SHA,
	)
	files, err := mergeConflict(out, err)
	return anc != nil, files, err
}

func mergeConflict(out []byte, err error) ([]string, error) {
	files := pathsAfterTree(out)
	if err == nil {
		return nil, nil
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		return nil, err
	}
	return files, nil
}

func pathsAfterTree(out []byte) []string {
	lines := splitLines(string(out))
	if len(lines) < 2 {
		return nil
	}
	return lines[1:]
}

func splitLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func writeRebase(stdout io.Writer, pr PR, rec Record, behind bool, files []string) error {
	state := "no"
	if behind {
		state = "yes"
	}
	_, _ = fmt.Fprintf(stdout, "#%d behind: %s\nworktree: %s\nagent: %s\n", pr.Number, state, rec.Worktree, rec.AgentID)
	for i, f := range files {
		if i == maxLines-5 {
			_, _ = fmt.Fprintf(stdout, "and %d more files\n", len(files)-i)
			break
		}
		_, _ = fmt.Fprintf(stdout, "file: %s\n", f)
	}
	_, _ = fmt.Fprintln(stdout, "update: after the root's gt sync --no-interactive --no-restack, run gt restack, "+
		"monacoctl agents check and gt submit --stack --no-interactive --draft in the worktree")
	return nil
}

func ownCmd(ctx context.Context, env *Env, args []string, _ io.Writer) error {
	return setAgent(ctx, env, args)
}

func setAgent(ctx context.Context, env *Env, args []string) error {
	const use = "own <ticket> <agent-id>"
	if len(args) != 2 {
		return usageError(use)
	}
	n, err := positiveInt(args[0], use)
	if err != nil {
		return err
	}
	r, err := env.record(ctx, n)
	if err != nil {
		return err
	}
	_, err = env.withRecordLock(r.Ticket, func(r Record) (Record, error) {
		r.AgentID = args[1]
		r.Changed = env.Now()
		return r, nil
	})
	return err
}

func doneCmd(ctx context.Context, env *Env, args []string, _ io.Writer) error {
	return setState(ctx, env, args, Done, "done <ticket>")
}

func exitedCmd(ctx context.Context, env *Env, args []string, _ io.Writer) error {
	return setState(ctx, env, args, Exited, "exited <ticket>")
}

func (r Record) heldTop() (int, bool) {
	switch {
	case r.Queued != nil:
		return r.Queued.Top, true
	case r.Armed != nil:
		return r.Armed.Top, true
	}
	return 0, false
}

func setState(ctx context.Context, env *Env, args []string, state State, use string) error {
	if len(args) != 1 {
		return usageError(use)
	}
	n, err := positiveInt(args[0], use)
	if err != nil {
		return err
	}
	if _, err := env.record(ctx, n); err != nil {
		return err
	}
	r, err := env.withRecordLock(n, func(r Record) (Record, error) {
		if top, held := r.heldTop(); held && state == Exited {
			return r, detailErr(errs.CodeInvalidInput, "monacoctl.agents.exited", fmt.Sprintf(
				"#%d still holds stack #%d in the merge queue; run monacoctl agents dequeue %d first", n, top, top))
		}
		r.State = state
		r.Changed = env.Now()
		return r, nil
	})
	if err != nil {
		return err
	}
	return env.publishRecord(ctx, r)
}
