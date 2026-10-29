package cabal_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/adapters"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type failingCards struct{}

func (failingCards) UsersByID(context.Context, []ids.UserID) (map[ids.UserID]identityport.UserCard, error) {
	return nil, errs.New(errs.CodeInternal, "test.users")
}

func TestInvitesByID_findsAnInviteInAnyStateWithItsCabalAndInviter(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	pot := testkit.NewCabal(t, f.pool, testkit.WithMembers(3))
	f.exec(t, `UPDATE cabals SET name = 'First pot' WHERE id = $1`, pot.ID.UUID())
	invitee := f.user(t)
	accepted := f.invite(t, pot.ID, invitee, pot.Members[2].ID)
	f.exec(t, `UPDATE cabal_access_requests SET status = 'approved' WHERE id = $1`, accepted)
	unknown := ids.Real{}.NewV7()

	got, err := adapters.NewQueries(f.pool).WithUsers(f.users).
		InvitesByID(t.Context(), []uuid.UUID{accepted, unknown})

	if err != nil || len(got) != 1 {
		t.Fatalf("InvitesByID = %+v, %v; want the one accepted invite", got, err)
	}
	wantInvite(t, got[accepted], wantedInvite{
		id: accepted, cabal: pot.ID, name: "First pot", members: 3, inviter: pot.Members[2].ID,
	})
}

func TestInvitesByID_answersNothingForNoIDs(t *testing.T) {
	t.Parallel()
	f := newAccess(t)

	got, err := adapters.NewQueries(f.pool).WithUsers(f.users).InvitesByID(t.Context(), nil)

	if err != nil || len(got) != 0 {
		t.Fatalf("InvitesByID(nil) = %+v, %v; want none", got, err)
	}
}

func TestInvitesByID_failsWithoutUserCardsAndWhenAReadFails(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	pot := testkit.NewCabal(t, f.pool)
	id := f.invite(t, pot.ID, f.user(t), pot.Creator.ID)
	requests := []uuid.UUID{id}

	_, noCards := adapters.NewQueries(f.pool).InvitesByID(t.Context(), requests)
	_, cardsDown := adapters.NewQueries(f.pool).WithUsers(failingCards{}).InvitesByID(t.Context(), requests)
	f.exec(t, `ALTER TABLE cabal_access_requests RENAME TO requests_gone`)
	_, readDown := adapters.NewQueries(f.pool).WithUsers(f.users).InvitesByID(t.Context(), requests)

	for name, err := range map[string]error{"no cards": noCards, "cards down": cardsDown, "read down": readDown} {
		if errs.CodeOf(err) != errs.CodeInternal {
			t.Errorf("%s: InvitesByID = %v, want internal", name, err)
		}
	}
}
