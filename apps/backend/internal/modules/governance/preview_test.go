package governance_test

import (
	"math"
	"net/http"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/governanceapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func (w *tradeWorld) preview(query string) string {
	return "/v1/cabals/" + w.cabal.String() + "/proposals/preview?" + query
}

func TestPreview_aBuyOverThePotAdvisesPotExceededAndWritesNothing(t *testing.T) {
	t.Parallel()
	w := newTradeWorld(t)
	w.scenario(t).Given(w.asMember()).
		When(
			scenario.Get(w.preview("kind=buy&symbol=AAPLx&usdc_micros=100000001")),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("advisory_code", "pot_exceeded"),
			scenario.ExpectJSON("advisory_message", errs.Message(errs.CodePotExceeded)),
			scenario.ExpectJSON("quote_out_amount", quoteUnits),
			scenario.ExpectJSON("pot_value_micros", potMicros),
			scenario.Get(w.proposals()),
			scenario.ExpectJSON("proposals", []any{}),
		).
		Then(scenario.ExpectEvents(events.TypeProposalCreated, 0))
}

func TestPreview_answersEachCheckAsAnAdvisoryOrAProblem(t *testing.T) {
	t.Parallel()
	w := newTradeWorld(t)
	advises := func(query string, code, quote any) []scenario.Step {
		return []scenario.Step{
			scenario.Get(w.preview(query)), scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("advisory_code", code), scenario.ExpectJSON("quote_out_amount", quote),
		}
	}
	steps := append(advises("kind=buy&symbol=AAPLx&usdc_micros=5000000", nil, quoteUnits),
		advises("kind=sell&symbol=AAPLx&token_amount=500000001", "insufficient_funds", quoteUnits)...)
	steps = append(steps, advises("kind=buy&symbol=NOPEx&usdc_micros=1", "asset_not_found", nil)...)
	steps = append(steps, advises("kind=buy&symbol=TSLAx&usdc_micros=1", "no_route", nil)...)
	steps = append(steps,
		scenario.Get(w.preview("kind=buy&symbol=AAPLx")), scenario.ExpectProblem(errs.CodeInvalidInput),
		scenario.AsUser("mallory"),
		scenario.Get(w.preview("kind=buy&symbol=AAPLx&usdc_micros=1")), scenario.ExpectProblem(errs.CodeNotCabalMember),
	)
	w.scenario(t).Given(w.asMember()).When(steps...).Then(scenario.ExpectEvents(events.TypeProposalCreated, 0))
}

func TestPreview_passesThroughWhatIsUnavailable(t *testing.T) {
	t.Parallel()
	h := newProposeHarness(t)
	down := errs.New(errs.CodeJupiterUnavailable, "test")
	for name, tc := range map[string]struct {
		arrange func(w *tradeWorld)
		trade   domain.Trade
	}{
		"pot":     {func(w *tradeWorld) { w.treasury.Fail("PotValue", down) }, buyAAPLFor(1)},
		"catalog": {func(w *tradeWorld) { w.catalog.Fail("AssetBySymbol", down) }, buyAAPLFor(1)},
		"balance": {func(w *tradeWorld) { w.chain.Fail("TokenBalance", down) }, sellAAPL(1)},
		"route":   {func(w *tradeWorld) { w.routes.Fail("CheckRoute", down) }, buyAAPLFor(potMicros + 1)},
	} {
		h.w = newTradeWorld(t)
		tc.arrange(h.w)
		req := app.ProposeTrade{CabalID: h.w.cabal, ProposerID: h.w.members[0], Trade: tc.trade}
		if got, err := h.handler(h.d.ids).Preview(t.Context(), req); errs.CodeOf(err) != errs.CodeJupiterUnavailable {
			t.Errorf("%s: Preview = %+v, %v, want jupiter_unavailable", name, got, err)
		}
	}
}

func TestHTTP_GetCabalProposalPreview_refusesWhatCannotGoOnTheWire(t *testing.T) {
	t.Parallel()
	h := newProposeHarness(t)
	usdc := int64(1)
	huge := money.NewBaseUnits(math.MaxUint64, marketfake.AAPLx().Decimals)
	for name, tc := range map[string]struct {
		arrange func(w *tradeWorld)
		params  api.GetCabalProposalPreviewParams
		want    errs.Code
	}{
		"no amount": {
			func(*tradeWorld) {},
			api.GetCabalProposalPreviewParams{Kind: "buy", Symbol: "AAPLx"},
			errs.CodeInvalidInput,
		},
		"pot past int64": {
			func(w *tradeWorld) { w.treasury.SetPotValue(w.cabal, money.MicrosFromUint64(math.MaxUint64)) },
			api.GetCabalProposalPreviewParams{Kind: "buy", Symbol: "AAPLx", UsdcMicros: &usdc},
			errs.CodeDecodeFailed,
		},
		"quote past int64": {
			func(w *tradeWorld) { w.routes.Ok(marketfake.AAPLx().ID, market.RouteCheck{OutAmount: huge}) },
			api.GetCabalProposalPreviewParams{Kind: "buy", Symbol: "AAPLx", UsdcMicros: &usdc},
			errs.CodeDecodeFailed,
		},
	} {
		h.w = newTradeWorld(t)
		tc.arrange(h.w)
		adapter := adapters.HTTP{Propose: h.handler(h.d.ids)}
		member := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: h.w.members[0].String()})
		req := api.GetCabalProposalPreviewRequestObject{Id: h.w.cabal.UUID(), Params: tc.params}
		if _, err := adapter.GetCabalProposalPreview(member, req); errs.CodeOf(err) != tc.want {
			t.Errorf("%s: GetCabalProposalPreview err = %v, want %s", name, err, tc.want)
		}
	}
	req := api.GetCabalProposalPreviewRequestObject{Id: h.w.cabal.UUID()}
	if _, err := (adapters.HTTP{}).GetCabalProposalPreview(
		t.Context(),
		req,
	); errs.CodeOf(
		err,
	) != errs.CodeUnauthorized {
		t.Errorf("anonymous GetCabalProposalPreview err = %v, want unauthorized", err)
	}
}
