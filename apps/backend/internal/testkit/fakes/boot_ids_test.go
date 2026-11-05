package fakes_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func issuedID(t *testing.T, srv *fakes.Server, path, body string, header ...string) string {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, strings.NewReader(body))
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	var got struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.ID == "" {
		t.Fatalf("POST %s = %d %q, want an id", path, rec.Code, rec.Body.String())
	}
	return got.ID
}

func TestFakes_twoBootsNeverIssueTheSameUserOrWalletID(t *testing.T) {
	t.Parallel()
	user := `{"linked_accounts":[{"type":"email","address":"a@example.test"}]}`
	wallet := `{"chain_type":"solana","owner_id":"u"}`
	evm := `{"chain_type":"ethereum","owner_id":"u"}`
	one, two := fakes.New(), fakes.New()
	for _, c := range []struct {
		name, path, body string
		header           []string
	}{
		{"user", "/privy/v1/users", user, nil},
		{"counted wallet", "/privy/v1/wallets", wallet, nil},
		{"keyed wallet", "/privy/v1/wallets", wallet, []string{"privy-idempotency-key", "same-key"}},
		{"evm wallet", "/privy/v1/wallets", evm, []string{"privy-idempotency-key", "same-key"}},
	} {
		a := issuedID(t, one, c.path, c.body, c.header...)
		b := issuedID(t, two, c.path, c.body, c.header...)
		if a == b {
			t.Fatalf("%s: two fakes boots both issued %q", c.name, a)
		}
	}
}
