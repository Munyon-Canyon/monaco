package identity_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/identityapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/privyfake"
)

type devXUser struct {
	app.DevUser
	suffix  string
	privyID app.PrivyUserID
}

func (f httpFixture) devUser(t *testing.T) devXUser {
	t.Helper()
	clk := testkit.NewClock(f.now)
	got, err := app.CreateDevUser(t.Context(), app.CreateDevUserDeps{
		Env: config.EnvLocal, UoW: db.New(f.pool, f.ids, clk), Users: adapters.Users{},
		Privy: f.privy, Wallets: f.wallets, IDs: f.ids, Clock: clk, Hints: f.hints, Rand: rand.Reader,
	})
	if err != nil {
		t.Fatal(err)
	}
	var privyID string
	if err := f.pool.QueryRow(t.Context(), `SELECT privy_user_id FROM users WHERE id = $1`, got.UserID.UUID()).
		Scan(&privyID); err != nil {
		t.Fatal(err)
	}
	return devXUser{DevUser: got, suffix: strings.TrimPrefix(got.Handle, "dev_"), privyID: app.PrivyUserID(privyID)}
}

func wantNoContent(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d %s, want 204", rec.Code, rec.Body)
	}
}

func (f httpFixture) storedX(t *testing.T, user ids.UserID) (xUserID, xUsername *string, linkedAt *time.Time) {
	t.Helper()
	err := f.pool.QueryRow(t.Context(), `SELECT x_user_id, x_username, x_linked_at FROM users WHERE id = $1`,
		user.UUID()).Scan(&xUserID, &xUsername, &linkedAt)
	if err != nil {
		t.Fatal(err)
	}
	return xUserID, xUsername, linkedAt
}

func (f httpFixture) wantStoredX(t *testing.T, user ids.UserID, xUserID, xUsername string) {
	t.Helper()
	id, name, at := f.storedX(t, user)
	if id == nil || *id != xUserID || name == nil || *name != xUsername || at == nil {
		t.Fatalf("stored X = %v %v %v, want %s %s with a link time", id, name, at, xUserID, xUsername)
	}
}

func TestDevXLink_Link(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	u := f.devUser(t)
	wantProblem(t, f.onboard(t, u.UserID, "socials", ``, "s0"), apibase.XNotLinked)
	wantNoContent(t, f.devXLink(t, u.UserID, http.MethodPost, `{"username":"qa_x"}`, "x1"))
	me := f.wantState(t, f.onboard(t, u.UserID, "socials", ``, "s1"), api.AWAITINGPHONE)
	if me.XUsername == nil || *me.XUsername != "qa_x" {
		t.Fatalf("x_username = %v, want qa_x", me.XUsername)
	}
	f.wantStoredX(t, u.UserID, "dev:"+u.suffix, "qa_x")
	wantNoContent(t, f.devXLink(t, u.UserID, http.MethodPost, `{"username":"qa_y"}`, "x2"))
	if f.devXRows(t, u.UserID) != 1 {
		t.Fatal("a second POST must replace the caller's row, not add one")
	}
	wantProblem(t, f.devXLink(t, u.UserID, http.MethodPost, `{"username":"not a handle"}`, "x3"),
		apibase.InvalidInput)
}

func TestDevXLink_SkipThenLinkLater(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	u := f.devUser(t)
	f.wantState(t, f.onboard(t, u.UserID, "skip", `{"step":"socials"}`, "k1"), api.AWAITINGPHONE)
	if id, _, _ := f.storedX(t, u.UserID); id != nil {
		t.Fatalf("x_user_id after skip = %q, want none", *id)
	}
	wantNoContent(t, f.devXLink(t, u.UserID, http.MethodPost, `{}`, "x1"))
	f.wantState(t, f.onboard(t, u.UserID, "socials", ``, "s1"), api.AWAITINGPHONE)
	f.wantStoredX(t, u.UserID, "dev:"+u.suffix, "dev_x_"+u.suffix)

	phoned := f.devUser(t)
	f.privy.Seed(app.PrivyUser{ID: phoned.privyID, Email: domain.DevEmail(phoned.suffix), PhoneE164: onboardNo})
	f.wantState(t, f.onboard(t, phoned.UserID, "phone", ``, "p1"), api.AWAITINGSOCIALS)
	wantNoContent(t, f.devXLink(t, phoned.UserID, http.MethodPost, ``, "x1"))
	f.wantState(t, f.onboard(t, phoned.UserID, "socials", ``, "s1"), api.ONBOARDINGCOMPLETED)
	f.wantStoredX(t, phoned.UserID, "dev:"+phoned.suffix, "dev_x_"+phoned.suffix)
}

