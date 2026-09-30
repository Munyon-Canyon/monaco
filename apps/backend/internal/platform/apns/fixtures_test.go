package apns_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type overFakes struct {
	srv   *fakes.Server
	mu    sync.Mutex
	hosts []string
}

func (o *overFakes) RoundTrip(r *http.Request) (*http.Response, error) {
	o.mu.Lock()
	o.hosts = append(o.hosts, r.URL.Host)
	o.mu.Unlock()
	req := r.Clone(r.Context())
	if !strings.HasPrefix(req.URL.Path, "/apns/") {
		req.URL.Path = "/apns" + req.URL.Path
	}
	rec := httptest.NewRecorder()
	o.srv.ServeHTTP(rec, req)
	return rec.Result(), nil
}

func (o *overFakes) requestedHosts() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return slices.Clone(o.hosts)
}

func scriptFixture(t *testing.T, srv *fakes.Server, fixture string) {
	t.Helper()
	raw, err := json.Marshal(fakes.Step{
		Route: "/apns/3/device/" + testToken, Action: fakes.ActionSucceed, Fixture: "/apns/" + fixture,
	})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/_script", bytes.NewReader(raw)))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("script %s = %d %q, want 204", raw, rec.Code, rec.Body.String())
	}
}

func fakesClient(t *testing.T, cfg config.Config, over *overFakes) *apns.Client {
	t.Helper()
	c, err := apns.New(cfg, apns.WithTransport(over))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestSend_eachFakesFixtureMapsToItsOutcome(t *testing.T) {
	t.Parallel()
	tests := []struct {
		fixture string
		want    apns.Result
		outcome apns.Outcome
	}{
		{"ok", apns.Result{Status: 200, APNsID: apnsID}, apns.Delivered},
		{"unregistered_410", apns.Result{Status: 410, Reason: "Unregistered"}, apns.TokenDead},
		{"bad_device_token_400", apns.Result{Status: 400, Reason: "BadDeviceToken"}, apns.TokenDead},
		{
			"too_many_requests_429",
			apns.Result{Status: 429, Reason: "TooManyRequests", RetryAfter: 3 * time.Second},
			apns.Retry,
		},
		{"internal_500", apns.Result{Status: 500, Reason: "InternalServerError"}, apns.Retry},
		{"forbidden_403", apns.Result{Status: 403, Reason: "InvalidProviderToken"}, apns.AuthFailed},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			t.Parallel()
			srv := fakes.New()
			scriptFixture(t, srv, tt.fixture)
			cfg := testConfig()
			cfg.APNs.BaseURL = "http://127.0.0.1:8099/apns"

			res := mustSend(t, fakesClient(t, cfg, &overFakes{srv: srv}), push(apns.Sandbox))

			if res != tt.want || apns.Classify(res) != tt.outcome {
				t.Fatalf("Send = %+v (outcome %d), want %+v (outcome %d)", res, apns.Classify(res), tt.want, tt.outcome)
			}
		})
	}
}

func TestSend_sandboxAndProductionPushesReachTheirHostsAndTheFakesServeThem(t *testing.T) {
	t.Parallel()
	over := &overFakes{srv: fakes.New()}
	c := fakesClient(t, testConfig(), over)

	for _, env := range []apns.Environment{apns.Sandbox, apns.Production} {
		if res := mustSend(t, c, push(env)); res.Status != http.StatusOK {
			t.Fatalf("%s: Send = %+v, want the fakes to accept a push that carries a topic", env, res)
		}
	}

	want := []string{"api.sandbox.push.apple.com", "api.push.apple.com"}
	if got := over.requestedHosts(); !slices.Equal(got, want) {
		t.Fatalf("hosts = %v, want %v", got, want)
	}
}
