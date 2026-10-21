package governance_test

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"sync"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	baseToken      = "0x00000000000000000000000000000000005851aa"
	robinhoodToken = "0x00000000000000000000000000000000005851bb"
	evmHeldUnits   = 305_000_000
	unitsPerMicro  = 2
	voteYes        = `{"choice":"yes"}`
)

var (
	_ app.EVMHoldings   = (*evmTreasury)(nil)
	_ app.Listings      = listingBook(nil)
	_ multichain.Quoter = (*saleQuoter)(nil)
)

type evmTreasury struct {
	*fakes.Treasury
	positions []treasury.ListingPosition
	err       error
}

func (e *evmTreasury) EVMPositions(context.Context, ids.CabalID) ([]treasury.ListingPosition, error) {
	if e.err != nil {
		return nil, e.err
	}
	return append([]treasury.ListingPosition{}, e.positions...), nil
}

type listingBook map[string]multichain.Listing

const brokenKey = "base:0xbroken"

func (b listingBook) ListingByAsset(_ context.Context, asset string) (multichain.Listing, error) {
	if asset == brokenKey {
		return multichain.Listing{}, errs.New(errs.CodeUpstreamUnavailable, "test.ListingByAsset")
	}
	if l, ok := b[asset]; ok {
		return l, nil
	}
	return multichain.Listing{}, errs.New(errs.CodeAssetNotFound, "test.ListingByAsset")
}

type saleQuoter struct {
	t     *testing.T
	mu    sync.Mutex
	err   error
	asked int
}

func (q *saleQuoter) Quote(
	_ context.Context, l multichain.Listing, side multichain.Side, micros money.Micros,
) (multichain.Quote, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.asked++
	if side != multichain.SideSell {
		q.t.Errorf("Quote(%s, %s) is not a sale", l.ID, side)
	}
	if q.err != nil {
		return multichain.Quote{}, q.err
	}
	return multichain.Quote{
		ListingID: l.ID, GrossOutput: new(big.Int).SetUint64(micros.Uint64() * unitsPerMicro),
	}, nil
}

func (q *saleQuoter) timesAsked() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.asked
}

type evmSellWorld struct {
	*tradeWorld
	evm      *evmTreasury
	base     multichain.Listing
	robinhod multichain.Listing
	listings listingBook
	quoter   *saleQuoter
}

func newEVMSellWorld(t *testing.T) *evmSellWorld {
	t.Helper()
	w := newTradeWorld(t)
	var stock multichain.StockID
	if err := stock.UnmarshalText([]byte(marketfake.AAPLx().ID.String())); err != nil {
		t.Fatal(err)
	}
	g := testkit.NewIDs(testkit.RandSeed(t))
	listing := func(n chain.Network, token string) multichain.Listing {
		return multichain.Listing{
			ID: multichain.ListingIDFrom(g.NewV7()), StockID: stock, Issuer: "coinbase", Symbol: "AAPLc",
			Network: n, TokenAddress: token, Decimals: 8, SharesPerToken: big.NewRat(1, 1),
			Venue: multichain.VenueUniswap, Enabled: true,
		}
	}
	e := &evmSellWorld{
		tradeWorld: w, evm: &evmTreasury{Treasury: w.treasury}, quoter: &saleQuoter{t: t},
		base:     listing(chain.NetworkBase, baseToken),
		robinhod: listing(chain.NetworkRobinhood, robinhoodToken),
	}
	e.listings = listingBook{"base:" + baseToken: e.base, "robinhood:" + robinhoodToken: e.robinhod}
	e.holdSolana(0)
	return e
}

func (e *evmSellWorld) holdSolana(units uint64) {
	aapl, positions := marketfake.AAPLx(), []treasury.Position{{
		Mint: marketfake.TSLAx().Mint.Address(), Units: money.NewBaseUnits(heldUnits*2, marketfake.TSLAx().Decimals),
	}}
	if units > 0 {
		positions = append(positions,
			treasury.Position{Mint: aapl.Mint.Address(), Units: money.NewBaseUnits(units, aapl.Decimals)})
	}
	e.treasury.SetPositions(e.cabal, positions)
	e.holdOnChain(units)
}

func (e *evmSellWorld) holdEVM(held ...treasury.ListingPosition) { e.evm.positions = held }

func evmHeld(n chain.Network, token string, units uint64) treasury.ListingPosition {
	return treasury.ListingPosition{Key: string(n) + ":" + token, Units: new(big.Int).SetUint64(units)}
}

