package trading_test

import (
	"context"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	treasury "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	soldUnits  = 400_000_000
	proceeds   = 101_000_000
	quotedSale = 101_500_000
	saleMinOut = 100_485_000
)

type heldFake struct {
	testkit.Faults
	cabal     ids.CabalID
	positions []treasury.Position
}

func (h *heldFake) SolanaPositions(context.Context, ids.CabalID) ([]treasury.Position, error) {
	return h.positions, nil
}

func (h *heldFake) EVMPositions(context.Context, ids.CabalID) ([]treasury.ListingPosition, error) {
	if err := h.Check("EVMPositions"); err != nil {
		return nil, err
	}
	var listings []treasury.ListingPosition
	for _, p := range h.positions {
		listings = append(listings, treasury.ListingPosition{
			Key: string(p.Mint), Units: new(big.Int).SetUint64(p.Units.Uint64()),
		})
	}
	return listings, nil
}

func (e *routeEnv) selling() app.SwapRequest {
	e.evm.plan.OutAmount, e.evm.plan.MinOut = quotedSale, saleMinOut
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: proceeds}}
	req := e.request()
	req.Action, req.InAmount = domain.ActionSell, soldUnits
	return req
}

func (e *routeEnv) token() string { return "base:" + strings.ToLower(e.listing.TokenAddress) }

func TestRouteExec_aSellSwapsToEVMCashConfirmsAndBridgesTheProceedsBack(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	req := e.selling()
	e.privy.script(multichain.ActionSucceeded)

	view := e.mustBuy(t, req, domain.StatusConfirmed)

	confirmed := e.payload(t, view.ID.UUID(), "trade.confirmed")
	want := map[string]any{
		"action": "sell", "in_mint": e.token(), "in_amount": "400000000", "out_mint": "base:" + baseCash,
		"out_amount": "101000000", "usdc_micros": "101000000", "fee_micros": "0", "tx_signature": swapHash,
	}
	for k, v := range want {
		if confirmed[k] != v {
			t.Fatalf("trade.confirmed %s = %v, want %v", k, confirmed[k], v)
		}
	}
	spec := app.EVMSwapSpec{
		Network: chain.NetworkBase, Owner: evmAddress(t, evmTreasury), TokenIn: evmAddress(t, e.listing.TokenAddress),
		TokenOut: evmAddress(t, baseCash), AmountIn: soldUnits, SlippageBps: 100,
	}
	if len(e.evm.specs) != 1 || e.evm.specs[0] != spec {
		t.Fatalf("built %+v, want the held tokens sold for EVM cash", e.evm.specs)
	}
	calls := e.privy.calls()
	back := multichain.TransferSpec{
		From: chain.NetworkBase, To: chain.NetworkSolana, Destination: string(e.wallet.Address),
		AmountMicros: money.MicrosFromUint64(proceeds),
	}
	if len(calls) != 1 || calls[0].spec != back || calls[0].key != "trade:"+view.ID.String()+":bridge_back" {
		t.Fatalf("transfers = %+v, want the proceeds bridged back", calls)
	}
	wantSteps(t, e.steps(t, view.ID.UUID()), "swap:pending:0", "swap:submitted:0", "swap:succeeded:0",
		"bridge_back:pending:0", "bridge_back:submitted:0", "bridge_back:succeeded:7500")
}

func TestRouteExec_aSellsPendingBridgeBackIsFinishedByThePoller(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	view := e.mustBuy(t, e.selling(), domain.StatusConfirmed)
	e.clk.Advance(app.SwapSweepAge + time.Second)
	e.privy.script(multichain.ActionSucceeded)

	e.mustTick(t, 1, 1)
	e.mustTick(t, 0, 0)

	if status, _ := e.bridgeBackOf(t, view.ID.UUID()); status != "succeeded" || e.privy.actions() != 1 {
		t.Fatalf("bridge_back %s after %d actions, want succeeded on one", status, e.privy.actions())
	}
}

func TestRouteExec_aSellWithNoQuotedProceedsOrAFillPastTheColumnSendsOrConfirmsNothing(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	unquoted := e.selling()
	e.evm.plan.OutAmount = 0

	view := e.mustBuy(t, unquoted, domain.StatusFailed)
	if code := e.failure(t, view.ID.UUID()); code != string(domain.FailurePriceMoved) || len(e.evm.keys()) != 0 {
		t.Fatalf("failure %s after sends %v, want price_moved and nothing sent", code, e.evm.keys())
	}

	huge := e.selling()
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: 1 << 63}}
	if _, err := e.buy(t.Context(), huge); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("Trade err = %v, want decode_failed", err)
	}
	if status, _, _, _ := e.row(t, e.swapIDs(t, huge.Source.ID)[0]); status != string(domain.StatusSubmitted) {
		t.Fatalf("swap is %s, want submitted", status)
	}
}

func (e *routeEnv) sellHandler(held app.Positions) *app.ExecuteTradeHandler {
	ports := e.ports()
	ports.Positions = held
	return app.NewExecuteTradeHandler(app.ExecuteTradeDeps{
		Layer: e.layer(), Routes: e.exec(), Listings: e.listings, UoW: e.uow, Reads: e.reads, Venue: e.venue,
		Ports: ports, USDC: usdcToken(), EVM: true,
	})
}

