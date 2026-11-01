package cabal_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestExCreator_cannotEditTheCabalOrHandleItsRequests(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"), testkit.WithMembers(2))
	asker := f.user(t)
	pending := f.filed(t, asker, c.ID)
	f.exec(t, `DELETE FROM cabal_members WHERE cabal_id = $1 AND user_id = $2`, c.ID.UUID(), c.Creator.ID.UUID())

	name, err := domain.ParseName("Renamed")
	if err != nil {
		t.Fatal(err)
	}
	err = app.NewUpdateCabalHandler(f.uow, f.clock).Handle(as(t.Context(), c.Creator.ID), app.UpdateCabal{
		ActorID: c.Creator.ID, CabalID: c.ID, Name: &name,
	})
	if errs.CodeOf(err) != errs.CodeNotCabalCreator {
		t.Errorf("UpdateCabal by the ex-creator: err = %v, want %s", err, errs.CodeNotCabalCreator)
	}
	_, err = app.ListAccessRequests(t.Context(), f.pool, f.users, c.ID, c.Creator.ID)
	if errs.CodeOf(err) != errs.CodeNotCabalAdmin {
		t.Errorf("ListAccessRequests by the ex-creator: err = %v, want %s", err, errs.CodeNotCabalAdmin)
	}
	_, err = f.decide(t.Context(), c.Creator.ID, c.ID, pending, app.Approve)
	if errs.CodeOf(err) != errs.CodeNotCabalAdmin {
		t.Errorf("DecideAccess by the ex-creator: err = %v, want %s", err, errs.CodeNotCabalAdmin)
	}
	if _, ok := f.membership(t, c.ID, asker); ok {
		t.Error("the ex-creator's approval admitted the asker")
	}
}

func TestSetCabalPicture_failsWithInternalWhenTheMembershipCannotBeRead(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool)
	f.exec(t, `ALTER TABLE cabal_members RENAME COLUMN joined_at TO joined_gone`)
	err := app.NewSetCabalPictureHandler(f.uow, f.pool, f.ids, f.clock, nil).Handle(
		as(t.Context(), c.Creator.ID), app.SetCabalPicture{
			ActorID: c.Creator.ID, CabalID: c.ID, Picture: &app.Picture{ContentType: "image/png", Ext: "png"},
		})
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Errorf("SetCabalPicture with the members table gone: err = %v, want %s", err, errs.CodeInternal)
	}
}
