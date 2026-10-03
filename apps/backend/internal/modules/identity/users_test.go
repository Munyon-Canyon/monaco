package identity_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
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
	return usersFixture{pool: pool, ids: g, clock: c, uow: db.New(pool, g, c), users: adapters.Users{}}
}

func (f usersFixture) newUserID(t *testing.T) ids.UserID {
	t.Helper()
	id, err := ids.ParseUserID(f.ids.NewV7().String())
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (f usersFixture) create(t *testing.T, u domain.NewUser) (bool, error) {
	t.Helper()
	var created bool
	err := f.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		var err error
		created, err = f.users.Create(ctx, tx.Queries(), u, f.clock.Now())
		return err
	})
	return created, err
}

func (f usersFixture) attachWallet(t *testing.T, id ids.UserID, w domain.Wallet) (bool, error) {
	t.Helper()
	var attached bool
	err := f.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		var err error
		attached, err = f.users.AttachWallet(ctx, tx.Queries(), id, w, f.clock.Now())
		return err
	})
	return attached, err
}

func (f usersFixture) updateAuthState(t *testing.T, id ids.UserID, expected, next domain.AuthState) error {
	t.Helper()
	return f.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		return f.users.UpdateAuthState(ctx, tx.Queries(), id, expected, next, f.clock.Now())
	})
}

func (f usersFixture) updateAccountStatus(t *testing.T, id ids.UserID, expected, next domain.AccountStatus) error {
	t.Helper()
	return f.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		return f.users.UpdateAccountStatus(ctx, tx.Queries(), id, expected, next, f.clock.Now())
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
	gotWallet, wantWallet, gotX, wantX := got.Wallet, want.Wallet, got.Links.X, want.Links.X
	got.Wallet, want.Wallet, got.Links.X, want.Links.X = nil, nil, nil, nil
	sameWallet := gotWallet == wantWallet || gotWallet != nil && wantWallet != nil && *gotWallet == *wantWallet
	sameX := gotX == wantX || gotX != nil && wantX != nil && *gotX == *wantX
	if got != want || !sameWallet || !sameX {
		t.Fatalf("user = %+v with wallet %+v and X %+v, want %+v with wallet %+v and X %+v",
			got, gotWallet, gotX, want, wantWallet, wantX)
	}
}

func TestUsers_createThenAttachThenFindReturnsTheUserAndItsWallet(t *testing.T) {
	t.Parallel()
	f := newUsersFixture(t)
	id := f.newUserID(t)
	wallet := domain.Wallet{PrivyWalletID: "wallet-1", Address: address(t, 7)}
	if created, err := f.create(
		t,
		domain.NewUser{ID: id, PrivyUserID: "did:privy:one", LoginProvider: domain.LoginEmail},
	); !created ||
		err != nil {
		t.Fatalf("Create = %v, %v, want created", created, err)
	}
	if attached, err := f.attachWallet(t, id, wallet); !attached || err != nil {
		t.Fatalf("AttachWallet = %v, %v, want attached", attached, err)
	}
	want := domain.User{
		ID: id, PrivyUserID: "did:privy:one", AuthState: domain.AuthCreated, AccountStatus: domain.AccountActive,
		Wallet: &wallet,
	}
	byPrivy, err := f.users.FindByPrivyUserID(t.Context(), f.pool, "did:privy:one")
	sameUser(t, byPrivy, err, want)
	byID, err := f.users.FindByID(t.Context(), f.pool, id)
	sameUser(t, byID, err, want)
	locked, err := f.users.Lock(t.Context(), f.pool, "did:privy:one")
	sameUser(t, locked, err, want)
	var provider string
	var changedAt, createdAt time.Time
	if err := f.pool.QueryRow(t.Context(), `SELECT login_provider, auth_state_changed_at, created_at FROM users
		WHERE id = $1`, id.UUID()).Scan(&provider, &changedAt, &createdAt); err != nil || provider != "email" ||
		!changedAt.Equal(f.clock.Now()) || !createdAt.Equal(f.clock.Now()) {
		t.Fatalf("stored provider %q, changed %s, created %s, %v; want email at %s", provider, changedAt, createdAt,
			err, f.clock.Now())
	}
}

