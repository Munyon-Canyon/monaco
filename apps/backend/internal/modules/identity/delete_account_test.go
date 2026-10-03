package identity_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type deleteFixture struct {
	pool     *pgxpool.Pool
	clock    *testkit.Clock
	treasury *fakes.Treasury
	balances *fakes.Balances
	hints    *recordedHints
}

func newDeleteFixture(t *testing.T) deleteFixture {
	t.Helper()
	return deleteFixture{
		pool: testkit.DB(t), clock: testkit.NewClock(clock.Real{}.Now().UTC().Truncate(time.Microsecond)),
		treasury: fakes.NewTreasury(), balances: fakes.NewBalances(), hints: &recordedHints{},
	}
}

func (f deleteFixture) handler(users app.DeletingUsers) *app.DeleteAccount {
	return app.NewDeleteAccount(app.DeleteAccountDeps{
		UoW: db.New(f.pool, testkit.NewIDs(642), f.clock), Users: users, Balances: f.balances, Stakes: f.treasury,
		Clock: f.clock, Hints: f.hints,
	})
}

func (f deleteFixture) seedWithPII(t *testing.T, status string) testkit.SeededUser {
	t.Helper()
	u := testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "gone_soon", AccountStatus: status, WithWallet: true})
	if _, err := f.pool.Exec(t.Context(), `UPDATE users SET email = 'a@b.co', phone_e164 = '+15555550100',
		phone_hash = decode(repeat('ab', 32), 'hex'), phone_verified_at = now(), x_user_id = 'x-1',
		x_username = 'gone', x_linked_at = now(), display_name = 'Gone Soon', photo_url = 'https://img/p.png'
		WHERE id = $1`, u.ID.UUID()); err != nil {
		t.Fatal(err)
	}
	return u
}

type scrubbed struct {
	Status, Handle, PrivyUserID, DisplayName string
	Deleted, Email, Phone, PhoneHash         bool
	PhoneVerified, XUserID, XUsername, XAt   bool
	Photo, Wallet                            bool
}

func (f deleteFixture) row(t *testing.T, id ids.UserID) scrubbed {
	t.Helper()
	var s scrubbed
	if err := f.pool.QueryRow(t.Context(), `SELECT account_status, handle, privy_user_id, display_name,
		deleted_at IS NOT NULL, email IS NOT NULL, phone_e164 IS NOT NULL, phone_hash IS NOT NULL,
		phone_verified_at IS NOT NULL, x_user_id IS NOT NULL, x_username IS NOT NULL, x_linked_at IS NOT NULL,
		photo_url IS NOT NULL, EXISTS (SELECT 1 FROM user_wallets w WHERE w.user_id = users.id)
		FROM users WHERE id = $1`, id.UUID()).Scan(&s.Status, &s.Handle, &s.PrivyUserID, &s.DisplayName,
		&s.Deleted, &s.Email, &s.Phone, &s.PhoneHash, &s.PhoneVerified, &s.XUserID, &s.XUsername, &s.XAt,
		&s.Photo, &s.Wallet); err != nil {
		t.Fatal(err)
	}
	return s
}

func (f deleteFixture) deletedEvents(t *testing.T) []events.UserDeleted {
	t.Helper()
	rows, err := f.pool.Query(t.Context(), `SELECT payload FROM events WHERE type = $1`, string(events.TypeUserDeleted))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []events.UserDeleted
	for rows.Next() {
		var raw []byte
		var ev events.UserDeleted
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &ev); err != nil {
			t.Fatal(err)
		}
		out = append(out, ev)
	}
	return out
}

func asUser(t *testing.T, id ids.UserID) context.Context {
	t.Helper()
	return auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: id.String()})
}

func untouched(handle, privyUserID string) scrubbed {
	return scrubbed{
		Status: "active", Handle: handle, PrivyUserID: privyUserID, DisplayName: "Gone Soon", Email: true, Phone: true,
		PhoneHash: true, PhoneVerified: true, XUserID: true, XUsername: true, XAt: true, Photo: true, Wallet: true,
	}
}

func TestDeleteAccount_scrubsPIIKeepsHandleAndWalletAndAppendsUserDeleted(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"active", "suspended", "banned"} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			f := newDeleteFixture(t)
			u := f.seedWithPII(t, status)
			if err := f.handler(adapters.Users{}).Handle(asUser(t, u.ID), u.ID); err != nil {
				t.Fatalf("DeleteAccount: %v", err)
			}
			want := scrubbed{
				Status: "deleted", Handle: "gone_soon", PrivyUserID: u.PrivyUserID, Deleted: true, Wallet: true,
			}
			if got := f.row(t, u.ID); got != want {
				t.Fatalf("row after delete = %+v, want %+v", got, want)
			}
			got := f.deletedEvents(t)
			if len(got) != 1 || got[0] != (events.UserDeleted{V: 1, UserID: u.ID.UUID(), At: f.clock.Now()}) {
				t.Fatalf("user.deleted events = %+v, want one for %s at %s", got, u.ID, f.clock.Now())
			}
			if keys := f.hints.sent(); !slices.Equal(keys, []string{"user." + u.ID.String() + ".me_changed"}) {
				t.Fatalf("hints = %q, want the caller's me_changed", keys)
			}
		})
	}
}

func (f deleteFixture) expectUntouched(t *testing.T, u testkit.SeededUser) {
	t.Helper()
	if got := f.row(t, u.ID); got != untouched("gone_soon", u.PrivyUserID) {
		t.Fatalf("row = %+v, want it untouched", got)
	}
	if got := f.deletedEvents(t); len(got) != 0 {
		t.Fatalf("user.deleted events = %+v, want none", got)
	}
	if keys := f.hints.sent(); len(keys) != 0 {
		t.Fatalf("hints = %q, want none without a commit", keys)
	}
}

