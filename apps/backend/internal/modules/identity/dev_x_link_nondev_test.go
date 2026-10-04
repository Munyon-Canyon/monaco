package identity_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func (f httpFixture) devXLink(t *testing.T, user ids.UserID, method, body, key string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, "/v1/dev/me/x-link", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+f.verifier.Mint(user.String(), f.now.Add(time.Hour)))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Idempotency-Key", key)
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}

func (f httpFixture) devXRows(t *testing.T, user ids.UserID) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM dev_x_links WHERE user_id = $1`, user.UUID()).
		Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestDevXLink_NonDevUser(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	for _, email := range []string{"qa@example.com", "dev-ab@example.org", ""} {
		u := f.onboardingUser(t, "", app.PrivyUser{Email: email})
		wantProblem(t, f.devXLink(t, u.ID, http.MethodPost, `{"username":"qa_x"}`, "x1"), apibase.NotFound)
		wantProblem(t, f.devXLink(t, u.ID, http.MethodDelete, ``, "d1"), apibase.NotFound)
		if f.devXRows(t, u.ID) != 0 {
			t.Fatalf("%q: a dev_x_links row exists", email)
		}
	}
	gone := f.seed(t, portSeed{wallet: true})
	wantProblem(t, f.devXLink(t, gone.ID, http.MethodPost, `{}`, "x1"), apibase.NotFound)
}
