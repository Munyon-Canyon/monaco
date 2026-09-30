package identity_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type usersFixture struct {
	pool  *pgxpool.Pool
	ids   *testkit.IDs
	clock *testkit.Clock
	uow   *db.UnitOfWork
	users adapters.Users
}

func newUsersFixture(t *testing.T) usersFixture {
	t.Helper()
	g := testkit.NewIDs(testkit.RandSeed(t))
	pool := testkit.DB(t)
	c := testkit.NewClock(clock.Real{}.Now().UTC().Truncate(time.Microsecond))
	return usersFixture{pool: pool, ids: g, clock: c, uow: db.New(pool, g, c), users: adapters.Users{Clock: c}}
}

func (f usersFixture) newUserID(t *testing.T) ids.UserID {
	t.Helper()
	id, err := ids.ParseUserID(f.ids.NewV7().String())
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (f usersFixture) insert(t *testing.T, u domain.NewUser) error {
	t.Helper()
	return f.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		return f.users.Insert(ctx, tx.Queries(), u)
	})
}

func (f usersFixture) updateAuthState(t *testing.T, id ids.UserID, expected, next domain.AuthState) error {
	t.Helper()
	return f.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		return f.users.UpdateAuthState(ctx, tx.Queries(), id, expected, next)
	})
}

func (f usersFixture) updateAccountStatus(t *testing.T, id ids.UserID, expected, next domain.AccountStatus) error {
	t.Helper()
	return f.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		return f.users.UpdateAccountStatus(ctx, tx.Queries(), id, expected, next)
	})
}

func (f usersFixture) exec(t *testing.T, sql string, args ...any) error {
	t.Helper()
	_, err := f.pool.Exec(t.Context(), sql, args...)
	return err
}

func (f usersFixture) authStateChangedAt(t *testing.T, id ids.UserID) (domain.AuthState, time.Time) {
	t.Helper()
	var (
		state string
		at    time.Time
	)
	if err := f.pool.QueryRow(t.Context(), `SELECT auth_state, auth_state_changed_at FROM users WHERE id = $1`,
		id.UUID()).Scan(&state, &at); err != nil {
		t.Fatal(err)
	}
	return domain.AuthState(state), at.UTC()
}

func uniqueViolation(t *testing.T, err error, constraint string) {
	t.Helper()
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != "23505" || pg.ConstraintName != constraint {
		t.Fatalf("err = %v, want a unique violation on %s", err, constraint)
	}
}

func wantCode(t *testing.T, err error, code errs.Code) {
	t.Helper()
	if got := errs.CodeOf(err); err == nil || got != code {
		t.Fatalf("err = %v (code %s), want %s", err, got, code)
	}
}

func address(t *testing.T, fill byte) chain.SolanaAddress {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = fill
	}
	return chain.AddressOf(key)
}

func sameUser(t *testing.T, got domain.User, err error, want domain.User) {
	t.Helper()
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	gotWallet, wantWallet := got.Wallet, want.Wallet
	got.Wallet, want.Wallet = nil, nil
	sameWallet := gotWallet == wantWallet || gotWallet != nil && wantWallet != nil && *gotWallet == *wantWallet
	if got != want || !sameWallet {
		t.Fatalf("user = %+v with wallet %+v, want %+v with wallet %+v", got, gotWallet, want, wantWallet)
	}
}

func TestUsers_insertThenFindReturnsTheUserAndItsWallet(t *testing.T) {
	t.Parallel()
	f := newUsersFixture(t)
	id := f.newUserID(t)
	wallet := &domain.Wallet{PrivyWalletID: "wallet-1", Address: address(t, 7)}
	if err := f.insert(t, domain.NewUser{
		ID: id, PrivyUserID: "did:privy:one", LoginProvider: domain.LoginEmail, Email: "one@x.io", Wallet: wallet,
	}); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	want := domain.User{
		ID: id, PrivyUserID: "did:privy:one", AuthState: domain.AuthCreated, AccountStatus: domain.AccountActive,
		Wallet: wallet,
	}
	byPrivy, err := f.users.FindByPrivyUserID(t.Context(), f.pool, "did:privy:one")
	sameUser(t, byPrivy, err, want)
	byID, err := f.users.FindByID(t.Context(), f.pool, id)
	sameUser(t, byID, err, want)
	var email string
	var changedAt time.Time
	if err := f.pool.QueryRow(t.Context(), `SELECT email, auth_state_changed_at FROM users WHERE id = $1`,
		id.UUID()).Scan(&email, &changedAt); err != nil || email != "one@x.io" || !changedAt.Equal(f.clock.Now()) {
		t.Fatalf("stored email %q, auth_state_changed_at %s, %v; want one@x.io at %s", email, changedAt, err,
			f.clock.Now())
	}
}

