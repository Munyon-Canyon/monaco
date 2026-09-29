package privy_test

import (
	"net/http"
	"reflect"
	"testing"
	"testing/synctest"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestGetUser_readsLinkedAccountsAndTheEmbeddedSolanaWallet(t *testing.T) {
	t.Parallel()
	c, u, _ := overFakes(t)
	got, err := c.GetUser(t.Context(), "did:privy:member-with-wallet")
	if err != nil {
		t.Fatal(err)
	}
	want := privy.User{
		ID: "did:privy:member-with-wallet", AppleEmail: "member@privaterelay.appleid.com", Phone: "+14155550100",
		X:              &privy.XAccount{UserID: "1234567890", Username: "monaco_member"},
		EmbeddedWallet: &chain.Wallet{ID: "wallet-member", Address: "Dht9c9YfstFWkNYXgqr8HZbhqVn563bCpNU6zL32Ftqf"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetUser = %+v, want %+v", got, want)
	}
	req := u.requests()[0]
	user, pass, _ := (&http.Request{Header: req.header}).BasicAuth()
	if req.method != http.MethodGet || req.path != "/privy/v1/users/did:privy:member-with-wallet" ||
		user != appID || pass != appHidden || req.header.Get("privy-app-id") != appID {
		t.Fatalf("request = %s %s as %q", req.method, req.path, user)
	}
}

func TestGetUser_ignoresWalletsPrivyDoesNotHold(t *testing.T) {
	t.Parallel()
	c, _, _ := overFakes(t)
	got, err := c.GetUser(t.Context(), "did:privy:member-new")
	if err != nil || got.GoogleEmail != "new.member@gmail.com" || got.EmbeddedWallet != nil || got.X != nil {
		t.Fatalf("GetUser = %+v, %v", got, err)
	}
	_, err = c.GetUser(t.Context(), "did:privy:nobody")
	wantCode(t, err, errs.CodeNotFound)
}

func TestGetUser_mapsProviderFailures(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		for name, tc := range map[string]struct {
			u    *upstream
			want errs.Code
		}{
			"bad credentials": {replying(http.StatusUnauthorized, `{}`), errs.CodeInternal},
			"forbidden":       {replying(http.StatusForbidden, `{}`), errs.CodeInternal},
			"bad request":     {replying(http.StatusBadRequest, `{}`), errs.CodeInvalidInput},
			"rate limited":    {replying(http.StatusTooManyRequests, `{}`), errs.CodePrivyUnavailable},
			"server error":    {replying(http.StatusInternalServerError, `{}`), errs.CodePrivyUnavailable},
			"transport":       {&upstream{transport: errs.New(errs.CodeUpstreamUnavailable, "test.dial")}, errs.CodePrivyUnavailable},
			"body cut":        {&upstream{handler: replying(http.StatusOK, `{}`).handler, badBody: true}, errs.CodePrivyUnavailable},
			"not json":        {replying(http.StatusOK, `<html>`), errs.CodeDecodeFailed},
		} {
			_, err := client(tc.u).GetUser(t.Context(), "did:privy:x")
			if errs.CodeOf(err) != tc.want {
				t.Fatalf("%s: err = %v, want %s", name, err, tc.want)
			}
		}
	})
}

func TestGetUser_scripted503IsRetryablePrivyUnavailable(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		c, u, srv := overFakes(t)
		route := "/privy/v1/users/did:privy:member-new"
		script(
			t,
			srv,
			fakes.Step{Route: route, Action: fakes.ActionFail, Status: http.StatusServiceUnavailable, Times: 3},
		)
		_, err := c.GetUser(t.Context(), "did:privy:member-new")
		wantCode(t, err, errs.CodePrivyUnavailable)
		if !errs.Retryable(errs.CodeOf(err)) || len(u.requests()) != 3 {
			t.Fatalf("err = %v after %d attempts, want retryable after 3", err, len(u.requests()))
		}
	})
}

func TestGetUser_hangHitsTheTenSecondPrivyDeadline(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		c, _, srv := overFakes(t)
		script(t, srv, fakes.Step{Route: "/privy/v1/users/did:privy:member-new", Action: fakes.ActionHang})
		start := clock.Real{}.Now()
		_, err := c.GetUser(t.Context(), "did:privy:member-new")
		wantCode(t, err, errs.CodeUpstreamTimeout)
		if took := (clock.Real{}).Now().Sub(start); took != 10*time.Second {
			t.Fatalf("gave up after %v, want the 10s Privy deadline", took)
		}
	})
}
