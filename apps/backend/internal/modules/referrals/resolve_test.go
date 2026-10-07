package referrals_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type owner struct {
	handle   string
	status   string
	code     string
	unlocked bool
	name     string
	photo    string
}

func seedOwner(t *testing.T, pool *pgxpool.Pool, o owner) ids.UserID {
	t.Helper()
	u := testkit.SeedUser(t, pool, testkit.UserOpts{Handle: o.handle, AccountStatus: o.status})
	if o.unlocked {
		_, err := pool.Exec(t.Context(), `UPDATE users SET first_deposit_at = now() WHERE id = $1`, u.ID.UUID())
		if err != nil {
			t.Fatal(err)
		}
	}
	if o.name != "" || o.photo != "" {
		if _, err := pool.Exec(t.Context(),
			`UPDATE users SET display_name = $2, photo_url = NULLIF($3, '') WHERE id = $1`,
			u.ID.UUID(), o.name, o.photo,
		); err != nil {
			t.Fatal(err)
		}
	}
	if o.code != "" {
		if _, err := pool.Exec(t.Context(),
			`INSERT INTO referral_codes (code, user_id, created_at) VALUES ($1, $2, now())`, o.code, u.ID.UUID(),
		); err != nil {
			t.Fatal(err)
		}
	}
	return u.ID
}

func TestResolve_returnsTheOwnerOfARandomCodeOrAnUnlockedHandle(t *testing.T) {
	t.Parallel()
	for name, tt := range map[string]struct {
		owner owner
		input string
		kind  domain.CodeKind
		code  domain.Code
	}{
		"random hit":            {owner{code: "k7m4qx2p"}, "k7m4qx2p", domain.CodeKindRandom, "k7m4qx2p"},
		"uppercase random":      {owner{code: "k7m4qx2p"}, "K7M4QX2P", domain.CodeKindRandom, "k7m4qx2p"},
		"random with spaces":    {owner{code: "k7m4qx2p"}, "  k7m4qx2p \n", domain.CodeKindRandom, "k7m4qx2p"},
		"handle unlocked":       {owner{handle: "kaicenat", unlocked: true}, "kaicenat", domain.CodeKindHandle, "kaicenat"},
		"uppercase handle":      {owner{handle: "kaicenat", unlocked: true}, "KaiCenat", domain.CodeKindHandle, "kaicenat"},
		"handle with spaces":    {owner{handle: "kaicenat", unlocked: true}, " kaicenat\t", domain.CodeKindHandle, "kaicenat"},
		"random-shaped handle":  {owner{handle: "mattcarr", unlocked: true}, "mattcarr", domain.CodeKindHandle, "mattcarr"},
		"random of an unlocked": {owner{handle: "kaicenat", code: "k7m4qx2p", unlocked: true}, "k7m4qx2p", domain.CodeKindRandom, "k7m4qx2p"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pool := testkit.DB(t)
			id := seedOwner(t, pool, tt.owner)
			got, err := referrals.New(module.Deps{Pool: pool}).Resolver().Resolve(t.Context(), tt.input)
			want := app.Resolved{UserID: id, CodeKind: tt.kind, Code: tt.code}
			if err != nil || got != want {
				t.Fatalf("Resolve(%q) = %+v, %v; want %+v", tt.input, got, err, want)
			}
		})
	}
}

func TestResolve_aMintedCodeWinsOverAHandleWithTheSameSpelling(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	minted := seedOwner(t, pool, owner{code: "mattcarr"})
	seedOwner(t, pool, owner{handle: "mattcarr", unlocked: true})
	got, err := referrals.New(module.Deps{Pool: pool}).Resolver().Resolve(t.Context(), "mattcarr")
	want := app.Resolved{UserID: minted, CodeKind: domain.CodeKindRandom, Code: "mattcarr"}
	if err != nil || got != want {
		t.Fatalf("Resolve = %+v, %v; want %+v", got, err, want)
	}
}

