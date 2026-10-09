package privyfake_test

import (
	"errors"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/testkit/privyfake"
)

func TestUsers_verifiesSeededUsersAndScriptsFaults(t *testing.T) {
	t.Parallel()
	var u privyfake.Users
	if _, err := u.Verify(t.Context(), "did:privy:a"); errs.CodeOf(err) != errs.CodeUnauthorized {
		t.Fatalf("Verify before Seed = %v, want unauthorized", err)
	}
	if _, err := u.User(t.Context(), "did:privy:a"); errs.CodeOf(err) != errs.CodeNotFound {
		t.Fatalf("User before Seed = %v, want not_found", err)
	}
	want := app.PrivyUser{ID: "did:privy:a", PhoneE164: "+14155550100"}
	u.Seed(want)
	if id, err := u.Verify(t.Context(), "did:privy:a"); err != nil || id != want.ID {
		t.Fatalf("Verify = %q, %v", id, err)
	}
	if got, err := u.User(t.Context(), want.ID); err != nil || got != want {
		t.Fatalf("User = %+v, %v", got, err)
	}
	down := errs.New(errs.CodePrivyUnavailable, "test")
	u.FailOnce("User", down)
	if _, err := u.User(t.Context(), want.ID); errs.CodeOf(err) != errs.CodePrivyUnavailable {
		t.Fatalf("FailOnce = %v", err)
	}
	if _, err := u.User(t.Context(), want.ID); err != nil {
		t.Fatalf("FailOnce stuck: %v", err)
	}
	u.Fail("Verify", down)
	if _, err := u.Verify(t.Context(), "did:privy:a"); errs.CodeOf(err) != errs.CodePrivyUnavailable {
		t.Fatalf("Fail = %v", err)
	}
}

func TestUsers_createReturnsAFakeIDAndStoresTheEmail(t *testing.T) {
	t.Parallel()
	var u privyfake.Users
	id, err := u.Create(t.Context(), "dev-ab@example.com")
	if err != nil || id != "did:privy:fake-1" {
		t.Fatalf("Create = %q, %v", id, err)
	}
	got, err := u.User(t.Context(), id)
	if err != nil || got != (app.PrivyUser{ID: id, Email: "dev-ab@example.com"}) {
		t.Fatalf("User = %+v, %v", got, err)
	}
	again, err := u.Create(t.Context(), "dev-cd@example.com")
	if err != nil || again != "did:privy:fake-2" {
		t.Fatalf("second Create = %q, %v", again, err)
	}
	u.FailOnce("Create", errs.New(errs.CodePrivyUnavailable, "test"))
	if _, err := u.Create(t.Context(), "dev-ef@example.com"); errs.CodeOf(err) != errs.CodePrivyUnavailable {
		t.Fatalf("FailOnce = %v", err)
	}
}

func TestWallets_reusesSeededWalletsAndCountsCreates(t *testing.T) {
	t.Parallel()
	var w privyfake.Wallets
	legacy := app.PrivyWallet{
		Wallet: domain.Wallet{PrivyWalletID: "wallet-legacy", Address: "BGQoQgGkjSQc6c5YjsyRjuj4M5LbYJMHVCdS8BJSrr2R"},
	}
	w.Seed("did:privy:legacy", legacy)
	if got, err := w.FindOrCreate(t.Context(), "did:privy:legacy"); err != nil || got != legacy {
		t.Fatalf("FindOrCreate(seeded) = %+v, %v", got, err)
	}
	first, err := w.FindOrCreate(t.Context(), "did:privy:new")
	if err != nil || !first.HasAppSigner || first.PrivyWalletID == "" || first.Address == "" {
		t.Fatalf("FindOrCreate(new) = %+v, %v", first, err)
	}
	if again, _ := w.FindOrCreate(t.Context(), "did:privy:new"); again != first || w.Creates() != 1 {
		t.Fatalf("second FindOrCreate = %+v after %d creates", again, w.Creates())
	}
	w.FailOnce("FindOrCreate", errs.New(errs.CodePrivyUnavailable, "test"))
	if _, err := w.FindOrCreate(t.Context(), "did:privy:new"); errs.CodeOf(err) != errs.CodePrivyUnavailable {
		t.Fatalf("FailOnce = %v", err)
	}
	if _, err := w.FindOrCreate(t.Context(), "did:privy:other"); err != nil || w.Creates() != 2 {
		t.Fatalf("after FailOnce = %v with %d creates", err, w.Creates())
	}
}

