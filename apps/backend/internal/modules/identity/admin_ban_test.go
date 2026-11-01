package identity_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters/authn"
	privyadapter "github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters/privy"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/treasuryapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/privyfake"
)

const (
	moderatorToken = "admin-moderator"
	viewerToken    = "admin-viewer"
	operatorToken  = "admin-operator"
	moderatorID    = "019cc330-1111-7000-8000-000000000001"
)

type adminOrUser struct{ users auth.TokenVerifier }

func (v adminOrUser) Verify(ctx context.Context, raw string) (auth.Actor, error) {
	switch raw {
	case moderatorToken:
		return auth.Actor{Kind: auth.ActorAdmin, ID: moderatorID, Role: "moderator"}, nil
	case operatorToken:
		return auth.Actor{Kind: auth.ActorAdmin, ID: moderatorID, Role: "operator"}, nil
	case viewerToken:
		return auth.Actor{Kind: auth.ActorAdmin, ID: moderatorID, Role: "viewer"}, nil
	}
	return v.users.Verify(ctx, raw)
}

type reachedHandler struct {
	treasuryapi.StrictServerInterface
}

func (reachedHandler) PostCashOut(
	context.Context, treasuryapi.PostCashOutRequestObject,
) (treasuryapi.PostCashOutResponseObject, error) {
	return nil, errs.New(errs.CodeLiveSwapExists, "test.PostCashOut")
}

func (reachedHandler) FundCabal(
	context.Context, treasuryapi.FundCabalRequestObject,
) (treasuryapi.FundCabalResponseObject, error) {
	return nil, errs.New(errs.CodeLiveSwapExists, "test.FundCabal")
}

type adminFixture struct {
	portFixture
	handler http.Handler
	clock   *testkit.Clock
}

func newAdminFixture(t *testing.T) adminFixture {
	t.Helper()
	f := newPortFixture(t)
	clk := testkit.NewClock(f.now)
	cfg := privyConfig()
	cfg.Env, cfg.Auth.DevTokenKey = config.EnvTest, devKey
	users, err := authn.New(cfg, clk, privyadapter.Users{Client: privyClient(t, cfg, clk)}, f.pool)
	if err != nil {
		t.Fatal(err)
	}
	v := adminOrUser{users: users}
	m := identity.New(module.Deps{
		Config: cfg, Pool: f.pool, UoW: db.New(f.pool, f.ids, clk), IDs: f.ids, Clock: clk,
	}, identity.WithPrivy(&privyfake.Users{}, &privyfake.Wallets{}))
	h, err := httpx.HandlerFor(httpx.Deps{
		Logger: observability.NewLogger(config.Config{Env: config.EnvTest}, io.Discard),
		Tracer: tracenoop.NewTracerProvider(), Clock: clk, IDs: f.ids, MaxBodyBytes: 1 << 20,
		Idempotency: db.NewIdempotencyStore(f.pool, clk), Verifier: v, AdminVerifier: v,
	}, func(r apibase.Mount) {
		m.Mount(r)
		treasuryapi.Mount(reachedHandler{}, r)
	}, specContract(t))
	if err != nil {
		t.Fatal(err)
	}
	return adminFixture{portFixture: f, handler: testkit.HTTP(t, h), clock: clk}
}

func (f adminFixture) call(
	t *testing.T, token, method, path, key, body string,
) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	if method != http.MethodGet {
		req.Header.Set("Idempotency-Key", key)
	}
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}

func (f adminFixture) moderate(
	t *testing.T, action string, user ids.UserID, key, reason string,
) *httptest.ResponseRecorder {
	t.Helper()
	return f.call(t, moderatorToken, http.MethodPost, "/v1/admin/users/"+user.String()+"/"+action, key,
		`{"reason":"`+reason+`"}`)
}

