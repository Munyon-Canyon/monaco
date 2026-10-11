package governance_test

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

type stubRouter struct {
	t   *testing.T
	c   multichain.Comparison
	err error
}

func (r stubRouter) Best(
	_ context.Context, stock multichain.StockID, side multichain.Side, amount money.Micros,
) (multichain.Comparison, error) {
	if side != multichain.SideBuy || stock.String() != marketfake.AAPLx().ID.String() || amount.Uint64() != 5_000_000 {
		r.t.Errorf("Best(%s, %s, %s) is not the proposed buy", stock, side, amount)
	}
	return r.c, r.err
}

func appleComparison(t *testing.T) multichain.Comparison {
	t.Helper()
	g := testkit.NewIDs(4980)
	var stock multichain.StockID
	if err := stock.UnmarshalText([]byte(marketfake.AAPLx().ID.String())); err != nil {
		t.Fatal(err)
	}
	at := clock.Real{}.Now().UTC().Truncate(time.Second)
	quote := func(issuer string, n chain.Network, v multichain.Venue, shares multichain.ShareNanos) multichain.Quote {
		return multichain.Quote{
			ListingID:   multichain.ListingIDFrom(g.NewV7()),
			Issuer:      issuer,
			Network:     n,
			Venue:       v,
			Side:        multichain.SideBuy,
			InputMicros: money.MicrosFromUint64(5_000_000),
			GrossOutput: big.NewInt(1),
			Costs:       []multichain.Cost{{Kind: multichain.CostBridge, Micros: money.MicrosFromUint64(3_750)}},
			NetShares:   shares,
			NetMicros:   money.MicrosFromUint64(5_000_000),
			QuotedAt:    at,
			ExpiresAt:   at.Add(30 * time.Second),
		}
	}
	base := quote("coinbase", chain.NetworkBase, multichain.VenueUniswap, 250_000_000)
	return multichain.Comparison{
		StockID: stock, Side: multichain.SideBuy, Winner: base.ListingID,
		Ranked: []multichain.Quote{base, quote("xstocks", chain.NetworkSolana, multichain.VenueJupiter, 249_000_000)},
	}
}

func (w *tradeWorld) routedScenario(t *testing.T, router multichain.Router) *scenario.Scenario {
	t.Helper()
	ports := w.ports()
	ports.Router = router
	return scenario.New(t, scenario.WithModules(func(d module.Deps) module.Module {
		return governance.New(d, governance.WithPorts(ports))
	}))
}

