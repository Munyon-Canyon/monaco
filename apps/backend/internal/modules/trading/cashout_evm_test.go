package trading_test

import (
	"context"
	"math/big"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

type cashOutLots struct {
	cabal ids.CabalID
	token string
	sale  app.EVMSale
	err   error
	asked []string
}

func (c *cashOutLots) SolanaPositions(context.Context, ids.CabalID) ([]treasuryport.Position, error) {
	return []treasuryport.Position{
		{Mint: usdcMint, Units: money.NewBaseUnits(900_000_000, 6)},
		{Mint: aaplxMint, Units: money.NewBaseUnits(1_000, 8)},
		{Mint: chain.SolanaAddress(c.token), Units: money.NewBaseUnits(soldUnits, 8)},
	}, nil
}

func (c *cashOutLots) CabalPositionsAt(context.Context, time.Time) ([]treasuryport.CabalPositions, error) {
	return []treasuryport.CabalPositions{
		{
			CabalID:  ids.CabalIDFrom(uuid.Nil),
			Listings: []treasuryport.ListingPosition{{Key: c.token, Units: big.NewInt(7)}},
		},
		{CabalID: c.cabal, Listings: []treasuryport.ListingPosition{
			{Key: c.token, Units: big.NewInt(soldUnits)},
			{Key: "base:0x00000000000000000000000000000000000000dd", Units: big.NewInt(0)},
		}},
	}, nil
}

func (c *cashOutLots) Sale(_ context.Context, asset string, _ uint64) (app.EVMSale, error) {
	c.asked = append(c.asked, asset)
	return c.sale, c.err
}

type evmCashOut struct {
	*routeEnv
	lots *cashOutLots
	job  uuid.UUID
}

const cashOutBridgeFee = 7_955

func newEVMCashOut(t *testing.T, solanaUSDC uint64) *evmCashOut {
	t.Helper()
	const evmProceeds = 40_000_000
	e := &evmCashOut{routeEnv: newRouteEnv(t)}
	e.job = e.ids.NewV7()
	e.lots = &cashOutLots{
		cabal: e.cabal, token: e.token(), sale: app.EVMSale{Listing: e.listing, ProceedsMicros: evmProceeds},
	}
	e.lots.sale.Listing.StockID = e.route.StockID
	e.quote(aaplxToken(), usdcToken(), solanaUSDC, true)
	e.jup.SetOrder(jupiterMint(aaplxToken()), jupiterMint(usdcToken()), jupiter.Order{
		RequestID: "req-sell", Transaction: swapTx(),
	})
	e.jup.SetExecute("req-sell", jupiter.ExecuteResult{Status: jupiter.StatusSuccess, OutAmount: solanaUSDC})
	e.evm.plan.OutAmount, e.evm.plan.MinOut = evmProceeds, evmProceeds
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: evmProceeds}}
	e.privy.script(multichain.ActionSucceeded)
	e.arrivals.short = cashOutBridgeFee
	return e
}

func (e *evmCashOut) cashOut(t *testing.T, needed uint64) error {
	t.Helper()
	return e.cashOutIn(actorContext(t.Context()), needed)
}

func (e *evmCashOut) cashOutIn(ctx context.Context, needed uint64) error {
	return app.NewSellForCashOutHandler(app.SellForCashOutDeps{
		Layer: e.layer(), UoW: e.uow, Reads: e.reads, Clock: e.clk, Venue: e.venue,
		Holdings: app.LedgerHoldings{
			Ledger: e.lots, Catalog: marketfake.NewCatalog(marketfake.Fixtures()...), USDC: usdcToken(), Clock: e.clk,
		},
		Wallets: app.CabalWallets{Cabals: liveCabals{e: e.engineEnv}}, USDC: usdcToken(), Routes: e.exec(),
		EVMSales: e.lots,
	}).Handle(ctx, app.SellForCashOut{
		CabalID: e.cabal, Source: domain.Source{Kind: domain.SourceCashout, ID: e.job},
		USDCNeeded: money.MicrosFromUint64(needed),
	}, nil)
}

type cashOutLeg struct {
	InMint, Status string
	InAmount       int64
	Batch          int32
}

