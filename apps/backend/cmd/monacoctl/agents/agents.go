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
	Root   string
	Config Config
	GitHub *GitHub
	Run    Runner
	Now    func() time.Time
}

type Runner func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error)

type command func(ctx context.Context, env *Env, args []string, stdout io.Writer) error

type failure string

func (f failure) Error() string { return string(f) }

func failf(format string, a ...any) error { return failure(fmt.Sprintf(format, a...)) }

type usageError string

func (u usageError) Error() string { return "usage: monacoctl agents " + string(u) }

func commands() map[string]command {
	return map[string]command{
		"forecast":    forecastCmd,
		"verify-plan": verifyPlanCmd,
	}
}

func Tool(environ []string) func(args []string, stdout, stderr io.Writer) int {
	return func(args []string, stdout, stderr io.Writer) int {
		return Main(context.Background(), environ, "", runCommand, args, stdout, stderr)
	}
}

func Main(ctx context.Context, environ []string, dir string, run Runner, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || commands()[args[0]] == nil {
		return usage(stderr)
	}
	env, err := load(ctx, environ, dir, run)
	if err == nil {
		err = commands()[args[0]](ctx, env, args[1:], stdout)
	}
	return exitCode(err, stderr)
}

func exitCode(err error, stderr io.Writer) int {
	if err == nil {
		return 0
	}
	_, _ = fmt.Fprintf(stderr, "monacoctl agents: %v\n", err)
	var u usageError
	if errors.As(err, &u) {
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
		out, err := run(ctx, dir, "", "gh", "auth", "token")
		return strings.TrimSpace(string(out)), err
	}
	return &Env{
		Root:   filepath.Dir(common),
		Config: cfg,
		GitHub: &GitHub{API: api, Repo: cfg.Repo, Token: token, HTTP: &http.Client{Timeout: 30 * time.Second}},
		Run:    run,
		Now:    time.Now,
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

func runCommand(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
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
