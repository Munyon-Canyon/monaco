package governance_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/governanceapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestHTTP_PostCabalProposal_refusesBeforeAndAfterTheCommand(t *testing.T) {
	t.Parallel()
	h := newProposeHarness(t)
	member := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: h.w.members[0].String()})
	thesis := "earnings"
	usdc := int64(5_000_000)
	stranger := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: ids.NewUserID(h.d.ids).String()})
	for name, tc := range map[string]struct {
		caller func() context.Context
		body   api.ProposeTradeRequest
		want   errs.Code
	}{
		"no actor": {t.Context, api.ProposeTradeRequest{Kind: "buy", Symbol: "AAPLx"}, errs.CodeUnauthorized},
		"no amount": {
			func() context.Context { return member },
			api.ProposeTradeRequest{Kind: "buy", Symbol: "AAPLx"},
			errs.CodeInvalidInput,
		},
		"blank symbol": {
			func() context.Context { return member },
			api.ProposeTradeRequest{Kind: "buy", Symbol: " ", UsdcMicros: &usdc},
			errs.CodeInvalidInput,
		},
		"not a member": {
			func() context.Context { return stranger },
			api.ProposeTradeRequest{Kind: "buy", Symbol: "AAPLx", UsdcMicros: &usdc},
			errs.CodeNotCabalMember,
		},
		"creation does not reread": {
			func() context.Context { return member },
			api.ProposeTradeRequest{Kind: "buy", Symbol: "AAPLx", UsdcMicros: &usdc, Thesis: &thesis},
			"",
		},
	} {
		adapter := adapters.HTTP{
			Propose: h.handler(h.d.ids),
			Reads:   app.NewProposalReads(h.d.pool, fakes.NewTrading()),
		}
		req := api.PostCabalProposalRequestObject{Id: h.w.cabal.UUID(), Body: &tc.body}
		got, err := adapter.PostCabalProposal(tc.caller(), req)
		if (err == nil && tc.want != "") || (err != nil && errs.CodeOf(err) != tc.want) {
			t.Errorf("%s: PostCabalProposal err = %v, want %s", name, err, tc.want)
		}
		if name == "creation does not reread" {
			created, ok := got.(api.PostCabalProposal201JSONResponse)
			if !ok || !openedProposal(created, len(h.w.members)) {
				t.Errorf("PostCabalProposal response = %#v, want opened proposal", got)
			}
		}
	}
}

func openedProposal(created api.PostCabalProposal201JSONResponse, voters int) bool {
	return created.Status == api.ProposalStatusOpen && created.Tally.Voters == voters && created.CanVote &&
		created.CanWithdraw && len(created.Voters) == voters
}
