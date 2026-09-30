package identity_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/metric/noop"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/privyfake"
)

const (
	alice = app.PrivyUserID("did:privy:alice")
	bob   = app.PrivyUserID("did:privy:bob")
)

func alicePhone() app.PrivyUser { return app.PrivyUser{ID: alice, PhoneE164: "+14155550100"} }

type recordedHints struct {
	mu   sync.Mutex
	keys []string
}

func (h *recordedHints) PublishHint(_ context.Context, key string, _ []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.keys = append(h.keys, key)
}

func (h *recordedHints) sent() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.keys...)
}

type sessionFixture struct {
	pool    *pgxpool.Pool
	clock   *testkit.Clock
	privy   *privyfake.Users
	wallets *privyfake.Wallets
	hints   *recordedHints
	handler *app.OpenSessionHandler
}

func newSessionFixture(t *testing.T) *sessionFixture {
	t.Helper()
	return newSessionFixtureWith(t, func(u adapters.Users) app.SessionUsers { return u })
}

func newSessionFixtureWith(t *testing.T, wrap func(adapters.Users) app.SessionUsers) *sessionFixture {
	t.Helper()
	pool := testkit.DB(t)
	g := testkit.NewIDs(testkit.RandSeed(t))
	clk := testkit.NewClock(clock.Real{}.Now().UTC().Truncate(time.Microsecond))
	f := &sessionFixture{
		pool: pool, clock: clk, privy: &privyfake.Users{}, wallets: &privyfake.Wallets{}, hints: &recordedHints{},
	}
	rule, err := app.NewWalletRule(f.wallets, noop.NewMeterProvider())
	if err != nil {
		t.Fatal(err)
	}
	f.handler = app.NewOpenSessionHandler(app.OpenSessionDeps{
		UoW: db.New(pool, g, clock.Real{}), Reads: pool, Users: wrap(adapters.Users{}), Privy: f.privy, Wallets: rule,
		IDs: g, Clock: clk, Hints: f.hints,
	})
	return f
}

func (f *sessionFixture) open(t *testing.T, id app.PrivyUserID) (app.Me, error) {
	t.Helper()
	return f.handler.Handle(t.Context(), app.OpenSession{Token: string(id)})
}

type tally struct {
	Users, Wallets, UserCreated, Hints, PrivyCreates int
}

func (f *sessionFixture) tally(ctx context.Context, t *testing.T) tally {
	t.Helper()
	counts := map[string]int{}
	for _, table := range []string{"users", "user_wallets", "events"} {
		var n int
		if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		counts[table] = n
	}
	return tally{
		Users: counts["users"], Wallets: counts["user_wallets"], UserCreated: counts["events"],
		Hints: len(f.hints.sent()), PrivyCreates: f.wallets.Creates(),
	}
}

