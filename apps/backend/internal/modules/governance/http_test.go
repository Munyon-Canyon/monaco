package governance_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/governanceapi"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestHTTP_refusesCallersThatAreNotAUserAndUnknownChoices(t *testing.T) {
	t.Parallel()
	member := testkit.NewIDs(2).NewV7().String()
	for name, tc := range map[string]struct {
		actor  *auth.Actor
		choice api.BallotChoice
		want   errs.Code
	}{
		"no actor":       {nil, "yes", errs.CodeUnauthorized},
		"agent":          {&auth.Actor{Kind: auth.ActorAgent, ID: "a1"}, "yes", errs.CodeForbidden},
		"bad user id":    {&auth.Actor{Kind: auth.ActorUser, ID: "u1"}, "yes", errs.CodeUnauthorized},
		"unknown choice": {&auth.Actor{Kind: auth.ActorUser, ID: member}, "abstain", errs.CodeInvalidInput},
	} {
		ctx := t.Context()
		if tc.actor != nil {
			ctx = auth.WithActor(ctx, *tc.actor)
		}
		req := api.PostProposalVoteRequestObject{Body: &api.CastVoteRequest{Choice: tc.choice}}
		if _, err := (adapters.HTTP{}).PostProposalVote(ctx, req); errs.CodeOf(err) != tc.want {
			t.Errorf("%s: PostProposalVote err = %v, want %s", name, err, tc.want)
		}
	}
}