func (e *evmSellWorld) scenario(t *testing.T) *scenario.Scenario {
	t.Helper()
	ports := e.ports()
	ports.Treasury, ports.Listings, ports.Quoter = e.evm, e.listings, e.quoter
	return scenario.New(t, scenario.WithModules(func(d module.Deps) module.Module {
		return governance.New(d, governance.WithPorts(ports))
	}))
}

func sellBody(units uint64, network string) string {
	body := map[string]any{"kind": "sell", "symbol": "AAPLx", "token_amount": units}
	if network != "" {
		body["network"] = network
	}
	raw, _ := json.Marshal(body)
	return string(raw)
}

func (e *evmSellWorld) passes() []scenario.Step {
	return []scenario.Step{
		scenario.Remember("id", "proposal"),
		scenario.Post("/v1/proposals/{proposal}/votes", voteYes),
		scenario.ExpectStatus(http.StatusOK),
		scenario.AsSeededUser("bob", e.members[1]),
		scenario.Post("/v1/proposals/{proposal}/votes", voteYes),
		scenario.ExpectStatus(http.StatusOK),
		scenario.ExpectJSON("status", "passed"),
	}
}

func storedComparison(t *testing.T) func(*scenario.Scenario) multichain.Comparison {
	t.Helper()
	return func(s *scenario.Scenario) multichain.Comparison {
		var raw []byte
		if err := s.DB().QueryRow(t.Context(), `SELECT price_comparison FROM proposals WHERE id = $1`,
			s.Recall("proposal")).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var c multichain.Comparison
		if len(raw) == 0 {
			return c
		}
		if err := json.Unmarshal(raw, &c); err != nil {
			t.Fatalf("stored comparison %s: %v", raw, err)
		}
		return c
	}
}

func TestProposeTrade_anEVMOnlyPositionSellsPartiallyThroughProposeVoteAndPass(t *testing.T) {
	t.Parallel()
	e := newEVMSellWorld(t)
	e.holdEVM(evmHeld(chain.NetworkBase, baseToken, evmHeldUnits))
	read := storedComparison(t)
	steps := append([]scenario.Step{
		scenario.Post(e.proposals(), sellBody(100_000_000, "base")),
		scenario.ExpectStatus(http.StatusCreated),
		scenario.ExpectJSON("kind", "sell"),
		scenario.ExpectJSON("network", "base"),
		scenario.ExpectJSON("token_amount", 100_000_000),
		scenario.ExpectJSON("quote_out_amount", 100_000_000/unitsPerMicro),
	}, e.passes()...)
	e.scenario(t).Given(e.asMember()).
		When(steps...).
		Then(
			scenario.ExpectEventPayload(events.TypeProposalPassed, map[string]any{
				"kind": "sell", "network": "base", "token_amount": "100000000", "quote_out_amount": "50000000",
				"mint": marketfake.AAPLx().Mint.String(),
			}),
			func(s *scenario.Scenario) {
				c := read(s)
				if c.Side != multichain.SideSell || c.Winner != e.base.ID || len(c.Ranked) != 1 ||
					c.Ranked[0].Network != chain.NetworkBase || c.Ranked[0].NetMicros.Uint64() != 50_000_000 ||
					c.Ranked[0].NetShares != 1_000_000_000 {
					t.Fatalf("stored comparison = %+v, want one Base sale of 1 share for 50 USDC", c)
				}
			},
		)
	if e.quoter.timesAsked() == 0 {
		t.Fatal("the sale was not quoted")
	}
}

func TestProposeTrade_anEVMOnlyPositionSellsInFullAndTakesItsNetworkWhenNoPinIsGiven(t *testing.T) {
	t.Parallel()
	e := newEVMSellWorld(t)
	e.holdEVM(evmHeld(chain.NetworkBase, baseToken, evmHeldUnits))
	steps := append([]scenario.Step{
		scenario.Post(e.proposals(), sellBody(evmHeldUnits, "")),
		scenario.ExpectStatus(http.StatusCreated),
		scenario.ExpectJSON("network", "base"),
		scenario.ExpectJSON("token_amount", evmHeldUnits),
		scenario.ExpectJSON("quote_out_amount", evmHeldUnits/unitsPerMicro),
	}, e.passes()...)
	e.scenario(t).Given(e.asMember()).
		When(steps...).
		Then(scenario.ExpectEventPayload(events.TypeProposalPassed, map[string]any{
			"kind": "sell", "network": "base", "token_amount": "305000000", "quote_out_amount": "152500000",
		}))
}

