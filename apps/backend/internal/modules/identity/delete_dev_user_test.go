package identity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/privyfake"
)

type devDeleteFixture struct {
	pool     *pgxpool.Pool
	made     app.DevUser
	privy    *privyfake.Users
	privyID  app.PrivyUserID
	balances *stubBalances
	deps     app.DeleteDevUserDeps
}

type stubBalances struct {
	usdc, lamports uint64
	err            error
	asked          chain.SolanaAddress
}

func (s *stubBalances) Balances(_ context.Context, address chain.SolanaAddress) (uint64, uint64, error) {
	s.asked = address
	return s.usdc, s.lamports, s.err
}

type countingPrivy struct {
	app.PrivyDevUsers
	deletes int
	err     error
}

func (c *countingPrivy) Delete(ctx context.Context, id app.PrivyUserID) error {
	c.deletes++
	if c.err != nil {
		return c.err
	}
	return c.PrivyDevUsers.Delete(ctx, id)
}

func newDevDeleteFixture(t *testing.T) (devDeleteFixture, *countingPrivy) {
	t.Helper()
	pool := testkit.DB(t)
	clk := testkit.NewClock(clock.Real{}.Now().UTC().Truncate(time.Microsecond))
	privy := &privyfake.Users{}
	made, err := app.CreateDevUser(
		t.Context(),
		devDeps(pool, clk, privy, &privyfake.Wallets{}, &recordedHints{}, devRand()),
	)
	if err != nil {
		t.Fatal(err)
	}
	counting := &countingPrivy{PrivyDevUsers: privy}
	balances := &stubBalances{}
	f := devDeleteFixture{
		pool: pool, made: made, privy: privy, privyID: "did:privy:fake-1", balances: balances,
		deps: app.DeleteDevUserDeps{
			UoW: db.New(pool, ids.Real{}, clk), Users: adapters.Users{}, Privy: counting, Balances: balances,
		},
	}
	return f, counting
}

