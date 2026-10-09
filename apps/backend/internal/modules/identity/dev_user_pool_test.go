package identity_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/privyfake"
)

func TestCreateDevUser_poolMintsOnceThenReuses(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	clk := testkit.NewClock(clock.Real{}.Now().UTC().Truncate(time.Microsecond))
	privy := &countUsers{PrivyUsers: &privyfake.Users{}}
	wallets := &privyfake.Wallets{}
	deps := devDeps(pool, clk, privy, wallets, &recordedHints{}, devRand())
	deps.Pool = "browse-other-1"
	first, err := app.CreateDevUser(t.Context(), deps)
	if err != nil || first.Handle != "dev_browse_other_1" {
		t.Fatalf("first = %+v, %v", first, err)
	}
	second, err := app.CreateDevUser(t.Context(), deps)
	if err != nil || second != first || privy.n != 1 {
		t.Fatalf("second = %+v, %v after %d Privy creates, want %+v and 1", second, err, privy.n, first)
	}
	deps.Pool = "browse-other-2"
	other, err := app.CreateDevUser(t.Context(), deps)
	if err != nil || other.UserID == first.UserID || privy.n != 2 {
		t.Fatalf("other = %+v, %v after %d creates", other, err, privy.n)
	}
}

func TestCreateDevUser_poolRefusesBadNamesAndFailures(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	clk := testkit.NewClock(clock.Real{}.Now().UTC().Truncate(time.Microsecond))
	for _, name := range []string{"Upper", "has_underscore", "-lead", "trail-", "a--b", "seventeen-chars-xx"} {
		deps := devDeps(pool, clk, &privyfake.Users{}, &privyfake.Wallets{}, &recordedHints{}, devRand())
		deps.Pool = name
		if _, err := app.CreateDevUser(t.Context(), deps); errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("pool %q = %v, want invalid_input", name, err)
		}
	}
	deps := devDeps(pool, clk, &privyfake.Users{}, &privyfake.Wallets{}, &recordedHints{}, devRand())
	deps.Pool = "ok"
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := app.CreateDevUser(canceled, deps); err == nil {
		t.Fatal("expected the lookup error")
	}
	deps = devDeps(pool, clk, &privyfake.Users{}, &privyfake.Wallets{}, &recordedHints{}, devRand())
	deps.Pool = "ok"
	if _, err := app.CreateDevUser(t.Context(), deps); err != nil {
		t.Fatal(err)
	}
	wallets := &privyfake.Wallets{}
	wallets.Fail("FindOrCreate", errs.New(errs.CodeUpstreamUnavailable, "test.wallet"))
	deps.Wallets = wallets
	if _, err := app.CreateDevUser(t.Context(), deps); err == nil {
		t.Fatal("expected the wallet error on reuse")
	}
}

func TestDeleteDevUser_removesPrivyThenTheRow(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	clk := testkit.NewClock(clock.Real{}.Now().UTC().Truncate(time.Microsecond))
	privy := &privyfake.Users{}
	wallets := &privyfake.Wallets{}
	made, err := app.CreateDevUser(t.Context(), devDeps(pool, clk, privy, wallets, &recordedHints{}, devRand()))
	if err != nil {
		t.Fatal(err)
	}
	del := app.DeleteDevUserDeps{
		Env: config.EnvLocal, UoW: db.New(pool, ids.Real{}, clk), Users: adapters.Users{}, Privy: privy,
	}
	if err := app.DeleteDevUser(t.Context(), del, made.UserID); err != nil {
		t.Fatal(err)
	}
	var users, wallet int
	if err := pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM users), (SELECT count(*) FROM user_wallets)`).
		Scan(&users, &wallet); err != nil || users != 0 || wallet != 0 {
		t.Fatalf("users %d wallets %d, %v", users, wallet, err)
	}
	if _, err := privy.User(t.Context(), "did:privy:fake-1"); errs.CodeOf(err) != errs.CodeNotFound {
		t.Fatalf("Privy still has the user: %v", err)
	}
	if err := app.DeleteDevUser(t.Context(), del, made.UserID); errs.CodeOf(err) != errs.CodeNotFound {
		t.Fatalf("second delete = %v, want not_found", err)
	}
}

func TestDeleteDevUser_finishesAfterPrivyForgotTheUserAndStopsOnFailures(t *testing.T) {
	t.Parallel()
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
	del := app.DeleteDevUserDeps{
		Env: config.EnvLocal, UoW: db.New(pool, ids.Real{}, clk), Users: adapters.Users{}, Privy: privy,
	}
	privy.Fail("Delete", errs.New(errs.CodeUpstreamUnavailable, "test.privy"))
	if err := app.DeleteDevUser(t.Context(), del, made.UserID); err == nil {
		t.Fatal("expected the Privy error")
	}
	var users int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM users`).Scan(&users); err != nil || users != 1 {
		t.Fatalf("a failed Privy delete left %d users, %v", users, err)
	}
	privy.Fail("Delete", errs.New(errs.CodeNotFound, "test.privy"))
	del.Users = failingDelete{}
	if err := app.DeleteDevUser(t.Context(), del, made.UserID); err == nil {
		t.Fatal("expected the row error")
	}
	del.Users = adapters.Users{}
	if err := app.DeleteDevUser(t.Context(), del, made.UserID); err != nil {
		t.Fatalf("a rerun after Privy forgot the user = %v", err)
	}
}

func TestDeleteDevUser_refusesDeployedEnvs(t *testing.T) {
	t.Parallel()
	for _, env := range []config.Env{config.EnvStaging, config.EnvProduction} {
		err := app.DeleteDevUser(t.Context(), app.DeleteDevUserDeps{Env: env}, ids.UserID{})
		if errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("%s = %v, want invalid_input", env, err)
		}
	}
}

func TestDevUserAdapters_failOnACanceledContext(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	store := adapters.Users{}
	if _, _, err := store.DevUserByHandle(ctx, pool, "dev_x"); err == nil {
		t.Error("DevUserByHandle with a canceled context succeeded")
	}
	if _, err := store.DevUserPrivyID(ctx, pool, ids.UserID{}); errs.CodeOf(err) != errs.CodeInternal {
		t.Errorf("DevUserPrivyID = %v, want internal", err)
	}
	if err := store.DeleteDevUser(ctx, pool, ids.UserID{}); err == nil {
		t.Error("DeleteDevUser with a canceled context succeeded")
	}
}

type failingDelete struct{ adapters.Users }

func (failingDelete) DeleteDevUser(context.Context, sqlc.DBTX, ids.UserID) error {
	return errs.New(errs.CodeInternal, "test.delete")
}
