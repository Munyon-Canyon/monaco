package admin_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type lookupFixture struct {
	pool  *pgxpool.Pool
	clock *testkit.Clock
	ids   *testkit.IDs
	h     http.Handler
}

func newLookupFixture(t *testing.T) lookupFixture {
	t.Helper()
	pool := testkit.DB(t)
	clk := testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	return lookupFixture{
		pool: pool, clock: clk, ids: testkit.NewIDs(61),
		h: wiredAdminHandler(t, module.Deps{Pool: pool, Clock: clk}),
	}
}

func (f lookupFixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(t.Context(), query, args...); err != nil {
		t.Fatal(err)
	}
}

func (f lookupFixture) get(t *testing.T, path string, want int) map[string]any {
	t.Helper()
	w := adminRequest(t, f.h, path, "viewer")
	if w.Code != want {
		t.Fatalf("GET %s = %d %s, want %d", path, w.Code, w.Body, want)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func (f lookupFixture) plant(t *testing.T, user uuid.UUID) {
	t.Helper()
	f.exec(t, `UPDATE users SET email = 'planted@example.com', phone_e164 = '+15555550123',
		phone_verified_at = $2, x_user_id = 'x-777', x_username = 'plantedxhandle', x_linked_at = $2,
		first_deposit_at = $2 WHERE id = $1`, user, f.clock.Now())
}

func (f lookupFixture) authChange(t *testing.T, user uuid.UUID, from, to, cause string, at time.Time) {
	t.Helper()
	payload, err := json.Marshal(
		map[string]any{"v": 1, "user_id": user, "from": from, "to": to, "cause": cause, "at": at},
	)
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `INSERT INTO events (id, aggregate_type, aggregate_id, type, payload, actor_type, actor_id, created_at)
		VALUES ($1, 'user', $2, 'user.auth_state_changed', $3, 'system', 'test', $4)`,
		f.ids.NewV7(), user, payload, at)
}

func (f lookupFixture) action(t *testing.T, targetType, target string) {
	t.Helper()
	f.exec(
		t,
		`INSERT INTO admin_actions (id, admin_id, action, target_type, target_id, reason, before, after, created_at)
		VALUES ($1, $2, 'user_ban', $3, $4, 'spam', '{}', '{}', $5)`,
		f.ids.NewV7(),
		f.ids.NewV7(),
		targetType,
		target,
		f.clock.Now(),
	)
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func TestAdminLookup_User_NoPII(t *testing.T) {
	t.Parallel()
	f := newLookupFixture(t)
	user := testkit.SeedUser(t, f.pool, testkit.UserOpts{
		Handle: "alice", AuthState: "ONBOARDING_COMPLETED", WithWallet: true,
	})
	f.plant(t, user.ID.UUID())
	w := adminRequest(t, f.h, "/v1/admin/users/"+user.ID.String(), "viewer")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d %s", w.Code, w.Body)
	}
	for _, planted := range []string{
		"planted@example.com", "+15555550123", "plantedxhandle", "x-777", string(user.Address), user.PrivyUserID,
	} {
		if strings.Contains(w.Body.String(), planted) {
			t.Errorf("response contains %q: %s", planted, w.Body)
		}
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"account_status", "auth_state", "auth_state_history", "cabals", "created_at", "first_deposit_at", "handle",
		"id", "phone_verified", "positions", "recent_admin_actions", "recent_txns", "wallet", "x_linked",
	}
	if got := keys(body); !slices.Equal(got, want) {
		t.Fatalf("keys = %v, want %v", got, want)
	}
	if body["wallet"] != app.ShortAddress(string(user.Address)) || body["handle"] != "alice" ||
		body["phone_verified"] != true || body["x_linked"] != true || body["first_deposit_at"] == nil {
		t.Fatalf("body = %v", body)
	}
}

func TestAdminLookup_User_ByHandleAndMissing(t *testing.T) {
	t.Parallel()
	f := newLookupFixture(t)
	user := testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "alice"})
	body := f.get(t, "/v1/admin/users?handle=alice", http.StatusOK)
	if body["id"] != user.ID.String() || body["wallet"] != nil || body["account_status"] != "active" {
		t.Fatalf("body = %v", body)
	}
	for _, path := range []string{
		"/v1/admin/users?handle=nobody", "/v1/admin/users/" + f.ids.NewV7().String(),
		"/v1/admin/users/" + uuid.NewString(),
	} {
		if got := f.get(t, path, http.StatusNotFound); got["code"] != string(errs.CodeUserNotFound) {
			t.Errorf("GET %s = %v", path, got)
		}
	}
}

