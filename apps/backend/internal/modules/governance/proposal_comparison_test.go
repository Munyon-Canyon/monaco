package governance_test

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/governanceapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const comparisonTTL = 15 * time.Second

type countingRouter struct {
	gate  chan struct{}
	mu    sync.Mutex
	c     multichain.Comparison
	err   error
	calls int
}

func (r *countingRouter) Best(
	context.Context, multichain.StockID, multichain.Side, money.Micros,
) (multichain.Comparison, error) {
	if r.gate != nil {
		<-r.gate
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	return r.c, r.err
}

func (r *countingRouter) quote(c multichain.Comparison) scenario.Step {
	return func(*scenario.Scenario) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.c = c
	}
}

func (r *countingRouter) fail(err error) scenario.Step {
	return func(*scenario.Scenario) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.err = err
	}
}

func (r *countingRouter) expectCalls(t *testing.T, want int) scenario.Step {
	t.Helper()
	return func(*scenario.Scenario) {
		t.Helper()
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.calls != want {
			t.Fatalf("router calls = %d, want %d", r.calls, want)
		}
	}
}

func (w *tradeWorld) clockedScenario(t *testing.T, router multichain.Router, clk clock.Clock) *scenario.Scenario {
	t.Helper()
	ports := w.ports()
	ports.Router = router
	return scenario.New(t, scenario.WithModules(func(d module.Deps) module.Module {
		d.Clock = clk
		return governance.New(d, governance.WithPorts(ports))
	}))
}

func (w *tradeWorld) liveComparison() string { return w.proposals() + "/{proposal}/price-comparison" }

func expectShares(t *testing.T, first string) func(*scenario.Scenario, json.RawMessage) {
	t.Helper()
	return func(_ *scenario.Scenario, raw json.RawMessage) {
		t.Helper()
		if _, quotes := comparisonOf(t, raw); len(quotes) == 0 || quotes[0] != first {
			t.Fatalf("price_comparison = %s, want best %s", raw, first)
		}
	}
}

func TestProposalComparison_quotesNowAndReusesTheQuoteFor15Seconds(t *testing.T) {
	t.Parallel()
	w := newTradeWorld(t)
	clk := testkit.NewClock(clock.Real{}.Now().UTC())
	proposed := appleComparison(t)
	now := appleComparison(t)
	now.Ranked[0].NetShares = 260_000_000
	later := appleComparison(t)
	later.Ranked[0].NetShares = 270_000_000
	router := &countingRouter{c: proposed}
	w.clockedScenario(t, router, clk).Given(w.asMember()).
		When(
			scenario.Post(w.proposals(), buyAAPL),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.Remember("id", "proposal"),
			router.quote(now),
			scenario.Get(w.liveComparison()),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectField("price_comparison", expectShares(t, "coinbase:260000000")),
			scenario.ExpectJSON("quoted_at", clk.Now().Format(time.RFC3339Nano)),
			router.expectCalls(t, 2),
			scenario.Get("/v1/proposals/{proposal}"),
			scenario.ExpectField("price_comparison", expectAppleComparison(t, proposed)),
			router.quote(later),
			func(*scenario.Scenario) { clk.Advance(14 * time.Second) },
			scenario.Get(w.liveComparison()),
			scenario.ExpectField("price_comparison", expectShares(t, "coinbase:260000000")),
			router.expectCalls(t, 2),
			func(*scenario.Scenario) { clk.Advance(time.Second) },
			scenario.Get(w.liveComparison()),
			scenario.ExpectField("price_comparison", expectShares(t, "coinbase:270000000")),
			router.expectCalls(t, 3),
			scenario.Get("/v1/proposals/{proposal}"),
			scenario.ExpectField("price_comparison", expectAppleComparison(t, proposed)),
			func(*scenario.Scenario) { clk.Advance(comparisonTTL) },
			scenario.Post(w.proposals(), buyAAPL),
			scenario.Remember("id", "proposal"),
			scenario.Get(w.liveComparison()),
			scenario.ExpectStatus(http.StatusOK),
			router.expectCalls(t, 5),
		)
}