func TestUsers_findsByEmailAndRefusesADuplicate(t *testing.T) {
	t.Parallel()
	var u privyfake.Users
	if _, found, err := u.ByEmail(t.Context(), "dev-ab@example.com"); found || err != nil {
		t.Fatalf("ByEmail before Create = %v, %v", found, err)
	}
	id, err := u.Create(t.Context(), "dev-ab@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got, found, err := u.ByEmail(t.Context(), "dev-ab@example.com"); got != id || !found || err != nil {
		t.Fatalf("ByEmail = %q, %v, %v, want %q", got, found, err, id)
	}
	if _, err := u.Create(t.Context(), "dev-ab@example.com"); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("duplicate Create = %v, want invalid_input", err)
	}
	down := errs.New(errs.CodePrivyUnavailable, "test")
	u.Fail("ByEmail", down)
	if _, _, err := u.ByEmail(t.Context(), "x@example.com"); !errors.Is(err, down) {
		t.Fatalf("ByEmail with a scripted fault = %v", err)
	}
}

func TestUsers_devOnlyAndDelete(t *testing.T) {
	t.Parallel()
	var u privyfake.Users
	if _, found, err := u.DevOnly(t.Context(), "did:privy:a"); found || err != nil {
		t.Fatalf("DevOnly before Seed = %v, %v", found, err)
	}
	for name, tc := range map[string]struct {
		user app.PrivyUser
		want bool
	}{
		"dev email":  {app.PrivyUser{ID: "did:privy:a", Email: "dev-0a1b2c3d@example.com"}, true},
		"real email": {app.PrivyUser{ID: "did:privy:a", Email: "a@gmail.com"}, false},
		"phone":      {app.PrivyUser{ID: "did:privy:a", Email: "dev-0a1b2c3d@example.com", PhoneE164: "+14155550100"}, false},
		"x":          {app.PrivyUser{ID: "did:privy:a", Email: "dev-0a1b2c3d@example.com", X: &domain.XAccount{UserID: "1"}}, false},
		"apple":      {app.PrivyUser{ID: "did:privy:a", Email: "dev-0a1b2c3d@example.com", AppleEmail: "a@icloud.com"}, false},
		"google":     {app.PrivyUser{ID: "did:privy:a", Email: "dev-0a1b2c3d@example.com", GoogleEmail: "a@gmail.com"}, false},
	} {
		u.Seed(tc.user)
		if got, found, err := u.DevOnly(t.Context(), "did:privy:a"); got != tc.want || !found || err != nil {
			t.Errorf("%s: DevOnly = %v, %v, %v, want %v", name, got, found, err, tc.want)
		}
	}
	if err := u.Delete(t.Context(), "did:privy:a"); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := u.DevOnly(t.Context(), "did:privy:a"); found {
		t.Fatal("user still there after Delete")
	}
	down := errs.New(errs.CodePrivyUnavailable, "test")
	u.Fail("DevOnly", down)
	u.Fail("Delete", down)
	if _, _, err := u.DevOnly(t.Context(), "x"); !errors.Is(err, down) {
		t.Errorf("DevOnly fault = %v", err)
	}
	if err := u.Delete(t.Context(), "x"); !errors.Is(err, down) {
		t.Errorf("Delete fault = %v", err)
	}
}