func TestDevXLink_Delete(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	u := f.devUser(t)
	wantNoContent(t, f.devXLink(t, u.UserID, http.MethodDelete, ``, "d0"))
	wantNoContent(t, f.devXLink(t, u.UserID, http.MethodPost, `{}`, "x1"))
	f.wantState(t, f.onboard(t, u.UserID, "socials", ``, "s1"), api.AWAITINGPHONE)
	wantNoContent(t, f.devXLink(t, u.UserID, http.MethodDelete, ``, "d1"))
	if f.devXRows(t, u.UserID) != 0 {
		t.Fatal("DELETE left the caller's row")
	}
	if rec := f.openSession(t, "Bearer "+string(u.privyID)); rec.Code != http.StatusOK {
		t.Fatalf("session = %d %s", rec.Code, rec.Body)
	}
	if id, name, _ := f.storedX(t, u.UserID); id != nil || name != nil {
		t.Fatalf("stored X after DELETE and a session = %v %v, want none", id, name)
	}
}

func TestDevXLink_ProductionRefuses(t *testing.T) {
	t.Parallel()
	for name, router := range map[string]config.Env{"router": config.EnvProduction, "module": config.EnvTest} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newHTTPFixtureIn(t, router, config.EnvProduction)
			u := f.devUser(t)
			wantProblem(t, f.devXLink(t, u.UserID, http.MethodPost, `{"username":"qa_x"}`, "x1"), apibase.NotFound)
			wantProblem(t, f.devXLink(t, u.UserID, http.MethodDelete, ``, "d1"), apibase.NotFound)
			if f.devXRows(t, u.UserID) != 0 {
				t.Fatal("a dev_x_links row exists")
			}
		})
	}
	fake := &privyfake.Users{}
	prod := identity.New(module.Deps{Config: config.Config{Env: config.EnvProduction}},
		identity.WithPrivy(fake, &privyfake.Wallets{}))
	if got, ok := prod.EnsuredPrivy().(*privyfake.Users); !ok || got != fake {
		t.Fatalf("production Privy adapter = %T, want the unwrapped *privyfake.Users", prod.EnsuredPrivy())
	}
	local := identity.New(module.Deps{Config: config.Config{Env: config.EnvLocal}},
		identity.WithPrivy(fake, &privyfake.Wallets{}))
	wrapped, ok := local.EnsuredPrivy().(adapters.DevX)
	if !ok || wrapped.PrivyUsers != app.PrivyUsers(fake) {
		t.Fatalf("local Privy adapter = %T, want adapters.DevX around the supplied fake", local.EnsuredPrivy())
	}
	if again, _ := local.EnsuredPrivy().(adapters.DevX); again.PrivyUsers != app.PrivyUsers(fake) {
		t.Fatal("a second ensurePrivy wrapped the decorator again")
	}
}

func TestDevX_passesThroughPrivyErrorsAndReportsReadFailures(t *testing.T) {
	t.Parallel()
	f := newHTTPFixture(t)
	u := f.devUser(t)
	down := errs.New(errs.CodePrivyUnavailable, "test")
	f.privy.FailOnce("User", down)
	dev := adapters.DevX{PrivyUsers: f.privy, Reads: f.pool}
	if _, err := dev.User(t.Context(), u.privyID); !errors.Is(err, down) {
		t.Fatalf("User with Privy down = %v, want the Privy error", err)
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := dev.User(canceled, u.privyID); err == nil || errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("User with a failed read = %v, want the read error", err)
	}
	f.privy.FailOnce("User", down)
	h := app.NewDevXLink(app.DevXLinkDeps{Reads: f.pool, Users: adapters.Users{}, Privy: f.privy})
	if err := h.Link(t.Context(), u.UserID, ""); !errors.Is(err, down) {
		t.Fatalf("Link with Privy down = %v, want the Privy error", err)
	}
	f.privy.FailOnce("User", down)
	if err := h.Unlink(t.Context(), u.UserID); !errors.Is(err, down) {
		t.Fatalf("Unlink with Privy down = %v, want the Privy error", err)
	}
	if err := h.Link(t.Context(), ids.NewUserID(ids.Real{}), ""); errs.CodeOf(err) == "" {
		t.Fatal("Link for an unknown user succeeded")
	}
	routes := adapters.HTTP{DevX: h}
	if _, err := routes.PostDevXLink(t.Context(), api.PostDevXLinkRequestObject{}); errs.CodeOf(err) == "" {
		t.Fatal("PostDevXLink without a caller succeeded")
	}
	if _, err := routes.DeleteDevXLink(t.Context(), api.DeleteDevXLinkRequestObject{}); errs.CodeOf(err) == "" {
		t.Fatal("DeleteDevXLink without a caller succeeded")
	}
}
