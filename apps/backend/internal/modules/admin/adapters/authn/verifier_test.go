package authn

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type verifierStub struct {
	actor auth.Actor
	err   error
}

func (s verifierStub) Verify(_ context.Context, _ string) (auth.Actor, error) { return s.actor, s.err }

func TestAdminVerifier(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{})
	base := verifierStub{actor: auth.Actor{Kind: auth.ActorUser, ID: user.ID.String(), Standing: auth.StandingActive}}
	verifier := NewAdminVerifier(base, pool)
	if _, err := verifier.Verify(t.Context(), "token"); errs.CodeOf(err) != errs.CodeAdminForbidden {
		t.Fatalf("ungranted = %v", err)
	}
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO admins (user_id, role, granted_at) VALUES ($1, 'moderator', now())`, user.ID.UUID()); err != nil {
		t.Fatal(err)
	}
	actor, err := verifier.Verify(t.Context(), "token")
	if err != nil || actor.Kind != auth.ActorAdmin || actor.Role != "moderator" || actor.ID != user.ID.String() {
		t.Fatalf("actor = %#v, err = %v", actor, err)
	}
	if _, err := pool.Exec(t.Context(),
		`UPDATE admins SET revoked_at = now() WHERE user_id = $1`, user.ID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.Verify(t.Context(), "token"); errs.CodeOf(err) != errs.CodeAdminForbidden {
		t.Fatalf("revoked = %v", err)
	}
	assertBanned(t, pool, user.ID.String())
	badID := NewAdminVerifier(verifierStub{actor: auth.Actor{ID: "not-a-user-id", Standing: auth.StandingActive}}, pool)
	if _, err := badID.Verify(t.Context(), "token"); errs.CodeOf(err) != errs.CodeAdminForbidden {
		t.Fatalf("bad id = %v", err)
	}
	assertBaseError(t, pool)
	if _, err := pool.Exec(t.Context(), "DROP TABLE admins"); err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.Verify(t.Context(), "token"); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("db error = %v", err)
	}
}

func assertBanned(t *testing.T, pool *pgxpool.Pool, userID string) {
	t.Helper()
	banned := NewAdminVerifier(verifierStub{actor: auth.Actor{ID: userID, Standing: auth.StandingBanned}}, pool)
	if _, err := banned.Verify(t.Context(), "token"); errs.CodeOf(err) != errs.CodeAdminForbidden {
		t.Fatalf("banned = %v", err)
	}
}

func assertBaseError(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	baseErr := errs.New(errs.CodeUnauthorized, "test")
	_, err := NewAdminVerifier(verifierStub{err: baseErr}, pool).Verify(t.Context(), "token")
	if !errors.Is(err, baseErr) {
		t.Fatalf("base error = %v, want %v", err, baseErr)
	}
}
