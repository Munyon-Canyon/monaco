package marketing_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/cmd/monacoctl/marketing"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	xToken   = "x-bearer-secret"
	botToken = "123456:tg-secret"
	chatID   = "-1001234567890"
)

const watchConfig = `
daily_cap = 30
daily_spend_usd = 5.0
min_score = 5.0
languages = ["en", "es"]
default_angle = "Read the thread first."

[exclude]
retweets = true
replies = true
terms = ["giveaway"]
cashtag_only = true
min_followers = 200

[score]
recency = 3.0
followers = 2.0
question = 1.5
no_link = 1.5
keyword = 2.0
recency_minutes = 100
follower_min = 500
follower_max = 200000

[[rule]]
name = "group chat investing"
query = '"group chat" (stocks OR invest OR portfolio)'
weight = 1.0

[[rule]]
name = "investment club"
query = '"investment club"'
weight = 1.0

[[rule]]
name = "tokenized stocks"
query = '"tokenized stocks"'
weight = 1.0

[[rule]]
name = "xStocks"
query = '"xStocks"'
weight = 1.0

[[rule]]
name = "FOMO app"
query = '"FOMO app"'
weight = 1.0

[[rule]]
name = "memecoin loss"
query = 'lost memecoin'
weight = 1.0

[[rule]]
name = "Robinhood Social"
query = '"Robinhood Social"'
weight = 1.0

[[rule]]
name = "copy trading"
query = '"copy trading"'
weight = 1.0

[[rule]]
name = "day trading discord"
query = '"day trading" discord'
weight = 1.0

[[angle]]
keywords = ["copy trading", "fomo app"]
text = "Copy trading copies one person."

[[angle]]
keywords = ["group chat"]
text = "This is the whole product."

[[angle]]
keywords = ["memecoin"]
text = "Rough week, no pitch."
`

func fixtureNow() time.Time { return time.Date(2026, 10, 18, 10, 5, 0, 0, time.UTC) }

type xRequest struct {
	method string
	path   string
	query  url.Values
	auth   string
}

type xFake struct {
	rules     []marketing.Rule
	mu        sync.Mutex
	pages     map[string]string
	fail      map[string]int
	requests  []xRequest
	onRequest func()
}

func (f *xFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	q := r.URL.Query()
	f.requests = append(
		f.requests,
		xRequest{method: r.Method, path: r.URL.Path, query: q, auth: r.Header.Get("Authorization")},
	)
	if f.onRequest != nil {
		f.onRequest()
	}
	name := f.ruleOf(q.Get("query"))
	if status := f.fail[name]; status != 0 {
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `{"title": "Forced", "detail": "the test said so"}`)
		return
	}
	body, ok := f.pages[name]
	if !ok {
		body = `{"meta": {"result_count": 0}}`
	}
	_, _ = io.WriteString(w, body)
}

func (f *xFake) ruleOf(query string) string {
	for _, r := range f.rules {
		if strings.Contains(query, "("+r.Query+")") {
			return r.Name
		}
	}
	return ""
}

func (f *xFake) sent() []xRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]xRequest(nil), f.requests...)
}

func (f *xFake) cancelOnRequest(cancel func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onRequest = cancel
}

func (f *xFake) forget() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = nil
}

func (f *xFake) serve(page map[string]string, fail map[string]int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pages, f.fail = page, fail
}

type tgFake struct {
	mu     sync.Mutex
	status int
	paths  []string
	chats  []string
	texts  []string
}

func (f *tgFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var body struct {
		ChatID string `json:"chat_id"`
		Text   string `json:"text"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.paths = append(f.paths, r.URL.Path)
	if f.status != 0 {
		w.WriteHeader(f.status)
		_, _ = io.WriteString(w, `{"ok": false, "description": "forced"}`)
		return
	}
	f.chats = append(f.chats, body.ChatID)
	f.texts = append(f.texts, body.Text)
	_, _ = io.WriteString(w, `{"ok": true, "result": {}}`)
}

func (f *tgFake) messages() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.texts...)
}

func (f *tgFake) calls() (paths, chats []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.paths...), append([]string(nil), f.chats...)
}

func (f *tgFake) failWith(status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status = status
}

type bench struct {
	t     *testing.T
	repo  string
	clock *testkit.Clock
	x     *xFake
	tg    *tgFake
	deps  marketing.Deps
}

func newBench(t *testing.T, config string) *bench {
	t.Helper()
	repo := t.TempDir()
	dir := filepath.Join(repo, "apps", "backend")
	for _, d := range []string{dir, filepath.Join(repo, ".monaco")} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, marketing.ConfigPath), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	b := &bench{
		t: t, repo: repo, clock: testkit.NewClock(fixtureNow()),
		x:  &xFake{rules: mustParse(t, config).Rules, pages: fixturePages(t)},
		tg: &tgFake{},
	}
	xsrv, tgsrv := httptest.NewServer(b.x), httptest.NewServer(b.tg)
	t.Cleanup(xsrv.Close)
	t.Cleanup(tgsrv.Close)
	b.deps = marketing.Deps{
		Environ: []string{
			"HOME=/nowhere", "X_BEARER_TOKEN=" + xToken, "TELEGRAM_BOT_TOKEN=" + botToken, "TELEGRAM_CHAT_ID=" + chatID,
		},
		Dir: dir, Clock: b.clock, XBase: xsrv.URL, TelegramBase: tgsrv.URL,
	}
	return b
}

func (b *bench) rewriteConfig(text string) {
	b.t.Helper()
	if err := os.WriteFile(filepath.Join(b.repo, marketing.ConfigPath), []byte(text), 0o600); err != nil {
		b.t.Fatal(err)
	}
}

func fixturePages(t *testing.T) map[string]string {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(marketing.Fixture(), &raw); err != nil {
		t.Fatal(err)
	}
	pages := make(map[string]string, len(raw))
	for name, body := range raw {
		pages[name] = string(body)
	}
	return pages
}

func (b *bench) run(args ...string) (code int, stdout, stderr string) {
	b.t.Helper()
	return b.runWith(b.t.Context(), args...)
}

func (b *bench) runWith(ctx context.Context, args ...string) (code int, stdout, stderr string) {
	b.t.Helper()
	var out, errOut bytes.Buffer
	code = marketing.Main(ctx, b.deps, args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func (b *bench) file(name string) string {
	b.t.Helper()
	raw, err := fs.ReadFile(os.DirFS(b.repo), name)
	if err != nil {
		b.t.Fatal(err)
	}
	return string(raw)
}

func (b *bench) state() marketing.State {
	b.t.Helper()
	var st marketing.State
	if err := json.Unmarshal([]byte(b.file(".build/marketing/state.json")), &st); err != nil {
		b.t.Fatal(err)
	}
	return st
}

func (b *bench) alerts() []map[string]any {
	b.t.Helper()
	var lines []map[string]any
	for line := range strings.Lines(b.file(".build/marketing/alerts.jsonl")) {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			b.t.Fatal(err)
		}
		lines = append(lines, m)
	}
	return lines
}

func onePost(id, handle, text string, created time.Time) string {
	return fmt.Sprintf(`{
  "data": [{"id": %q, "text": %q, "author_id": "u1", "created_at": %q, "entities": {}}],
  "includes": {"users": [{"id": "u1", "username": %q, "public_metrics": {"followers_count": 1500}}]},
  "meta": {"newest_id": %q, "result_count": 1}
}`, id, text, created.UTC().Format(time.RFC3339), handle, id)
}