func (f deleteFixture) stakeIn(t *testing.T, user ids.UserID, cabals ...string) {
	t.Helper()
	for _, raw := range cabals {
		cabal, err := ids.ParseCabalID(raw)
		if err != nil {
			t.Fatal(err)
		}
		f.treasury.SetStake(treasury.Stake{CabalID: cabal, UserID: user, ShareUnits: money.SharesUnitsFromUint64(1)})
	}
}

func TestDeleteAccount_aStakeInAnyCabalRefusesWithTheCabalCount(t *testing.T) {
	t.Parallel()
	f := newDeleteFixture(t)
	u := f.seedWithPII(t, "active")
	f.stakeIn(t, u.ID, "01890a5d-ac96-774b-bcce-b302099a8001", "01890a5d-ac96-774b-bcce-b302099a8002")
	err := f.handler(adapters.Users{}).Handle(asUser(t, u.ID), u.ID)
	if errs.CodeOf(err) != errs.CodeAccountHasPositions {
		t.Fatalf("DeleteAccount = %v, want AccountHasPositions", err)
	}
	if detail := errs.Detail(err); !slices.ContainsFunc(detail, func(a slog.Attr) bool {
		return a.Equal(slog.Int("cabal_count", 2))
	}) {
		t.Fatalf("detail = %v, want cabal_count=2", detail)
	}
	f.expectUntouched(t, u)
}

func TestDeleteAccount_refusesWhileMoneyIsLeftOrItCannotTell(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		given func(f deleteFixture, user ids.UserID)
		code  errs.Code
	}{
		{"one micro of platform balance", func(f deleteFixture, user ids.UserID) {
			f.balances.Set(user, funding.Balance{AvailableMicros: money.MicrosFromUint64(1)})
		}, errs.CodeAccountHasBalance},
		{"treasury down", func(f deleteFixture, _ ids.UserID) {
			f.treasury.Fail("StakesOf", errs.New(errs.CodeUpstreamUnavailable, "treasury.fake"))
		}, errs.CodeUpstreamUnavailable},
		{"balance down", func(f deleteFixture, _ ids.UserID) {
			f.balances.Fail("Available", errs.New(errs.CodeRPCUnavailable, "funding.fake"))
		}, errs.CodeRPCUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newDeleteFixture(t)
			u := f.seedWithPII(t, "active")
			tc.given(f, u.ID)
			if err := f.handler(adapters.Users{}).Handle(asUser(t, u.ID), u.ID); errs.CodeOf(err) != tc.code {
				t.Fatalf("DeleteAccount = %v, want %s", err, tc.code)
			}
			f.expectUntouched(t, u)
		})
	}
}

type deletingUsers struct {
	adapters.Users
	status    domain.AccountStatus
	deleteErr error
}

func (u deletingUsers) LockByID(ctx context.Context, q sqlc.DBTX, id ids.UserID) (domain.User, error) {
	user, err := u.Users.LockByID(ctx, q, id)
	if u.status != "" {
		user.AccountStatus = u.status
	}
	return user, err
}

func (u deletingUsers) Delete(
	ctx context.Context, q sqlc.DBTX, id ids.UserID, expected domain.AccountStatus, at time.Time,
) error {
	if u.deleteErr != nil {
		return u.deleteErr
	}
	return u.Users.Delete(ctx, q, id, expected, at)
}

func TestDeleteAccount_writesNothingWhenTheTransactionFails(t *testing.T) {
	t.Parallel()
	scrubFailed := errs.New(errs.CodeDBUnavailable, "identity.test")
	for _, tc := range []struct {
		name  string
		users app.DeletingUsers
		setup string
		code  errs.Code
	}{
		{
			"an account already marked deleted",
			deletingUsers{status: domain.AccountDeleted},
			"",
			errs.CodeAccountStatusTransition,
		},
		{"the scrub fails", deletingUsers{deleteErr: scrubFailed}, "", errs.CodeDBUnavailable},
		{
			"the status moved under the lock",
			deletingUsers{status: domain.AccountSuspended},
			"",
			errs.CodeAccountStatusTransition,
		},
		{
			"user.deleted cannot be appended",
			adapters.Users{},
			`ALTER TABLE events RENAME TO events_gone`,
			errs.CodeInternal,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newDeleteFixture(t)
			u := f.seedWithPII(t, "active")
			f.exec(t, tc.setup)
			if err := f.handler(tc.users).Handle(asUser(t, u.ID), u.ID); errs.CodeOf(err) != tc.code {
				t.Fatalf("DeleteAccount = %v, want %s", err, tc.code)
			}
			if tc.setup != "" {
				f.exec(t, `ALTER TABLE events_gone RENAME TO events`)
			}
			f.expectUntouched(t, u)
		})
	}
}

func (f deleteFixture) exec(t *testing.T, sql string) {
	t.Helper()
	if sql == "" {
		return
	}
	if _, err := f.pool.Exec(t.Context(), sql); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteAccount_aDeletedAccountIsNotFoundTheSecondTime(t *testing.T) {
	t.Parallel()
	f := newDeleteFixture(t)
	u := f.seedWithPII(t, "active")
	if err := f.handler(adapters.Users{}).Handle(asUser(t, u.ID), u.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.handler(adapters.Users{}).Handle(asUser(t, u.ID), u.ID); errs.CodeOf(err) != errs.CodeUserNotFound {
		t.Fatalf("second DeleteAccount = %v, want UserNotFound", err)
	}
	if got := f.deletedEvents(t); len(got) != 1 {
		t.Fatalf("user.deleted events = %d, want 1", len(got))
	}
}
