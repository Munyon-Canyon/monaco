package privy_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestCreateUser_postsOneEmailAccount(t *testing.T) {
	t.Parallel()
	c, u, _ := overFakes(t)
	id, err := c.CreateUser(t.Context(), "dev-ab@example.com")
	if err != nil || id != "did:privy:fake-1" {
		t.Fatalf("CreateUser = %q, %v", id, err)
	}
	req := u.requests()[0]
	want := `{"linked_accounts":[{"type":"email","address":"dev-ab@example.com"}]}`
	if req.method != http.MethodPost || req.path != "/privy/v1/users" || req.body != want {
		t.Fatalf("request = %s %s body %s", req.method, req.path, req.body)
	}
	again, err := c.CreateUser(t.Context(), "dev-cd@example.com")
	if err != nil || again != "did:privy:fake-2" {
		t.Fatalf("second CreateUser = %q, %v", again, err)
	}
	got, err := c.GetUser(t.Context(), id)
	if err != nil || got.ID != id || got.Email != "dev-ab@example.com" {
		t.Fatalf("GetUser(created) = %+v, %v", got, err)
	}
}

func TestCreateUser_refusesAnEmptyEmailAndABodyWithNoID(t *testing.T) {
	t.Parallel()
	c, _, _ := overFakes(t)
	if _, err := c.CreateUser(t.Context(), ""); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("empty email = %v, want invalid_input", err)
	}
	_, err := client(replying(http.StatusOK, `{}`)).CreateUser(t.Context(), "dev@example.com")
	wantCode(t, err, errs.CodeDecodeFailed)
}

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

func TestGetUser_readsTheEmailAccountAMemberSignedInWith(t *testing.T) {
	t.Parallel()
	c, _, _ := overFakes(t)
	got, err := c.GetUser(t.Context(), "did:privy:member-legacy")
	want := privy.User{
		ID: "did:privy:member-legacy", Email: "legacy@example.com",
		EmbeddedWallet: &chain.Wallet{ID: "wallet-legacy", Address: "BGQoQgGkjSQc6c5YjsyRjuj4M5LbYJMHVCdS8BJSrr2R"},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("GetUser = %+v, %v, want %+v", got, err, want)
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

func TestGetUser_waitsBetweenRetriesWithinTheBackoffCeilings(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		c, u, srv := overFakes(t)
		script(
			t,
			srv,
			fakes.Step{
				Route:  "/privy/v1/users/did:privy:member-new",
				Action: fakes.ActionFail,
				Status: http.StatusServiceUnavailable,
				Times:  3,
			},
		)
		start := clock.Real{}.Now()
		_, err := c.GetUser(t.Context(), "did:privy:member-new")
		waited := clock.Real{}.Now().Sub(start)
		wantCode(t, err, errs.CodePrivyUnavailable)
		if len(u.requests()) != 3 || waited <= 0 || waited > 750*time.Millisecond {
			t.Fatalf("%d attempts waited %v, want 3 attempts and between 0 and 250ms + 500ms of backoff",
				len(u.requests()), waited)
		}
	})
}

func TestGetUser_aCallerThatWentAwayIsClientClosedNotPrivyUnavailable(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	u := replying(http.StatusOK, `{}`)

	_, err := client(u).GetUser(ctx, "did:privy:member")

	wantCode(t, err, errs.CodeClientClosed)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want it to wrap context.Canceled", err)
	}
}

func TestGetUser_aPrivyTransportFailureStaysPrivyUnavailable(t *testing.T) {
	t.Parallel()
	down := &upstream{transport: errs.New(errs.CodeUpstreamUnavailable, "test.transport")}

	_, err := client(down).GetUser(t.Context(), "did:privy:member")

	wantCode(t, err, errs.CodePrivyUnavailable)
}

func TestCreateUser_namesTheUserLimit(t *testing.T) {
	t.Parallel()
	body := `{"error":"User limit reached","code":"max_accounts_reached"}`
	_, err := client(replying(http.StatusBadRequest, body)).CreateUser(t.Context(), "dev-ab@example.com")
	wantCode(t, err, errs.CodePrivyUserLimit)
	if !strings.Contains(fmt.Sprint(errs.Detail(err)), "User limit reached") {
		t.Fatalf("detail = %v, want Privy's message", errs.Detail(err))
	}
	_, err = client(replying(http.StatusBadRequest, `{"error":"bad email"}`)).CreateUser(t.Context(), "x@example.com")
	wantCode(t, err, errs.CodeInvalidInput)
	_, err = client(replying(http.StatusInternalServerError, body)).CreateUser(t.Context(), "x@example.com")
	wantCode(t, err, errs.CodePrivyUnavailable)
}

func TestDeleteUser_deletesByID(t *testing.T) {
	t.Parallel()
	c, u, _ := overFakes(t)
	id, err := c.CreateUser(t.Context(), "dev-ab@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteUser(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	req := u.requests()[1]
	if req.method != http.MethodDelete || req.path != "/privy/v1/users/"+string(id) {
		t.Fatalf("request = %s %s", req.method, req.path)
	}
	wantCode(t, c.DeleteUser(t.Context(), id), errs.CodeNotFound)
}

func TestListUsers_followsTheCursorAndReadsAgeAndAccounts(t *testing.T) {
	t.Parallel()
	pages := map[string]string{
		"": `{"data":[{"id":"did:privy:a","created_at":1760000000,"linked_accounts":[{"type":"email","address":"dev-a@example.com"}]}],
			"next_cursor":"c 2"}`,
		"c 2": `{"data":[{"id":"did:privy:b","created_at":1760000100,"linked_accounts":[{"type":"phone","number":"+1"},` +
			`{"type":"email","address":"b@example.com"}]}]}`,
	}
	u := &upstream{handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, pages[r.URL.Query().Get("cursor")])
	})}
	got, err := client(u).ListUsers(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := []privy.ListedUser{
		{ID: "did:privy:a", CreatedAt: time.Unix(1760000000, 0).UTC(), Accounts: 1, Email: "dev-a@example.com"},
		{ID: "did:privy:b", CreatedAt: time.Unix(1760000100, 0).UTC(), Accounts: 2, Email: "b@example.com"},
	}
	if !reflect.DeepEqual(got, want) || u.requests()[1].query != "cursor=c+2" {
		t.Fatalf("ListUsers = %+v, query %q", got, u.requests()[1].query)
	}
	_, err = client(replying(http.StatusInternalServerError, `{}`)).ListUsers(t.Context())
	wantCode(t, err, errs.CodePrivyUnavailable)
}

func TestListUsers_overTheFakesListsWhatWasCreated(t *testing.T) {
	t.Parallel()
	c, _, _ := overFakes(t)
	for _, email := range []string{"dev-ab@example.com", "dev-cd@example.com"} {
		if _, err := c.CreateUser(t.Context(), email); err != nil {
			t.Fatal(err)
		}
	}
	got, err := c.ListUsers(t.Context())
	if err != nil || len(got) != 2 || got[0].Email != "dev-ab@example.com" || got[1].ID != "did:privy:fake-2" {
		t.Fatalf("ListUsers = %+v, %v", got, err)
	}
}