func (f *sessionFixture) stored(t *testing.T, column string) map[string]string {
	t.Helper()
	rows, err := f.pool.Query(t.Context(), `SELECT privy_user_id, coalesce(`+column+`, '<null>') FROM users`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, value string
		if err := rows.Scan(&id, &value); err != nil {
			t.Fatal(err)
		}
		out[id] = value
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestOpenSession_aFirstSignInCreatesTheUserItsWalletAndOneUserCreated(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	f.privy.Seed(alicePhone())
	me, err := f.open(t, alice)
	if err != nil {
		t.Fatal(err)
	}
	wallet, _ := f.wallets.FindOrCreate(t.Context(), alice)
	if me.AuthState != domain.AuthCreated || me.AccountStatus != domain.AccountActive ||
		me.MemberWalletAddress != wallet.Address || me.PhoneLinked || me.Handle != "" ||
		!me.CreatedAt.Equal(f.clock.Now()) {
		t.Fatalf("Me = %+v, want a CREATED active user with the Privy wallet and no phone stored", me)
	}
	if got := f.tally(t.Context(), t); got != (tally{1, 1, 1, 1, 1}) {
		t.Fatalf("rows = %+v, want one user, wallet, user.created, hint and Privy create", got)
	}
	if hint := "user." + me.ID.String() + ".me_changed"; f.hints.sent()[0] != hint {
		t.Fatalf("hints = %v, want %s", f.hints.sent(), hint)
	}
	f.expectUserCreated(t, me, "sms")
}

func (f *sessionFixture) expectUserCreated(t *testing.T, me app.Me, provider string) {
	t.Helper()
	var actorType, actorID string
	var payload []byte
	if err := f.pool.QueryRow(t.Context(), `SELECT actor_type, actor_id, payload FROM events WHERE type = $1`,
		string(events.TypeUserCreated)).Scan(&actorType, &actorID, &payload); err != nil {
		t.Fatal(err)
	}
	got, err := events.Decode(events.TypeUserCreated, 1, payload)
	want := events.UserCreated{V: 1, UserID: me.ID.UUID(), LoginProvider: provider, CreatedAt: f.clock.Now()}
	if err != nil || got != want || actorType != "user" || actorID != me.ID.String() {
		t.Fatalf("user.created = %+v by %s:%s, %v, want %+v by the new user", got, actorType, actorID, err, want)
	}
}

func TestOpenSession_aReturningUserGetsTheSameAccountWithNoNewEventOrHint(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	f.privy.Seed(alicePhone())
	first, err := f.open(t, alice)
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.open(t, alice)
	if err != nil || second != first {
		t.Fatalf("second sign-in = %+v, %v, want %+v", second, err, first)
	}
	if got := f.tally(t.Context(), t); got != (tally{1, 1, 1, 1, 1}) {
		t.Fatalf("rows after two sign-ins = %+v, want the first sign-in's only", got)
	}
}

func TestOpenSession_aPrivyUserWhoAlreadyHasAWalletSignsInWithoutCreatingOne(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	f.privy.Seed(alicePhone())
	held := app.PrivyWallet{
		Wallet: domain.Wallet{PrivyWalletID: "wallet-held", Address: address(t, 3)}, HasAppSigner: true,
	}
	f.wallets.Seed(alice, held)
	me, err := f.open(t, alice)
	if err != nil || me.MemberWalletAddress != held.Address || f.wallets.Creates() != 0 {
		t.Fatalf("Me = %+v, %v after %d creates, want the held wallet and no create", me, err, f.wallets.Creates())
	}
}

func TestOpenSession_aStoredWalletNeedsNoPrivyCall(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	f.privy.Seed(alicePhone())
	if _, err := f.open(t, alice); err != nil {
		t.Fatal(err)
	}
	f.wallets.Fail("FindOrCreate", errs.New(errs.CodePrivyUnavailable, "test"))
	if _, err := f.open(t, alice); err != nil {
		t.Fatalf("sign-in with the wallet stored and Privy's wallet API down = %v, want success", err)
	}
}

func TestOpenSession_theLoginProviderFollowsWhatIsLinked(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	want := map[string]string{
		"did:privy:phone": "sms", "did:privy:email": "email", "did:privy:apple": "apple", "did:privy:google": "google",
	}
	for _, u := range []app.PrivyUser{
		{ID: "did:privy:phone", PhoneE164: "+14155550100", Email: "a@example.com"},
		{ID: "did:privy:email", Email: "b@example.com", X: &domain.XAccount{UserID: "1", Username: "b"}},
		{ID: "did:privy:apple", AppleEmail: "c@privaterelay.appleid.com"},
		{ID: "did:privy:google", GoogleEmail: "d@gmail.com"},
	} {
		f.privy.Seed(u)
		if _, err := f.open(t, u.ID); err != nil {
			t.Fatalf("%s: %v", u.ID, err)
		}
	}
	got := f.stored(t, "login_provider")
	for id, provider := range want {
		if got[id] != provider {
			t.Errorf("login_provider of %s = %q, want %q", id, got[id], provider)
		}
	}
}

func TestOpenSession_theEmailFollowsPrivyOnEverySignIn(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	for _, tc := range []struct{ linked, want string }{
		{"first@example.com", "first@example.com"},
		{"second@example.com", "second@example.com"},
		{"", "<null>"},
	} {
		f.privy.Seed(app.PrivyUser{ID: alice, PhoneE164: "+14155550100", Email: tc.linked})
		if _, err := f.open(t, alice); err != nil {
			t.Fatal(err)
		}
		if got := f.stored(t, "email")[string(alice)]; got != tc.want {
			t.Fatalf("email after Privy linked %q = %q, want %q", tc.linked, got, tc.want)
		}
	}
	if n := len(f.hints.sent()); n != 1 {
		t.Fatalf("hints = %d, want only the one for the creation, since Me carries no email", n)
	}
}

func TestOpenSession_refusesWhatItCannotSignIn(t *testing.T) {
	t.Parallel()
	down := errs.New(errs.CodePrivyUnavailable, "test")
	gone := errs.New(errs.CodeNotFound, "test")
	for name, tc := range map[string]struct {
		setup func(f *sessionFixture)
		token app.PrivyUserID
		want  errs.Code
	}{
		"unknown token":         {func(*sessionFixture) {}, "did:privy:nobody", errs.CodeUnauthorized},
		"privy cannot verify":   {func(f *sessionFixture) { f.privy.Fail("Verify", down) }, alice, errs.CodePrivyUnavailable},
		"privy down":            {func(f *sessionFixture) { f.privy.Fail("User", down) }, alice, errs.CodePrivyUnavailable},
		"wallet api down":       {func(f *sessionFixture) { f.wallets.Fail("FindOrCreate", down) }, alice, errs.CodePrivyUnavailable},
		"privy forgot the user": {func(f *sessionFixture) { f.privy.FailOnce("User", gone) }, alice, errs.CodeUnauthorized},
		"no login method": {func(f *sessionFixture) {
			f.privy.Seed(app.PrivyUser{ID: alice, X: &domain.XAccount{UserID: "1", Username: "a"}})
		}, alice, errs.CodeLoginMethodNotAllowed},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newSessionFixture(t)
			f.privy.Seed(alicePhone())
			tc.setup(f)
			if _, err := f.open(t, tc.token); errs.CodeOf(err) != tc.want {
				t.Fatalf("sign-in = %v, want %s", err, tc.want)
			}
			if got := f.tally(t.Context(), t); got != (tally{}) {
				t.Fatalf("rows = %+v, want nothing written and no wallet created", got)
			}
		})
	}
}