func TestUsers_createLeavesAnExistingPrivyUserUntouched(t *testing.T) {
	t.Parallel()
	f := newUsersFixture(t)
	first, second := f.newUserID(t), f.newUserID(t)
	if created, err := f.create(
		t,
		domain.NewUser{ID: first, PrivyUserID: "did:privy:dup", LoginProvider: domain.LoginSMS},
	); !created ||
		err != nil {
		t.Fatalf("first Create = %v, %v", created, err)
	}
	created, err := f.create(
		t,
		domain.NewUser{ID: second, PrivyUserID: "did:privy:dup", LoginProvider: domain.LoginEmail},
	)
	if created || err != nil {
		t.Fatalf("second Create = %v, %v, want no row and no error", created, err)
	}
	u, err := f.users.FindByPrivyUserID(t.Context(), f.pool, "did:privy:dup")
	if err != nil || u.ID != first {
		t.Fatalf("FindByPrivyUserID = %+v, %v, want the first user", u, err)
	}
	if _, err := f.users.FindByID(t.Context(), f.pool, second); errs.CodeOf(err) != errs.CodeUserNotFound {
		t.Fatalf("FindByID(second) = %v, want user_not_found", err)
	}
}

func TestUsers_attachWalletKeepsTheFirstWalletAndRefusesAnAddressAnotherUserHolds(t *testing.T) {
	t.Parallel()
	f := newUsersFixture(t)
	seeded := testkit.SeedUser(t, f.pool, testkit.UserOpts{WithWallet: true})
	id := f.newUserID(t)
	if created, err := f.create(
		t,
		domain.NewUser{ID: id, PrivyUserID: "did:privy:late", LoginProvider: domain.LoginSMS},
	); !created ||
		err != nil {
		t.Fatalf("Create = %v, %v", created, err)
	}
	stolen := domain.Wallet{PrivyWalletID: "wallet-other", Address: seeded.Address}
	if attached, err := f.attachWallet(t, id, stolen); attached || err != nil {
		t.Fatalf("AttachWallet with another user's address = %v, %v, want no row and no error", attached, err)
	}
	mine := domain.Wallet{PrivyWalletID: "wallet-mine", Address: address(t, 9)}
	if attached, err := f.attachWallet(t, id, mine); !attached || err != nil {
		t.Fatalf("AttachWallet = %v, %v", attached, err)
	}
	if attached, err := f.attachWallet(
		t,
		id,
		domain.Wallet{PrivyWalletID: "wallet-new", Address: address(t, 10)},
	); attached ||
		err != nil {
		t.Fatalf("second AttachWallet = %v, %v, want the first wallet kept", attached, err)
	}
	u, err := f.users.FindByID(t.Context(), f.pool, id)
	if err != nil || u.Wallet == nil || *u.Wallet != mine {
		t.Fatalf("FindByID = %+v, %v, want the first wallet", u, err)
	}
}

func TestUsers_refreshEmailWritesOnlyWhenTheEmailChanges(t *testing.T) {
	t.Parallel()
	f := newUsersFixture(t)
	id := f.newUserID(t)
	if _, err := f.create(
		t,
		domain.NewUser{ID: id, PrivyUserID: "did:privy:mail", LoginProvider: domain.LoginSMS},
	); err != nil {
		t.Fatal(err)
	}
	refresh := func(email string) (*string, time.Time) {
		t.Helper()
		f.clock.Advance(time.Minute)
		if err := f.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
			return f.users.RefreshEmail(ctx, tx.Queries(), id, email, f.clock.Now())
		}); err != nil {
			t.Fatal(err)
		}
		var stored *string
		var updatedAt time.Time
		if err := f.pool.QueryRow(t.Context(), `SELECT email, updated_at FROM users WHERE id = $1`, id.UUID()).
			Scan(&stored, &updatedAt); err != nil {
			t.Fatal(err)
		}
		return stored, updatedAt.UTC()
	}
	set, changedAt := refresh("a@example.com")
	if set == nil || *set != "a@example.com" || !changedAt.Equal(f.clock.Now()) {
		t.Fatalf("after the first refresh: %v at %s, want a@example.com at %s", set, changedAt, f.clock.Now())
	}
	same, sameAt := refresh("a@example.com")
	if same == nil || *same != "a@example.com" || !sameAt.Equal(changedAt) {
		t.Fatalf(
			"after a refresh with the same email: %v at %s, want a@example.com still at %s",
			same,
			sameAt,
			changedAt,
		)
	}
	cleared, clearedAt := refresh("")
	if cleared != nil || !clearedAt.Equal(f.clock.Now()) {
		t.Fatalf("after clearing: %v at %s, want NULL at %s", cleared, clearedAt, f.clock.Now())
	}
}

