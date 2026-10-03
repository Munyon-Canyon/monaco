package identity_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	privyadapter "github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters/privy"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const privyAppID = "app-fixture"

type privyUpstream struct {
	handler http.Handler
	mu      sync.Mutex
	sent    []string
}

func (u *privyUpstream) RoundTrip(r *http.Request) (*http.Response, error) {
	u.mu.Lock()
	u.sent = append(u.sent, r.Method+" "+r.URL.Path)
	u.mu.Unlock()
	rec := httptest.NewRecorder()
	u.handler.ServeHTTP(rec, r)
	return rec.Result(), nil
}

func (u *privyUpstream) requests() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]string(nil), u.sent...)
}

func privyConfig() config.Config {
	return config.Config{
		Privy: config.Privy{
			AppID: privyAppID, BaseURL: "http://privy.test/privy", VerificationKey: fakes.PrivyVerificationKey(),
			AuthorizationKeyID: fakes.PrivyAuthorizationKeyID,
		},
		Timeouts: config.Timeouts{Privy: 10 * time.Second},
	}
}

func privyClient(t *testing.T, cfg config.Config, clk clock.Clock, opts ...httpclient.Option) *privy.Client {
	t.Helper()
	c, err := privy.New(cfg, clk, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func privyOver(t *testing.T, cfg config.Config, h http.Handler) (*privy.Client, *privyUpstream) {
	t.Helper()
	u := &privyUpstream{handler: h}
	return privyClient(t, cfg, clock.Real{}, httpclient.WithTransport(u)), u
}

func overPrivyFakes(t *testing.T) (*privy.Client, *privyUpstream, *fakes.Server) {
	t.Helper()
	srv := fakes.New()
	c, u := privyOver(t, privyConfig(), srv)
	return c, u, srv
}

func scriptPrivy(t *testing.T, srv *fakes.Server, step fakes.Step) {
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

func privyUserReplying(phone string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":              "did:privy:phone",
			"linked_accounts": []map[string]string{{"type": "phone", "number": phone}},
		})
	})
}

func TestPrivyUsers_verifyNamesTheTokenSubject(t *testing.T) {
	t.Parallel()
	c, u, _ := overPrivyFakes(t)
	users := privyadapter.Users{Client: c}
	now := clock.Real{}.Now()
	id, err := users.Verify(t.Context(), fakes.PrivyAccessToken(privyAppID, "did:privy:member", now, time.Hour))
	if err != nil || id != "did:privy:member" {
		t.Fatalf("Verify = %q, %v", id, err)
	}
	expired := fakes.PrivyAccessToken(privyAppID, "did:privy:member", now.Add(-2*time.Hour), time.Hour)
	if id, err := users.Verify(t.Context(), expired); id != "" || errs.CodeOf(err) != errs.CodeUnauthorized {
		t.Fatalf("Verify(expired) = %q, %v, want unauthorized", id, err)
	}
	if sent := u.requests(); len(sent) != 0 {
		t.Fatalf("Verify called Privy: %v", sent)
	}
}

func TestPrivyUsers_mapsLinkedAccountsIntoIdentityTerms(t *testing.T) {
	t.Parallel()
	c, _, _ := overPrivyFakes(t)
	users := privyadapter.Users{Client: c}
	for id, want := range map[app.PrivyUserID]app.PrivyUser{
		"did:privy:member-with-wallet": {
			ID: "did:privy:member-with-wallet", AppleEmail: "member@privaterelay.appleid.com",
			PhoneE164: "+14155550100", X: &domain.XAccount{UserID: "1234567890", Username: "monaco_member"},
		},
		"did:privy:member-new":    {ID: "did:privy:member-new", GoogleEmail: "new.member@gmail.com"},
		"did:privy:member-legacy": {ID: "did:privy:member-legacy", Email: "legacy@example.com"},
	} {
		got, err := users.User(t.Context(), id)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("User(%s) = %+v, %v, want %+v", id, got, err, want)
		}
	}
	if _, err := users.User(t.Context(), "did:privy:nobody"); errs.CodeOf(err) != errs.CodeNotFound {
		t.Fatalf("User(unknown) = %v, want not_found", err)
	}
}

func TestPrivyUsers_normalizesThePhoneToE164(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]string{
		"+14155550100":      "+14155550100",
		"+1 415 555 0100":   "+14155550100",
		"+1 (415) 555-0100": "+14155550100",
		"+44.20.7946.0958":  "+442079460958",
	} {
		c, _ := privyOver(t, privyConfig(), privyUserReplying(raw))
		got, err := privyadapter.Users{Client: c}.User(t.Context(), "did:privy:phone")
		if err != nil || got.PhoneE164 != want {
			t.Fatalf("User with phone %q = %q, %v, want %q", raw, got.PhoneE164, err, want)
		}
	}
}

