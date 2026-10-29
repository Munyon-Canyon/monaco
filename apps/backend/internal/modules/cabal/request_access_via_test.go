package cabal_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type fakeCodes struct {
	owners map[string]ids.UserID
	fail   error
}

func (c fakeCodes) Resolve(_ context.Context, code string) (ids.UserID, bool, error) {
	if c.fail != nil {
		return ids.UserID{}, false, c.fail
	}
	owner, ok := c.owners[code]
	return owner, ok, nil
}

func (f accessFixture) requestVia(
	user ids.UserID, c ids.CabalID, codes app.CodeResolver, code string,
) (app.Access, error) {
	return app.NewRequestAccessHandler(f.uow, f.ids, f.clock, codes).Handle(as(context.Background(), user),
		app.RequestAccess{ActorID: user, CabalID: c, ViaCode: code})
}

func (f accessFixture) viaOf(t *testing.T, id uuid.UUID) *uuid.UUID {
	t.Helper()
	var via *uuid.UUID
	if err := f.pool.QueryRow(t.Context(), `SELECT via_user_id FROM cabal_access_requests WHERE id = $1`, id).
		Scan(&via); err != nil {
		t.Fatal(err)
	}
	return via
}

func TestRequestAccess_storesTheMemberWhoseCodeCameWithIt(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"), testkit.WithMembers(2))
	jordan := c.Members[1].ID
	asker := f.user(t)

	filed, err := f.requestVia(asker, c.ID, fakeCodes{owners: map[string]ids.UserID{"jordan": jordan}}, "jordan")
	if err != nil {
		t.Fatal(err)
	}
	if via := f.viaOf(t, filed.ID); via == nil || *via != jordan.UUID() {
		t.Fatalf("via_user_id = %v, want %s", via, jordan)
	}
	got := decoded[events.CabalAccessRequested](t, f, events.TypeCabalAccessRequested)
	if len(got) != 1 || got[0].ViaUserID == nil || *got[0].ViaUserID != jordan.UUID() {
		t.Fatalf("access_requested events = %+v, want via_user_id %s", got, jordan)
	}
}

func TestRequestAccess_storesNoViaButStillFilesTheRequest(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"))
	outsider := f.user(t)
	codes := fakeCodes{owners: map[string]ids.UserID{"outsider": outsider}}
	for _, tt := range []struct {
		name  string
		codes app.CodeResolver
		code  string
	}{
		{"a non-member's code", codes, "outsider"},
		{"an unknown code", codes, "nobody"},
		{"a resolver error", fakeCodes{fail: errs.New(errs.CodeInternal, "referrals down")}, "outsider"},
		{"no code", codes, ""},
		{"no resolver wired", nil, "outsider"},
	} {
		asker := f.user(t)
		filed, err := f.requestVia(asker, c.ID, tt.codes, tt.code)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if via := f.viaOf(t, filed.ID); via != nil {
			t.Errorf("%s: via_user_id = %s, want null", tt.name, via)
		}
	}
	for _, e := range decoded[events.CabalAccessRequested](t, f, events.TypeCabalAccessRequested) {
		if e.ViaUserID != nil {
			t.Errorf("event %+v carries via_user_id, want none", e)
		}
	}
}
