package agents

import (
	"context"
	"fmt"
	"io"
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
		rec, err = env.record(ticket)
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
	out, err := env.Run(ctx, env.Work, "", "git", "merge-tree", "--write-tree", "--name-only", base, pr.Head.SHA)
	files := splitLines(string(out))
	if err != nil && len(files) == 0 {
		return anc != nil, nil, err
	}
	return anc != nil, files, nil
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
		"monacoctl agents check and gt submit --stack --no-interactive in the worktree")
	return nil
}

func ownCmd(_ context.Context, env *Env, args []string, _ io.Writer) error {
	return setAgent(env, args)
}

func setAgent(env *Env, args []string) error {
	const use = "own <ticket> <agent-id>"
	if len(args) != 2 {
		return usageError(use)
	}
	n, err := positiveInt(args[0], use)
	if err != nil {
		return err
	}
	r, err := env.record(n)
	if err != nil {
		return err
	}
	r.AgentID = args[1]
	r.Changed = env.Now()
	return env.saveRecord(r)
}

func doneCmd(_ context.Context, env *Env, args []string, _ io.Writer) error {
	return setState(env, args, Done, "done <ticket>")
}

func exitedCmd(_ context.Context, env *Env, args []string, _ io.Writer) error {
	return setState(env, args, Exited, "exited <ticket>")
}

func setState(env *Env, args []string, state State, use string) error {
	if len(args) != 1 {
		return usageError(use)
	}
	n, err := positiveInt(args[0], use)
	if err != nil {
		return err
	}
	r, err := env.record(n)
	if err != nil {
		return err
	}
	r.State = state
	r.Changed = env.Now()
	return env.saveRecord(r)
}