func TestProposalComparison_refusesWhatTheCatalogOrJupiterRefuses(t *testing.T) {
	t.Parallel()
	w := newTradeWorld(t)
	aapl := marketfake.AAPLx()
	renamed := aapl
	renamed.Symbol = "AAPLz"
	router := &countingRouter{c: appleComparison(t)}
	w.clockedScenario(t, router, clock.Real{}).Given(w.asMember()).
		When(
			scenario.Post(w.proposals(), buyAAPL),
			scenario.Remember("id", "proposal"),
			router.fail(errs.New(errs.CodeNoRoute, "test")),
			func(*scenario.Scenario) { w.routes.ProbeFails(aapl.ID) },
			scenario.Get(w.liveComparison()),
			scenario.ExpectProblem(errs.CodeNoRoute),
			func(*scenario.Scenario) { w.routes.Paused(aapl.ID) },
			scenario.Get(w.liveComparison()),
			scenario.ExpectProblem(errs.CodeAssetPaused),
			router.fail(errs.New(errs.CodeQuoteUnavailable, "test")),
			scenario.Get(w.liveComparison()),
			scenario.ExpectProblem(errs.CodeQuoteUnavailable),
			func(*scenario.Scenario) { w.catalog.Put(renamed) },
			scenario.Get(w.liveComparison()),
			scenario.ExpectProblem(errs.CodeAssetNotFound),
		)
}

func TestProposalComparison_fallsBackToTheSolanaQuoteWhenRoutingFails(t *testing.T) {
	t.Parallel()
	w := newTradeWorld(t)
	router := &countingRouter{err: errs.New(errs.CodeNoRoute, "test")}
	w.clockedScenario(t, router, clock.Real{}).Given(w.asMember()).
		When(
			scenario.Post(w.proposals(), buyAAPL),
			scenario.Remember("id", "proposal"),
			scenario.Get(w.liveComparison()),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("quote_out_amount", quoteUnits),
			scenario.ExpectNoField("price_comparison"),
			router.expectCalls(t, 2),
		)
}

func TestProposalComparison_withoutARouterQuotesTheSolanaMint(t *testing.T) {
	t.Parallel()
	w := newTradeWorld(t)
	clk := testkit.NewClock(clock.Real{}.Now().UTC())
	w.clockedScenario(t, nil, clk).Given(w.asMember()).
		When(
			scenario.Post(w.proposals(), buyAAPL),
			scenario.Remember("id", "proposal"),
			scenario.Get(w.liveComparison()),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("quote_out_amount", quoteUnits),
			scenario.ExpectNoField("price_comparison"),
			func(*scenario.Scenario) { w.routes.ProbeFails(marketfake.AAPLx().ID) },
			func(*scenario.Scenario) { clk.Advance(comparisonTTL) },
			scenario.Get(w.liveComparison()),
			scenario.ExpectProblem(errs.CodeNoRoute),
		)
}

