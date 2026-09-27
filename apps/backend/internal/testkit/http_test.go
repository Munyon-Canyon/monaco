package testkit

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type recordingTB struct {
	testing.TB
	failures []string
}

func (r *recordingTB) Helper() {}

func (r *recordingTB) Errorf(format string, args ...any) {
	r.failures = append(r.failures, fmt.Sprintf(format, args...))
}

func respond(status int, contentType, body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("X-Probe", "kept")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	})
}

const validProblem = `{"type":"about:blank","title":"Not Found","status":404,"code":"not_found",` +
	`"message":"Not found.","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","retryable":false}`

func TestHTTP_passesResponsesTheSpecAllowsThroughUnchanged(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		method, path string
		handler      http.Handler
		status       int
		body         string
	}{
		"healthz ok":            {"GET", "/healthz", respond(200, "text/plain", "ok\n"), 200, "ok\n"},
		"healthz problem":       {"GET", "/healthz", respond(404, "application/problem+json", validProblem), 404, validProblem},
		"undeclared as problem": {"GET", "/nope", respond(404, "application/problem+json", validProblem), 404, validProblem},
		"wrong method problem":  {"POST", "/healthz", respond(404, "application/problem+json", validProblem), 404, validProblem},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tb := &recordingTB{TB: t}
			rec := httptest.NewRecorder()
			HTTP(tb, tc.handler).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), tc.method, tc.path, nil))
			if len(tb.failures) != 0 {
				t.Fatalf("contract failures: %v", tb.failures)
			}
			if rec.Code != tc.status || rec.Body.String() != tc.body || rec.Header().Get("X-Probe") != "kept" {
				t.Fatalf("passthrough = %d %q %v", rec.Code, rec.Body.String(), rec.Header())
			}
		})
	}
}

func TestHTTP_failsTheTestOnAResponseTheSpecForbids(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		method, path string
		handler      http.Handler
		want         string
	}{
		"wrong healthz body": {"GET", "/healthz", respond(200, "text/plain", "fine"), "GET /healthz answered 200"},
		"json healthz":       {"GET", "/healthz", respond(200, "application/json", `{"ok":true}`), "answered 200"},
		"unknown error code": {
			"GET", "/healthz",
			respond(500, "application/problem+json", strings.Replace(validProblem, "not_found", "oops", 1)),
			"answered 500",
		},
		"problem missing trace_id": {
			"GET", "/healthz",
			respond(404, "application/problem+json",
				strings.Replace(validProblem, `"trace_id":"4bf92f3577b34da6a3ce929d0e0e4736",`, "", 1)),
			"answered 404",
		},
		"problem missing message": {
			"GET", "/healthz",
			respond(404, "application/problem+json", strings.Replace(validProblem, `"message":"Not found.",`, "", 1)),
			"answered 404",
		},
		"problem extra field": {
			"GET", "/healthz",
			respond(404, "application/problem+json", strings.Replace(validProblem, "{", `{"err":"db.Begin",`, 1)),
			"answered 404",
		},
		"undeclared as text":  {"GET", "/nope", respond(404, "text/plain", "404 page not found"), "application/problem+json"},
		"undeclared bad json": {"GET", "/nope", respond(404, "application/problem+json", "{"), "GET /nope answered 404"},
		"undeclared bad problem": {
			"GET", "/nope", respond(404, "application/problem+json", `{"code":"not_found"}`), "GET /nope answered 404",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tb := &recordingTB{TB: t}
			HTTP(tb, tc.handler).ServeHTTP(httptest.NewRecorder(),
				httptest.NewRequestWithContext(t.Context(), tc.method, tc.path, nil))
			if len(tb.failures) != 1 || !strings.Contains(tb.failures[0], tc.want) {
				t.Fatalf("failures = %v, want one containing %q", tb.failures, tc.want)
			}
		})
	}
}