func TestAdminLookup_User_AuthStateHistory(t *testing.T) {
	t.Parallel()
	f := newLookupFixture(t)
	user := testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "alice"})
	other := testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "bob"})
	at := f.clock.Now()
	f.authChange(t, user.ID.UUID(), "CREATED", "AWAITING_PHONE", "otp_sent", at)
	f.authChange(t, other.ID.UUID(), "CREATED", "AWAITING_PHONE", "otp_sent", at)
	f.authChange(t, user.ID.UUID(), "AWAITING_PHONE", "AWAITING_SOCIALS", "phone_verified", at.Add(time.Minute))
	body := f.get(t, "/v1/admin/users/"+user.ID.String(), http.StatusOK)
	history, _ := body["auth_state_history"].([]any)
	if len(history) != 2 {
		t.Fatalf("history = %v", history)
	}
	first, _ := history[0].(map[string]any)
	second, _ := history[1].(map[string]any)
	if first["to"] != "AWAITING_PHONE" || first["cause"] != "otp_sent" || second["from"] != "AWAITING_PHONE" ||
		second["cause"] != "phone_verified" || second["at"] != at.Add(time.Minute).Format(time.RFC3339) {
		t.Fatalf("history = %v", history)
	}
}

func TestAdminLookup_User_CabalsPositionsTxnsAndActions(t *testing.T) {
	t.Parallel()
	f := newLookupFixture(t)
	user := testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "alice", WithWallet: true})
	c := testkit.NewCabal(t, f.pool, testkit.WithCreator(user.ID), testkit.WithName("Tech bros"))
	f.exec(t, `INSERT INTO user_positions (user_id, cabal_id, share_units, contributed_micros, withdrawn_micros,
		updated_at) VALUES ($1, $2, 25000000, 0, 0, $3)`, user.ID.UUID(), c.ID.UUID(), f.clock.Now())
	txn := f.ids.NewV7()
	f.exec(t, `INSERT INTO user_txns (id, user_id, cabal_id, kind, status, tx_signature, created_at)
		VALUES ($1, $2, $3, 'fund', 'settled', 'sig-1', $4)`, txn, user.ID.UUID(), c.ID.UUID(), f.clock.Now())
	f.action(t, "user", user.ID.String())
	f.action(t, "user", f.ids.NewV7().String())
	body := f.get(t, "/v1/admin/users/"+user.ID.String(), http.StatusOK)
	cabals, _ := body["cabals"].([]any)
	positions, _ := body["positions"].([]any)
	txns, _ := body["recent_txns"].([]any)
	actions, _ := body["recent_admin_actions"].([]any)
	if len(cabals) != 1 || len(positions) != 1 || len(txns) != 1 || len(actions) != 1 {
		t.Fatalf("body = %v", body)
	}
	if got, _ := cabals[0].(map[string]any); got["id"] != c.ID.String() || got["name"] != "Tech bros" {
		t.Fatalf("cabal = %v", cabals[0])
	}
	if got, _ := positions[0].(map[string]any); got["share_units"] != "25000000" {
		t.Fatalf("position = %v", positions[0])
	}
	if got, _ := txns[0].(map[string]any); got["id"] != txn.String() || got["scope"] != "user" ||
		got["tx_signature"] != "sig-1" || got["cabal_id"] != c.ID.String() {
		t.Fatalf("txn = %v", txns[0])
	}
}

func TestAdminLookup_User_QueryCountDoesNotGrowWithCabals(t *testing.T) {
	t.Parallel()
	f := newLookupFixture(t)
	user := testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "alice", WithWallet: true})
	read := func() {
		if w := adminRequest(t, f.h, "/v1/admin/users/"+user.ID.String(), "viewer"); w.Code != http.StatusOK {
			t.Fatalf("status = %d %s", w.Code, w.Body)
		}
	}
	testkit.AssertQueries(t, "admin GetAdminUser", read)
	for range 3 {
		testkit.NewCabal(t, f.pool, testkit.WithCreator(user.ID))
	}
	testkit.AssertQueries(t, "admin GetAdminUser", read)
}
