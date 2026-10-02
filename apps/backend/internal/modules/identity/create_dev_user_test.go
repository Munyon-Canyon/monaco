package identity_test

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/privyfake"
)

func TestCreateDevUser_Ok(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	clk := testkit.NewClock(clock.Real{}.Now().UTC().Truncate(time.Microsecond))
	privy := &privyfake.Users{}
	wallets := &privyfake.Wallets{}
	hints := &recordedHints{}
	got, err := app.CreateDevUser(t.Context(), devDeps(pool, clk, privy, wallets, hints, devRand()))
	if err != nil {
		t.Fatal(err)
	}
	assertDevRow(t, pool, got)
	assertDevEvents(t, pool, got, clk, hints)
	assertDevPrivy(t, privy, wallets)
}

func TestCreateDevUser_Production(t *testing.T) {
	t.Parallel()
	privy := &countUsers{PrivyUsers: &privyfake.Users{}}
	wallets := &privyfake.Wallets{}
	_, err := app.CreateDevUser(t.Context(), app.CreateDevUserDeps{
		Env: config.EnvProduction, Privy: privy, Wallets: wallets, Rand: devRand(),
	})
	if errs.CodeOf(err) != errs.CodeInvalidInput || privy.n != 0 || wallets.Creates() != 0 {
		t.Fatalf("err %v privy %d wallets %d", err, privy.n, wallets.Creates())
	}
}

func TestCreateDevUser_rejectsAFailedDrawAndAFailedPrivyCall(t *testing.T) {
	t.Parallel()
	privy := &countUsers{PrivyUsers: &privyfake.Users{}}
	_, err := app.CreateDevUser(t.Context(), app.CreateDevUserDeps{
		Env: config.EnvLocal, Privy: privy, Rand: failRead{},
	})
	if errs.CodeOf(err) != errs.CodeInternal || privy.n != 0 {
		t.Fatalf("rand err %v calls %d", err, privy.n)
	}
	users := &privyfake.Users{}
	users.Fail("Create", errs.New(errs.CodeUpstreamUnavailable, "test.privy"))
	wallets := &privyfake.Wallets{}
	_, err = app.CreateDevUser(t.Context(), app.CreateDevUserDeps{
		Env: config.EnvLocal, Privy: users, Wallets: wallets, Rand: devRand(),
	})
	if err == nil || wallets.Creates() != 0 {
		t.Fatalf("privy err %v creates %d", err, wallets.Creates())
	}
}

func TestCreateDevUser_stopsWhenTheWalletCannotBeCreated(t *testing.T) {
	t.Parallel()
	privy := &privyfake.Users{}
	wallets := &privyfake.Wallets{}
	wallets.Fail("FindOrCreate", errs.New(errs.CodeUpstreamUnavailable, "test.wallet"))
	_, err := app.CreateDevUser(t.Context(), app.CreateDevUserDeps{
		Env: config.EnvLocal, Privy: privy, Wallets: wallets, Rand: devRand(), IDs: ids.Real{},
		Clock: clock.Real{},
	})
	if err == nil {
		t.Fatal("expected the wallet error")
	}
}

