package fakes_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const apnsRoute = "/apns/3/device/abc"

type apnsAnswer struct {
	status int
	header http.Header
	body   string
}

func pushTo(t *testing.T, srv *fakes.Server, topic string) apnsAnswer {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, apnsRoute, strings.NewReader("{}"))
	if topic != "" {
		req.Header.Set("apns-topic", topic)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	resp := rec.Result()
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return apnsAnswer{status: resp.StatusCode, header: resp.Header, body: string(body)}
}

func scriptFixture(t *testing.T, srv *fakes.Server, fixture string) {
	t.Helper()
	raw, err := json.Marshal(fakes.Step{Route: apnsRoute, Action: fakes.ActionSucceed, Fixture: fixture})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(
		rec,
		httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/_script", strings.NewReader(string(raw))),
	)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("script %s = %d %q, want 204", raw, rec.Code, rec.Body.String())
	}
}

func TestApnsRoute_acceptsAPushWithTheOKFixture(t *testing.T) {
	t.Parallel()

	got := pushTo(t, fakes.New(), "com.monaco.app")

	if got.status != http.StatusOK || got.body != "" ||
		got.header.Get("apns-id") != "eabeae54-14a8-11e5-b60b-1697f925ec7b" {
		t.Fatalf("push = %d %q %v, want 200 with the apns-id and no body", got.status, got.body, got.header)
	}
}

func TestApnsRoute_refusesAPushWithoutATopicLikeAPNs(t *testing.T) {
	t.Parallel()

	got := pushTo(t, fakes.New(), "")

	if got.status != http.StatusBadRequest || !strings.Contains(got.body, `"MissingTopic"`) {
		t.Fatalf("push without a topic = %d %q, want 400 MissingTopic", got.status, got.body)
	}
}

func TestApnsRoute_servesEachScriptedFixtureOnceThenTheDefaultAgain(t *testing.T) {
	t.Parallel()
	tests := []struct {
		fixture    string
		status     int
		reason     string
		retryAfter string
	}{
		{"unregistered_410", 410, "Unregistered", ""},
		{"bad_device_token_400", 400, "BadDeviceToken", ""},
		{"too_many_requests_429", 429, "TooManyRequests", "3"},
		{"internal_500", 500, "InternalServerError", ""},
		{"forbidden_403", 403, "InvalidProviderToken", ""},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			t.Parallel()
			srv := fakes.New()
			scriptFixture(t, srv, "/apns/"+tt.fixture)

			got := pushTo(t, srv, "com.monaco.app")

			var body struct {
				Reason string `json:"reason"`
			}
			if err := json.Unmarshal([]byte(got.body), &body); err != nil {
				t.Fatalf("body %q: %v", got.body, err)
			}
			if got.status != tt.status || body.Reason != tt.reason || got.header.Get("Retry-After") != tt.retryAfter {
				t.Fatalf("push = %d %q Retry-After %q, want %d %s Retry-After %q",
					got.status, got.body, got.header.Get("Retry-After"), tt.status, tt.reason, tt.retryAfter)
			}
			if again := pushTo(t, srv, "com.monaco.app"); again.status != http.StatusOK {
				t.Fatalf("next push = %d, want the default 200 after the scripted fixture", again.status)
			}
		})
	}
}