func TestOpenSession_aDeletedAccountIsRefusedWithoutANewRowOrWallet(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	f.privy.Seed(alicePhone())
	me, err := f.open(t, alice)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(t.Context(),
		`UPDATE users SET account_status = 'deleted', deleted_at = now() WHERE id = $1`, me.ID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(t.Context(), `DELETE FROM user_wallets`); err != nil {
		t.Fatal(err)
	}
	_, err = f.open(t, alice)
	wantCode(t, err, errs.CodeAccountDeleted)
	if got := f.tally(t.Context(), t); got != (tally{Users: 1, UserCreated: 1, Hints: 1, PrivyCreates: 1}) {
		t.Fatalf("after the refused sign-in: %+v, want the row kept, no wallet and nothing new", got)
	}
}

func TestOpenSession_suspendedAndBannedUsersGetTheirAccountToo(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	f.privy.Seed(alicePhone())
	f.privy.Seed(app.PrivyUser{ID: bob, Email: "bob@example.com"})
	for id, status := range map[app.PrivyUserID]domain.AccountStatus{
		alice: domain.AccountSuspended, bob: domain.AccountBanned,
	} {
		me, err := f.open(t, id)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(t.Context(), `UPDATE users SET account_status = $2 WHERE id = $1`,
			me.ID.UUID(), string(status)); err != nil {
			t.Fatal(err)
		}
		if got, err := f.open(t, id); err != nil || got.AccountStatus != status || got.ID != me.ID {
			t.Fatalf("sign-in of a %s user = %+v, %v, want the account with its status", status, got, err)
		}
	}
}

func TestOpenSession_tenConcurrentFirstSignInsMakeOneUserOneWalletAndOneEvent(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	f.privy.Seed(alicePhone())
	const callers = 10
	results := make([]app.Me, callers)
	failures := make([]error, callers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range callers {
		wg.Go(func() {
			<-start
			results[i], failures[i] = f.open(t, alice)
		})
	}
	close(start)
	wg.Wait()
	for i := range callers {
		if failures[i] != nil || results[i] != results[0] {
			t.Fatalf("caller %d: %+v, %v, want %+v", i, results[i], failures[i], results[0])
		}
	}
	if got := f.tally(t.Context(), t); got != (tally{1, 1, 1, 1, 1}) {
		t.Fatalf("rows = %+v, want one user, wallet, user.created, hint and Privy create", got)
	}
}

type racingUsers struct {
	adapters.Users
	afterLookup func(ctx context.Context)
}

func (u racingUsers) FindByPrivyUserID(ctx context.Context, q sqlc.DBTX, id string) (domain.User, error) {
	got, err := u.Users.FindByPrivyUserID(ctx, q, id)
	u.afterLookup(ctx)
	return got, err
}

func TestOpenSession_aDeletionBetweenTheLookupAndTheTransactionIsStillRefused(t *testing.T) {
	t.Parallel()
	var f *sessionFixture
	f = newSessionFixtureWith(t, func(u adapters.Users) app.SessionUsers {
		return racingUsers{Users: u, afterLookup: func(ctx context.Context) {
			if _, err := f.pool.Exec(
				ctx,
				`UPDATE users SET account_status = 'deleted', deleted_at = now()`,
			); err != nil {
				t.Error(err)
			}
		}}
	})
	f.privy.Seed(alicePhone())
	if _, err := f.open(t, alice); err != nil {
		t.Fatal(err)
	}
	_, err := f.open(t, alice)
	wantCode(t, err, errs.CodeAccountDeleted)
	if got := f.tally(t.Context(), t); got != (tally{1, 1, 1, 1, 1}) {
		t.Fatalf("rows = %+v, want the first sign-in's only", got)
	}
}

func TestOpenSession_aWalletThatDiffersFromTheStoredOneIsAWalletMismatch(t *testing.T) {
	t.Parallel()
	var f *sessionFixture
	f = newSessionFixtureWith(t, func(u adapters.Users) app.SessionUsers {
		return racingUsers{Users: u, afterLookup: func(ctx context.Context) {
			_, err := f.pool.Exec(ctx, `INSERT INTO user_wallets (user_id, privy_wallet_id, address, created_at)
				SELECT id, 'wallet-other', $1, now() FROM users`, string(address(t, 5)))
			if err != nil {
				t.Error(err)
			}
		}}
	})
	f.privy.Seed(alicePhone())
	f.wallets.Seed(alice, app.PrivyWallet{Wallet: domain.Wallet{PrivyWalletID: "wallet-privy", Address: address(t, 6)}})
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO users (id, privy_user_id, login_provider, auth_state_changed_at,
		created_at, updated_at) VALUES ($1, $2, 'sms', now(), now(), now())`,
		testkit.NewIDs(1).NewV7(), string(alice)); err != nil {
		t.Fatal(err)
	}
	_, err := f.open(t, alice)
	wantCode(t, err, errs.CodeWalletMismatch)
	var stored string
	if err := f.pool.QueryRow(t.Context(), `SELECT address FROM user_wallets`).Scan(&stored); err != nil ||
		stored != string(address(t, 5)) {
		t.Fatalf("stored wallet = %q, %v, want the first one kept", stored, err)
	}
}

func TestOpenSession_anAddressAnotherUserHoldsIsAWalletMismatchAndCreatesNoUser(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	f.privy.Seed(alicePhone())
	other := testkit.SeedUser(t, f.pool, testkit.UserOpts{WithWallet: true})
	f.wallets.Seed(alice, app.PrivyWallet{Wallet: domain.Wallet{PrivyWalletID: "wallet-dup", Address: other.Address}})
	_, err := f.open(t, alice)
	wantCode(t, err, errs.CodeWalletMismatch)
	if got := f.tally(t.Context(), t); got != (tally{Users: 1, Wallets: 1}) {
		t.Fatalf("after the mismatch: %+v, want only the seeded user and its wallet", got)
	}
}

type failingUsers struct {
	adapters.Users
	failing string
}

func (u failingUsers) fails(op string) error {
	if op != u.failing {
		return nil
	}
	return errs.New(errs.CodeDBUnavailable, "test."+op)
}

func (u failingUsers) FindByPrivyUserID(ctx context.Context, q sqlc.DBTX, id string) (domain.User, error) {
	if err := u.fails("FindByPrivyUserID"); err != nil {
		return domain.User{}, err
	}
	return u.Users.FindByPrivyUserID(ctx, q, id)
}

func (u failingUsers) Lock(ctx context.Context, q sqlc.DBTX, id string) (domain.User, error) {
	if err := u.fails("Lock"); err != nil {
		return domain.User{}, err
	}
	return u.Users.Lock(ctx, q, id)
}

func (u failingUsers) Create(ctx context.Context, q sqlc.DBTX, n domain.NewUser, at time.Time) (bool, error) {
	if err := u.fails("Create"); err != nil {
		return false, err
	}
	return u.Users.Create(ctx, q, n, at)
}

func (u failingUsers) AttachWallet(
	ctx context.Context, q sqlc.DBTX, id ids.UserID, w domain.Wallet, at time.Time,
) (bool, error) {
	if err := u.fails("AttachWallet"); err != nil {
		return false, err
	}
	return u.Users.AttachWallet(ctx, q, id, w, at)
}

func (u failingUsers) RefreshEmail(ctx context.Context, q sqlc.DBTX, id ids.UserID, email string, at time.Time) error {
	if err := u.fails("RefreshEmail"); err != nil {
		return err
	}
	return u.Users.RefreshEmail(ctx, q, id, email, at)
}

func TestOpenSession_aFailureAtAnyStepRollsEverythingBack(t *testing.T) {
	t.Parallel()
	for _, step := range []string{"FindByPrivyUserID", "Create", "Lock", "AttachWallet", "RefreshEmail"} {
		t.Run(step, func(t *testing.T) {
			t.Parallel()
			f := newSessionFixtureWith(t, func(u adapters.Users) app.SessionUsers {
				return failingUsers{Users: u, failing: step}
			})
			f.privy.Seed(alicePhone())
			if _, err := f.open(t, alice); errs.CodeOf(err) != errs.CodeDBUnavailable {
				t.Fatalf("sign-in = %v, want db_unavailable", err)
			}
			want := tally{PrivyCreates: 1}
			if step == "FindByPrivyUserID" {
				want = tally{}
			}
			if got := f.tally(t.Context(), t); got != want {
				t.Fatalf(
					"rows = %+v, want %+v: nothing written, and the Privy wallet only once it was asked for",
					got,
					want,
				)
			}
		})
	}
}

func TestOpenSession_anEventThatCannotBeAppendedRollsTheUserBack(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	f.privy.Seed(alicePhone())
	if _, err := f.pool.Exec(t.Context(), `ALTER TABLE events RENAME TO events_gone`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.open(t, alice); err == nil {
		t.Fatal("sign-in succeeded without being able to append user.created")
	}
	var users int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM users`).Scan(&users); err != nil || users != 0 {
		t.Fatalf("%d users after the failed append, %v, want none", users, err)
	}
}
