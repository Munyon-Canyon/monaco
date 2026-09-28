package agents

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
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

type Runner func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error)

type failure string

func (f failure) Error() string           { return string(f) }
func failf(format string, a ...any) error { return failure(fmt.Sprintf(format, a...)) }

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
