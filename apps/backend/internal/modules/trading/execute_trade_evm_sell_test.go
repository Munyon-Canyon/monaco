package trading_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	busevents "github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	treasury "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func (e *routeEnv) heldOnBase(units uint64) *heldFake {
	return &heldFake{cabal: e.cabal, positions: []treasury.Position{
		{Mint: chain.SolanaAddress(e.token()), Units: money.NewBaseUnits(units, 8)},
	}}
}

func (e *routeEnv) passedSale(
	t *testing.T, h *app.ExecuteTradeHandler, cmd app.ExecuteTrade, pin chain.Network,
) string {
	t.Helper()
	d := e.delivery(t, cmd)
	ev := busevents.ProposalPassed{
		V: 1, ProposalID: cmd.ProposalID.UUID(), CabalID: cmd.CabalID.UUID(), Kind: string(cmd.Action),
		Symbol: cmd.Symbol, Mint: cmd.Mint, TokenAmount: cmd.TokenAmount, QuoteOutAmount: cmd.QuoteOutAmount,
		ProposerID: e.ids.NewV7(), PriceComparison: cmd.Route, Network: pin,
	}
	if err := (adapters.Engine{Trades: h}).Handle(actorContext(t.Context()), d, ev); err != nil {
		t.Fatal(err)
	}
	return e.deliveryCode(t, d)
}

func TestExecuteTrade_aPinnedEVMSaleSellsPartiallyAndInFullWithTheLedgerUnitsAndTheReturnedCash(t *testing.T) {
	t.Parallel()
	const held = 700_000_000
	for name, units := range map[string]uint64{"partially": soldUnits, "in full": held} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newRouteEnv(t)
			e.selling()
			e.privy.script(multichain.ActionSucceeded)
			cmd := e.sell(units)
			cmd.Route = e.route

			code := e.passedSale(t, e.sellHandler(e.heldOnBase(held)), cmd, chain.NetworkBase)

			swaps := e.swapsOf(t, cmd.ProposalID)
			if code != "ok" || len(swaps) != 1 {
				t.Fatalf("delivery %s with %d swaps, want one routed sale", code, len(swaps))
			}
			e.wantSold(t, swaps[0], units)
		})
	}
}

func (e *routeEnv) wantSold(t *testing.T, swap uuid.UUID, units uint64) {
	t.Helper()
	if winner, pin := e.pinnedRoute(t, swap); winner != e.listing.ID.UUID() || pin != "base" {
		t.Fatalf("route %s pinned %q, want the Base listing pinned to base", winner, pin)
	}
	confirmed := e.payload(t, swap, "trade.confirmed")
	if confirmed["in_mint"] != e.token() || confirmed["in_amount"] != uitoa(units) ||
		confirmed["out_amount"] != uitoa(proceeds) || confirmed["usdc_micros"] != uitoa(proceeds) {
		t.Fatalf("trade.confirmed = %v, want %d token units sold for %d cash", confirmed, units, proceeds)
	}
	if len(e.evm.specs) != 1 || e.evm.specs[0].AmountIn != units {
		t.Fatalf("built %+v, want a swap of exactly the proposed %d token units", e.evm.specs, units)
	}
	back := multichain.TransferSpec{
		From: chain.NetworkBase, To: chain.NetworkSolana, Destination: string(e.wallet.Address),
		AmountMicros: money.MicrosFromUint64(proceeds),
	}
	if calls := e.privy.calls(); len(calls) != 1 || calls[0].spec != back {
		t.Fatalf("transfers = %+v, want the %d cash bridged back to Solana", calls, proceeds)
	}
}

func TestExecuteTrade_aPinnedEVMSaleOfMoreThanTheLedgerHoldsDebitsNothing(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.selling()
	e.ledger.SetTokens(e.wallet.Address, aaplxToken(), soldUnits*2)
	cmd := e.sell(soldUnits)
	cmd.Route = e.route

	code := e.passedSale(t, e.sellHandler(e.heldOnBase(soldUnits-1)), cmd, chain.NetworkBase)

	e.wantPinRefused(t, cmd, code, errs.CodeCabalSharesShort)
	if len(e.privy.calls()) != 0 || len(e.evm.specs) != 0 {
		t.Fatalf("%d transfers and %d swaps built, want none", len(e.privy.calls()), len(e.evm.specs))
	}
}

func TestExecuteTrade_aPinnedEVMSaleWithEVMTradingOffIsBlockedAndNeverSellsOnSolana(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.selling()
	e.ledger.SetTokens(e.wallet.Address, aaplxToken(), soldUnits)
	e.quote(aaplxToken(), usdcToken(), proceeds, true)
	cmd := e.sell(soldUnits)
	cmd.Route = e.route
	ports := e.ports()
	ports.Positions = e.heldOnBase(soldUnits)
	h := app.NewExecuteTradeHandler(app.ExecuteTradeDeps{
		Layer: e.layer(), Routes: e.exec(), Listings: e.listings, UoW: e.uow, Reads: e.reads, Venue: e.venue,
		Ports: ports, USDC: usdcToken(),
	})

	code := e.passedSale(t, h, cmd, chain.NetworkBase)

	e.wantPinRefused(t, cmd, code, errs.CodeChainTradingOff)
}

func TestExecuteTrade_aSolanaPinnedSaleSellsOnSolanaEvenWhenTheCabalAlsoHoldsOnBase(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.ledger.SetTokens(e.wallet.Address, aaplxToken(), soldUnits)
	e.quote(aaplxToken(), usdcToken(), proceeds, true)
	e.jup.SetOrder(jupiterMint(aaplxToken()), jupiterMint(usdcToken()),
		jupiter.Order{RequestID: "req-1", Transaction: swapTx()})
	cmd := e.sell(soldUnits)

	code := e.passedSale(t, e.sellHandler(e.heldOnBase(soldUnits)), cmd, chain.NetworkSolana)

	var inMint, status string
	if err := e.pool.QueryRow(t.Context(), `SELECT in_mint, status FROM swaps WHERE source_id = $1`,
		cmd.ProposalID.UUID()).Scan(&inMint, &status); err != nil {
		t.Fatal(err)
	}
	if code != "ok" || inMint != aaplxMint || status != string(domain.StatusConfirmed) ||
		e.count(t, `SELECT count(*) FROM trade_steps`) != 0 || len(e.privy.calls()) != 0 {
		t.Fatalf("delivery %s sold %s (%s) with %d transfers, want the Solana mint sold through Jupiter only",
			code, inMint, status, len(e.privy.calls()))
	}
}

func uitoa(n uint64) string { return money.MicrosFromUint64(n).String() }