func TestUsers_insertWithoutWalletOrEmailStoresNulls(t *testing.T) {
	t.Parallel()
	f := newUsersFixture(t)
	id := f.newUserID(t)
	err := f.insert(t, domain.NewUser{ID: id, PrivyUserID: "did:privy:two", LoginProvider: domain.LoginSMS})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	u, err := f.users.FindByID(t.Context(), f.pool, id)
	if err != nil || u.Wallet != nil {
		t.Fatalf("FindByID = %+v, %v, want no wallet", u, err)
	}
	var emailIsNull bool
	if err := f.pool.QueryRow(t.Context(), `SELECT email IS NULL FROM users WHERE id = $1`, id.UUID()).
		Scan(&emailIsNull); err != nil || !emailIsNull {
		t.Fatalf("email IS NULL = %v, %v", emailIsNull, err)
	}
}

func TestUsers_insertRollsBackTheUserWhenTheWalletCollides(t *testing.T) {
	t.Parallel()
	f := newUsersFixture(t)
	seeded := testkit.SeedUser(t, f.pool, testkit.UserOpts{WithWallet: true})
	err := f.insert(t, domain.NewUser{
		ID: f.newUserID(t), PrivyUserID: "did:privy:late", LoginProvider: domain.LoginSMS,
		Wallet: &domain.Wallet{PrivyWalletID: "wallet-other", Address: seeded.Address},
	})
	uniqueViolation(t, err, "user_wallets_address_key")
	_, err = f.users.FindByPrivyUserID(t.Context(), f.pool, "did:privy:late")
	wantCode(t, err, errs.CodeUserNotFound)
}

func TestUsers_findMissingUserIsUserNotFound(t *testing.T) {
	t.Parallel()
	f := newUsersFixture(t)
	_, err := f.users.FindByPrivyUserID(t.Context(), f.pool, "did:privy:nobody")
	wantCode(t, err, errs.CodeUserNotFound)
	_, err = f.users.FindByID(t.Context(), f.pool, f.newUserID(t))
	wantCode(t, err, errs.CodeUserNotFound)
}

func TestUsers_deletedUserIsHiddenByIDButFoundByPrivyID(t *testing.T) {
	t.Parallel()
	f := newUsersFixture(t)
	seeded := testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "gone", WithWallet: true})
	if err := f.updateAccountStatus(t, seeded.ID, domain.AccountActive, domain.AccountDeleted); err != nil {
		t.Fatalf("UpdateAccountStatus: %v", err)
	}
	_, err := f.users.FindByID(t.Context(), f.pool, seeded.ID)
	wantCode(t, err, errs.CodeUserNotFound)
	u, err := f.users.FindByPrivyUserID(t.Context(), f.pool, seeded.PrivyUserID)
	if err != nil || u.AccountStatus != domain.AccountDeleted || u.Handle != "gone" {
		t.Fatalf("FindByPrivyUserID = %+v, %v, want the deleted user with its handle", u, err)
	}
	var deletedAt time.Time
	if err := f.pool.QueryRow(t.Context(), `SELECT deleted_at FROM users WHERE id = $1`, seeded.ID.UUID()).
		Scan(&deletedAt); err != nil || !deletedAt.Equal(f.clock.Now()) {
		t.Fatalf("deleted_at = %s, %v, want %s", deletedAt, err, f.clock.Now())
	}
}

func TestUsers_updateAuthStateMovesOnlyFromTheExpectedState(t *testing.T) {
	t.Parallel()
	f := newUsersFixture(t)
	seeded := testkit.SeedUser(t, f.pool, testkit.UserOpts{})
	_, before := f.authStateChangedAt(t, seeded.ID)
	f.clock.Advance(time.Minute)
	err := f.updateAuthState(t, seeded.ID, domain.AuthAwaitingPhone, domain.AuthAwaitingSocials)
	wantCode(t, err, errs.CodeAuthStateTransition)
	if state, at := f.authStateChangedAt(t, seeded.ID); state != domain.AuthCreated || !at.Equal(before) {
		t.Fatalf("after a stale update: %s at %s, want CREATED at %s", state, at, before)
	}
	if err := f.updateAuthState(t, seeded.ID, domain.AuthCreated, domain.AuthAwaitingPhone); err != nil {
		t.Fatalf("UpdateAuthState: %v", err)
	}
	if state, at := f.authStateChangedAt(t, seeded.ID); state != domain.AuthAwaitingPhone || !at.Equal(f.clock.Now()) {
		t.Fatalf("after the update: %s at %s, want AWAITING_PHONE at %s", state, at, f.clock.Now())
	}
}

