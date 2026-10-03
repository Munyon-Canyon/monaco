package governance_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func TestHTTP_PostCabalProposal_refusesBeforeAndAfterTheCommand(t *testing.T) {
	t.Parallel()
	h := newProposeHarness(t)
	member := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: h.w.members[0].String()})
	gone := errs.New(errs.CodeCabalNotFound, "test")
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
		"reads fail": {
			func() context.Context { return member },
			api.ProposeTradeRequest{Kind: "buy", Symbol: "AAPLx", UsdcMicros: &usdc, Thesis: &thesis},
			errs.CodeCabalNotFound,
		},
	} {
		adapter := adapters.HTTP{
			Propose: h.handler(h.d.ids),
			Reads:   app.NewProposalReads(h.d.pool, threshold{err: gone}, fakes.NewTrading()),
		}
		req := api.PostCabalProposalRequestObject{Id: h.w.cabal.UUID(), Body: &tc.body}
		if _, err := adapter.PostCabalProposal(tc.caller(), req); errs.CodeOf(err) != tc.want {
			t.Errorf("%s: PostCabalProposal err = %v, want %s", name, err, tc.want)
		}
	}
}