func (f adminFixture) status(t *testing.T, user ids.UserID) string {
	t.Helper()
	var s string
	row := f.pool.QueryRow(t.Context(), `SELECT account_status FROM users WHERE id = $1`, user.UUID())
	if err := row.Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func (f adminFixture) payloads(t *testing.T, typ events.Type) [][]byte {
	t.Helper()
	rows, err := f.pool.Query(t.Context(), `SELECT payload FROM events WHERE type = $1 ORDER BY id`, string(typ))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		out = append(out, raw)
	}
	return out
}

func (f adminFixture) audits(t *testing.T, kind events.AdminActionKind) []events.AdminAction {
	t.Helper()
	var out []events.AdminAction
	for _, raw := range f.payloads(t, events.TypeAdminAction) {
		var a events.AdminAction
		if err := json.Unmarshal(raw, &a); err != nil {
			t.Fatal(err)
		}
		if a.Action == kind {
			out = append(out, a)
		}
	}
	return out
}

func problemCode(t *testing.T, rec *httptest.ResponseRecorder) apibase.ErrorCode {
	t.Helper()
	return decodeProblem(t, rec).Code
}

func TestAdminBanUser_Ok(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	user := f.seed(t, portSeed{handle: "mallory", wallet: true})
	token := fakes.PrivyAccessToken(privyAppID, user.PrivyUserID, f.clock.Now(), time.Hour)
	fund := "/v1/cabals/" + testkit.NewIDs(3).NewV7().String()
	if rec := f.call(t, token, http.MethodGet, "/v1/me", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/me before the ban = %d %s, want 200", rec.Code, rec.Body)
	}
	if rec := f.moderate(t, "ban", user.ID, "ban-1", "fraud"); rec.Code != http.StatusNoContent {
		t.Fatalf("ban = %d %s, want 204", rec.Code, rec.Body)
	}
	if got := f.status(t, user.ID); got != "banned" {
		t.Fatalf("account_status = %s, want banned", got)
	}
	rec := f.call(t, token, http.MethodPost, fund+"/fund", "fund-1", `{"usdc_micros":"1000000"}`)
	if rec.Code != http.StatusForbidden || problemCode(t, rec) != apibase.AccountBanned {
		t.Fatalf("fund after the ban = %d %s, want 403 account_banned", rec.Code, rec.Body)
	}
	rec = f.call(t, token, http.MethodPost, fund+"/cashouts", "cash-1", `{"all":true}`)
	if got := problemCode(t, rec); got != apibase.ErrorCode(errs.CodeLiveSwapExists) {
		t.Fatalf("cashouts after the ban = %d %s, want it to reach the handler", rec.Code, rec.Body)
	}
	f.expectBanEvents(t, user.ID)
}

func (f adminFixture) expectBanEvents(t *testing.T, user ids.UserID) {
	t.Helper()
	changes := f.payloads(t, events.TypeUserAccountStatusChanged)
	var change events.UserAccountStatusChanged
	if len(changes) != 1 || json.Unmarshal(changes[0], &change) != nil {
		t.Fatalf("user.account_status_changed = %s, want one", changes)
	}
	change.At = time.Time{}
	want := events.UserAccountStatusChanged{V: 1, UserID: user.UUID(), From: "active", To: "banned", Cause: "admin"}
	if change != want {
		t.Fatalf("user.account_status_changed = %+v, want %+v", change, want)
	}
	audit := f.audits(t, events.AdminActionUserBan)
	if len(audit) != 1 {
		t.Fatalf("%d user_ban admin.action events, want 1", len(audit))
	}
	got := [4]string{audit[0].TargetID, audit[0].AdminID.String(), audit[0].Reason, string(audit[0].TargetType)}
	if got != [4]string{user.String(), moderatorID, "fraud", "user"} {
		t.Fatalf("admin.action = %v, want the moderator's user_ban of the user", got)
	}
	if !sameJSON(audit[0].Before, `{"account_status":"active"}`) ||
		!sameJSON(audit[0].After, `{"account_status":"banned"}`) {
		t.Fatalf("admin.action before %s after %s, want active to banned", audit[0].Before, audit[0].After)
	}
}

