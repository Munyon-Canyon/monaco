package trading_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const aaplOndoMint = "gEGtLTPNQ7jcg25zTetkbmF7teoDLcrfTnQfmn2test"

func aaplOndoToken() chain.Mint { return chain.Mint{Address: aaplOndoMint, Decimals: 9} }

func (e *engineEnv) ondoThenXStocks(t *testing.T) (ondo, xstocks multichain.ListingID, cmd app.ExecuteTrade) {
	t.Helper()
	ondo = e.solanaWinner(t, aaplOndoToken(), true)
	xstocks = e.solanaWinner(t, aaplxToken(), true)
	e.quote(usdcToken(), aaplOndoToken(), quotedOut*10, true)
	cmd = e.buy()
	cmd.Route = e.comparison(t, ondo)
	at := e.clk.Now()
	cmd.Route.Ranked = []multichain.Quote{
		{
			ListingID: ondo, Network: chain.NetworkSolana, Venue: multichain.VenueJupiter, Side: multichain.SideBuy,
			InputMicros: money.MicrosFromUint64(buyMicros), NetShares: 306_100_000,
			NetMicros: money.MicrosFromUint64(buyMicros), QuotedAt: at, ExpiresAt: at.Add(30 * time.Second),
		},
		{
			ListingID: xstocks, Network: chain.NetworkSolana, Venue: multichain.VenueJupiter, Side: multichain.SideBuy,
			InputMicros: money.MicrosFromUint64(buyMicros), NetShares: 306_000_000,
			NetMicros: money.MicrosFromUint64(buyMicros), QuotedAt: at, ExpiresAt: at.Add(30 * time.Second),
		},
	}
	return ondo, xstocks, cmd
}

func (e *engineEnv) outMintOf(t *testing.T, swap uuid.UUID) (outMint, status string) {
	t.Helper()
	err := e.pool.QueryRow(t.Context(), `SELECT out_mint, status FROM swaps WHERE id = $1`, swap).
		Scan(&outMint, &status)
	if err != nil {
		t.Fatal(err)
	}
	return outMint, status
}

func TestExecuteTrade_aSolanaWinnerJupiterRefusesFallsBackToTheNextSolanaListing(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	_, xstocks, cmd := e.ondoThenXStocks(t)

	if err := e.handle(t, e.delivery(t, cmd), cmd); err != nil {
		t.Fatal(err)
	}
	swaps := e.swapsOf(t, cmd.ProposalID)
	if len(swaps) != 1 {
		t.Fatalf("%d swaps, want one: the refused winner creates no row", len(swaps))
	}
	if out, status := e.outMintOf(t, swaps[0]); out != aaplxMint || status != "confirmed" {
		t.Fatalf("out_mint %s status %s, want the xStocks mint %s confirmed", out, status, aaplxMint)
	}
	if winner, _, ok := e.routeOf(t, swaps[0]); !ok || winner != xstocks.UUID() {
		t.Fatalf("route winner %v %s, want the xStocks listing %s", ok, winner, xstocks.UUID())
	}
}

func TestExecuteTrade_aSolanaWinnerJupiterAcceptsKeepsItsListing(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	ondo, _, cmd := e.ondoThenXStocks(t)
	e.jup.SetOrder(jupiterMint(usdcToken()), jupiterMint(aaplOndoToken()), jupiter.Order{
		RequestID: "req-1", Transaction: swapTx(),
	})

	if err := e.handle(t, e.delivery(t, cmd), cmd); err != nil {
		t.Fatal(err)
	}
	swaps := e.swapsOf(t, cmd.ProposalID)
	if len(swaps) != 1 {
		t.Fatalf("%d swaps, want one", len(swaps))
	}
	if out, status := e.outMintOf(t, swaps[0]); out != aaplOndoMint || status != "confirmed" {
		t.Fatalf("out_mint %s status %s, want the Ondo mint confirmed", out, status)
	}
	if winner, _, ok := e.routeOf(t, swaps[0]); !ok || winner != ondo.UUID() {
		t.Fatalf("route winner %v %s, want the Ondo listing %s", ok, winner, ondo.UUID())
	}
}

type payerlessSigner struct {
	app.Signer
	err error
}

func (p payerlessSigner) FeePayer() (chain.SolanaAddress, error) { return "", p.err }

func TestExecuteTrade_aFeePayerFailureBeforeTheFallbackOrderFailsTheTrade(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	_, _, cmd := e.ondoThenXStocks(t)
	cause := errs.New(errs.CodeRPCUnavailable, "relayer.FeePayer")
	e.signer = payerlessSigner{Signer: e.signer, err: cause}

	err := e.handle(t, e.delivery(t, cmd), cmd)
	if !errors.Is(err, cause) {
		t.Fatalf("handle = %v, want the fee payer error %v", err, cause)
	}
	if n := len(e.swapsOf(t, cmd.ProposalID)); n != 0 {
		t.Fatalf("%d swaps, want none", n)
	}
}