func (e *evmCashOut) legs(t *testing.T) []cashOutLeg {
	t.Helper()
	rows, err := e.pool.Query(t.Context(), `SELECT in_mint, status, in_amount, source_batch_size FROM swaps
		WHERE source_kind = 'cashout' AND source_id = $1 ORDER BY created_at, id`, e.job)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []cashOutLeg
	for rows.Next() {
		var l cashOutLeg
		if err := rows.Scan(&l.InMint, &l.Status, &l.InAmount, &l.Batch); err != nil {
			t.Fatal(err)
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSellForCashOut_sellsTheSolanaAndTheEVMLotAndBridgesTheEVMProceedsBack(t *testing.T) {
	t.Parallel()
	e := newEVMCashOut(t, 50_000_000)

	if err := e.cashOut(t, 60_000_000); err != nil {
		t.Fatal(err)
	}

	want := []cashOutLeg{
		{InMint: aaplxMint, Status: string(domain.StatusConfirmed), InAmount: 1_000, Batch: 2},
		{InMint: e.token(), Status: string(domain.StatusConfirmed), InAmount: 106_079_560, Batch: 2},
	}
	if legs := e.legs(t); !slices.Equal(legs, want) {
		t.Fatalf("legs = %+v, want all the Solana lot then the part of the EVM lot that raises the rest", legs)
	}
	swaps := e.swapIDs(t, e.job)
	got := e.steps(t, swaps[1])
	if got[len(got)-1] != "bridge_back:succeeded:"+strconv.Itoa(cashOutBridgeFee) {
		t.Fatalf("EVM leg steps = %v, want its proceeds bridged back at the observed fee", got)
	}
	if winner, _, ok := e.routeOf(t, swaps[1]); !ok || winner != e.listing.ID.UUID() {
		t.Fatalf("EVM leg route %v %s, want the lot's listing", ok, winner)
	}
}

func TestSellForCashOut_ranksLotsByNetProceedsAfterTheBridgeBack(t *testing.T) {
	t.Parallel()
	e := newEVMCashOut(t, 39_990_000)

	if err := e.cashOut(t, 30_000_000); err != nil {
		t.Fatal(err)
	}

	legs := e.legs(t)
	if len(legs) != 1 || legs[0].InMint != aaplxMint {
		t.Fatalf("legs = %+v, want the Solana lot, which nets more than the EVM lot after its bridge back", legs)
	}
}

func TestSellForCashOut_anEVMSaleQuoteErrorStopsThePlan(t *testing.T) {
	t.Parallel()
	e := newEVMCashOut(t, 50_000_000)
	e.lots.err = errs.New(errs.CodeNotImplemented, "market.Sale")

	if err := e.cashOut(t, 60_000_000); errs.CodeOf(err) != errs.CodeNotImplemented {
		t.Fatalf("Handle err = %v, want not_implemented", err)
	}
	if n := len(e.legs(t)); n != 0 || len(e.lots.asked) != 1 || e.lots.asked[0] != e.token() {
		t.Fatalf("%d legs after asking %v, want none and one quote of the EVM asset", n, e.lots.asked)
	}
}

func TestUnwired_theEVMSaleQuotesAreNotImplementedYet(t *testing.T) {
	t.Parallel()
	if _, err := (app.UnwiredEVMSales{}).Sale(t.Context(), "base:0x", 1); errs.CodeOf(err) != errs.CodeNotImplemented {
		t.Fatalf("Sale err = %v, want not_implemented", err)
	}
}

func TestSellForCashOut_refusesAnEVMLotWithoutAListingID(t *testing.T) {
	t.Parallel()
	e := newEVMCashOut(t, 50_000_000)
	e.lots.sale.Listing.ID = multichain.ListingID{}

	if err := e.cashOut(t, 60_000_000); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("Handle err = %v, want invalid_input", err)
	}
	if n := len(e.legs(t)); n != 0 {
		t.Fatalf("%d legs, want none", n)
	}
}

func TestSellForCashOut_aSolanaAndABaseLotBothSellToCoverMoreThanTheLargerLot(t *testing.T) {
	t.Parallel()
	e := newEVMCashOut(t, 50_000_000)
	const needed = 80_000_000

	if err := e.cashOut(t, needed); err != nil {
		t.Fatal(err)
	}

	var quoted, legs int64
	if err := e.pool.QueryRow(t.Context(), `SELECT coalesce(sum(quote_out_amount), 0), count(*) FROM swaps
		WHERE source_kind = 'cashout' AND source_id = $1 AND status = 'confirmed'`, e.job).Scan(&quoted, &legs); err != nil {
		t.Fatal(err)
	}
	if legs != 2 || quoted < needed {
		t.Fatalf("%d confirmed legs quoting %d, want both lots sold for at least the %d needed", legs, quoted, needed)
	}
	if got := e.legs(t); got[0].InMint != aaplxMint || got[0].InAmount != 1_000 || got[1].InMint != e.token() {
		t.Fatalf("legs = %+v, want the whole Solana lot then the Base lot read from the chain-aware ledger", got)
	}
}

type brokenListings struct{ cashOutLots }

func (brokenListings) CabalPositionsAt(context.Context, time.Time) ([]treasuryport.CabalPositions, error) {
	return []treasuryport.CabalPositions{{Listings: []treasuryport.ListingPosition{{Key: "base:0x"}}}}, nil
}

func TestLedgerHoldings_refusesAListingPositionItCannotRead(t *testing.T) {
	t.Parallel()
	holdings := app.LedgerHoldings{
		Ledger: &brokenListings{}, Catalog: marketfake.NewCatalog(marketfake.Fixtures()...), USDC: usdcToken(),
		Clock: testkit.NewClock(clock.Real{}.Now()),
	}
	if _, err := holdings.Positions(t.Context(), ids.CabalID{}); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("Positions err = %v, want decode_failed", err)
	}
	if _, err := (app.UnwiredPositions{}).CabalPositionsAt(
		t.Context(),
		time.Time{},
	); errs.CodeOf(
		err,
	) != errs.CodeUpstreamUnavailable {
		t.Fatalf("CabalPositionsAt err = %v, want upstream_unavailable", err)
	}
}