func TestResolve_givesTheSameUnknownCodeWhateverTheReason(t *testing.T) {
	t.Parallel()
	for name, tt := range map[string]struct {
		owner owner
		input string
	}{
		"random miss":                 {owner{code: "k7m4qx2p"}, "k7m4qx2q"},
		"handle miss":                 {owner{handle: "kaicenat", unlocked: true}, "kaicenot"},
		"handle locked":               {owner{handle: "kaicenat"}, "kaicenat"},
		"random-shaped handle locked": {owner{handle: "mattcarr"}, "mattcarr"},
		"handle of a banned user":     {owner{handle: "kaicenat", unlocked: true, status: "banned"}, "kaicenat"},
		"handle of a suspended user":  {owner{handle: "kaicenat", unlocked: true, status: "suspended"}, "kaicenat"},
		"handle of a deleted user":    {owner{handle: "kaicenat", unlocked: true, status: "deleted"}, "kaicenat"},
		"random of a banned user":     {owner{code: "k7m4qx2p", status: "banned"}, "k7m4qx2p"},
		"random of a deleted user":    {owner{code: "k7m4qx2p", status: "deleted"}, "k7m4qx2p"},
		"empty input":                 {owner{}, "   "},
		"not a handle":                {owner{}, "kai cenat!"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pool := testkit.DB(t)
			seedOwner(t, pool, tt.owner)
			got, err := referrals.New(module.Deps{Pool: pool}).Resolver().Resolve(t.Context(), tt.input)
			want, code := errs.New(errs.CodeReferralCodeUnknown, "referrals.Resolve"), errs.CodeOf(err)
			if got != (app.Resolved{}) || err == nil || err.Error() != want.Error() ||
				errs.KindOf(code) != errs.KindNotFound || errs.Message(code) != "That code isn't valid" {
				t.Fatalf("Resolve(%q) = %+v, %v; want only %v", tt.input, got, err, want)
			}
		})
	}
}

func TestResolve_treatsACodeWhoseOwnerHasNoUserRowAsUnknown(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	owner := testkit.NewIDs(7).NewV7()
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO referral_codes (code, user_id, created_at) VALUES ('k7m4qx2p', $1, now())`, owner,
	); err != nil {
		t.Fatal(err)
	}
	_, err := referrals.New(module.Deps{Pool: pool}).Resolver().Resolve(t.Context(), "k7m4qx2p")
	if errs.CodeOf(err) != errs.CodeReferralCodeUnknown {
		t.Fatalf("Resolve = %v, want referral_code_unknown", err)
	}
}

type failingUsers struct{ identity.UserReader }

func (failingUsers) UserByHandle(context.Context, string) (identity.UserCard, error) {
	return identity.UserCard{}, errs.New(errs.CodeDBUnavailable, "test.UserByHandle")
}

func (failingUsers) UsersByID(context.Context, []ids.UserID) (map[ids.UserID]identity.UserCard, error) {
	return nil, errs.New(errs.CodeDBUnavailable, "test.UsersByID")
}

func TestResolve_passesOnFailuresThatAreNotAMiss(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	if _, err := pool.Exec(t.Context(), `INSERT INTO referral_codes (code, user_id, created_at)
		VALUES ('k7m4qx2p', $1, now()), ('v4v4v4v4', $2, now())`,
		testkit.NewIDs(7).NewV7(), uuid.NewSHA1(uuid.Nil, []byte("not a v7 id")),
	); err != nil {
		t.Fatal(err)
	}
	failing := app.Resolver{Reads: pool, Users: failingUsers{}}
	for input, want := range map[string]errs.Code{
		"kaicenat": errs.CodeDBUnavailable,
		"k7m4qx2p": errs.CodeDBUnavailable,
		"v4v4v4v4": errs.CodeDecodeFailed,
	} {
		if _, err := failing.Resolve(t.Context(), input); errs.CodeOf(err) != want {
			t.Errorf("Resolve(%q) = %v, want %s", input, err, want)
		}
	}
	if _, err := pool.Exec(t.Context(), `ALTER TABLE referral_codes RENAME TO referral_codes_gone`); err != nil {
		t.Fatal(err)
	}
	if _, err := failing.Resolve(t.Context(), "k7m4qx2p"); errs.CodeOf(err) != errs.CodeInternal {
		t.Errorf("Resolve without referral_codes = %v, want internal", err)
	}
}
