package identity_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/privyfake"
)

type poolFixture struct {
	db      *pgxpool.Pool
	deps    app.CreateDevUserDeps
	privy   *countUsers
	wallets *privyfake.Wallets
}

func newPoolFixture(t *testing.T, poolName string) poolFixture {
	t.Helper()
	pool := testkit.DB(t)
	clk := testkit.NewClock(clock.Real{}.Now().UTC().Truncate(time.Microsecond))
	privy := &countUsers{PrivyUsers: &privyfake.Users{}}
	wallets := &privyfake.Wallets{}
	deps := devDeps(pool, clk, privy, wallets, &recordedHints{}, devRand())
	deps.Pools, deps.Pool = adapters.Users{}, poolName
	return poolFixture{db: pool, deps: deps, privy: privy, wallets: wallets}
}

func TestCreateDevUser_poolMintsOnceThenReuses(t *testing.T) {
	t.Parallel()
	f := newPoolFixture(t, "browse-host")
	first, err := app.CreateDevUser(t.Context(), f.deps)
	if err != nil || first.Handle != "dev_browse_host" {
		t.Fatalf("first = %+v, %v", first, err)
	}
	second, err := app.CreateDevUser(t.Context(), f.deps)
	if err != nil || second != first || f.privy.n != 1 {
		t.Fatalf("second = %+v, %v after %d Privy creates, want %+v and 1", second, err, f.privy.n, first)
	}
	f.deps.Pool = "browse-other"
	other, err := app.CreateDevUser(t.Context(), f.deps)
	if err != nil || other.UserID == first.UserID || f.privy.n != 2 {
		t.Fatalf("other = %+v, %v after %d creates", other, err, f.privy.n)
	}
}

