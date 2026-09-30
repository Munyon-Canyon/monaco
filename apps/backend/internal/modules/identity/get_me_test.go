package identity_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func sameMe(t *testing.T, got, want app.Me) {
	t.Helper()
	gotAt, wantAt := got.HandleChangeableAt, want.HandleChangeableAt
	gotCreated, wantCreated := got.CreatedAt, want.CreatedAt
	got.HandleChangeableAt, want.HandleChangeableAt = nil, nil
	got.CreatedAt, want.CreatedAt = time.Time{}, time.Time{}
	sameChangeable := gotAt == nil && wantAt == nil || gotAt != nil && wantAt != nil && gotAt.Equal(*wantAt) &&
		gotAt.Location() == time.UTC
	if got != want || !sameChangeable || !gotCreated.Equal(wantCreated) || gotCreated.Location() != time.UTC {
		t.Fatalf("Me = %+v created %s changeable %v, want %+v created %s changeable %v",
			got, gotCreated, gotAt, want, wantCreated, wantAt)
	}
}

func TestGetMe_carriesEveryFieldOfTheAccount(t *testing.T) {
	t.Parallel()
	f := newPortFixture(t)
	u := f.seed(t, portSeed{
		handle: "kaicenat", name: "Kai Cenat", photo: "https://img.example/kai.png", authState: "AWAITING_SOCIALS",
		status: "suspended", phoneHash: portHash("kai"), phoneVerified: true, wallet: true,
	})
	changed := f.now.Add(-48 * time.Hour)
	if _, err := f.pool.Exec(
		t.Context(),
		`UPDATE users SET x_username = 'kai_on_x', handle_changed_at = $2 WHERE id = $1`,
		u.ID.UUID(),
		changed,
	); err != nil {
		t.Fatal(err)
	}
	got, err := app.GetMe(t.Context(), f.pool, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	changeable := changed.Add(domain.HandleChangeInterval)
	sameMe(t, got, app.Me{
		ID: u.ID, Handle: "kaicenat", DisplayName: "Kai Cenat", PhotoURL: "https://img.example/kai.png",
		AuthState: domain.AuthAwaitingSocials, AccountStatus: domain.AccountSuspended,
		MemberWalletAddress: u.Address, PhoneLinked: true, XUsername: "kai_on_x", HandleChangeableAt: &changeable,
		CreatedAt: f.created(),
	})
}

func TestGetMe_aNewUserHasNoHandleNameOrLinks(t *testing.T) {
	t.Parallel()
	f := newPortFixture(t)
	u := f.seed(t, portSeed{wallet: true})
	got, err := app.GetMe(t.Context(), f.pool, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	sameMe(t, got, app.Me{
		ID: u.ID, AuthState: domain.AuthCreated, AccountStatus: domain.AccountActive, MemberWalletAddress: u.Address,
		CreatedAt: f.created(),
	})
}

func TestGetMe_theDisplayNameFallsBackToTheHandle(t *testing.T) {
	t.Parallel()
	f := newPortFixture(t)
	named := f.seed(t, portSeed{handle: "named_one", name: "Real Name", wallet: true})
	quiet := f.seed(t, portSeed{handle: "quiet_one", wallet: true})
	for id, want := range map[ids.UserID]string{named.ID: "Real Name", quiet.ID: "quiet_one"} {
		got, err := app.GetMe(t.Context(), f.pool, id)
		if err != nil || got.DisplayName != want {
			t.Fatalf("GetMe display name = %q, %v, want %q", got.DisplayName, err, want)
		}
	}
}

func TestGetMe_isNotFoundForAnAccountThatCannotBeShown(t *testing.T) {
	t.Parallel()
	f := newPortFixture(t)
	for name, id := range map[string]ids.UserID{
		"unknown user":    f.newID(t),
		"deleted user":    f.seed(t, portSeed{wallet: true, softDeleted: true, status: "deleted"}).ID,
		"user no wallet":  f.seed(t, portSeed{}).ID,
		"deleted no wall": f.seed(t, portSeed{softDeleted: true, status: "deleted"}).ID,
	} {
		_, err := app.GetMe(t.Context(), f.pool, id)
		if errs.CodeOf(err) != errs.CodeUserNotFound {
			t.Fatalf("%s: GetMe = %v, want user_not_found", name, err)
		}
	}
}

func TestGetMe_aClosedDatabaseIsInternal(t *testing.T) {
	t.Parallel()
	f := newPortFixture(t)
	u := f.seed(t, portSeed{wallet: true})
	f.pool.Close()
	if _, err := app.GetMe(t.Context(), f.pool, u.ID); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("GetMe on a closed pool = %v, want internal", err)
	}
}
