package notify_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/trace/noop"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type server struct {
	pool     *pgxpool.Pool
	handler  http.Handler
	verifier *auth.DevVerifier
	clock    *testkit.Clock
	logs     *testkit.Logs
}

type device struct {
	userID      uuid.UUID
	environment string
	createdAt   time.Time
	lastSeenAt  time.Time
	disabled    bool
}

func newServer(t *testing.T, opts ...notify.Option) server {
	t.Helper()
	pool := testkit.DB(t)
	g := testkit.NewIDs(1)
	clk := testkit.NewClock(clock.Real{}.Now().UTC().Truncate(time.Microsecond))
	verifier, err := auth.NewDevVerifier(
		config.Config{Env: config.EnvTest, Auth: config.Auth{DevTokenKey: "test-only"}}, clk)
	if err != nil {
		t.Fatal(err)
	}
	logs := &testkit.Logs{}
	deps := module.Deps{Pool: pool, UoW: db.New(pool, g, clk), IDs: g, Clock: clk}
	mount := notify.New(deps, opts...).Mount
	h, err := httpx.Handler(httpx.Deps{
		Logger:       observability.NewLogger(config.Config{Env: config.EnvTest}, logs),
		Tracer:       noop.NewTracerProvider(),
		Clock:        clk,
		IDs:          g,
		MaxBodyBytes: 1 << 20,
		Idempotency:  db.NewIdempotencyStore(pool, clk),
		Verifier:     verifier,
	}, mount, openapi.Spec)
	if err != nil {
		t.Fatal(err)
	}
	return server{pool: pool, handler: testkit.HTTP(t, h), verifier: verifier, clock: clk, logs: logs}
}

func (s server) user(t *testing.T, status string) ids.UserID {
	t.Helper()
	return testkit.SeedUser(t, s.pool, testkit.UserOpts{AccountStatus: status}).ID
}

func (s server) do(t *testing.T, method, path string, user ids.UserID, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.verifier.Mint(user.String(), s.clock.Now().Add(time.Hour)))
	req.Header.Set("Idempotency-Key", key)
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)
	return rec
}

func (s server) register(t *testing.T, user ids.UserID, key, token, env string) *httptest.ResponseRecorder {
	t.Helper()
	return s.do(t, http.MethodPost, "/v1/devices", user, key, `{"token":"`+token+`","environment":"`+env+`"}`)
}

func (s server) unregister(t *testing.T, user ids.UserID, key, token string) *httptest.ResponseRecorder {
	t.Helper()
	return s.do(t, http.MethodDelete, "/v1/devices/"+token, user, key, "")
}

func (s server) devices(t *testing.T, token string) []device {
	t.Helper()
	rows, err := s.pool.Query(
		t.Context(),
		`SELECT user_id, environment, created_at, last_seen_at, disabled_at IS NOT NULL
		FROM device_tokens WHERE token = $1`,
		token,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []device
	for rows.Next() {
		var d device
		if err := rows.Scan(&d.userID, &d.environment, &d.createdAt, &d.lastSeenAt, &d.disabled); err != nil {
			t.Fatal(err)
		}
		d.createdAt, d.lastSeenAt = d.createdAt.UTC(), d.lastSeenAt.UTC()
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func (s server) active(t *testing.T, user ids.UserID) []string {
	t.Helper()
	rows, err := sqlc.New(s.pool).ActiveTokensForUser(t.Context(), user.UUID())
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Token
	}
	return out
}

func token(c byte) string { return strings.Repeat(string(c), 64) }

func wantCode(t *testing.T, step string, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("%s = %d %s, want %d", step, rec.Code, rec.Body, want)
	}
}

func TestRegisterDevice_savesANewTokenForTheCaller(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	user := s.user(t, "active")
	wantCode(t, "POST", s.register(t, user, "k1", token('a'), "production"), http.StatusNoContent)
	got := s.devices(t, token('a'))
	want := device{userID: user.UUID(), environment: "production", createdAt: s.clock.Now(), lastSeenAt: s.clock.Now()}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("devices = %+v, want [%+v]", got, want)
	}
	if active := s.active(t, user); len(active) != 1 || active[0] != token('a') {
		t.Fatalf("active tokens = %v, want the new token", active)
	}
}

