package agents

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const defaultAPI = "https://api.github.com"

type Env struct {
	Work   string
	Common string
	Home   string
	Config Config
	GitHub *GitHub
	Run    Runner
	Start  func(name string, args ...string) error
	Now    func() time.Time
}
type (
	Runner  func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error)
	command func(ctx context.Context, env *Env, args []string, stdout io.Writer) error
	failure string
)

func (f failure) Error() string           { return string(f) }
func failf(format string, a ...any) error { return failure(fmt.Sprintf(format, a...)) }

type usageError string

func (u usageError) Error() string { return "usage: monacoctl agents " + string(u) }

type exitError struct {
	code int
	msg  string
}

func (e exitError) Error() string { return e.msg }
func commands() map[string]command {
	return map[string]command{
		"conflicts":   conflictsCmd,
		"dispatch":    dispatchCmd,
		"done":        doneCmd,
		"exited":      exitedCmd,
		"forecast":    forecastCmd,
		"own":         ownCmd,
		"resume":      resumeCmd,
		"status":      statusCmd,
		"verify-plan": verifyPlanCmd,
		"watch":       watchCmd,
		"verdict":     verdictCmd,
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
	body := output + err.Error() + "\n"
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
	if err.Error() != "" {
		_, _ = fmt.Fprintf(stderr, "monacoctl agents: %v\n", err)
	}
	var u usageError
	if errors.As(err, &u) {
		return 2
	}
	var x exitError
	if errors.As(err, &x) {
		return x.code
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
