package cabal_test

import (
	"maps"
	"slices"
	"testing"

	"github.com/google/uuid"

	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/cabalapi"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestAdminRole_theCabalReadNamesTheAdminsCreatorFirstAndMeReadsAdmin(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(3))
	admin := c.Members[2].ID
	f.makeAdmin(t, c.ID, admin)
	f.exec(t, `UPDATE cabal_members SET joined_at = joined_at - interval '1 hour' WHERE cabal_id = $1 AND user_id = $2`,
		c.ID.UUID(), admin.UUID())

	res, err := f.routes(nil).GetCabal(as(t.Context(), admin), api.GetCabalRequestObject{Id: c.ID.UUID()})
	got, ok := res.(api.GetCabal200JSONResponse)
	if err != nil || !ok || got.Admins == nil {
		t.Fatalf("GetCabal = %T, %v, admins %v", res, err, got.Admins)
	}
	listed := make([]uuid.UUID, 0, len(*got.Admins))
	for _, person := range *got.Admins {
		listed = append(listed, person.UserId)
	}
	if want := []uuid.UUID{c.Creator.ID.UUID(), admin.UUID()}; !slices.Equal(listed, want) {
		t.Fatalf("admins = %v, want the creator then the admin %v", listed, want)
	}
	if got.Me == nil || got.Me.Role != "admin" || !got.Me.CanVote {
		t.Fatalf("me = %+v, want a voting admin", got.Me)
	}
	roles := map[uuid.UUID]string{}
	for _, member := range got.Members {
		roles[member.UserId] = member.Role
	}
	want := map[uuid.UUID]string{
		admin.UUID(): "admin", c.Creator.ID.UUID(): "creator", c.Members[1].ID.UUID(): "member",
	}
	if !maps.Equal(roles, want) {
		t.Fatalf("member roles = %v, want %v", roles, want)
	}
}