func TestCreateDevUser_rollsBackWhenTheWriteFails(t *testing.T) {
	t.Parallel()
	cases := []devStore{
		{insertErr: errs.New(errs.CodeInternal, "test.insert")},
		{attachErr: errs.New(errs.CodeInternal, "test.attach"), attached: true},
		{attached: false},
		{attached: true, abort: "attach"},
		{attached: true, updateErr: errs.New(errs.CodeAuthStateTransition, "test.update")},
		{attached: true, abort: "update"},
	}
	for _, store := range cases {
		t.Run(store.name(), func(t *testing.T) {
			t.Parallel()
			pool := testkit.DB(t)
			clk := testkit.NewClock(clock.Real{}.Now().UTC().Truncate(time.Microsecond))
			hints := &recordedHints{}
			deps := devDeps(pool, clk, &privyfake.Users{}, &privyfake.Wallets{}, hints, devRand())
			deps.Users = store
			if _, err := app.CreateDevUser(t.Context(), deps); err == nil {
				t.Fatal("expected an error")
			}
			var n int
			if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM users`).Scan(&n); err != nil || n != 0 {
				t.Fatalf("users = %d, %v", n, err)
			}
			if hints.sent() != nil {
				t.Fatalf("hints = %v", hints.sent())
			}
		})
	}
}

func devDeps(
	pool *pgxpool.Pool, clk clock.Clock, privy app.PrivyUsers, wallets app.MemberWallets,
	hints app.Hints, rand io.Reader,
) app.CreateDevUserDeps {
	return app.CreateDevUserDeps{
		Env: config.EnvLocal, UoW: db.New(pool, ids.Real{}, clk), Users: adapters.Users{},
		Privy: privy, Wallets: wallets, IDs: ids.Real{}, Clock: clk, Hints: hints, Rand: rand,
	}
}

func assertDevRow(t *testing.T, pool *pgxpool.Pool, got app.DevUser) {
	t.Helper()
	const suffix = "0a1b2c3d"
	if got.Handle != "dev_"+suffix || got.WalletAddress == "" {
		t.Fatalf("DevUser = %+v", got)
	}
	var handle, name, state, provider string
	err := pool.QueryRow(t.Context(),
		`SELECT handle, display_name, auth_state, login_provider FROM users WHERE id = $1`, got.UserID.UUID(),
	).Scan(&handle, &name, &state, &provider)
	if err != nil || handle != got.Handle || name != "Dev "+suffix || state != string(domain.AuthAwaitingPhone) ||
		provider != string(domain.LoginEmail) {
		t.Fatalf("user %s %s %s %s, %v", handle, name, state, provider, err)
	}
	var walletsN int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM user_wallets WHERE user_id = $1`, got.UserID.UUID()).
		Scan(&walletsN); err != nil || walletsN != 1 {
		t.Fatalf("user_wallets = %d, %v", walletsN, err)
	}
}

func assertDevEvents(
	t *testing.T, pool *pgxpool.Pool, got app.DevUser, clk clock.Clock, hints *recordedHints,
) {
	t.Helper()
	created := devCreated(t, pool)
	changed := devAuthChanges(t, pool)
	wantCreated := events.UserCreated{
		V: 1, UserID: got.UserID.UUID(), LoginProvider: string(domain.LoginEmail), CreatedAt: clk.Now(),
	}
	wantChanged := events.UserAuthStateChanged{
		V: 1, UserID: got.UserID.UUID(), From: string(domain.AuthCreated), To: string(domain.AuthAwaitingPhone),
		Cause: string(domain.CauseOnboarding), At: clk.Now(),
	}
	if len(created) != 1 || created[0] != wantCreated || len(changed) != 1 || changed[0] != wantChanged {
		t.Fatalf("events created %+v changed %+v", created, changed)
	}
	if sent := hints.sent(); len(sent) != 1 || sent[0] != "user."+got.UserID.String()+".me_changed" {
		t.Fatalf("hints = %v", sent)
	}
}

func assertDevPrivy(t *testing.T, privy *privyfake.Users, wallets *privyfake.Wallets) {
	t.Helper()
	stored, err := privy.User(t.Context(), "did:privy:fake-1")
	if err != nil || stored.Email != "dev-0a1b2c3d@example.com" || wallets.Creates() != 1 {
		t.Fatalf("privy user %+v creates %d, %v", stored, wallets.Creates(), err)
	}
}

func devRand() io.Reader { return bytes.NewReader([]byte{0x0a, 0x1b, 0x2c, 0x3d}) }

type failRead struct{}

func (failRead) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

type countUsers struct {
	app.PrivyUsers
	n int
}

func (c *countUsers) Create(ctx context.Context, email string) (app.PrivyUserID, error) {
	c.n++
	return c.PrivyUsers.Create(ctx, email)
}

type devStore struct {
	insertErr error
	attachErr error
	attached  bool
	updateErr error
	abort     string
}

func (s devStore) name() string {
	switch {
	case s.insertErr != nil:
		return "insert"
	case s.attachErr != nil:
		return "attach"
	case s.abort == "attach":
		return "created event"
	case s.updateErr != nil:
		return "auth state"
	case s.abort == "update":
		return "auth event"
	default:
		return "wallet mismatch"
	}
}

func (s devStore) InsertDevUser(
	context.Context, sqlc.DBTX, ids.UserID, string, string, string, time.Time,
) error {
	return s.insertErr
}

func (s devStore) AttachWallet(
	ctx context.Context, q sqlc.DBTX, _ ids.UserID, _ domain.Wallet, _ time.Time,
) (bool, error) {
	if s.abort == "attach" {
		_, _ = q.Exec(ctx, "SELECT 1/0")
	}
	return s.attached, s.attachErr
}

func (s devStore) UpdateAuthState(
	ctx context.Context, q sqlc.DBTX, _ ids.UserID, _, _ domain.AuthState, _ time.Time,
) error {
	if s.abort == "update" {
		_, _ = q.Exec(ctx, "SELECT 1/0")
	}
	return s.updateErr
}

func devCreated(t *testing.T, pool *pgxpool.Pool) []events.UserCreated {
	t.Helper()
	rows, err := pool.Query(t.Context(), `SELECT payload FROM events WHERE type = $1 ORDER BY id`,
		string(events.TypeUserCreated))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []events.UserCreated
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			t.Fatal(err)
		}
		ev, err := events.Decode(events.TypeUserCreated, 1, payload)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, ev.(events.UserCreated))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func devAuthChanges(t *testing.T, pool *pgxpool.Pool) []events.UserAuthStateChanged {
	t.Helper()
	rows, err := pool.Query(t.Context(), `SELECT payload FROM events WHERE type = $1 ORDER BY id`,
		string(events.TypeUserAuthStateChanged))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []events.UserAuthStateChanged
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			t.Fatal(err)
		}
		ev, err := events.Decode(events.TypeUserAuthStateChanged, 1, payload)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, ev.(events.UserAuthStateChanged))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}