func TestUsers_lockFindsTheRowIncludingADeletedUserAndNotFoundOtherwise(t *testing.T) {
	t.Parallel()
	f := newUsersFixture(t)
	seeded := testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "gone", WithWallet: true, AccountStatus: "deleted"})
	got, err := f.users.Lock(t.Context(), f.pool, seeded.PrivyUserID)
	if err != nil || got.ID != seeded.ID || got.AccountStatus != domain.AccountDeleted || got.Wallet == nil {
		t.Fatalf("Lock = %+v, %v, want the deleted user with its wallet", got, err)
	}
	_, err = f.users.Lock(t.Context(), f.pool, "did:privy:nobody")
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
	id, at := f.newUserID(t), f.clock.Now()
	_, create := f.users.Create(ctx, f.pool, domain.NewUser{ID: id, LoginProvider: domain.LoginSMS}, at)
	_, attach := f.users.AttachWallet(ctx, f.pool, id, domain.Wallet{}, at)
	checks := map[string]error{
		"Create":       create,
		"AttachWallet": attach,
		"RefreshEmail": f.users.RefreshEmail(ctx, f.pool, id, "a@example.com", at),
		"Delete":       f.users.Delete(ctx, f.pool, id, domain.AccountActive, at),
		"ApplyLinks phone": f.users.ApplyLinks(ctx, f.pool, id, domain.LinkSync{
			Phone: domain.Write[string]{Changed: true, Value: "+15550100"},
		}, at),
		"ApplyLinks X": f.users.ApplyLinks(ctx, f.pool, id, domain.LinkSync{
			X: domain.Write[*domain.XAccount]{Changed: true, Value: &domain.XAccount{UserID: "x"}},
		}, at),
		"ApplyLinks step": f.users.ApplyLinks(ctx, f.pool, id, domain.LinkSync{
			Steps: []domain.AuthStep{{From: domain.AuthCreated, To: domain.AuthAwaitingPhone}},
		}, at),
		"UpdateAuthState": f.users.UpdateAuthState(
			ctx,
			f.pool,
			id,
			domain.AuthCreated,
			domain.AuthAwaitingPhone,
			at,
		),
		"UpdateAccountStatus": f.users.UpdateAccountStatus(
			ctx,
			f.pool,
			id,
			domain.AccountActive,
			domain.AccountBanned,
			at,
		),
	}
	_, checks["FindByPrivyUserID"] = f.users.FindByPrivyUserID(ctx, f.pool, "did:privy:x")
	_, checks["Lock"] = f.users.Lock(ctx, f.pool, "did:privy:x")
	_, checks["HeldLinks"] = f.users.HeldLinks(
		ctx,
		f.pool,
		id,
		domain.Claims{Phone: true},
		domain.Links{Phone: "+15550100"},
	)
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

func TestUsers_applyLinksStoresClearsAndMovesTheStateAsPlanned(t *testing.T) {
	t.Parallel()
	f := newUsersFixture(t)
	id := f.newUserID(t)
	if _, err := f.create(
		t,
		domain.NewUser{ID: id, PrivyUserID: "did:privy:links", LoginProvider: domain.LoginSMS},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(
		t.Context(),
		`UPDATE users SET auth_state = 'AWAITING_PHONE' WHERE id = $1`,
		id.UUID(),
	); err != nil {
		t.Fatal(err)
	}
	x := &domain.XAccount{UserID: "x-77", Username: "seventy_seven"}
	apply := func(sync domain.LinkSync) error {
		return f.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
			return f.users.ApplyLinks(ctx, tx.Queries(), id, sync, f.clock.Now())
		})
	}
	set := domain.LinkSync{
		Phone: domain.Write[string]{
			Changed: true,
			Value:   "+15550100",
		},
		X: domain.Write[*domain.XAccount]{Changed: true, Value: x},
		Steps: []domain.AuthStep{
			{From: domain.AuthAwaitingPhone, To: domain.AuthOnboardingCompleted, Cause: domain.CauseLink},
		},
	}
	if err := apply(set); err != nil {
		t.Fatalf("ApplyLinks: %v", err)
	}
	got, err := f.users.FindByID(t.Context(), f.pool, id)
	sameUser(t, got, err, domain.User{
		ID: id, PrivyUserID: "did:privy:links", AuthState: domain.AuthOnboardingCompleted,
		AccountStatus: domain.AccountActive, Links: domain.Links{Phone: "+15550100", X: x},
	})
	var hash []byte
	var verified, linked, updated time.Time
	if err := f.pool.QueryRow(t.Context(), `SELECT phone_hash, phone_verified_at, x_linked_at, updated_at FROM users
		WHERE id = $1`, id.UUID()).Scan(&hash, &verified, &linked, &updated); err != nil ||
		!bytes.Equal(hash, portHash("+15550100")) || !verified.Equal(f.clock.Now()) || !linked.Equal(f.clock.Now()) ||
		!updated.Equal(f.clock.Now()) {
		t.Fatalf("stored hash %x, verified %s, linked %s, updated %s, %v; want the SHA-256 of the number at %s", hash,
			verified, linked, updated, err, f.clock.Now())
	}
	cleared := domain.LinkSync{
		Phone: domain.Write[string]{Changed: true},
		X:     domain.Write[*domain.XAccount]{Changed: true},
		Steps: []domain.AuthStep{
			{From: domain.AuthOnboardingCompleted, To: domain.AuthAwaitingPhone, Cause: domain.CauseUnlink},
		},
	}
	if err := apply(cleared); err != nil {
		t.Fatalf("ApplyLinks(clear): %v", err)
	}
	var nulls int
	if err := f.pool.QueryRow(t.Context(), `SELECT (phone_e164 IS NULL)::int + (phone_hash IS NULL)::int +
		(phone_verified_at IS NULL)::int + (x_user_id IS NULL)::int + (x_username IS NULL)::int +
		(x_linked_at IS NULL)::int FROM users WHERE id = $1`, id.UUID()).Scan(&nulls); err != nil || nulls != 6 {
		t.Fatalf("NULL link columns after clearing = %d, %v, want all 6", nulls, err)
	}
	got, err = f.users.FindByID(t.Context(), f.pool, id)
	sameUser(t, got, err, domain.User{
		ID:            id,
		PrivyUserID:   "did:privy:links",
		AuthState:     domain.AuthAwaitingPhone,
		AccountStatus: domain.AccountActive,
	})
}

