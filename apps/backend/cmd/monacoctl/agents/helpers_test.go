package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	testRepo   = "o/r"
	testConfig = "repo = \"o/r\"\nfeature_branch = \"fb-checkpoint-1\"\nlanes = 2\nbatch = 2\n" +
		"verifier_app = \"99\"\nverifier_installation = 100\nmilestone = \"ms\"\n[features.fb]\ntracking = 7\n"
)

type hub struct {
	t      *testing.T
	mu     sync.Mutex
	routes map[string]string
	pages  [][2]string
	sent   map[string]string
	auth   map[string]string
}

func newHub(t *testing.T) (*hub, *httptest.Server) {
	t.Helper()
	h := &hub{t: t, routes: map[string]string{}, sent: map[string]string{}, auth: map[string]string{}}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return h, srv
}

func (h *hub) on(route string, v any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if s, ok := v.(string); ok {
		h.routes[route] = s
		return
	}
	b, err := json.Marshal(v)
	if err != nil {
		h.t.Fatal(err)
	}
	h.routes[route] = string(b)
}

func (h *hub) onQuery(has, resp string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pages = append(h.pages, [2]string{has, resp})
}

func (h *hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	route := r.Method + " " + r.URL.RequestURI()
	body, _ := io.ReadAll(r.Body)
	h.mu.Lock()
	h.sent[route] = string(body)
	h.auth[route] = r.Header.Get("Authorization")
	resp, ok := h.routes[route]
	for _, p := range h.pages {
		if strings.Contains(string(body), p[0]) {
			resp, ok = p[1], true
		}
	}
	h.mu.Unlock()
	if !ok {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		return
	}
	_, _ = io.WriteString(w, resp)
}

func (h *hub) body(route string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sent[route]
}

func (h *hub) authOf(route string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.auth[route]
}

func list(path string) string {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return "GET /repos/" + testRepo + path + sep + "per_page=100&page=1"
}
func get(path string) string { return "GET /repos/" + testRepo + path }

type fixture struct {
	dir    string
	hub    *hub
	env    []string
	run    Runner
	now    time.Time
	home   string
	repo   string
	lookup []byte
	whoami error
}

type repoSnapshot struct {
	dirs  []string
	files map[string][]byte
}

const ghUser = "me"

const repoLookup = "rev-parse --path-format=absolute --show-toplevel --git-common-dir"

func gitArgs(args ...string) []string {
	return append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)
}

func snapshotRepo(steps ...[]string) (repoSnapshot, error) {
	snap := repoSnapshot{files: map[string][]byte{}}
	dir, err := os.MkdirTemp("", "agents-repo")
	if err != nil {
		return snap, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	for _, step := range append([][]string{{"init", "-q", "--template=", "-b", "main"}}, steps...) {
		if _, err := Exec(context.Background(), dir, "", "git", gitArgs(step...)...); err != nil {
			return snap, err
		}
	}
	err = fs.WalkDir(os.DirFS(dir), ".git", func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir():
			snap.dirs = append(snap.dirs, path)
			return nil
		default:
			snap.files[path], err = os.ReadFile(filepath.Join(dir, path))
			return err
		}
	})
	return snap, err
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	return newFixtureFrom(t, emptyRepo)
}

func newFixtureFrom(t *testing.T, snap repoSnapshot) *fixture {
	t.Helper()
	dir := t.TempDir()
	home := t.TempDir()
	for _, sub := range snap.dirs {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	for path, b := range snap.files {
		writeFile(t, filepath.Join(dir, path), string(b))
	}
	top, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, configPath), testConfig)
	h, srv := newHub(t)
	return &fixture{
		dir: dir, hub: h, home: home,
		env: []string{"MONACO_GITHUB_API=" + srv.URL, "GH_TOKEN=tok", "HOME=" + home},
		run: hostless, now: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
		repo: dir, lookup: []byte(top + "\n" + filepath.Join(top, ".git") + "\n"),
	}
}

func (f *fixture) cached(run Runner) Runner {
	return func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
		if name == "git" && dir == f.repo && strings.Join(args, " ") == repoLookup {
			return f.lookup, nil
		}
		if name == "gh" && strings.Join(args, " ") == "api user --jq .login" {
			if f.whoami != nil {
				return nil, f.whoami
			}
			return []byte(ghUser + "\n"), nil
		}
		return run(ctx, dir, stdin, name, args...)
	}
}

func hostless(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
	if name == "pgrep" {
		return nil, errors.New("pgrep: no process")
	}
	return Exec(ctx, dir, stdin, name, args...)
}

func (f *fixture) agents(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := runCLI(
		context.Background(), f.env, f.dir, f.cached(f.run), args, &stdout, &stderr,
		func() time.Time { return f.now },
	)
	return code, stdout.String(), stderr.String()
}

func (f *fixture) Env(t *testing.T) *Env {
	t.Helper()
	env, err := load(context.Background(), f.env, f.dir, f.cached(f.run), "")
	if err != nil {
		t.Fatal(err)
	}
	env.Now = func() time.Time { return f.now }
	env.Start = func(string, ...string) error { return nil }
	return env
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", gitArgs(args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %q: %v\n%s", args, err, out)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func pr(n int, head, base, body string) PR {
	return PR{
		Number: n, Body: body, State: "open",
		Head: Ref{Ref: head, SHA: strings.Repeat(string(rune('a'+n%6)), 40)},
		Base: Ref{Ref: base},
	}
}

func (f *fixture) batch(t *testing.T, tickets ...int) {
	t.Helper()
	b := Batch{Created: f.now}
	for _, n := range tickets {
		b.Tickets = append(b.Tickets, BatchTicket{Ticket: n, Touches: []string{"x/**"}})
	}
	if err := f.Env(t).saveBatch(b); err != nil {
		t.Fatal(err)
	}
}

func (env *Env) statusText(ctx context.Context) (string, error) {
	open, err := env.GitHub.PRs(ctx, "state=open")
	if err != nil {
		return "", err
	}
	trunks, err := env.trunks(ctx)
	if err != nil {
		return "", err
	}
	return env.statusBody(ctx, open, trunks, "")
}