func TestAdminBanUser_FromSuspended(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	user := f.seed(t, portSeed{handle: "sam", status: "suspended"})
	if rec := f.moderate(t, "ban", user.ID, "ban-1", "fraud"); rec.Code != http.StatusNoContent {
		t.Fatalf("ban = %d %s, want 204", rec.Code, rec.Body)
	}
	if got := f.status(t, user.ID); got != "banned" {
		t.Fatalf("account_status = %s, want banned", got)
	}
}

func TestAdminBanUser_ViewerForbidden(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	user := f.seed(t, portSeed{handle: "vic"})
	path := "/v1/admin/users/" + user.ID.String() + "/ban"
	rec := f.call(t, viewerToken, http.MethodPost, path, "k1", `{"reason":"fraud"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("ban as a viewer = %d %s, want 403", rec.Code, rec.Body)
	}
	if got := f.status(t, user.ID); got != "active" {
		t.Fatalf("account_status = %s, want active", got)
	}
	if n := len(f.payloads(t, events.TypeAdminAction)); n != 0 {
		t.Fatalf("%d admin.action events after a refused ban, want 0", n)
	}
}

func (f adminFixture) grantAdmin(t *testing.T, user ids.UserID, role string, revoked bool) {
	t.Helper()
	revokedAt := "NULL"
	if revoked {
		revokedAt = "now()"
	}
	_, err := f.pool.Exec(t.Context(),
		`INSERT INTO admins (user_id, role, granted_at, revoked_at) VALUES ($1, $2, now(), `+revokedAt+`)`,
		user.UUID(), role)
	if err != nil {
		t.Fatal(err)
	}
}

func TestAdminBanUser_AnAdminOfEqualOrHigherRankIsProtected(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		caller, target string
		revoked, ok    bool
	}{
		"moderator bans operator":        {moderatorToken, "operator", false, false},
		"moderator bans moderator":       {moderatorToken, "moderator", false, false},
		"operator bans operator":         {operatorToken, "operator", false, false},
		"moderator bans viewer admin":    {moderatorToken, "viewer", false, true},
		"operator bans moderator":        {operatorToken, "moderator", false, true},
		"moderator bans a revoked admin": {moderatorToken, "operator", true, true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newAdminFixture(t)
			user := f.seed(t, portSeed{handle: "opsadmin"})
			f.grantAdmin(t, user.ID, tc.target, tc.revoked)
			path := "/v1/admin/users/" + user.ID.String() + "/ban"
			rec := f.call(t, tc.caller, http.MethodPost, path, "k1", `{"reason":"fraud"}`)
			f.expectBanOutcome(t, rec, user.ID, tc.ok)
		})
	}
}

func (f adminFixture) expectBanOutcome(t *testing.T, rec *httptest.ResponseRecorder, user ids.UserID, ok bool) {
	t.Helper()
	want, code, events := "active", http.StatusForbidden, 0
	if ok {
		want, code, events = "banned", http.StatusNoContent, 2
	}
	if rec.Code != code || f.status(t, user) != want {
		t.Fatalf("ban = %d %s, status %s, want %d %s", rec.Code, rec.Body, f.status(t, user), code, want)
	}
	if !ok && problemCode(t, rec) != apiCode(errs.CodeForbidden) {
		t.Fatalf("ban code = %s, want forbidden", problemCode(t, rec))
	}
	n := len(f.payloads(t, "admin.action")) + len(f.payloads(t, "user.account_status_changed"))
	if n != events {
		t.Fatalf("%d events after the ban, want %d", n, events)
	}
}

func TestAdminBanUser_Refusals(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	banned := f.seed(t, portSeed{handle: "already", status: "banned"})
	deleted := f.seed(t, portSeed{handle: "gone", status: "deleted", softDeleted: true})
	active := f.seed(t, portSeed{handle: "fine"})
	missing := ids.UserIDFrom(testkit.NewIDs(7).NewV7())
	for name, tc := range map[string]struct {
		action, reason string
		user           ids.UserID
		status         int
		code           apibase.ErrorCode
	}{
		"already banned":   {"ban", "fraud", banned.ID, http.StatusConflict, apibase.ErrorCode(errs.CodeAlreadyBanned)},
		"deleted":          {"ban", "fraud", deleted.ID, http.StatusConflict, apibase.ErrorCode(errs.CodeUserDeleted)},
		"unknown":          {"ban", "fraud", missing, http.StatusNotFound, apibase.ErrorCode(errs.CodeUserNotFound)},
		"short reason":     {"ban", "no", active.ID, http.StatusBadRequest, apibase.ErrorCode(errs.CodeReasonRequired)},
		"unban not banned": {"unban", "mistake", active.ID, http.StatusConflict, apibase.ErrorCode(errs.CodeNotBanned)},
		"unban deleted":    {"unban", "mistake", deleted.ID, http.StatusConflict, apibase.ErrorCode(errs.CodeUserDeleted)},
		"unban unknown":    {"unban", "mistake", missing, http.StatusNotFound, apibase.ErrorCode(errs.CodeUserNotFound)},
		"unban short":      {"unban", "no", banned.ID, http.StatusBadRequest, apibase.ErrorCode(errs.CodeReasonRequired)},
	} {
		rec := f.moderate(t, tc.action, tc.user, "k-"+name, tc.reason)
		if rec.Code != tc.status || problemCode(t, rec) != tc.code {
			t.Errorf("%s: %d %s, want %d %s", name, rec.Code, rec.Body, tc.status, tc.code)
		}
	}
	audits, changes := f.payloads(t, events.TypeAdminAction), f.payloads(t, events.TypeUserAccountStatusChanged)
	if n := len(audits) + len(changes); n != 0 {
		t.Errorf("%d events after refusals, want 0", n)
	}
}

func TestAdminUnbanUser_Ok(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	user := f.seed(t, portSeed{handle: "back", status: "banned"})
	if rec := f.moderate(t, "unban", user.ID, "unban-1", "appeal upheld"); rec.Code != http.StatusNoContent {
		t.Fatalf("unban = %d %s, want 204", rec.Code, rec.Body)
	}
	if got := f.status(t, user.ID); got != "active" {
		t.Fatalf("account_status = %s, want active", got)
	}
	audit := f.audits(t, events.AdminActionUserUnban)
	if len(audit) != 1 || !sameJSON(audit[0].Before, `{"account_status":"banned"}`) ||
		!sameJSON(audit[0].After, `{"account_status":"active"}`) {
		t.Fatalf("admin.action = %+v, want one user_unban from banned to active", audit)
	}
	if n := len(f.payloads(t, events.TypeUserAccountStatusChanged)); n != 1 {
		t.Fatalf("%d user.account_status_changed events, want 1", n)
	}
}

func TestAdminBanUser_NeedsAnAdminActor(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	user := f.seed(t, portSeed{handle: "target"})
	status := app.NewAdminAccountStatus(app.AdminAccountStatusDeps{
		UoW: db.New(f.pool, f.ids, f.clock), IDs: f.ids, Clock: f.clock, Hints: &recordedHints{},
	})
	reason, _ := events.NewReason("fraud")
	for name, ctx := range map[string]context.Context{
		"no actor":     t.Context(),
		"a user":       auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: user.ID.String()}),
		"a service id": auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorAdmin, ID: "ops-1"}),
	} {
		want := map[string]errs.Code{
			"no actor": errs.CodeUnauthorized, "a user": errs.CodeForbidden, "a service id": errs.CodeAdminForbidden,
		}[name]
		err := status.Ban(ctx, app.BanUser{UserID: user.ID, Reason: reason})
		if errs.CodeOf(err) != want {
			t.Errorf("%s: Ban = %v, want %s", name, err, want)
		}
	}
}

func sameJSON(got json.RawMessage, want string) bool {
	var a, b any
	return json.Unmarshal(got, &a) == nil && json.Unmarshal([]byte(want), &b) == nil && reflect.DeepEqual(a, b)
}

func adminCtx(t *testing.T) context.Context {
	t.Helper()
	return auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorAdmin, ID: moderatorID})
}

func TestAdminBanUser_RollsBackWhenAWriteFails(t *testing.T) {
	t.Parallel()
	reason, err := events.NewReason("fraud")
	if err != nil {
		t.Fatal(err)
	}
	for name, ddl := range map[string]string{
		"the status update": `CREATE FUNCTION no_update() RETURNS trigger LANGUAGE plpgsql AS
			$$ BEGIN RAISE EXCEPTION 'no'; END $$;
			CREATE TRIGGER no_update BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION no_update()`,
		"the status event": `ALTER TABLE events ADD CONSTRAINT no_event
			CHECK (type <> 'user.account_status_changed') NOT VALID`,
		"the audit": `ALTER TABLE events ADD CONSTRAINT no_audit CHECK (type <> 'admin.action') NOT VALID`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newAdminFixture(t)
			user := f.seed(t, portSeed{handle: "keep"})
			if _, err := f.pool.Exec(t.Context(), ddl); err != nil {
				t.Fatal(err)
			}
			status := app.NewAdminAccountStatus(app.AdminAccountStatusDeps{
				UoW: db.New(f.pool, f.ids, f.clock), IDs: f.ids, Clock: f.clock, Hints: &recordedHints{},
			})
			if err := status.Ban(adminCtx(t), app.BanUser{UserID: user.ID, Reason: reason}); err == nil {
				t.Error("Ban = nil, want an error")
			}
			if _, err := f.pool.Exec(t.Context(), `DROP TRIGGER IF EXISTS no_update ON users`); err != nil {
				t.Fatal(err)
			}
			if got := f.status(t, user.ID); got != "active" {
				t.Errorf("account_status = %s after the failed ban, want active", got)
			}
		})
	}
}

func TestAdminBanUser_RequiresAReason(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	user := f.seed(t, portSeed{handle: "nobody"})
	status := app.NewAdminAccountStatus(app.AdminAccountStatusDeps{
		UoW: db.New(f.pool, f.ids, f.clock), IDs: f.ids, Clock: f.clock, Hints: &recordedHints{},
	})
	err := status.Ban(adminCtx(t), app.BanUser{UserID: user.ID})
	if errs.CodeOf(err) != errs.CodeReasonRequired {
		t.Fatalf("Ban without a reason = %v, want %s", err, errs.CodeReasonRequired)
	}
	if got := f.status(t, user.ID); got != "active" {
		t.Fatalf("account_status = %s, want active", got)
	}
}

func TestAdminBanUser_AnUnreadableRowIsInternal(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	user := f.seed(t, portSeed{handle: "broken"})
	if _, err := f.pool.Exec(t.Context(), `ALTER TABLE users RENAME COLUMN account_status TO standing`); err != nil {
		t.Fatal(err)
	}
	status := app.NewAdminAccountStatus(app.AdminAccountStatusDeps{
		UoW: db.New(f.pool, f.ids, f.clock), IDs: f.ids, Clock: f.clock, Hints: &recordedHints{},
	})
	reason, _ := events.NewReason("fraud")
	err := status.Ban(adminCtx(t), app.BanUser{UserID: user.ID, Reason: reason})
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Ban on an unreadable row = %v, want %s", err, errs.CodeInternal)
	}
}

func apiCode(code errs.Code) apibase.ErrorCode { return apibase.ErrorCode(code) }

func userCtx(t *testing.T, user ids.UserID) context.Context {
	t.Helper()
	return auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: user.String()})
}
