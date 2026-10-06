package system_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/system/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/systemapi"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestFlagSystemPing_refusesAnActorThatIsNotAnAdminAndAReasonThatIsTooShort(t *testing.T) {
	t.Parallel()
	admin := auth.Actor{Kind: auth.ActorAdmin, ID: testkit.NewIDs(1).NewV7().String(), Role: "moderator"}
	for _, tc := range []struct {
		name   string
		actor  *auth.Actor
		reason string
		want   errs.Code
	}{
		{"no actor", nil, "spam", errs.CodeAdminForbidden},
		{"a user", &auth.Actor{Kind: auth.ActorUser, ID: admin.ID}, "spam", errs.CodeAdminForbidden},
		{"an admin without a user id", &auth.Actor{Kind: auth.ActorAdmin, ID: "nope"}, "spam", errs.CodeAdminForbidden},
		{"a reason of two characters", &admin, "ab", errs.CodeReasonRequired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			if tc.actor != nil {
				ctx = auth.WithActor(ctx, *tc.actor)
			}
			req := systemapi.FlagSystemPingRequestObject{
				Id:   testkit.NewIDs(2).NewV7(),
				Body: &api.ReasonBody{Reason: tc.reason},
			}
			if _, err := (adapters.HTTP{}).FlagSystemPing(ctx, req); errs.CodeOf(err) != tc.want {
				t.Fatalf("FlagSystemPing err = %v, want %s", err, tc.want)
			}
		})
	}
}
