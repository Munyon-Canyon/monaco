package governance_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	buyAAPLOnBase     = `{"kind":"buy","symbol":"AAPLx","usdc_micros":5000000,"network":"base"}`
	buyAAPLOnArbitrum = `{"kind":"buy","symbol":"AAPLx","usdc_micros":5000000,"network":"arbitrum"}`
	sellAAPLOnBase    = `{"kind":"sell","symbol":"AAPLx","token_amount":1000,"network":"base"}`
	solanaQuoteOut    = 25_000_000
	baseQuoteOut      = 24_900_000
)

func solanaFirstComparison(t *testing.T) multichain.Comparison {
	t.Helper()
	c := appleComparison(t)
	base, solana := c.Ranked[0], c.Ranked[1]
	base.NetShares, solana.NetShares = 249_000_000, 250_000_000
	c.Ranked, c.Winner = []multichain.Quote{solana, base}, solana.ListingID
	return c
}

func expectWinner(t *testing.T, want multichain.ListingID) func(*scenario.Scenario, json.RawMessage) {
	t.Helper()
	return func(_ *scenario.Scenario, raw json.RawMessage) {
		t.Helper()
		if winner, _ := comparisonOf(t, raw); winner != want.String() {
			t.Fatalf("price_comparison winner = %s, want %s: %s", winner, want, raw)
		}
	}
}

func expectListedNetwork(t *testing.T, want string) func(*scenario.Scenario, json.RawMessage) {
	t.Helper()
	return func(_ *scenario.Scenario, raw json.RawMessage) {
		t.Helper()
		var list []struct {
			Network string `json:"network"`
		}
		if err := json.Unmarshal(raw, &list); err != nil || len(list) != 1 || list[0].Network != want {
			t.Fatalf("proposals = %s, %v, want one on %s", raw, err, want)
		}
	}
}

func proposalRows(t *testing.T, s *scenario.Scenario, cabal ids.CabalID) int {
	t.Helper()
	var n int
	if err := s.DB().QueryRow(t.Context(), `SELECT count(*) FROM proposals WHERE cabal_id = $1`,
		cabal.UUID()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestProposeTrade_pinsABuyToItsChainAndQuotesThatChain(t *testing.T) {
	t.Parallel()
	w := newTradeWorld(t)
	c := solanaFirstComparison(t)
	s := w.routedScenario(t, stubRouter{t: t, c: c})
	s.Given(w.asMember()).
		When(
			scenario.Post(w.proposals(), buyAAPLOnBase),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.ExpectJSON("network", "base"),
			scenario.ExpectJSON("quote_out_amount", baseQuoteOut),
			scenario.ExpectField("price_comparison", expectWinner(t, c.Winner)),
			scenario.Remember("id", "proposal"),
			scenario.Get("/v1/proposals/{proposal}"),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("network", "base"),
			scenario.ExpectJSON("quote_out_amount", baseQuoteOut),
			scenario.ExpectField("price_comparison", expectWinner(t, c.Winner)),
			scenario.Get(w.proposals()),
			scenario.ExpectField("proposals", expectListedNetwork(t, "base")),
		)
	var stored multichain.Comparison
	var raw []byte
	var network string
	if err := s.DB().QueryRow(t.Context(), `SELECT price_comparison, network FROM proposals WHERE cabal_id = $1`,
		w.cabal.UUID()).Scan(&raw, &network); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &stored); err != nil || stored.Winner != c.Winner || network != "base" {
		t.Fatalf("stored winner %s on %q, %v, want %s on base", stored.Winner, network, err, c.Winner)
	}
}

func TestProposeTrade_leavesAnUnpinnedBuyOffTheNetworkField(t *testing.T) {
	t.Parallel()
	w := newTradeWorld(t)
	w.routedScenario(t, stubRouter{t: t, c: solanaFirstComparison(t)}).Given(w.asMember()).
		When(
			scenario.Post(w.proposals(), buyAAPL),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.ExpectNoField("network"),
			scenario.ExpectJSON("quote_out_amount", solanaQuoteOut),
			scenario.Remember("id", "proposal"),
			scenario.Get("/v1/proposals/{proposal}"),
			scenario.ExpectNoField("network"),
		)
}

func TestProposeTrade_refusesAPinItCannotHonor(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		router stubRouter
		body   string
		want   errs.Code
	}{
		{"no arbitrum quote", stubRouter{c: solanaFirstComparison(t)}, buyAAPLOnArbitrum, errs.CodeInvalidNetwork},
		{
			"router failed",
			stubRouter{err: errs.New(errs.CodeQuoteUnavailable, "test")},
			buyAAPLOnBase,
			errs.CodeQuoteUnavailable,
		},
		{"sell", stubRouter{c: solanaFirstComparison(t)}, sellAAPLOnBase, errs.CodeInvalidNetwork},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := newTradeWorld(t)
			tc.router.t = t
			s := w.routedScenario(t, tc.router)
			s.Given(w.asMember()).
				When(scenario.Post(w.proposals(), tc.body)).
				Then(scenario.ExpectProblem(tc.want), scenario.ExpectEvents(events.TypeProposalCreated, 0))
			if n := proposalRows(t, s, w.cabal); n != 0 {
				t.Fatalf("proposals rows = %d, want 0", n)
			}
		})
	}
}

func TestPreview_quotesThePinnedChainAndAdvisesAMissingOne(t *testing.T) {
	t.Parallel()
	w := newTradeWorld(t)
	w.routedScenario(t, stubRouter{t: t, c: solanaFirstComparison(t)}).Given(w.asMember()).
		When(
			scenario.Get(w.preview("kind=buy&symbol=AAPLx&usdc_micros=5000000&network=arbitrum")),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("advisory_code", "invalid_network"),
			scenario.ExpectJSON("advisory_message", errs.Message(errs.CodeInvalidNetwork)),
			scenario.Get(w.preview("kind=buy&symbol=AAPLx&usdc_micros=5000000&network=base")),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("advisory_code", nil),
			scenario.ExpectJSON("quote_out_amount", baseQuoteOut),
		)
}

func TestProposalComparison_quotesThePinnedChain(t *testing.T) {
	t.Parallel()
	w := newTradeWorld(t)
	router := &countingRouter{c: solanaFirstComparison(t)}
	w.clockedScenario(t, router, clock.Real{}).Given(w.asMember()).
		When(
			scenario.Post(w.proposals(), buyAAPLOnBase),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.Remember("id", "proposal"),
			scenario.Get(w.liveComparison()),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("quote_out_amount", baseQuoteOut),
		)
}

func TestCastVote_carriesThePinOnThePassEvent(t *testing.T) {
	t.Parallel()
	s := scenario.New(t, withGovernance())
	d := proposalDB{q: sqlc.New(s.DB()), ids: testkit.NewIDs(5263), now: clock.Real{}.Now().UTC()}
	voter, err := ids.ParseUserID(d.ids.NewV7().String())
	if err != nil {
		t.Fatal(err)
	}
	p := d.buy(voter.UUID())
	d.insert(t, p)
	if _, err := s.DB().Exec(t.Context(), `UPDATE proposals SET network = $2 WHERE id = $1`,
		p.ID, string(chain.NetworkBase)); err != nil {
		t.Fatal(err)
	}
	s.Given(scenario.AsSeededUser("alice", voter)).
		When(scenario.Post("/v1/proposals/"+p.ID.String()+"/votes", `{"choice":"yes"}`)).
		Then(
			scenario.ExpectJSON("status", "passed"),
			scenario.ExpectEventPayload(events.TypeProposalPassed, map[string]any{"network": "base"}),
		)
}