func TestProposeTrade_aMixedHoldingSellsFromTheLocationItIsPinnedTo(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		body    string
		network string
		quote   uint64
		onBase  bool
	}{
		"unpinned stays on Solana": {sellBody(400_000_000, ""), "", quoteUnits, false},
		"pinned to Solana":         {sellBody(400_000_000, "solana"), "solana", quoteUnits, false},
		"pinned to Base":           {sellBody(200_000_000, "base"), "base", 100_000_000, true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newEVMSellWorld(t)
			e.holdSolana(heldUnits)
			e.holdEVM(evmHeld(chain.NetworkBase, baseToken, evmHeldUnits))
			e.scenario(t).Given(e.asMember()).
				When(
					scenario.Post(e.proposals(), tc.body),
					scenario.ExpectStatus(http.StatusCreated),
					scenario.ExpectJSON("quote_out_amount", tc.quote),
					expectNetwork(tc.network),
					scenario.Remember("id", "proposal"),
				).
				Then(e.expectSaleStored(t, tc.onBase))
		})
	}
}

func expectNetwork(want string) scenario.Step {
	if want == "" {
		return scenario.ExpectNoField("network")
	}
	return scenario.ExpectJSON("network", want)
}

func (e *evmSellWorld) expectSaleStored(t *testing.T, onBase bool) scenario.Step {
	t.Helper()
	read := storedComparison(t)
	return func(s *scenario.Scenario) {
		t.Helper()
		c := read(s)
		if onBase && (c.Winner != e.base.ID || c.Side != multichain.SideSell) {
			t.Fatalf("stored comparison = %+v, want the Base sale", c)
		}
		if !onBase && len(c.Ranked) != 0 {
			t.Fatalf("a Solana sale stored the comparison %+v, want none", c)
		}
	}
}

func TestProposeTrade_aSellNeverFallsBackToAnotherLocation(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		solana uint64
		evm    uint64
		body   string
	}{
		"pinned Base is short while Solana has enough":   {heldUnits, 100_000_000, sellBody(200_000_000, "base")},
		"unpinned Solana is short while Base has enough": {100_000_000, evmHeldUnits, sellBody(200_000_000, "")},
		"pinned Solana is short while Base has enough":   {100_000_000, evmHeldUnits, sellBody(200_000_000, "solana")},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newEVMSellWorld(t)
			e.holdSolana(tc.solana)
			e.holdEVM(evmHeld(chain.NetworkBase, baseToken, tc.evm))
			s := e.scenario(t)
			s.Given(e.asMember()).
				When(scenario.Post(e.proposals(), tc.body)).
				Then(scenario.ExpectProblem(errs.CodeCabalSharesShort), scenario.ExpectEvents(events.TypeProposalCreated, 0))
			if n := proposalRows(t, s, e.cabal); n != 0 {
				t.Fatalf("proposals rows = %d, want 0", n)
			}
		})
	}
}

func TestProposeTrade_refusesASellPinItCannotHonor(t *testing.T) {
	t.Parallel()
	disabled := func(e *evmSellWorld) {
		e.base.Enabled = false
		e.listings["base:"+baseToken] = e.base
	}
	for name, tc := range map[string]struct {
		held  []treasury.ListingPosition
		edit  func(*evmSellWorld)
		body  string
		want  errs.Code
		asked bool
	}{
		"a network that does not trade": {
			held: []treasury.ListingPosition{evmHeld(chain.NetworkBase, baseToken, evmHeldUnits)},
			body: sellBody(1_000, "arbitrum"), want: errs.CodeInvalidNetwork,
		},
		"a network the cabal holds nothing on": {
			held: []treasury.ListingPosition{evmHeld(chain.NetworkRobinhood, robinhoodToken, evmHeldUnits)},
			body: sellBody(1_000, "base"), want: errs.CodeInvalidNetwork,
		},
		"two EVM networks and no pin": {
			held: []treasury.ListingPosition{
				evmHeld(chain.NetworkBase, baseToken, evmHeldUnits),
				evmHeld(chain.NetworkRobinhood, robinhoodToken, evmHeldUnits),
			},
			edit: func(e *evmSellWorld) { e.holdSolana(0) }, body: sellBody(1_000, ""), want: errs.CodeInvalidNetwork,
		},
		"a listing that is gated": {
			held: []treasury.ListingPosition{evmHeld(chain.NetworkBase, baseToken, evmHeldUnits)},
			edit: disabled, body: sellBody(1_000, "base"), want: errs.CodeAssetUntradable,
		},
		"a sale with no route": {
			held: []treasury.ListingPosition{evmHeld(chain.NetworkBase, baseToken, evmHeldUnits)},
			edit: func(e *evmSellWorld) { e.quoter.err = errs.New(errs.CodeNoRoute, "test") },
			body: sellBody(1_000, "base"), want: errs.CodeNoRoute, asked: true,
		},
		"a venue that is down": {
			held: []treasury.ListingPosition{evmHeld(chain.NetworkBase, baseToken, evmHeldUnits)},
			edit: func(e *evmSellWorld) { e.quoter.err = errs.New(errs.CodeUpstreamUnavailable, "test") },
			body: sellBody(1_000, "base"), want: errs.CodeUpstreamUnavailable, asked: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newEVMSellWorld(t)
			e.holdSolana(heldUnits)
			e.holdEVM(tc.held...)
			if tc.edit != nil {
				tc.edit(e)
			}
			s := e.scenario(t)
			s.Given(e.asMember()).
				When(scenario.Post(e.proposals(), tc.body)).
				Then(scenario.ExpectProblem(tc.want), scenario.ExpectEvents(events.TypeProposalCreated, 0))
			if n := proposalRows(t, s, e.cabal); n != 0 {
				t.Fatalf("proposals rows = %d, want 0", n)
			}
			if asked := e.quoter.timesAsked() > 0; asked != tc.asked {
				t.Fatalf("quoted = %t, want %t", asked, tc.asked)
			}
		})
	}
}