func TestProposalComparison_servesOnlyAnOpenBuyToAMember(t *testing.T) {
	t.Parallel()
	w := newTradeWorld(t)
	router := &countingRouter{c: appleComparison(t)}
	closeIt := func(s *scenario.Scenario) {
		if _, err := s.DB().Exec(t.Context(), `UPDATE proposals SET status = 'withdrawn' WHERE id = $1`,
			s.Recall("proposal")); err != nil {
			t.Fatal(err)
		}
	}
	w.clockedScenario(t, router, clock.Real{}).Given(w.asMember()).
		When(
			scenario.Post(w.proposals(), `{"kind":"sell","symbol":"AAPLx","token_amount":1000}`),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.Remember("id", "proposal"),
			scenario.Get(w.liveComparison()),
			scenario.ExpectProblem(errs.CodeNotFound),
			scenario.Post(w.proposals(), buyAAPL),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.Remember("id", "proposal"),
			scenario.AsUser("mallory"),
			scenario.Get(w.liveComparison()),
			scenario.ExpectProblem(errs.CodeNotCabalMember),
			w.asMember(),
			closeIt,
			scenario.Get(w.liveComparison()),
			scenario.ExpectProblem(errs.CodeNotFound),
			func(s *scenario.Scenario) {
				d := proposalDB{q: sqlc.New(s.DB()), ids: testkit.NewIDs(5201), now: clock.Real{}.Now().UTC()}
				elsewhere := d.buy(d.ids.NewV7())
				d.insert(t, elsewhere)
				scenario.Get(w.proposals() + "/" + elsewhere.ID.String() + "/price-comparison")(s)
			},
			scenario.ExpectProblem(errs.CodeProposalNotFound),
			scenario.Get(w.proposals()+"/"+uuid.NewString()+"/price-comparison"),
			scenario.ExpectProblem(errs.CodeProposalNotFound),
			router.expectCalls(t, 1),
		)
}

func TestProposalComparison_refusesAQuotePastInt64AndAnAnonymousCaller(t *testing.T) {
	t.Parallel()
	w := newTradeWorld(t)
	aapl := marketfake.AAPLx()
	huge := market.RouteCheck{OutAmount: money.NewBaseUnits(math.MaxUint64, aapl.Decimals)}
	w.scenario(t).Given(w.asMember()).
		When(
			scenario.Post(w.proposals(), buyAAPL),
			scenario.Remember("id", "proposal"),
			func(*scenario.Scenario) { w.routes.Ok(aapl.ID, huge) },
			scenario.Get(w.liveComparison()),
			scenario.ExpectProblem(errs.CodeDecodeFailed),
		)
	req := governanceapi.GetCabalProposalPriceComparisonRequestObject{
		Id: w.cabal.UUID(), ProposalId: testkit.NewIDs(5203).NewV7(),
	}
	if _, err := (adapters.HTTP{}).GetCabalProposalPriceComparison(t.Context(), req); errs.CodeOf(err) !=
		errs.CodeUnauthorized {
		t.Errorf("anonymous GetCabalProposalPriceComparison err = %v, want unauthorized", err)
	}
}

func TestProposalComparison_concurrentViewersShareOneRouterCall(t *testing.T) {
	t.Parallel()
	const viewers = 8
	h := newProposeHarness(t)
	p := h.d.buy(h.w.members[0].UUID())
	p.CabalID = h.w.cabal.UUID()
	h.d.insert(t, p)
	router := &countingRouter{gate: make(chan struct{}), c: appleComparison(t)}
	ports := h.w.ports()
	ports.Router = router
	propose := app.NewProposeTradeHandler(db.New(h.d.pool, h.d.ids, clock.Real{}), h.d.ids, clock.Real{}, ports,
		app.NoHints{})
	comparisons := app.NewProposalComparisons(h.d.pool, propose, nil)
	req := app.GetProposalComparison{
		CabalID: h.w.cabal, ProposalID: ids.ProposalIDFrom(p.ID), Caller: h.w.members[0],
	}
	var started, done sync.WaitGroup
	quotedAt := make([]time.Time, viewers)
	for i := range viewers {
		started.Add(1)
		done.Go(func() {
			started.Done()
			live, err := comparisons.Get(t.Context(), req)
			if err != nil {
				t.Errorf("viewer %d: %v", i, err)
			}
			quotedAt[i] = live.QuotedAt
		})
	}
	started.Wait()
	close(router.gate)
	done.Wait()
	router.expectCalls(t, 1)(nil)
	for i, at := range quotedAt {
		if !at.Equal(quotedAt[0]) {
			t.Fatalf("viewer %d got the quote from %s, viewer 0 from %s", i, at, quotedAt[0])
		}
	}
}
