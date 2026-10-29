package cabal_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestListAccessRequests_anAdminSeesWhoseLinkEachCameThroughNewestFirst(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"), testkit.WithMembers(3))
	admin, jordan := c.Members[1].ID, c.Members[2].ID
	f.makeAdmin(t, c.ID, admin)
	f.exec(t, `UPDATE users SET display_name = 'Jordan' WHERE id = $1`, jordan.UUID())
	codes := fakeCodes{owners: map[string]ids.UserID{"jordan": jordan}}
	direct, through := f.user(t), f.user(t)
	directFiled, err := f.requestVia(direct, c.ID, codes, "")
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Minute)
	throughFiled, err := f.requestVia(through, c.ID, codes, "jordan")
	if err != nil {
		t.Fatal(err)
	}

	got, err := app.ListAccessRequests(t.Context(), f.pool, f.users, c.ID, admin)

	if err != nil || len(got) != 2 {
		t.Fatalf("ListAccessRequests = %+v, %v; want two requests", got, err)
	}
	if got[0].ID != throughFiled.ID || got[0].Via == nil || got[0].Via.UserID != jordan.UUID() ||
		got[0].Via.DisplayName != "Jordan" {
		t.Fatalf("first = %+v, want the newer request via Jordan", got[0])
	}
	if got[1].ID != directFiled.ID || got[1].Via != nil {
		t.Fatalf("second = %+v, want the older request with no via", got[1])
	}
}