func TestCreateDevUser_poolAdoptsThePrivyUserAnotherMachineMade(t *testing.T) {
	t.Parallel()
	f := newPoolFixture(t, "join-host")
	existing, err := f.privy.PrivyUsers.Create(t.Context(), domain.DevEmail("join-host"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := app.CreateDevUser(t.Context(), f.deps)
	if err != nil || f.privy.n != 0 {
		t.Fatalf("CreateDevUser = %+v, %v after %d creates, want an adopted user and 0", got, err, f.privy.n)
	}
	var privyID string
	if err := f.db.QueryRow(t.Context(), `SELECT privy_user_id FROM users WHERE id = $1`, got.UserID.UUID()).
		Scan(&privyID); err != nil || privyID != string(existing) {
		t.Fatalf("row privy id = %q, %v, want %q", privyID, err, existing)
	}
}

func TestCreateDevUser_poolTwoLanesAtOnceMakeOneUser(t *testing.T) {
	t.Parallel()
	f := newPoolFixture(t, "vote-host")
	var group errgroup.Group
	users := make([]app.DevUser, 4)
	for i := range users {
		group.Go(func() (err error) {
			users[i], err = app.CreateDevUser(t.Context(), f.deps)
			return err
		})
	}
	if err := group.Wait(); err != nil {
		t.Fatal(err)
	}
	for _, u := range users {
		if u != users[0] {
			t.Fatalf("users = %+v, want one", users)
		}
	}
	var rows int
	if err := f.db.QueryRow(t.Context(), `SELECT count(*) FROM users`).Scan(&rows); err != nil ||
		rows != 1 || f.privy.n != 1 {
		t.Fatalf("%d rows, %d Privy creates, %v, want 1 and 1", rows, f.privy.n, err)
	}
}

func TestCreateDevUser_poolRestoresADeletedUser(t *testing.T) {
	t.Parallel()
	f := newPoolFixture(t, "leave-host")
	first, err := app.CreateDevUser(t.Context(), f.deps)
	if err != nil {
		t.Fatal(err)
	}
	pool := f.db
	if _, err := pool.Exec(
		t.Context(),
		`UPDATE users SET account_status = 'deleted', deleted_at = now(), display_name = ''`,
	); err != nil {
		t.Fatal(err)
	}
	again, err := app.CreateDevUser(t.Context(), f.deps)
	if err != nil || again != first || f.privy.n != 1 {
		t.Fatalf("restored = %+v, %v after %d creates, want %+v and 1", again, err, f.privy.n, first)
	}
	var status, name string
	if err := pool.QueryRow(t.Context(), `SELECT account_status, display_name FROM users`).
		Scan(&status, &name); err != nil ||
		status != "active" ||
		name != "Dev leave-host" {
		t.Fatalf("row = %q %q, %v", status, name, err)
	}
	if _, err := pool.Exec(
		t.Context(),
		`UPDATE users SET account_status = 'deleted', deleted_at = now(), privy_user_id = 'did:privy:other'`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := app.CreateDevUser(t.Context(), f.deps); errs.CodeOf(err) != errs.CodeWalletMismatch {
		t.Fatalf("restore against another Privy user = %v, want wallet_mismatch", err)
	}
}

func TestCreateDevUser_poolRefusesBadNames(t *testing.T) {
	t.Parallel()
	f := newPoolFixture(t, "")
	for _, name := range []string{"Upper", "has_underscore", "-lead", "trail-", "a--b", "seventeen-chars-xx", "0a1b2c3d"} {
		f.deps.Pool = name
		if _, err := app.CreateDevUser(
			t.Context(),
			f.deps,
		); errs.CodeOf(err) != errs.CodeInvalidInput ||
			f.privy.n != 0 {
			t.Errorf("pool %q = %v after %d creates, want invalid_input and 0", name, err, f.privy.n)
		}
	}
}

func TestCreateDevUser_poolStopsOnEachFailure(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "test.boom")
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	tests := map[string]func(f *poolFixture) context.Context{
		"canceled context": func(*poolFixture) context.Context { return canceled },
		"lock":             func(f *poolFixture) context.Context { f.deps.Pools = poolStore{lockErr: boom}; return t.Context() },
		"lookup":           func(f *poolFixture) context.Context { f.deps.Pools = poolStore{lookupErr: boom}; return t.Context() },
		"email lookup": func(f *poolFixture) context.Context {
			f.deps.Privy = &flakyPrivy{PrivyUsers: f.privy, byEmailErr: boom}
			return t.Context()
		},
		"create": func(f *poolFixture) context.Context {
			f.deps.Privy = &flakyPrivy{PrivyUsers: f.privy, createErr: boom}
			return t.Context()
		},
		"create duplicate that lookup cannot find": func(f *poolFixture) context.Context {
			f.deps.Privy = &flakyPrivy{PrivyUsers: f.privy, createErr: errs.New(errs.CodeInvalidInput, "test.dup")}
			return t.Context()
		},
		"create duplicate and lookup fails": func(f *poolFixture) context.Context {
			f.deps.Privy = &flakyPrivy{
				PrivyUsers: f.privy, createErr: errs.New(errs.CodeInvalidInput, "test.dup"), laterByEmailErr: boom,
			}
			return t.Context()
		},
		"insert": func(f *poolFixture) context.Context {
			f.deps.Users = devStore{insertErr: boom}
			return t.Context()
		},
		"wallet on insert": func(f *poolFixture) context.Context {
			w := &privyfake.Wallets{}
			w.Fail("FindOrCreate", boom)
			f.deps.Wallets = w
			return t.Context()
		},
	}
	for name, arrange := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newPoolFixture(t, "ok-host")
			ctx := arrange(&f)
			if _, err := app.CreateDevUser(ctx, f.deps); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestCreateDevUser_poolSurvivesALostCreateRace(t *testing.T) {
	t.Parallel()
	f := newPoolFixture(t, "race-host")
	f.deps.Privy = &flakyPrivy{PrivyUsers: f.privy, raceOnCreate: true}
	got, err := app.CreateDevUser(t.Context(), f.deps)
	if err != nil || got.Handle != "dev_race_host" {
		t.Fatalf("CreateDevUser = %+v, %v", got, err)
	}
}

func TestCreateDevUser_poolReuseAndRestoreFailures(t *testing.T) {
	t.Parallel()
	f := newPoolFixture(t, "reuse-host")
	if _, err := app.CreateDevUser(t.Context(), f.deps); err != nil {
		t.Fatal(err)
	}
	w := &privyfake.Wallets{}
	w.Fail("FindOrCreate", errs.New(errs.CodeUpstreamUnavailable, "test.wallet"))
	f.deps.Wallets = w
	if _, err := app.CreateDevUser(t.Context(), f.deps); err == nil {
		t.Fatal("expected the wallet error on reuse")
	}
	if _, err := f.db.Exec(t.Context(), `UPDATE users SET account_status = 'deleted', deleted_at = now()`); err != nil {
		t.Fatal(err)
	}
	f.deps.Wallets = &privyfake.Wallets{}
	f.deps.Pools = poolStore{restoreErr: errs.New(errs.CodeInternal, "test.restore")}
	if _, err := app.CreateDevUser(t.Context(), f.deps); err == nil {
		t.Fatal("expected the restore error")
	}
	f.deps.Pools = poolStore{restoreNothing: true}
	if _, err := app.CreateDevUser(t.Context(), f.deps); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("restore that changed no row = %v, want internal", err)
	}
}

func TestDevPoolAdapters_failOnACanceledContext(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	store := adapters.Users{}
	if err := store.LockDevHandle(ctx, pool, "dev_x"); err == nil {
		t.Error("LockDevHandle with a canceled context succeeded")
	}
	if _, _, err := store.DevUserByHandle(ctx, pool, "dev_x"); err == nil {
		t.Error("DevUserByHandle with a canceled context succeeded")
	}
	if _, err := store.RestoreDevUser(ctx, pool, ids.UserID{}, "x", time.Time{}); err == nil {
		t.Error("RestoreDevUser with a canceled context succeeded")
	}
}

type poolStore struct {
	adapters.Users
	lockErr        error
	lookupErr      error
	restoreErr     error
	restoreNothing bool
}

func (s poolStore) LockDevHandle(ctx context.Context, q sqlc.DBTX, handle string) error {
	if s.lockErr != nil {
		return s.lockErr
	}
	return s.Users.LockDevHandle(ctx, q, handle)
}

func (s poolStore) DevUserByHandle(ctx context.Context, q sqlc.DBTX, handle string) (app.DevUserRow, bool, error) {
	if s.lookupErr != nil {
		return app.DevUserRow{}, false, s.lookupErr
	}
	return s.Users.DevUserByHandle(ctx, q, handle)
}

func (s poolStore) RestoreDevUser(
	ctx context.Context, q sqlc.DBTX, id ids.UserID, name string, at time.Time,
) (bool, error) {
	if s.restoreErr != nil || s.restoreNothing {
		return false, s.restoreErr
	}
	return s.Users.RestoreDevUser(ctx, q, id, name, at)
}

type flakyPrivy struct {
	app.PrivyUsers
	byEmailErr      error
	createErr       error
	laterByEmailErr error
	raceOnCreate    bool
	lookups         int
}

func (p *flakyPrivy) ByEmail(ctx context.Context, email string) (app.PrivyUserID, bool, error) {
	p.lookups++
	switch {
	case p.byEmailErr != nil:
		return "", false, p.byEmailErr
	case p.lookups > 1 && p.laterByEmailErr != nil:
		return "", false, p.laterByEmailErr
	case p.raceOnCreate && p.lookups == 1:
		return "", false, nil
	}
	return p.PrivyUsers.ByEmail(ctx, email)
}

func (p *flakyPrivy) Create(ctx context.Context, email string) (app.PrivyUserID, error) {
	if p.raceOnCreate {
		if _, err := p.PrivyUsers.Create(ctx, email); err != nil {
			return "", err
		}
		return "", errs.New(errs.CodeInvalidInput, "test.lost_race")
	}
	if p.createErr != nil {
		return "", p.createErr
	}
	return p.PrivyUsers.Create(ctx, email)
}
