package privy_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const (
	appID     = "app-fixture"
	appHidden = "app-5ecret-fixture"
)

type sent struct {
	method string
	path   string
	query  string
	header http.Header
	body   string
}

type upstream struct {
	handler   http.Handler
	transport error
	badBody   bool

	mu   sync.Mutex
	sent []sent
}

func (u *upstream) RoundTrip(r *http.Request) (*http.Response, error) {
	var body []byte
	if r.Body != nil {
		body, _ = io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
	}
	u.mu.Lock()
	u.sent = append(u.sent, sent{
		method: r.Method, path: r.URL.Path, query: r.URL.RawQuery, header: r.Header.Clone(), body: string(body),
	})
	u.mu.Unlock()
	if u.transport != nil {
		return nil, u.transport
	}
	rec := httptest.NewRecorder()
	u.handler.ServeHTTP(rec, r)
	if err := r.Context().Err(); err != nil {
		return nil, err
	}
	resp := rec.Result()
	if u.badBody {
		resp.Body = io.NopCloser(errReader{})
	}
	return resp, nil
}

func (u *upstream) requests() []sent {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]sent(nil), u.sent...)
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, errs.New(errs.CodeUpstreamUnavailable, "test.read")
}

func testConfig() config.Config {
	return config.Config{
		Privy: config.Privy{
			AppID: appID, AppSecret: appHidden, BaseURL: "http://privy.test/privy",
			VerificationKey:         fakes.PrivyVerificationKey(),
			AuthorizationPrivateKey: fakes.PrivyAuthorizationKeyConfig(),
			AuthorizationKeyID:      fakes.PrivyAuthorizationKeyID,
			WebhookSecret:           "whsec_" + webhookKey,
		},
		Timeouts: config.Timeouts{Privy: 10 * time.Second},
	}
}

func client(u *upstream) *privy.Client { return clientWith(testConfig(), u, clock.Real{}) }

func clientWith(cfg config.Config, u *upstream, clk clock.Clock) *privy.Client {
	c, err := privy.New(cfg, clk, httpclient.WithTransport(u))
	if err != nil {
		panic(err)
	}
	return c
}

func overFakes(t *testing.T) (*privy.Client, *upstream, *fakes.Server) {
	t.Helper()
	srv := fakes.New()
	u := &upstream{handler: srv}
	return client(u), u, srv
}

func script(t *testing.T, srv *fakes.Server, step fakes.Step) {
	t.Helper()
	raw, err := json.Marshal(step)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/_script", bytes.NewReader(raw)))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("script %s = %d %q", raw, rec.Code, rec.Body.String())
	}
}

func replying(status int, body string) *upstream {
	return &upstream{handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	})}
}

func wantCode(t *testing.T, err error, code errs.Code) {
	t.Helper()
	if err == nil || errs.CodeOf(err) != code {
		t.Fatalf("err = %v, want %s", err, code)
	}
}