func TestUsers_updateAccountStatusMovesOnlyFromTheExpectedStatus(t *testing.T) {
	t.Parallel()
	f := newUsersFixture(t)
	seeded := testkit.SeedUser(t, f.pool, testkit.UserOpts{AccountStatus: "suspended"})
	err := f.updateAccountStatus(t, seeded.ID, domain.AccountActive, domain.AccountBanned)
	wantCode(t, err, errs.CodeAccountStatusTransition)
	u, err := f.users.FindByID(t.Context(), f.pool, seeded.ID)
	if err != nil || u.AccountStatus != domain.AccountSuspended {
		t.Fatalf("after a stale update: %+v, %v, want suspended", u, err)
	}
	if err := f.updateAccountStatus(t, seeded.ID, domain.AccountSuspended, domain.AccountBanned); err != nil {
		t.Fatalf("UpdateAccountStatus: %v", err)
	}
	u, err = f.users.FindByID(t.Context(), f.pool, seeded.ID)
	if err != nil || u.AccountStatus != domain.AccountBanned {
		t.Fatalf("after the update: %+v, %v, want banned", u, err)
	}
}

func TestUsers_databaseErrorsPassThrough(t *testing.T) {
	t.Parallel()
	f := newUsersFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	id := f.newUserID(t)
	checks := map[string]error{
		"Insert":              f.users.Insert(ctx, f.pool, domain.NewUser{ID: id, LoginProvider: domain.LoginSMS}),
		"UpdateAuthState":     f.users.UpdateAuthState(ctx, f.pool, id, domain.AuthCreated, domain.AuthAwaitingPhone),
		"UpdateAccountStatus": f.users.UpdateAccountStatus(ctx, f.pool, id, domain.AccountActive, domain.AccountBanned),
	}
	_, checks["FindByPrivyUserID"] = f.users.FindByPrivyUserID(ctx, f.pool, "did:privy:x")
	_, checks["FindByID"] = f.users.FindByID(ctx, f.pool, id)
	for name, err := range checks {
		if !errors.Is(err, context.Canceled) {
			t.Errorf("%s err = %v, want the context error", name, err)
		}
	}
}

func TestUsers_aRowWhoseIDIsNotV7FailsToDecode(t *testing.T) {
	t.Parallel()
	f := newUsersFixture(t)
	if err := f.exec(t, `INSERT INTO users (id, privy_user_id, login_provider, auth_state_changed_at, created_at,
		updated_at) VALUES (gen_random_uuid(), 'did:privy:v4', 'sms', now(), now(), now())`); err != nil {
		t.Fatal(err)
	}
	_, err := f.users.FindByPrivyUserID(t.Context(), f.pool, "did:privy:v4")
	wantCode(t, err, errs.CodeDecodeFailed)
}

func TestUsersSchema_handleIsNeverReusedAfterDelete(t *testing.T) {
	t.Parallel()
	f := newUsersFixture(t)
	testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "taken_once", AccountStatus: "deleted"})
	err := f.exec(t, `INSERT INTO users (id, privy_user_id, handle, login_provider, auth_state_changed_at, created_at,
		updated_at) VALUES ($1, 'did:privy:second', 'taken_once', 'sms', now(), now(), now())`, f.newUserID(t).UUID())
	uniqueViolation(t, err, "users_handle_key")
}

func TestUsersSchema_uniqueIdentityColumns(t *testing.T) {
	t.Parallel()
	hash := sha256.Sum256([]byte("+15550100"))
	tests := []struct {
		column     string
		value      any
		constraint string
	}{
		{"phone_hash", hash[:], "users_phone_hash_key"},
		{"x_user_id", "x-42", "users_x_user_id_key"},
		{"privy_user_id", "did:privy:dup", "users_privy_user_id_key"},
	}
	for _, tt := range tests {
		t.Run(tt.column, func(t *testing.T) {
			t.Parallel()
			f := newUsersFixture(t)
			insert := func(privyUserID string) error {
				if tt.column == "privy_user_id" {
					return f.exec(t, `INSERT INTO users (id, privy_user_id, login_provider, auth_state_changed_at,
						created_at, updated_at) VALUES ($1, $2, 'sms', now(), now(), now())`,
						f.newUserID(t).UUID(), tt.value)
				}
				return f.exec(t, `INSERT INTO users (id, privy_user_id, login_provider, auth_state_changed_at,
					created_at, updated_at, `+tt.column+`) VALUES ($1, $2, 'sms', now(), now(), now(), $3)`,
					f.newUserID(t).UUID(), privyUserID, tt.value)
			}
			if err := insert("did:privy:a"); err != nil {
				t.Fatalf("first row: %v", err)
			}
			uniqueViolation(t, insert("did:privy:b"), tt.constraint)
		})
	}
}

func TestUsersSchema_handleMustBeLowercase(t *testing.T) {
	t.Parallel()
	f := newUsersFixture(t)
	err := f.exec(t, `INSERT INTO users (id, privy_user_id, handle, login_provider, auth_state_changed_at, created_at,
		updated_at) VALUES ($1, 'did:privy:upper', 'KaiCenat', 'sms', now(), now(), now())`, f.newUserID(t).UUID())
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != "23514" || pg.ConstraintName != "users_handle_check" {
		t.Fatalf("err = %v, want the users_handle_check violation", err)
	}
}
