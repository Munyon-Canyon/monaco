package governance_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const buyAAPL = `{"kind":"buy","symbol":"AAPLx","usdc_micros":5000000,"thesis":"earnings"}`

func (w *tradeWorld) scenario(t *testing.T) *scenario.Scenario {
	t.Helper()
	return scenario.New(t, scenario.WithModules(func(d module.Deps) module.Module {
		return governance.New(d, governance.WithPorts(w.ports()))
	}))
}

func (w *tradeWorld) proposals() string { return "/v1/cabals/" + w.cabal.String() + "/proposals" }

func (w *tradeWorld) refused(t *testing.T, as scenario.Step, body string, code errs.Code) {
	t.Helper()
	w.scenario(t).Given(as).
		When(scenario.Post(w.proposals(), body)).
		Then(scenario.ExpectProblem(code), scenario.ExpectEvents(events.TypeProposalCreated, 0))
}

func (w *tradeWorld) asMember() scenario.Step { return scenario.AsSeededUser("alice", w.members[0]) }

func TestFlow09_ProposeTrade_OK(t *testing.T) {
	t.Parallel()
	w := newTradeWorld(t)
	late := ids.NewUserID(testkit.NewIDs(testkit.RandSeed(t)))
	w.scenario(t).Given(w.asMember()).
		When(
			scenario.Post(w.proposals(), buyAAPL),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.ExpectJSON("status", "open"),
			scenario.ExpectJSON("symbol", "AAPLx"),
			scenario.ExpectJSON("usdc_micros", 5_000_000),
			scenario.ExpectJSON("quote_out_amount", quoteUnits),
			scenario.ExpectJSON("thesis", "earnings"),
			scenario.ExpectJSON("my_ballot", nil),
			scenario.ExpectJSON("can_vote", true),
			scenario.ExpectJSON("tally", map[string]int{"yes": 0, "no": 0, "voters": 3, "needed": 2}),
			scenario.ExpectJSON("voters", w.voters()),
			scenario.Replay(),
			scenario.Remember("id", "proposal"),
			func(*scenario.Scenario) { w.join(late, true) },
			scenario.AsSeededUser("late", late),
			scenario.Get("/v1/proposals/{proposal}"),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("voters", w.voters()),
			scenario.ExpectJSON("can_vote", false),
		).
		Then(
			scenario.ExpectEvents(events.TypeProposalCreated, 1),
			scenario.ExpectEventPayload(events.TypeProposalCreated, map[string]any{
				"kind": "buy", "symbol": "AAPLx", "usdc_micros": "5000000", "voter_count": 3,
			}),
		)
}

func TestFlow09_ProposeTrade_InvalidInput(t *testing.T) {
	t.Parallel()
	w := newTradeWorld(t)
	w.refused(t, w.asMember(), `{"kind":"buy","symbol":"AAPLx","usdc_micros":5000000,"token_amount":1}`,
		errs.CodeInvalidInput)
}

func TestFlow09_ProposeTrade_Unauthorized(t *testing.T) {
	t.Parallel()
	w := newTradeWorld(t)
	w.refused(t, scenario.Anonymous(), buyAAPL, errs.CodeUnauthorized)
}

func TestFlow09_ProposeTrade_NotCabalMember(t *testing.T) {
	t.Parallel()
	w := newTradeWorld(t)
	w.refused(t, scenario.AsUser("mallory"), buyAAPL, errs.CodeNotCabalMember)
}

func TestFlow09_ProposeTrade_AssetNotFound(t *testing.T) {
	t.Parallel()
	w := newTradeWorld(t)
	w.refused(t, w.asMember(), `{"kind":"buy","symbol":"NOPEx","usdc_micros":5000000}`, errs.CodeAssetNotFound)
}

func TestFlow09_ProposeTrade_AssetUntradable(t *testing.T) {
	t.Parallel()
	w := newTradeWorld(t)
	w.refused(t, w.asMember(), `{"kind":"buy","symbol":"JPSTx","usdc_micros":5000000}`, errs.CodeAssetUntradable)
}

func TestFlow09_ProposeTrade_NoRoute(t *testing.T) {
	t.Parallel()
	w := newTradeWorld(t)
	w.refused(t, w.asMember(), `{"kind":"buy","symbol":"TSLAx","usdc_micros":5000000}`, errs.CodeNoRoute)
}

func TestFlow09_ProposeTrade_PotExceeded(t *testing.T) {
	t.Parallel()
	w := newTradeWorld(t)
	w.refused(t, w.asMember(), `{"kind":"buy","symbol":"AAPLx","usdc_micros":100000001}`, errs.CodePotExceeded)
}

func TestFlow09_ProposeTrade_InsufficientFunds(t *testing.T) {
	t.Parallel()
	w := newTradeWorld(t)
	w.refused(t, w.asMember(), `{"kind":"sell","symbol":"AAPLx","token_amount":500000001}`,
		errs.CodeInsufficientFunds)
}

func TestFlow09_ProposeTrade_JupiterUnavailable(t *testing.T) {
	t.Parallel()
	w := newTradeWorld(t)
	w.routes.Fail("CheckRoute", errs.New(errs.CodeJupiterUnavailable, "test"))
	w.refused(t, w.asMember(), buyAAPL, errs.CodeJupiterUnavailable)
}

func TestFlow09_ProposeTrade_PriceUnavailable(t *testing.T) {
	t.Parallel()
	w := newTradeWorld(t)
	w.treasury.Fail("PotValue", errs.New(errs.CodePriceUnavailable, "test"))
	w.refused(t, w.asMember(), buyAAPL, errs.CodePriceUnavailable)
}

func (w *tradeWorld) voters() []map[string]any {
	sorted := slices.Clone(w.members)
	slices.SortFunc(sorted, func(a, b ids.UserID) int { return strings.Compare(a.String(), b.String()) })
	out := make([]map[string]any, len(sorted))
	for i, v := range sorted {
		out[i] = map[string]any{"user_id": v.String(), "choice": nil, "cast_at": nil}
	}
	return out
}