func TestRegisterDevice_reregisteringBumpsLastSeenAndKeepsOneRow(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	user := s.user(t, "active")
	created := s.clock.Now()
	wantCode(t, "first POST", s.register(t, user, "k1", token('b'), "sandbox"), http.StatusNoContent)
	s.clock.Advance(time.Hour)
	wantCode(t, "second POST", s.register(t, user, "k2", token('b'), "production"), http.StatusNoContent)
	got := s.devices(t, token('b'))
	want := device{userID: user.UUID(), environment: "production", createdAt: created, lastSeenAt: s.clock.Now()}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("devices = %+v, want [%+v]", got, want)
	}
}

func TestRegisterDevice_ReassignsOwner(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	first, second := s.user(t, "active"), s.user(t, "active")
	wantCode(t, "first user POST", s.register(t, first, "k1", token('c'), "sandbox"), http.StatusNoContent)
	wantCode(t, "second user POST", s.register(t, second, "k1", token('c'), "sandbox"), http.StatusNoContent)
	if got := s.devices(t, token('c')); len(got) != 1 || got[0].userID != second.UUID() {
		t.Fatalf("devices = %+v, want one row owned by %s", got, second)
	}
	if active := s.active(t, first); len(active) != 0 {
		t.Fatalf("first user's active tokens = %v, want none", active)
	}
}

func TestRegisterDevice_reactivatesADisabledToken(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	user := s.user(t, "active")
	wantCode(t, "POST", s.register(t, user, "k1", token('d'), "sandbox"), http.StatusNoContent)
	n, err := sqlc.New(s.pool).
		DisableToken(t.Context(), sqlc.DisableTokenParams{Token: token('d'), DisabledAt: s.clock.Now()})
	if err != nil || n != 1 || len(s.active(t, user)) != 0 {
		t.Fatalf("DisableToken = %d, %v with active %v, want the token disabled", n, err, s.active(t, user))
	}
	wantCode(t, "POST again", s.register(t, user, "k2", token('d'), "sandbox"), http.StatusNoContent)
	if got := s.devices(t, token('d')); len(got) != 1 || got[0].disabled {
		t.Fatalf("devices = %+v, want one active row", got)
	}
}

func TestRegisterDevice_replaysTheSameKeyByteIdentical(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	user := s.user(t, "active")
	first := s.register(t, user, "k1", token('e'), "sandbox")
	seen := s.clock.Now()
	s.clock.Advance(time.Minute)
	replay := s.register(t, user, "k1", token('e'), "sandbox")
	if first.Code != http.StatusNoContent || replay.Code != first.Code ||
		!bytes.Equal(replay.Body.Bytes(), first.Body.Bytes()) {
		t.Fatalf(
			"POST twice = %d %q then %d %q, want one 204 replayed",
			first.Code,
			first.Body,
			replay.Code,
			replay.Body,
		)
	}
	if got := s.devices(t, token('e')); len(got) != 1 || !got[0].lastSeenAt.Equal(seen) {
		t.Fatalf("devices = %+v, want one row last seen at %v, untouched by the replay", got, seen)
	}
}

func TestRegisterDevice_refusesMalformedInputAndADeletedAccount(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	user := s.user(t, "active")
	for name, tc := range map[string]struct {
		user       ids.UserID
		token, env string
		status     int
	}{
		"short token":     {user, strings.Repeat("a", 63), "sandbox", http.StatusBadRequest},
		"long token":      {user, strings.Repeat("a", 201), "sandbox", http.StatusBadRequest},
		"uppercase token": {user, strings.Repeat("A", 64), "sandbox", http.StatusBadRequest},
		"environment":     {user, token('f'), "staging", http.StatusBadRequest},
		"deleted account": {s.user(t, "deleted"), token('f'), "sandbox", http.StatusUnauthorized},
	} {
		if rec := s.register(t, tc.user, "k-"+name, tc.token, tc.env); rec.Code != tc.status {
			t.Errorf("%s: POST = %d %s, want %d", name, rec.Code, rec.Body, tc.status)
		}
	}
	wantCode(t, "DELETE malformed", s.unregister(t, user, "k-delete", strings.Repeat("g", 64)), http.StatusBadRequest)
	var n int
	if err := s.pool.QueryRow(t.Context(), `SELECT count(*) FROM device_tokens`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("device_tokens = %d, %v, want none", n, err)
	}
}