func TestUsers_applyLinksRefusesAStepFromAStateTheUserLeft(t *testing.T) {
	t.Parallel()
	f := newUsersFixture(t)
	seeded := testkit.SeedUser(t, f.pool, testkit.UserOpts{})
	err := f.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		return f.users.ApplyLinks(ctx, tx.Queries(), seeded.ID, domain.LinkSync{
			Steps: []domain.AuthStep{
				{From: domain.AuthAwaitingPhone, To: domain.AuthAwaitingSocials, Cause: domain.CauseLink},
			},
		}, f.clock.Now())
	})
	wantCode(t, err, errs.CodeAuthStateTransition)
}

func TestUsers_heldLinksAreTheOnesAnotherUserStores(t *testing.T) {
	t.Parallel()
	f := newUsersFixture(t)
	holder := testkit.SeedUser(t, f.pool, testkit.UserOpts{})
	if err := f.exec(t, `UPDATE users SET phone_hash = $2, x_user_id = 'x-held' WHERE id = $1`, holder.ID.UUID(),
		portHash("+15550199")); err != nil {
		t.Fatal(err)
	}
	me := f.newUserID(t)
	held := func(id ids.UserID, claims domain.Claims, links domain.Links) domain.Claims {
		t.Helper()
		got, err := f.users.HeldLinks(t.Context(), f.pool, id, claims, links)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	both := domain.Links{Phone: "+15550199", X: &domain.XAccount{UserID: "x-held"}}
	free := domain.Links{Phone: "+15550123", X: &domain.XAccount{UserID: "x-free"}}
	for name, tc := range map[string]struct {
		as     ids.UserID
		claims domain.Claims
		links  domain.Links
		want   domain.Claims
	}{
		"both held by the other user": {me, domain.Claims{Phone: true, X: true}, both, domain.Claims{Phone: true, X: true}},
		"only the phone asked":        {me, domain.Claims{Phone: true}, both, domain.Claims{Phone: true}},
		"only X asked":                {me, domain.Claims{X: true}, both, domain.Claims{X: true}},
		"nothing held":                {me, domain.Claims{Phone: true, X: true}, free, domain.Claims{}},
		"nothing asked":               {me, domain.Claims{}, both, domain.Claims{}},
		"the holder itself":           {holder.ID, domain.Claims{Phone: true, X: true}, both, domain.Claims{}},
	} {
		if got := held(tc.as, tc.claims, tc.links); got != tc.want {
			t.Errorf("%s: HeldLinks = %+v, want %+v", name, got, tc.want)
		}
	}
}

func TestUsers_anEmptyXUserIDStoredOnAnotherRowIsNotHeld(t *testing.T) {
	t.Parallel()
	f := newUsersFixture(t)
	holder := testkit.SeedUser(t, f.pool, testkit.UserOpts{})
	if err := sqlc.New(f.pool).SetUserX(t.Context(), sqlc.SetUserXParams{
		XUserID: pgtype.Text{String: "", Valid: true},
		Now:     f.clock.Now(),
		ID:      holder.ID.UUID(),
	}); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := f.pool.QueryRow(t.Context(), `SELECT x_user_id FROM users WHERE id = $1 AND x_user_id IS NOT NULL`,
		holder.ID.UUID()).Scan(&stored); err != nil || stored != "" {
		t.Fatalf("stored x_user_id = %q, %v, want an empty string", stored, err)
	}
	me := f.newUserID(t)
	got, err := f.users.HeldLinks(t.Context(), f.pool, me, domain.Claims{}, domain.Links{})
	if err != nil || got.X {
		t.Fatalf("HeldLinks = %+v, %v, want X false", got, err)
	}
}
