package cabal_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/cabalapi"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func moderatorContext(t *testing.T) context.Context {
	t.Helper()
	return auth.WithActor(t.Context(), auth.Actor{
		Kind: auth.ActorAdmin, ID: testkit.NewIDs(9).NewV7().String(), Role: "moderator",
	})
}

func TestAdminRemoveCabalPicture_clearsItOnceAndAudits(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool)
	if _, err := f.pool.Exec(
		t.Context(),
		`UPDATE cabals SET picture_url = 'https://cdn.example.com/p.png' WHERE id = $1`,
		c.ID.UUID(),
	); err != nil {
		t.Fatal(err)
	}
	h := f.routes(nil)
	for range 2 {
		_, err := h.RemoveAdminCabalPicture(moderatorContext(t), api.RemoveAdminCabalPictureRequestObject{
			Id: c.ID.UUID(), Body: &apibase.ReasonBody{Reason: "abusive image"},
		})
		if err != nil {
			t.Fatalf("RemoveAdminCabalPicture = %v, want nil", err)
		}
	}
	if url := f.pictureURL(t, c.ID); url != nil {
		t.Fatalf("picture_url = %q, want null", *url)
	}
	if got := pictureChanges(t, f); len(got) != 1 || got[0] != "" {
		t.Fatalf("cabal.updated picture changes = %q, want one clear", got)
	}
	audits := decoded[events.AdminAction](t, f, events.TypeAdminAction)
	if len(audits) != 1 || audits[0].Action != events.AdminActionPictureRemove ||
		audits[0].TargetType != events.AdminTargetCabal || audits[0].TargetID != c.ID.String() ||
		audits[0].Reason != "abusive image" {
		t.Fatalf("admin.action = %+v, want one cabal_picture_remove of the cabal", audits)
	}
}

func TestAdminRemoveCabalPicture_aCabalWithNoPictureChangesNothing(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool)
	_, err := f.routes(nil).RemoveAdminCabalPicture(moderatorContext(t), api.RemoveAdminCabalPictureRequestObject{
		Id: c.ID.UUID(), Body: &apibase.ReasonBody{Reason: "abusive image"},
	})
	if err != nil {
		t.Fatalf("RemoveAdminCabalPicture = %v, want nil", err)
	}
	if n := len(f.updates(t)) + len(decoded[events.AdminAction](t, f, events.TypeAdminAction)); n != 0 {
		t.Fatalf("%d events, want none", n)
	}
}

func TestAdminRemoveCabalPicture_Refusals(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool)
	user := &auth.Actor{Kind: auth.ActorUser, ID: c.Creator.ID.String()}
	admin := &auth.Actor{Kind: auth.ActorAdmin, ID: testkit.NewIDs(9).NewV7().String(), Role: "moderator"}
	for name, tc := range map[string]struct {
		actor  *auth.Actor
		cabal  uuid.UUID
		reason string
		want   errs.Code
	}{
		"no actor":     {nil, c.ID.UUID(), "abusive image", errs.CodeAdminForbidden},
		"a user":       {user, c.ID.UUID(), "abusive image", errs.CodeAdminForbidden},
		"short reason": {admin, c.ID.UUID(), "no", errs.CodeReasonRequired},
		"unknown":      {admin, f.ids.NewV7(), "abusive image", errs.CodeCabalNotFound},
	} {
		ctx := t.Context()
		if tc.actor != nil {
			ctx = auth.WithActor(ctx, *tc.actor)
		}
		_, err := f.routes(nil).RemoveAdminCabalPicture(ctx, api.RemoveAdminCabalPictureRequestObject{
			Id: tc.cabal, Body: &apibase.ReasonBody{Reason: tc.reason},
		})
		if errs.CodeOf(err) != tc.want {
			t.Errorf("%s: RemoveAdminCabalPicture = %v, want %s", name, err, tc.want)
		}
	}
}

func TestAdminRemoveCabalPicture_failsWithInternalWhenTheStoreFails(t *testing.T) {
	t.Parallel()
	for name, breaker := range map[string]string{
		"the lock fails":               `DROP TABLE cabals CASCADE`,
		"the update fails":             `ALTER TABLE cabals ADD CONSTRAINT keeps_picture CHECK (picture_url IS NOT NULL) NOT VALID`,
		"the event cannot be appended": `DROP TABLE events CASCADE`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newAccess(t)
			c := testkit.NewCabal(t, f.pool)
			if _, err := f.pool.Exec(
				t.Context(),
				`UPDATE cabals SET picture_url = 'https://cdn.example.com/p.png' WHERE id = $1`,
				c.ID.UUID(),
			); err != nil {
				t.Fatal(err)
			}
			if _, err := f.pool.Exec(t.Context(), breaker); err != nil {
				t.Fatal(err)
			}
			_, err := f.routes(nil).
				RemoveAdminCabalPicture(moderatorContext(t), api.RemoveAdminCabalPictureRequestObject{
					Id: c.ID.UUID(), Body: &apibase.ReasonBody{Reason: "abusive image"},
				})
			if errs.CodeOf(err) != errs.CodeInternal {
				t.Fatalf("RemoveAdminCabalPicture = %v, want %s", err, errs.CodeInternal)
			}
		})
	}
}
