package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCORS_answersPreflightOnlyForWebOperationsFromAnAllowedOrigin(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.deps.WebOrigins = []string{"https://monacolabs.xyz", "http://localhost:5173"}
	handler := mustHandler(t, h.deps, healthz{})
	tests := []struct {
		name, origin, method, path string
		allowed                    bool
	}{
		{"the fund page exchanging", "https://monacolabs.xyz", http.MethodPost, "/v1/onramp/sessions/exchange", true},
		{
			"the local fund page reporting", "http://localhost:5173", http.MethodPatch,
			"/v1/onramp/sessions/01890a5d-ac96-774b-bcce-b302099a8057", true,
		},
		{"another site", "https://evil.example", http.MethodPost, "/v1/onramp/sessions/exchange", false},
		{"an app-only route", "https://monacolabs.xyz", http.MethodPost, "/v1/onramp/sessions", false},
		{"an unknown route", "https://monacolabs.xyz", http.MethodPost, "/v1/nowhere", false},
	}
	for _, tt := range tests {
		rec := serveRaw(t, handler, http.MethodOptions, tt.path, http.Header{
			"Origin": {tt.origin}, "Access-Control-Request-Method": {tt.method},
			"Access-Control-Request-Headers": {"authorization, content-type, idempotency-key"},
		})
		got := rec.Header().Get("Access-Control-Allow-Origin")
		if tt.allowed && (rec.Code != http.StatusNoContent || got != tt.origin ||
			rec.Header().Get("Access-Control-Allow-Methods") != tt.method ||
			rec.Header().Get("Access-Control-Allow-Headers") != corsHeaders) {
			t.Errorf("%s: preflight = %d %v, want 204 allowing %s", tt.name, rec.Code, rec.Header(), tt.origin)
		}
		if !tt.allowed && got != "" {
			t.Errorf("%s: preflight Access-Control-Allow-Origin = %q, want none", tt.name, got)
		}
	}
}

func TestCORS_marksTheWebOperationsResponseForAnAllowedOriginOnly(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.deps.WebOrigins = []string{"https://monacolabs.xyz"}
	handler := mustHandler(t, h.deps, healthz{})
	for origin, want := range map[string]string{
		"https://monacolabs.xyz": "https://monacolabs.xyz", "https://evil.example": "", "": "",
	} {
		header := http.Header{"Content-Type": {"application/json"}}
		if origin != "" {
			header.Set("Origin", origin)
		}
		rec := serveBody(t, handler, http.MethodPost, "/v1/onramp/sessions/exchange", header, `{"token":"t"}`)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != want {
			t.Errorf("origin %q: Access-Control-Allow-Origin = %q, want %q", origin, got, want)
		}
	}
}

func serveBody(
	t *testing.T, handler http.Handler, method, target string, header http.Header, body string,
) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, target, strings.NewReader(body))
	for k, v := range header {
		req.Header[k] = v
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