func TestRegisterDevice_letsSuspendedAndBannedUsersRegister(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	for i, status := range []string{"suspended", "banned"} {
		user := s.user(t, status)
		tok := token("12"[i])
		wantCode(t, status+" POST", s.register(t, user, "k1", tok, "sandbox"), http.StatusNoContent)
		if got := s.devices(t, tok); len(got) != 1 || got[0].userID != user.UUID() {
			t.Fatalf("%s: devices = %+v, want one row owned by %s", status, got, user)
		}
	}
}

func TestUnregisterDevice_deletesTheCallersOwnToken(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	user := s.user(t, "active")
	wantCode(t, "POST", s.register(t, user, "k1", token('a'), "sandbox"), http.StatusNoContent)
	wantCode(t, "DELETE", s.unregister(t, user, "k2", token('a')), http.StatusNoContent)
	if got := s.devices(t, token('a')); len(got) != 0 {
		t.Fatalf("devices = %+v, want none", got)
	}
}

func TestUnregisterDevice_byANonOwnerLeavesTheRowIntact(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	owner, other := s.user(t, "active"), s.user(t, "active")
	wantCode(t, "POST", s.register(t, owner, "k1", token('b'), "sandbox"), http.StatusNoContent)
	before := s.devices(t, token('b'))
	wantCode(t, "DELETE by another user", s.unregister(t, other, "k1", token('b')), http.StatusNoContent)
	wantCode(t, "DELETE unknown", s.unregister(t, other, "k2", token('c')), http.StatusNoContent)
	if after := s.devices(t, token('b')); len(after) != 1 || after[0] != before[0] {
		t.Fatalf("devices = %+v, want %+v unchanged", after, before)
	}
}

func TestDevices_logTheDecisionAndNeverTheToken(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	owner, other := s.user(t, "active"), s.user(t, "active")
	wantCode(t, "POST", s.register(t, owner, "k1", token('a'), "production"), http.StatusNoContent)
	wantCode(t, "DELETE by another user", s.unregister(t, other, "k2", token('a')), http.StatusNoContent)
	wantCode(t, "DELETE", s.unregister(t, owner, "k3", token('a')), http.StatusNoContent)
	wantCode(t, "DELETE malformed", s.unregister(t, owner, "k4", strings.Repeat("b", 63)), http.StatusBadRequest)
	logs := string(s.logs.Bytes())
	for _, secret := range []string{token('a'), strings.Repeat("b", 63)} {
		if strings.Contains(logs, secret) {
			t.Fatalf("logs contain a device token:\n%s", logs)
		}
	}
	for _, want := range []string{
		`"msg":"notify.device.registered"`,
		`"msg":"notify.device.unregister_skipped"`,
		`"msg":"notify.device.unregistered"`,
		`"user_id":"` + owner.String() + `","environment":"production"`,
		`"user_id":"` + other.String() + `"`,
	} {
		if !strings.Contains(logs, want) {
			t.Errorf("logs lack %s:\n%s", want, logs)
		}
	}
}

type users struct {
	cards map[ids.UserID]identity.UserCard
	err   error
}

func (u users) UsersByID(context.Context, []ids.UserID) (map[ids.UserID]identity.UserCard, error) {
	return u.cards, u.err
}

func TestRegisterDevice_refusesAnUnknownUserAndSurfacesFailures(t *testing.T) {
	t.Parallel()
	ghost, err := ids.ParseUserID(testkit.NewIDs(9).NewV7().String())
	if err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		users  users
		status int
	}{
		"unknown user":  {users{}, http.StatusUnauthorized},
		"identity down": {users{err: errs.New(errs.CodeInternal, "test.identity")}, http.StatusInternalServerError},
		"no users row":  {users{cards: map[ids.UserID]identity.UserCard{ghost: {ID: ghost}}}, http.StatusInternalServerError},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := newServer(t, notify.WithUsers(tc.users))
			if rec := s.register(t, ghost, "k1", token('a'), "sandbox"); rec.Code != tc.status {
				t.Errorf("POST = %d %s, want %d", rec.Code, rec.Body, tc.status)
			}
			if got := s.devices(t, token('a')); len(got) != 0 {
				t.Errorf("devices = %+v, want none", got)
			}
		})
	}
}
