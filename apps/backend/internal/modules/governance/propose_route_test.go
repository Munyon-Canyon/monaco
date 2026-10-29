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
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
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
			Reads: app.NewProposalReads(
				h.d.pool, fakes.NewTrading(), h.w.cabals.current.Load(),
				marketfake.NewCatalog(marketfake.Fixtures()...),
				&cardBook{}, &returnBook{}, &replyBook{},
			),
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
	return created.CabalName == "Friends pot" && created.Status == api.ProposalStatusOpen &&
		created.Tally.Voters == voters && created.CanVote && created.CanWithdraw && len(created.Voters) == voters
}

func TestHTTP_PostCabalProposal_failsWhenTheProposerCardCannotBeRead(t *testing.T) {
	t.Parallel()
	h := newProposeHarness(t)
	member := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: h.w.members[0].String()})
	usdc := int64(5_000_000)
	adapter := adapters.HTTP{
		Propose: h.handler(h.d.ids),
		Reads: app.NewProposalReads(
			h.d.pool, fakes.NewTrading(), h.w.cabals.current.Load(),
			marketfake.NewCatalog(marketfake.Fixtures()...),
			&cardBook{err: errs.New(errs.CodeInternal, "test")}, &returnBook{}, &replyBook{},
		),
	}
	body := api.ProposeTradeRequest{Kind: "buy", Symbol: "AAPLx", UsdcMicros: &usdc}
	_, err := adapter.PostCabalProposal(member, api.PostCabalProposalRequestObject{Id: h.w.cabal.UUID(), Body: &body})
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("PostCabalProposal err = %v, want %s", err, errs.CodeInternal)
	}
}

func TestHTTP_PostCabalRuleProposal_refusesWithoutAnActorAndWhenTheDetailReadFails(t *testing.T) {
	t.Parallel()
	w := newRuleWorld(t, ruleWorldOpts{})
	adapter := adapters.HTTP{
		RuleChange: w.handler(),
		Reads: app.NewProposalReads(
			w.pool, fakes.NewTrading(), w.fake,
			marketfake.NewCatalog(marketfake.Fixtures()...),
			&cardBook{err: errs.New(errs.CodeInternal, "test")}, &returnBook{}, &replyBook{},
		),
	}
	body := api.RuleChangeRequest{Change: api.RuleChangeRequestChange{Rule: "show_sides", To: true}}
	req := api.PostCabalRuleProposalRequestObject{Id: w.cabal.UUID(), Body: &body}
	if _, err := adapter.PostCabalRuleProposal(t.Context(), req); errs.CodeOf(err) != errs.CodeUnauthorized {
		t.Errorf("PostCabalRuleProposal without an actor err = %v, want %s", err, errs.CodeUnauthorized)
	}
	member := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: w.members[1].String()})
	if _, err := adapter.PostCabalRuleProposal(member, req); errs.CodeOf(err) != errs.CodeInternal {
		t.Errorf("PostCabalRuleProposal with the card read down err = %v, want %s", errs.CodeOf(err), errs.CodeInternal)
	}
}
