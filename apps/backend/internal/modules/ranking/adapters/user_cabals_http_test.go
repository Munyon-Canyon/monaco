package adapters_test

import (
	"context"
	"math"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/rankingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type noBlocks struct{}

func (noBlocks) BlockedByMe(context.Context, ids.UserID, ids.UserID) (bool, error) { return false, nil }

func TestGetUserCabals_needsASignedInUser(t *testing.T) {
	t.Parallel()
	h := historyHTTP(0)
	if _, err := h.GetUserCabals(
		t.Context(),
		api.GetUserCabalsRequestObject{},
	); errs.CodeOf(
		err,
	) != errs.CodeUnauthorized {
		t.Fatalf("anonymous: err = %v, want unauthorized", err)
	}
}

func TestGetUserCabals_failsOnAMemberCountThatDoesNotFit(t *testing.T) {
	t.Parallel()
	viewer := ids.UserIDFrom(ids.Real{}.NewV7())
	other := ids.UserIDFrom(ids.Real{}.NewV7())
	cabalID := ids.CabalIDFrom(ids.Real{}.NewV7())
	h := historyHTTP(0)
	h.Users = stubUsers{known: map[ids.UserID]app.UserCard{other: {ID: other, SharePerformance: true}}}
	h.Blocks = noBlocks{}
	h.Members = stubMembers{cabalID}
	h.Cards = stubCards{cabalID: {ID: cabalID, Name: "Alpha", MemberCount: math.MaxInt64}}
	h.Roles = stubRoles{}
	h.Ownership = stubOwnership{}
	ctx := asActor(t.Context(), auth.ActorUser, viewer.String())
	if _, err := h.GetUserCabals(
		ctx,
		api.GetUserCabalsRequestObject{Id: other.UUID()},
	); errs.CodeOf(
		err,
	) != errs.CodeInternal {
		t.Fatalf("err = %v, want internal", err)
	}
}
