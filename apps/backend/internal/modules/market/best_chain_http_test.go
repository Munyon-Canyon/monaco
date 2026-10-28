package market_test

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/marketapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

type dollarRouter struct {
	mu      sync.Mutex
	clk     clock.Clock
	listing multichain.ListingID
	asked   []money.Micros
}

func (r *dollarRouter) Best(
	_ context.Context, stock multichain.StockID, side multichain.Side, amount money.Micros,
) (multichain.Comparison, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.asked = append(r.asked, amount)
	at := r.clk.Now()
	q := multichain.Quote{
		ListingID: r.listing, Issuer: "xstocks", Network: chain.NetworkSolana, Venue: multichain.VenueJupiter,
		Side: side, InputMicros: amount, NetMicros: amount, GrossOutput: big.NewInt(1), NetShares: 2_970_763,
		Costs: []multichain.Cost{}, QuotedAt: at, ExpiresAt: at.Add(30 * time.Second),
	}
	return multichain.Comparison{
		StockID: stock, Side: side, Winner: q.ListingID, Ranked: []multichain.Quote{q},
		Unavailable: []multichain.UnavailableVenue{},
	}, nil
}

func (r *dollarRouter) calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.asked)
}

func newBestChainAPI(t *testing.T) (marketAPI, *dollarRouter) {
	t.Helper()
	router := &dollarRouter{listing: multichain.ListingIDFrom(testkit.NewIDs(5364).NewV7())}
	s := newMarketAPIMounting(t, marketWhen(), func(d module.Deps) func(apibase.Mount) {
		router.clk = d.Clock
		return func(r apibase.Mount) {
			api.Mount(adapters.HTTP{
				BestChain: app.NewBestChains(app.NewCatalog(d.Pool), router, d.Clock), Clock: d.Clock,
			}, r)
		}
	})
	s.seedFixtures(t)
	return s, router
}

func TestBestChain_answersTheWinnerOfADollarBuyAndSharesItForTwentySeconds(t *testing.T) {
	t.Parallel()
	s, router := newBestChainAPI(t)
	rec := s.get(t, "/v1/assets/AAPLx/best-chain")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET best-chain = %d %s", rec.Code, rec.Body)
	}
	expectDollarWinner(t, rec.Body.Bytes(), router.listing.UUID(), s.when)
	if len(router.asked) != 1 || router.asked[0].Uint64() != 1_000_000 {
		t.Fatalf("router asked for %v, want one $1 buy", router.asked)
	}
	s.clock.Advance(app.BestChainFresh - time.Second)
	if again := s.get(t, "/v1/assets/AAPLx/best-chain"); again.Code != http.StatusOK || router.calls() != 1 {
		t.Fatalf("second member = %d, router calls %d, want the shared answer", again.Code, router.calls())
	}
	s.clock.Advance(app.BestChainServed)
	if late := s.get(t, "/v1/assets/AAPLx/best-chain"); late.Code != http.StatusOK || router.calls() != 2 {
		t.Fatalf("a minute later = %d, router calls %d, want a refresh before answering", late.Code, router.calls())
	}
}

func expectDollarWinner(t *testing.T, body []byte, listing uuid.UUID, at time.Time) {
	t.Helper()
	var got api.AssetBestChain
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Symbol != "AAPLx" || got.AmountMicros != "1000000" || !got.QuotedAt.Equal(at) {
		t.Fatalf("best chain = %s, want AAPLx for $1 at %v", body, at)
	}
	if got.Network != apibase.Solana || got.Venue != api.AssetBestChainVenueJupiter || got.ListingId != listing ||
		got.NetShareNanos != "2970763" || got.PriceComparison.WinnerListingId != listing ||
		len(got.PriceComparison.Quotes) != 1 {
		t.Fatalf("best chain = %s, want the Solana listing", body)
	}
}

func TestBestChain_refusesAnUnknownStockAndAStranger(t *testing.T) {
	t.Parallel()
	s, router := newBestChainAPI(t)
	for _, tc := range []struct {
		path, token string
		status      int
		code        apibase.ErrorCode
	}{
		{"/v1/assets/NOPEx/best-chain", s.token, http.StatusNotFound, apibase.AssetNotFound},
		{"/v1/assets/" + marketfake.JPSTx().Symbol + "/best-chain", s.token, http.StatusUnprocessableEntity, apibase.AssetUntradable},
		{"/v1/assets/AAPLx/best-chain", "", http.StatusUnauthorized, apibase.Unauthorized},
	} {
		rec := s.getAs(t, tc.path, tc.token)
		if rec.Code != tc.status || problemCode(t, rec) != tc.code {
			t.Errorf("%s = %d %s, want %d %s", tc.path, rec.Code, rec.Body, tc.status, tc.code)
		}
	}
	if router.calls() != 0 {
		t.Fatalf("router calls = %d, want none", router.calls())
	}
}