func comparisonOf(t *testing.T, raw json.RawMessage) (winner string, issuers []string) {
	t.Helper()
	var got struct {
		Winner string `json:"winner_listing_id"`
		Quotes []struct {
			Issuer        string `json:"issuer"`
			NetShareNanos string `json:"net_share_nanos"`
			Costs         []struct {
				Kind   string `json:"kind"`
				Micros string `json:"micros"`
			} `json:"costs"`
		} `json:"quotes"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("price_comparison %s: %v", raw, err)
	}
	for _, q := range got.Quotes {
		issuers = append(issuers, q.Issuer+":"+q.NetShareNanos)
	}
	return got.Winner, issuers
}

func expectAppleComparison(t *testing.T, c multichain.Comparison) func(*scenario.Scenario, json.RawMessage) {
	t.Helper()
	return func(_ *scenario.Scenario, raw json.RawMessage) {
		t.Helper()
		winner, issuers := comparisonOf(t, raw)
		if winner != c.Winner.String() || len(issuers) != 2 || issuers[0] != "coinbase:250000000" ||
			issuers[1] != "xstocks:249000000" {
			t.Fatalf("price_comparison = %s", raw)
		}
	}
}

func expectListedComparison(t *testing.T, want bool) func(*scenario.Scenario, json.RawMessage) {
	t.Helper()
	return func(_ *scenario.Scenario, raw json.RawMessage) {
		t.Helper()
		var list []map[string]json.RawMessage
		if err := json.Unmarshal(raw, &list); err != nil || len(list) != 1 {
			t.Fatalf("proposals = %s, %v", raw, err)
		}
		if _, ok := list[0]["price_comparison"]; ok != want {
			t.Fatalf("listed proposal has price_comparison = %v, want %v: %s", ok, want, raw)
		}
	}
}

func TestProposeTrade_storesAndShowsTheBestPriceAcrossVenues(t *testing.T) {
	t.Parallel()
	w := newTradeWorld(t)
	c := appleComparison(t)
	w.routedScenario(t, stubRouter{t: t, c: c}).Given(w.asMember()).
		When(
			scenario.Post(w.proposals(), buyAAPL),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.ExpectJSON("quote_out_amount", 25_000_000),
			scenario.Get(w.proposals()),
			scenario.ExpectField("proposals", expectListedComparison(t, true)),
			scenario.Get(w.preview("kind=buy&symbol=AAPLx&usdc_micros=5000000")),
			scenario.ExpectJSON("quote_out_amount", 25_000_000),
			scenario.ExpectField("price_comparison", expectAppleComparison(t, c)),
		)
}

func TestProposeTrade_keepsTheSolanaQuoteWhenRoutingHasNothing(t *testing.T) {
	t.Parallel()
	w := newTradeWorld(t)
	w.routedScenario(t, stubRouter{t: t, err: errs.New(errs.CodeNoRoute, "test")}).Given(w.asMember()).
		When(
			scenario.Post(w.proposals(), buyAAPL),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.ExpectJSON("quote_out_amount", quoteUnits),
			scenario.Get(w.proposals()),
			scenario.ExpectField("proposals", expectListedComparison(t, false)),
		)
}

func TestProposeTrade_refusesAComparisonItCannotStore(t *testing.T) {
	t.Parallel()
	w := newTradeWorld(t)
	c := appleComparison(t)
	c.StockID = multichain.StockID{}
	w.routedScenario(t, stubRouter{t: t, c: c}).Given(w.asMember()).
		When(scenario.Post(w.proposals(), buyAAPL)).
		Then(scenario.ExpectProblem(errs.CodeInternal), scenario.ExpectEvents(events.TypeProposalCreated, 0))
}

func TestCastVote_carriesTheStoredComparisonOnThePassEvent(t *testing.T) {
	t.Parallel()
	s := scenario.New(t, withGovernance())
	d := proposalDB{q: sqlc.New(s.DB()), ids: testkit.NewIDs(4981), now: clock.Real{}.Now().UTC()}
	voter, err := ids.ParseUserID(d.ids.NewV7().String())
	if err != nil {
		t.Fatal(err)
	}
	p := d.buy(voter.UUID())
	d.insert(t, p)
	c := appleComparison(t)
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().
		Exec(t.Context(), `UPDATE proposals SET price_comparison = $2 WHERE id = $1`, p.ID, raw); err != nil {
		t.Fatal(err)
	}
	var want map[string]any
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	s.Given(scenario.AsSeededUser("alice", voter)).
		When(scenario.Post("/v1/proposals/"+p.ID.String()+"/votes", `{"choice":"yes"}`)).
		Then(
			scenario.ExpectJSON("status", "passed"),
			scenario.ExpectEventPayload(events.TypeProposalPassed, map[string]any{"price_comparison": want}),
		)
}

func TestCastVote_refusesAStoredComparisonItCannotRead(t *testing.T) {
	t.Parallel()
	s := scenario.New(t, withGovernance())
	d := proposalDB{q: sqlc.New(s.DB()), ids: testkit.NewIDs(4982), now: clock.Real{}.Now().UTC()}
	voter, err := ids.ParseUserID(d.ids.NewV7().String())
	if err != nil {
		t.Fatal(err)
	}
	p := d.buy(voter.UUID())
	d.insert(t, p)
	if _, err := s.DB().Exec(t.Context(), `UPDATE proposals SET price_comparison = '{"ranked":"x"}' WHERE id = $1`,
		p.ID); err != nil {
		t.Fatal(err)
	}
	s.Given(scenario.AsSeededUser("alice", voter)).
		When(
			scenario.Post("/v1/proposals/"+p.ID.String()+"/votes", `{"choice":"yes"}`),
			scenario.ExpectProblem(errs.CodeDecodeFailed),
			scenario.Get("/v1/cabals/"+p.CabalID.String()+"/proposals"),
			scenario.ExpectProblem(errs.CodeDecodeFailed),
		).
		Then(scenario.ExpectEvents(events.TypeProposalPassed, 0))
}