func TestProposeTrade_anEVMSellWithoutAWiredQuoterHasNoRoute(t *testing.T) {
	t.Parallel()
	e := newEVMSellWorld(t)
	e.holdEVM(evmHeld(chain.NetworkBase, baseToken, evmHeldUnits))
	ports := e.ports()
	ports.Treasury, ports.Listings = e.evm, e.listings
	s := scenario.New(t, scenario.WithModules(func(d module.Deps) module.Module {
		return governance.New(d, governance.WithPorts(ports))
	}))
	s.Given(e.asMember()).
		When(scenario.Post(e.proposals(), sellBody(1_000, "base"))).
		Then(scenario.ExpectProblem(errs.CodeNoRoute), scenario.ExpectEvents(events.TypeProposalCreated, 0))
}

func TestPreview_quotesAnEVMSaleAndAdvisesWhatItCannotDo(t *testing.T) {
	t.Parallel()
	e := newEVMSellWorld(t)
	e.holdEVM(evmHeld(chain.NetworkBase, baseToken, evmHeldUnits))
	e.scenario(t).Given(e.asMember()).
		When(
			scenario.Get(e.preview("kind=sell&symbol=AAPLx&token_amount=100000000&network=base")),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("advisory_code", nil),
			scenario.ExpectJSON("quote_out_amount", 50_000_000),
			scenario.Get(e.preview("kind=sell&symbol=AAPLx&token_amount=400000000&network=base")),
			scenario.ExpectJSON("advisory_code", "cabal_shares_short"),
			scenario.ExpectJSON("quote_out_amount", 200_000_000),
			scenario.Get(e.preview("kind=sell&symbol=AAPLx&token_amount=100000000&network=arbitrum")),
			scenario.ExpectJSON("advisory_code", "invalid_network"),
			scenario.ExpectJSON("quote_out_amount", nil),
			scenario.Get(e.preview("kind=sell&symbol=AAPLx&token_amount=100000000")),
			scenario.ExpectJSON("advisory_code", nil),
			scenario.ExpectJSON("quote_out_amount", 50_000_000),
		)
}

func TestTreasuryQueries_satisfyTheEVMHoldingsPortTheGovernanceModuleReads(t *testing.T) {
	t.Parallel()
	e := newEVMSellWorld(t)
	scenario.New(t, scenario.WithModules(func(d module.Deps) module.Module {
		if _, ok := treasury.New(d).Queries().(app.EVMHoldings); !ok {
			t.Error("the treasury queries do not read EVM positions, so no sell can see an EVM holding")
		}
		return governance.New(d, governance.WithPorts(e.ports()))
	}))
}

type oddQuoter struct {
	calls   int
	failOn  int
	noGross bool
}

func (q *oddQuoter) Quote(
	_ context.Context, l multichain.Listing, _ multichain.Side, micros money.Micros,
) (multichain.Quote, error) {
	q.calls++
	switch {
	case q.calls == q.failOn:
		return multichain.Quote{}, errs.New(errs.CodeUpstreamUnavailable, "test")
	case q.noGross:
		return multichain.Quote{ListingID: l.ID}, nil
	}
	return multichain.Quote{ListingID: l.ID, GrossOutput: new(big.Int).SetUint64(micros.Uint64() * unitsPerMicro)}, nil
}