func (f devDeleteFixture) count(t *testing.T, table string) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM `+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (f devDeleteFixture) privyHasUser() bool {
	_, err := f.privy.User(context.Background(), f.privyID)
	return err == nil
}

func (f devDeleteFixture) linkDevX(t *testing.T) {
	t.Helper()
	_, err := f.pool.Exec(t.Context(),
		`INSERT INTO dev_x_links (user_id, x_user_id, x_username, created_at) VALUES ($1, 'dev:1', 'dev_x_1', now())`,
		f.made.UserID.UUID())
	if err != nil {
		t.Fatal(err)
	}
}

func TestDeleteDevUser_removesTheRowsAndThePrivyUser(t *testing.T) {
	t.Parallel()
	f, privy := newDevDeleteFixture(t)
	f.privy.Seed(app.PrivyUser{ID: f.privyID, Email: "dev-0a1b2c3d@example.com"})
	f.linkDevX(t)
	if err := app.DeleteDevUser(t.Context(), f.deps, f.made.UserID); err != nil {
		t.Fatal(err)
	}
	if n := f.count(t, "users") + f.count(t, "user_wallets") + f.count(t, "dev_x_links"); n != 0 || f.privyHasUser() ||
		privy.deletes != 1 || f.balances.asked != f.made.WalletAddress {
		t.Fatalf("%d rows left, privy user %v, %d deletes, balances asked about %q", n, f.privyHasUser(),
			privy.deletes, f.balances.asked)
	}
}

func TestDeleteDevUser_refusesAUserThatIsNotAThrowaway(t *testing.T) {
	t.Parallel()
	f, privy := newDevDeleteFixture(t)
	if _, err := f.pool.Exec(t.Context(), `UPDATE users SET handle = 'qa_actor_b'`); err != nil {
		t.Fatal(err)
	}
	err := app.DeleteDevUser(t.Context(), f.deps, f.made.UserID)
	if errs.CodeOf(err) != errs.CodeInvalidInput || privy.deletes != 0 || f.count(t, "users") != 1 ||
		!f.privyHasUser() {
		t.Fatalf("qa_ handle: %v, %d Privy deletes, %d users", err, privy.deletes, f.count(t, "users"))
	}
}

func TestDeleteDevUser_refusesAPrivyUserWithMoreThanADevEmail(t *testing.T) {
	t.Parallel()
	f, privy := newDevDeleteFixture(t)
	for name, user := range map[string]app.PrivyUser{
		"phone":  {ID: f.privyID, Email: "dev-0a1b2c3d@example.com", PhoneE164: "+14155550100"},
		"real":   {ID: f.privyID, Email: "person@gmail.com"},
		"google": {ID: f.privyID, Email: "dev-0a1b2c3d@example.com", GoogleEmail: "a@gmail.com"},
	} {
		f.privy.Seed(user)
		err := app.DeleteDevUser(t.Context(), f.deps, f.made.UserID)
		if errs.CodeOf(err) != errs.CodeInvalidInput || privy.deletes != 0 || f.count(t, "users") != 1 {
			t.Errorf("%s: %v, %d Privy deletes, %d users", name, err, privy.deletes, f.count(t, "users"))
		}
	}
}

func TestDeleteDevUser_refusesAFundedWalletButNotDust(t *testing.T) {
	t.Parallel()
	f, privy := newDevDeleteFixture(t)
	f.privy.Seed(app.PrivyUser{ID: f.privyID, Email: "dev-0a1b2c3d@example.com"})
	for name, funds := range map[string]stubBalances{
		"usdc": {usdc: 1}, "sol": {lamports: app.DustLamports + 1},
	} {
		*f.balances = funds
		err := app.DeleteDevUser(t.Context(), f.deps, f.made.UserID)
		var funded *app.FundedError
		if !errors.As(err, &funded) || funded.Address != string(f.made.WalletAddress) || privy.deletes != 0 ||
			f.count(t, "users") != 1 {
			t.Errorf("%s: %v, %d Privy deletes", name, err, privy.deletes)
		}
		if funded != nil && funded.Error() == "" {
			t.Errorf("%s: empty message", name)
		}
	}
	*f.balances = stubBalances{lamports: app.DustLamports}
	if err := app.DeleteDevUser(t.Context(), f.deps, f.made.UserID); err != nil {
		t.Fatalf("dust only = %v, want deleted", err)
	}
}

func TestDeleteDevUser_aRowItCannotDeleteLeavesPrivyAlone(t *testing.T) {
	t.Parallel()
	f, privy := newDevDeleteFixture(t)
	f.privy.Seed(app.PrivyUser{ID: f.privyID, Email: "dev-0a1b2c3d@example.com"})
	f.linkDevX(t)
	_, err := f.pool.Exec(t.Context(),
		`INSERT INTO device_tokens (id, user_id, token, environment, created_at, last_seen_at)
		 VALUES ($1, $2, 'tok', 'sandbox', now(), now())`, ids.NewUserID(ids.Real{}).UUID(), f.made.UserID.UUID())
	if err != nil {
		t.Fatal(err)
	}
	if err := app.DeleteDevUser(t.Context(), f.deps, f.made.UserID); err == nil {
		t.Fatal("expected the foreign key to stop the delete")
	}
	if privy.deletes != 0 || !f.privyHasUser() || f.count(t, "users") != 1 || f.count(t, "dev_x_links") != 1 {
		t.Fatalf("%d Privy deletes, privy user %v, %d users, %d x links", privy.deletes, f.privyHasUser(),
			f.count(t, "users"), f.count(t, "dev_x_links"))
	}
}

func TestDeleteDevUser_aFailedPrivyDeleteRollsTheRowsBack(t *testing.T) {
	t.Parallel()
	f, privy := newDevDeleteFixture(t)
	f.privy.Seed(app.PrivyUser{ID: f.privyID, Email: "dev-0a1b2c3d@example.com"})
	privy.err = errs.New(errs.CodePrivyUnavailable, "test.privy")
	err := app.DeleteDevUser(t.Context(), f.deps, f.made.UserID)
	if errs.CodeOf(err) != errs.CodePrivyUnavailable || f.count(t, "users") != 1 || f.count(t, "user_wallets") != 1 {
		t.Fatalf("err %v, %d users, %d wallets", err, f.count(t, "users"), f.count(t, "user_wallets"))
	}
}

func TestDeleteDevUser_finishesWhenPrivyAlreadyForgotTheUser(t *testing.T) {
	t.Parallel()
	f, privy := newDevDeleteFixture(t)
	if err := f.privy.Delete(t.Context(), f.privyID); err != nil {
		t.Fatal(err)
	}
	if err := app.DeleteDevUser(t.Context(), f.deps, f.made.UserID); err != nil || privy.deletes != 0 ||
		f.count(t, "users") != 0 {
		t.Fatalf("err %v, %d Privy deletes, %d users", err, privy.deletes, f.count(t, "users"))
	}
	if err := app.DeleteDevUser(t.Context(), f.deps, f.made.UserID); errs.CodeOf(err) != errs.CodeNotFound {
		t.Fatalf("second delete = %v, want not_found", err)
	}
}

func TestDeleteDevUser_stopsOnEachFailure(t *testing.T) {
	t.Parallel()
	f, _ := newDevDeleteFixture(t)
	f.privy.Seed(app.PrivyUser{ID: f.privyID, Email: "dev-0a1b2c3d@example.com"})
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := app.DeleteDevUser(canceled, f.deps, f.made.UserID); err == nil {
		t.Error("canceled context succeeded")
	}
	f.privy.Fail("DevOnly", errs.New(errs.CodePrivyUnavailable, "test.privy"))
	if err := app.DeleteDevUser(t.Context(), f.deps, f.made.UserID); err == nil {
		t.Error("a Privy read failure succeeded")
	}
	f.privy.Fail("DevOnly", nil)
	f.balances.err = errs.New(errs.CodeRPCUnavailable, "test.rpc")
	if err := app.DeleteDevUser(t.Context(), f.deps, f.made.UserID); err == nil {
		t.Error("a balance failure succeeded")
	}
	if f.count(t, "users") != 1 {
		t.Fatal("a failed delete removed the user")
	}
}

func TestDevDeleteAdapters_failOnACanceledContext(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var store app.DevUserEraser = adapters.Users{}
	if _, _, err := store.DevUserForDelete(ctx, pool, ids.UserID{}); err == nil {
		t.Error("DevUserForDelete with a canceled context succeeded")
	}
	if err := store.DeleteDevUser(ctx, pool, ids.UserID{}); err == nil {
		t.Error("DeleteDevUser with a canceled context succeeded")
	}
}

type brokenEraser struct{ adapters.Users }

func (brokenEraser) DevUserForDelete(context.Context, sqlc.DBTX, ids.UserID) (app.DevUserDeletion, bool, error) {
	return app.DevUserDeletion{}, false, errs.New(errs.CodeInternal, "test.read")
}

func TestDeleteDevUser_aRowItCannotReadStopsBeforePrivy(t *testing.T) {
	t.Parallel()
	f, privy := newDevDeleteFixture(t)
	f.deps.Users = brokenEraser{}
	if err := app.DeleteDevUser(
		t.Context(),
		f.deps,
		f.made.UserID,
	); errs.CodeOf(err) != errs.CodeInternal ||
		privy.deletes != 0 {
		t.Fatalf("err %v, %d Privy deletes", err, privy.deletes)
	}
}

func (f devDeleteFixture) dropWalletRow(t *testing.T) {
	t.Helper()
	if _, err := f.pool.Exec(t.Context(), `DELETE FROM user_wallets`); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteDevUser_withoutAWalletRowChecksEveryWalletPrivyLists(t *testing.T) {
	t.Parallel()
	f, privy := newDevDeleteFixture(t)
	f.privy.Seed(app.PrivyUser{ID: f.privyID, Email: "dev-0a1b2c3d@example.com"})
	f.privy.SeedWallets(f.privyID, "WalletOne", "WalletTwo")
	f.dropWalletRow(t)
	f.balances.usdc = 1
	var funded *app.FundedError
	if err := app.DeleteDevUser(t.Context(), f.deps, f.made.UserID); !errors.As(err, &funded) ||
		funded.Address != "WalletOne" || privy.deletes != 0 || f.count(t, "users") != 1 {
		t.Fatalf("err %v, %d Privy deletes", err, privy.deletes)
	}
	f.balances.usdc = 0
	if err := app.DeleteDevUser(t.Context(), f.deps, f.made.UserID); err != nil || f.balances.asked != "WalletTwo" ||
		f.count(t, "users") != 0 {
		t.Fatalf("err %v, asked %q", err, f.balances.asked)
	}
}

func TestDeleteDevUser_withoutAWalletRowRefusesWhenPrivyListsNone(t *testing.T) {
	t.Parallel()
	f, privy := newDevDeleteFixture(t)
	f.privy.Seed(app.PrivyUser{ID: f.privyID, Email: "dev-0a1b2c3d@example.com"})
	f.dropWalletRow(t)
	if err := app.DeleteDevUser(t.Context(), f.deps, f.made.UserID); errs.CodeOf(err) != errs.CodeInvalidInput ||
		privy.deletes != 0 || f.count(t, "users") != 1 {
		t.Fatalf("err %v, %d Privy deletes", err, privy.deletes)
	}
}

func TestDeleteDevUser_withoutAWalletRowRefusesWhenTheListingFails(t *testing.T) {
	t.Parallel()
	f, privy := newDevDeleteFixture(t)
	f.privy.Seed(app.PrivyUser{ID: f.privyID, Email: "dev-0a1b2c3d@example.com"})
	f.privy.Fail("Wallets", errs.New(errs.CodePrivyUnavailable, "test.privy"))
	f.dropWalletRow(t)
	if err := app.DeleteDevUser(t.Context(), f.deps, f.made.UserID); errs.CodeOf(err) != errs.CodePrivyUnavailable ||
		privy.deletes != 0 || f.count(t, "users") != 1 {
		t.Fatalf("err %v, %d Privy deletes", err, privy.deletes)
	}
}

func TestDeleteDevUser_withoutAWalletRowAndNoPrivyUserStillDeletes(t *testing.T) {
	t.Parallel()
	f, _ := newDevDeleteFixture(t)
	f.dropWalletRow(t)
	if err := f.privy.Delete(t.Context(), f.privyID); err != nil {
		t.Fatal(err)
	}
	if err := app.DeleteDevUser(t.Context(), f.deps, f.made.UserID); err != nil || f.count(t, "users") != 0 {
		t.Fatalf("err %v, %d users", err, f.count(t, "users"))
	}
}