func TestExecuteTrade_anEVMSellWinnerWithEVMOffSellsTheProposalMintOnSolana(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.ledger.SetTokens(e.wallet.Address, aaplxToken(), soldUnits)
	e.quote(aaplxToken(), usdcToken(), proceeds, true)
	order := jupiter.Order{RequestID: "req-1", Transaction: swapTx()}
	e.jup.SetOrder(jupiterMint(aaplxToken()), jupiterMint(usdcToken()), order)
	cmd := e.sell(soldUnits)
	cmd.Route = e.route
	h := app.NewExecuteTradeHandler(app.ExecuteTradeDeps{
		Layer: e.layer(), Routes: e.exec(), Listings: e.listings, UoW: e.uow, Reads: e.reads, Venue: e.venue,
		Ports: e.ports(), USDC: usdcToken(),
	})

	if err := h.Handle(actorContext(t.Context()), e.delivery(t, cmd), cmd, nil); err != nil {
		t.Fatal(err)
	}
	var inMint, status string
	if err := e.pool.QueryRow(t.Context(), `SELECT in_mint, status FROM swaps WHERE source_id = $1`,
		cmd.ProposalID.UUID()).Scan(&inMint, &status); err != nil {
		t.Fatal(err)
	}
	steps := e.count(t, `SELECT count(*) FROM trade_steps`)
	if inMint != aaplxMint || status != "confirmed" || steps != 0 || len(e.privy.calls()) != 0 {
		t.Fatalf("sold %s (%s) with %d steps and %d transfers, want the proposal mint sold through Jupiter",
			inMint, status, steps, len(e.privy.calls()))
	}
}

func TestExecuteTrade_anEVMWinnerSellsTheTokensHeldOnThatChain(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		held uint64
		want string
	}{
		"held":  {soldUnits, "ok"},
		"short": {soldUnits - 1, string(errs.CodeCabalSharesShort)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newRouteEnv(t)
			e.selling()
			e.privy.script(multichain.ActionSucceeded)
			cmd := e.sell(soldUnits)
			cmd.Route = e.route
			d := e.delivery(t, cmd)
			held := &heldFake{cabal: e.cabal, positions: []treasury.Position{
				{Mint: chain.SolanaAddress(e.token()), Units: money.NewBaseUnits(tc.held, 8)},
				{Mint: aaplxMint, Units: money.NewBaseUnits(soldUnits, 8)},
			}}

			if err := e.sellHandler(held).Handle(actorContext(t.Context()), d, cmd, nil); err != nil {
				t.Fatal(err)
			}
			if code := e.deliveryCode(t, d); code != tc.want {
				t.Fatalf("delivery %s, want %s", code, tc.want)
			}
		})
	}
}

func TestExecuteTrade_aPositionsOutageOnAnEVMSellIsRetried(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	cmd := e.sell(soldUnits)
	cmd.Route = e.route
	held := &heldFake{cabal: e.cabal}
	held.FailOnce("EVMPositions", errs.New(errs.CodeDBUnavailable, "treasury.Positions"))

	err := e.sellHandler(held).Handle(actorContext(t.Context()), e.delivery(t, cmd), cmd, nil)
	if errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("Handle err = %v, want db_unavailable", err)
	}
}

func TestExecuteTrade_aSolanaWinnerOnAnotherTokenSellsThatToken(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	ondo := chain.Mint{Address: "AAPLonDo1111111111111111111111111111111111111", Decimals: 9}
	e.ledger.SetTokens(e.wallet.Address, ondo, soldUnits)
	e.quote(ondo, usdcToken(), proceeds, true)
	e.jup.SetOrder(
		jupiterMint(ondo),
		jupiterMint(usdcToken()),
		jupiter.Order{RequestID: "req-1", Transaction: swapTx()},
	)
	cmd := e.sell(soldUnits)
	cmd.Route = e.comparison(t, e.solanaWinner(t, ondo, true))

	if err := e.handle(t, e.delivery(t, cmd), cmd); err != nil {
		t.Fatal(err)
	}
	var inMint string
	if err := e.pool.QueryRow(t.Context(), `SELECT in_mint FROM swaps WHERE source_id = $1`,
		cmd.ProposalID.UUID()).Scan(&inMint); err != nil || inMint != string(ondo.Address) {
		t.Fatalf("sold %s, %v, want the winner's token", inMint, err)
	}
}

type unreadableListings struct{ heldFake }

func (u *unreadableListings) EVMPositions(context.Context, ids.CabalID) ([]treasury.ListingPosition, error) {
	return []treasury.ListingPosition{{Key: "base:0x"}}, nil
}

func TestExecuteTrade_anEVMSellRefusesAPositionItCannotRead(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	cmd := e.sell(soldUnits)
	cmd.Route = e.route
	d := e.delivery(t, cmd)
	d.Final = true

	if err := e.sellHandler(&unreadableListings{heldFake{cabal: e.cabal}}).
		Handle(actorContext(t.Context()), d, cmd, nil); err != nil {
		t.Fatal(err)
	}
	if code := e.deliveryCode(t, d); code != string(errs.CodeTradeNotStarted) {
		t.Fatalf("delivery %s, want trade_not_started", code)
	}
	if _, err := (app.UnwiredPositions{}).EVMPositions(t.Context(), ids.CabalID{}); errs.CodeOf(err) !=
		errs.CodeUpstreamUnavailable {
		t.Fatalf("unwired EVMPositions err = %v, want upstream_unavailable", err)
	}
}

func (h *heldFake) CashOutReservations(context.Context) (map[ids.CabalID]money.Micros, error) {
	return map[ids.CabalID]money.Micros{}, h.Check("CashOutReservations")
}
