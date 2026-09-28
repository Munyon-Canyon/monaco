package agents

import (
	"context"
	"encoding/json"
	"io"
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
	testConfig = "repo = \"o/r\"\nfeature_branch = \"fb\"\ntracking = 7\nlanes = 2\n" +
		"verifier_app = \"99\"\nverifier_installation = 100\n"
)

type hub struct {
	t      *testing.T
	mu     sync.Mutex
	routes map[string]string
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

func (h *hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	route := r.Method + " " + r.URL.RequestURI()
	body, _ := io.ReadAll(r.Body)
	h.mu.Lock()
	h.sent[route] = string(body)
	h.auth[route] = r.Header.Get("Authorization")
	resp, ok := h.routes[route]
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
	dir  string
	hub  *hub
	env  []string
	run  Runner
	now  time.Time
	home string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	home := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	writeFile(t, filepath.Join(dir, configPath), testConfig)
	h, srv := newHub(t)
	return &fixture{
		dir: dir, hub: h, home: home,
		env: []string{"MONACO_GITHUB_API=" + srv.URL, "GH_TOKEN=tok", "HOME=" + home},
		run: Exec, now: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
	}
}

func (f *fixture) Env(t *testing.T) *Env {
	t.Helper()
	env, err := load(context.Background(), f.env, f.dir, f.run)
	if err != nil {
		t.Fatal(err)
	}
	env.Now = func() time.Time { return f.now }
	env.Start = func(string, ...string) error { return nil }
	return env
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)
	cmd := exec.CommandContext(t.Context(), "git", full...)
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
