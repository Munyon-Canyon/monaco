package agents

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	resumeTokenCap = 250_000
	ownerBrief     = "docs/agents/owner.md"
)

type resumeIn struct {
	ticket     int
	transcript string
}

func resumeCmd(ctx context.Context, env *Env, args []string, stdout io.Writer) error {
	in, err := parseResume(args)
	if err != nil {
		return err
	}
	r, err := env.record(in.ticket)
	if err != nil {
		return err
	}
	path, err := in.path(env, r)
	if err != nil {
		return err
	}
	n, err := transcriptTokens(path)
	if err != nil {
		return err
	}
	if n > resumeTokenCap {
		return env.refuseResume(ctx, stdout, r, n)
	}
	_, _ = fmt.Fprintf(stdout, "resume allowed: #%d %d tokens\n", r.Ticket, n)
	return nil
}

func parseResume(args []string) (resumeIn, error) {
	const use = "resume <ticket> [--transcript <path>]"
	var in resumeIn
	var rest []string
	for i := 0; i < len(args); i++ {
		if args[i] != "--transcript" {
			rest = append(rest, args[i])
			continue
		}
		if in.transcript != "" || i+1 >= len(args) {
			return resumeIn{}, usageError(use)
		}
		in.transcript = args[i+1]
		i++
	}
	if len(rest) != 1 {
		return resumeIn{}, usageError(use)
	}
	n, err := positiveInt(rest[0], use)
	if err != nil {
		return resumeIn{}, err
	}
	in.ticket = n
	return in, nil
}

func (in resumeIn) path(env *Env, r Record) (string, error) {
	if in.transcript != "" {
		return in.transcript, nil
	}
	return env.defaultTranscript(r)
}

func (env *Env) defaultTranscript(r Record) (string, error) {
	switch {
	case r.AgentID == "":
		return "", failure("owner has no transcript; pass --transcript")
	case env.Home == "":
		return "", failure("HOME is unset")
	default:
		slug := strings.NewReplacer("/", "-", "\\", "-").Replace(r.Worktree)
		return filepath.Join(env.Home, ".claude", "projects", slug, r.AgentID+".jsonl"), nil
	}
}

func transcriptTokens(path string) (int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read transcript: %w", err)
	}
	return (len(b) + 3) / 4, nil
}

func (env *Env) refuseResume(ctx context.Context, stdout io.Writer, r Record, tokens int) error {
	body, err := env.freshOwner(ctx, r, tokens)
	if err != nil {
		return err
	}
	_, _ = io.WriteString(stdout, body)
	return exitError{code: 1, msg: "transcript is over 250k tokens"}
}

func (env *Env) freshOwner(ctx context.Context, r Record, tokens int) (string, error) {
	findings, err := env.openFindings()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"fresh owner\nticket: %d\nworktree: %s\nparent: %s\nbrief: %s\ntokens: %d\ntrail:\n%s\nbranch:\n%s\nfindings:\n%s\n",
		r.Ticket,
		r.Worktree,
		r.Base,
		ownerBrief,
		tokens,
		env.trail(ctx, r.Worktree),
		env.branchState(ctx, r.Worktree),
		findings,
	), nil
}

func (env *Env) trail(ctx context.Context, dir string) string {
	text, ok := env.gitText(ctx, dir, "log", "--oneline", "-n", "5")
	if !ok {
		return "unavailable"
	}
	return capBlock(text, 5, "none")
}

func (env *Env) branchState(ctx context.Context, dir string) string {
	sha, ok := env.gitText(ctx, dir, "rev-parse", "HEAD")
	if !ok {
		return "unavailable"
	}
	name, ok := env.gitText(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD")
	if !ok {
		return "unavailable"
	}
	st, ok := env.gitText(ctx, dir, "status", "--porcelain")
	if !ok {
		return "unavailable"
	}
	return sha + " " + name + "\n" + capBlock(st, 3, "clean")
}

func (env *Env) gitText(ctx context.Context, dir string, args ...string) (string, bool) {
	out, err := env.Run(ctx, dir, "", "git", args...)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

func capBlock(text string, n int, empty string) string {
	lines := splitLines(text)
	if len(lines) == 0 {
		return empty
	}
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	lines = append(lines[:n], fmt.Sprintf("and %d more", len(lines)-n))
	return strings.Join(lines, "\n")
}

func (env *Env) openFindings() (string, error) {
	dir := filepath.Join(env.Common, recordsDir, "verdicts")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return "none", nil
	}
	if err != nil {
		return "", fmt.Errorf("list findings: %w", err)
	}
	var lines []string
	for _, e := range entries {
		line, err := env.findingLine(e.Name())
		if err != nil {
			return "", err
		}
		if line != "" {
			lines = append(lines, line)
		}
	}
	return capBlock(strings.Join(lines, "\n"), 3, "none"), nil
}

func findingPR(name string) (int, bool) {
	if !strings.HasSuffix(name, ".json") {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSuffix(name, ".json"))
	if err != nil {
		return 0, false
	}
	return n, true
}

func (env *Env) findingLine(name string) (string, error) {
	n, ok := findingPR(name)
	if !ok {
		return "", nil
	}
	v, err := env.loadVerdict(n)
	if err != nil {
		return "", err
	}
	if v.State != "failure" {
		return "", nil
	}
	return fmt.Sprintf("#%d %s", v.PR, v.Description), nil
}