func TestPrivyUsers_refusesAnXAccountWithAnEmptySubject(t *testing.T) {
	t.Parallel()
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "did:privy:empty-x",
			"linked_accounts": []map[string]string{{
				"type": "twitter_oauth", "subject": "", "username": "blank",
			}},
		})
	})
	c, _ := privyOver(t, privyConfig(), h)
	got, err := privyadapter.Users{Client: c}.User(t.Context(), "did:privy:empty-x")
	if errs.CodeOf(err) != errs.CodeDecodeFailed || got != (app.PrivyUser{}) {
		t.Fatalf("User = %+v, %v, want decode_failed", got, err)
	}
	if detail := errs.Detail(err); len(detail) != 1 || detail[0].Key != "privy_user_id" ||
		detail[0].Value.String() != "did:privy:empty-x" {
		t.Fatalf("detail = %v, want the Privy user id", detail)
	}
}

func TestPrivyUsers_refusesAPhoneThatIsNotE164(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		"4155550100", "+", "+1", "+04155550100", "+1415555010012345", "+1415555O100", "+1 415 555 0100 ext 7",
	} {
		c, _ := privyOver(t, privyConfig(), privyUserReplying(raw))
		got, err := privyadapter.Users{Client: c}.User(t.Context(), "did:privy:phone")
		if errs.CodeOf(err) != errs.CodeDecodeFailed || got != (app.PrivyUser{}) {
			t.Fatalf("User with phone %q = %+v, %v, want decode_failed", raw, got, err)
		}
		if detail := errs.Detail(err); len(detail) != 1 || detail[0].Value.String() != "phone_not_e164" {
			t.Fatalf("detail = %v, want only the reason and never the number", detail)
		}
	}
}

func TestPrivyUsers_createsAnEmailUser(t *testing.T) {
	t.Parallel()
	c, u, _ := overPrivyFakes(t)
	users := privyadapter.Users{Client: c}
	id, err := users.Create(t.Context(), "dev-ab@example.com")
	if err != nil || id != "did:privy:fake-1" || u.requests()[0] != "POST /privy/v1/users" {
		t.Fatalf("Create = %q, %v, requests %v", id, err, u.requests())
	}
	if _, err := users.Create(t.Context(), ""); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("empty email = %v, want invalid_input", err)
	}
}

func TestPrivyWallets_reusesTheWalletPrivyHoldsAndReportsItsSigner(t *testing.T) {
	t.Parallel()
	c, u, _ := overPrivyFakes(t)
	wallets := privyadapter.Wallets{Client: c}
	for id, want := range map[app.PrivyUserID]app.PrivyWallet{
		"did:privy:member-with-wallet": {
			Wallet: domain.Wallet{
				PrivyWalletID: "wallet-member", Address: "Dht9c9YfstFWkNYXgqr8HZbhqVn563bCpNU6zL32Ftqf",
			},
			HasAppSigner: true,
		},
		"did:privy:member-legacy": {
			Wallet: domain.Wallet{
				PrivyWalletID: "wallet-legacy", Address: "BGQoQgGkjSQc6c5YjsyRjuj4M5LbYJMHVCdS8BJSrr2R",
			},
		},
	} {
		got, err := wallets.FindOrCreate(t.Context(), id)
		if err != nil || got != want {
			t.Fatalf("FindOrCreate(%s) = %+v, %v, want %+v", id, got, err, want)
		}
	}
	for _, req := range u.requests() {
		if req != "GET /privy/v1/wallets" {
			t.Fatalf("reusing a wallet sent %q", req)
		}
	}
}

func TestPrivyWallets_createsOneWalletWithTheAppSigner(t *testing.T) {
	t.Parallel()
	c, u, _ := overPrivyFakes(t)
	wallets := privyadapter.Wallets{Client: c}
	first, err := wallets.FindOrCreate(t.Context(), "did:privy:member-new")
	if err != nil || !first.HasAppSigner || first.PrivyWalletID == "" || first.Address == "" {
		t.Fatalf("FindOrCreate = %+v, %v", first, err)
	}
	again, err := wallets.FindOrCreate(t.Context(), "did:privy:member-new")
	if err != nil || again != first {
		t.Fatalf("second FindOrCreate = %+v, %v, want %+v", again, err, first)
	}
	want := []string{"GET /privy/v1/wallets", "POST /privy/v1/wallets", "GET /privy/v1/wallets"}
	if got := u.requests(); !reflect.DeepEqual(got, want) {
		t.Fatalf("requests = %v, want %v", got, want)
	}
}

func TestPrivyWallets_passesAPrivyRefusalThrough(t *testing.T) {
	t.Parallel()
	c, _, srv := overPrivyFakes(t)
	scriptPrivy(t, srv, fakes.Step{Route: "/privy/v1/wallets", Action: fakes.ActionFail, Status: http.StatusBadRequest})
	got, err := privyadapter.Wallets{Client: c}.FindOrCreate(t.Context(), "did:privy:member-new")
	if errs.CodeOf(err) != errs.CodeInvalidInput || got != (app.PrivyWallet{}) {
		t.Fatalf("FindOrCreate = %+v, %v, want invalid_input", got, err)
	}
}
