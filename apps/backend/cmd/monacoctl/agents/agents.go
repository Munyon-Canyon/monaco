package agents

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const defaultAPI = "https://api.github.com"

type Env struct {
	Work    string
	Common  string
	Home    string
	Config  Config
	GitHub  *GitHub
	Run     Runner
	Start   func(name string, args ...string) error
	Now     func() time.Time
	Actions bool
}
type (
	Runner  func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error)
	command func(ctx context.Context, env *Env, args []string, stdout io.Writer) error
)

func usageError(use string) error {
	cmd, _, _ := strings.Cut(use, " ")
	return detailErr(errs.CodeInvalidInput, "monacoctl.agents."+cmd, "usage: monacoctl agents "+use)
}

func detailErr(code errs.Code, op, detail string) error {
	return errs.New(code, op, slog.String("detail", detail))
}

func cliText(err error) string {
	var e *errs.Error
	if !errors.As(err, &e) {
		return err.Error()
	}
	for _, attr := range e.Attrs {
		if attr.Key != "detail" {
			continue
		}
		if text := attr.Value.String(); text != "" {
			return text
		}
	}
	return ""
}

func commands() map[string]command {
	return map[string]command{
		"batch":       batchCmd,
		"conflicts":   conflictsCmd,
		"dispatch":    dispatchCmd,
		"done":        doneCmd,
		"exited":      exitedCmd,
		"forecast":    forecastCmd,
		"handoff":     handoffCmd,
		"land-stack":  landStackCmd,
		"own":         ownCmd,
		"resume":      resumeCmd,
		"status":      statusCmd,
		"timeline":    timelineCmd,
		"verify-plan": verifyPlanCmd,
		"watch":       watchCmd,
		"verdict":     verdictCmd,
		"check":       checkCmd,
	}
}

func Main(ctx context.Context, environ []string, dir string, run Runner, args []string, stdout, stderr io.Writer) int {
	return runCLI(ctx, environ, dir, run, args, stdout, stderr, nil)
}

func runCLI(
	ctx context.Context,
	environ []string,
	dir string,
	run Runner,
	args []string,
	stdout, stderr io.Writer,
	now func() time.Time,
) int {
	args, verbose := stripFlag(args, "--verbose")
	if len(args) == 0 || commands()[args[0]] == nil {
		return usage(stderr)
	}
	env, err := load(ctx, environ, dir, run)
	var buf bytes.Buffer
	if err == nil {
		if now != nil {
			env.Now = now
		}
		err = commands()[args[0]](ctx, env, args[1:], &buf)
	}
	writeLimited(stdout, buf.String(), verbose)
	return exitCode(logged(env, args[0], buf.String(), err), stderr)
}

func logged(env *Env, command, output string, err error) error {
	if err == nil || env == nil {
		return err
	}
	if logErr := env.writeFailLog(command, output, err); logErr != nil {
		return fmt.Errorf("%w; %w", err, logErr)
	}
	return err
}

func (env *Env) writeFailLog(command, output string, err error) error {
	dir := filepath.Join(env.Common, "pstack", env.Config.Milestone, "logs")
	if mkErr := os.MkdirAll(dir, 0o750); mkErr != nil {
		return fmt.Errorf("write log: %w", mkErr)
	}
	body := output + cliText(err) + "\n"
	path := filepath.Join(dir, command+".log")
	if werr := os.WriteFile(path, []byte(body), 0o600); werr != nil {
		return fmt.Errorf("write log: %w", werr)
	}
	return nil
}

func stripFlag(args []string, flag string) ([]string, bool) {
	out := make([]string, 0, len(args))
	found := false
	for _, a := range args {
		if a == flag {
			found = true
			continue
		}
		out = append(out, a)
	}
	return out, found
}

func writeLimited(w io.Writer, s string, verbose bool) {
	if verbose || s == "" {
		_, _ = io.WriteString(w, s)
		return
	}
	lines := strings.Split(s, "\n")
	if strings.HasSuffix(s, "\n") {
		lines = lines[:len(lines)-1]
	}
	if len(lines) <= maxLines {
		_, _ = io.WriteString(w, s)
		return
	}
	kept := lines[:maxLines-1]
	_, _ = fmt.Fprintf(w, "%s\n  and %d more\n", strings.Join(kept, "\n"), len(lines)-(maxLines-1))
}

func exitCode(err error, stderr io.Writer) int {
	if err == nil {
		return 0
	}
	msg := cliText(err)
	if msg != "" {
		_, _ = fmt.Fprintf(stderr, "monacoctl agents: %s\n", msg)
	}
	if errs.KindOf(errs.CodeOf(err)) == errs.KindInvalid && strings.HasPrefix(msg, "usage:") {
		return 2
	}
	return 1
}

func usage(stderr io.Writer) int {
	_, _ = fmt.Fprintln(stderr, "usage: monacoctl agents <command> [args]")
	for _, name := range slices.Sorted(maps.Keys(commands())) {
		_, _ = fmt.Fprintf(stderr, "  %s\n", name)
	}
	return 2
}

func load(ctx context.Context, environ []string, dir string, run Runner) (*Env, error) {
	out, err := run(ctx, dir, "", "git", "rev-parse", "--path-format=absolute", "--show-toplevel", "--git-common-dir")
	if err != nil {
		return nil, fmt.Errorf("find the repository: %w", err)
	}
	top, common, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	f, err := os.Open(filepath.Join(top, configPath))
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	defer func() { _ = f.Close() }()
	cfg, err := parseConfig(f)
	if err != nil {
		return nil, err
	}
	api := cmp.Or(lookup(environ, "MONACO_GITHUB_API"), defaultAPI)
	token := func(ctx context.Context) (string, error) {
		if t := cmp.Or(lookup(environ, "GH_TOKEN"), lookup(environ, "GITHUB_TOKEN")); t != "" {
			return t, nil
		}
		got, err := run(ctx, dir, "", "gh", "auth", "token")
		return strings.TrimSpace(string(got)), err
	}
	return &Env{
		Work: top, Common: common, Home: lookup(environ, "HOME"), Config: cfg,
		GitHub: &GitHub{API: api, Repo: cfg.Repo, Token: token, HTTP: &http.Client{Timeout: 30 * time.Second}},
		Run:    run, Start: spawn, Now: time.Now,
		Actions: lookup(environ, "GITHUB_ACTIONS") == "true",
	}, nil
}

func lookup(environ []string, key string) string {
	for _, kv := range slices.Backward(environ) {
		if k, v, _ := strings.Cut(kv, "="); k == key {
			return v
		}
	}
	return ""
}

func Exec(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, bytes.TrimSpace(stderr.Bytes()))
	}
	return out, nil
}

func spawn(name string, args ...string) error {
	cmd := exec.CommandContext(context.Background(), name, args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", name, err)
	}
	return nil
}