func TestProposeTrade_anEVMSaleTheQuoterCannotPriceIsRefused(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		quoter *oddQuoter
		units  uint64
		want   errs.Code
	}{
		"the refining quote fails":   {&oddQuoter{failOn: 2}, 100_000_000, errs.CodeUpstreamUnavailable},
		"a quote with no output":     {&oddQuoter{noGross: true}, 100_000_000, errs.CodeDecodeFailed},
		"a sale worth under a micro": {&oddQuoter{}, 1, errs.CodeNoRoute},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newEVMSellWorld(t)
			e.holdEVM(evmHeld(chain.NetworkBase, baseToken, evmHeldUnits))
			ports := e.ports()
			ports.Treasury, ports.Listings, ports.Quoter = e.evm, e.listings, tc.quoter
			s := scenario.New(t, scenario.WithModules(func(d module.Deps) module.Module {
				return governance.New(d, governance.WithPorts(ports))
			}))
			s.Given(e.asMember()).
				When(scenario.Post(e.proposals(), sellBody(tc.units, "base"))).
				Then(scenario.ExpectProblem(tc.want), scenario.ExpectEvents(events.TypeProposalCreated, 0))
		})
	}
}

func TestProposeTrade_aListingWithoutARatioOrWithAHugeSaleStoresNoShares(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		ratio *big.Rat
		units uint64
	}{
		"no shares per token": {nil, 100_000_000},
		"shares past uint64":  {new(big.Rat).SetInt(new(big.Int).Lsh(big.NewInt(1), 70)), 100_000_000},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newEVMSellWorld(t)
			e.base.SharesPerToken = tc.ratio
			e.listings["base:"+baseToken] = e.base
			e.holdEVM(evmHeld(chain.NetworkBase, baseToken, evmHeldUnits))
			s := e.scenario(t)
			s.Given(e.asMember()).
				When(
					scenario.Post(e.proposals(), sellBody(tc.units, "base")),
					scenario.ExpectStatus(http.StatusCreated),
					scenario.Remember("id", "proposal"),
				).
				Then(func(s *scenario.Scenario) {
					c := storedComparison(t)(s)
					if len(c.Ranked) != 1 || c.Ranked[0].NetShares != 0 {
						t.Fatalf("stored comparison = %+v, want one sale with no share count", c)
					}
				})
		})
	}
}

func TestProposeTrade_aSellReadsOnlyThePositionsItCanUse(t *testing.T) {
	t.Parallel()
	e := newEVMSellWorld(t)
	other := e.robinhod
	other.StockID = multichain.StockID{}
	e.listings["robinhood:"+robinhoodToken] = other
	e.holdEVM(
		evmHeld(chain.NetworkBase, "0xzero", 0),
		treasury.ListingPosition{Key: "base:0xnil"},
		evmHeld(chain.NetworkBase, "0xunlisted", evmHeldUnits),
		evmHeld(chain.NetworkRobinhood, robinhoodToken, evmHeldUnits),
		evmHeld(chain.NetworkBase, baseToken, evmHeldUnits),
	)
	e.scenario(t).Given(e.asMember()).
		When(
			scenario.Post(e.proposals(), sellBody(100_000_000, "")),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.ExpectJSON("network", "base"),
		)
}

func TestProposeTrade_aSellFailsWhenItsHoldingsCannotBeRead(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		edit func(*evmSellWorld)
		want errs.Code
	}{
		"the positions are down": {
			func(e *evmSellWorld) { e.evm.err = errs.New(errs.CodeDBUnavailable, "test") }, errs.CodeDBUnavailable,
		},
		"a listing lookup is down": {
			func(e *evmSellWorld) { e.holdEVM(evmHeld(chain.NetworkBase, "0xbroken", evmHeldUnits)) },
			errs.CodeUpstreamUnavailable,
		},
		"two listings on one chain": {
			func(e *evmSellWorld) {
				twin := e.base
				e.listings["base:0xtwin"] = twin
				e.holdEVM(
					evmHeld(chain.NetworkBase, baseToken, evmHeldUnits), evmHeld(chain.NetworkBase, "0xtwin", evmHeldUnits))
			},
			errs.CodeInvalidNetwork,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newEVMSellWorld(t)
			tc.edit(e)
			e.scenario(t).Given(e.asMember()).
				When(scenario.Post(e.proposals(), sellBody(1_000, "base"))).
				Then(scenario.ExpectProblem(tc.want), scenario.ExpectEvents(events.TypeProposalCreated, 0))
		})
	}
}
