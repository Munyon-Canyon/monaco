package adapters

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type oneAccount struct{}

func (oneAccount) SimulationAccount(context.Context) (chain.EVMAddress, bool, error) {
	a, err := chain.ParseEVMAddress("0x000000000000000000000000000000000000dead")
	return a, true, err
}

func TestEveryRoutingUniswapClientSharesTheProcessRateLimit(t *testing.T) {
	t.Parallel()
	var quotes atomic.Int32
	uni := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		quotes.Add(1)
		_, _ = io.WriteString(w, `{"errorCode":"NoRouteFoundError"}`)
	}))
	t.Cleanup(uni.Close)
	deps := routingDeps("uni-key")
	deps.Config.Routing.UniswapBaseURL = uni.URL
	deps.Uniswap = rate.NewLimiter(rate.Every(time.Hour), 1)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	sell := func(v app.Venues, n chain.Network, token string) error {
		l := multichain.Listing{Network: n, TokenAddress: token, Venue: multichain.VenueUniswap}
		_, err := v.Quote(ctx, l, multichain.SideSell, money.MicrosFromUint64(1_000_000))
		return err
	}

	first := sell(
		routingVenues(deps, oneAccount{}),
		chain.NetworkEthereum,
		"0x14c3abf95cb9c93a8b82c1cdcb76d72cb87b2d4c",
	)
	second := sell(routingVenues(deps, oneAccount{}), chain.NetworkBase, "0xb200000000000000000000c2e324d24d7eecd1fb")

	if errs.CodeOf(first) != errs.CodeNoRoute || errs.CodeOf(second) != errs.CodeUpstreamUnavailable ||
		quotes.Load() != 1 {
		t.Fatalf("first = %v, second = %v after %d calls, want the second client held by the first's limit",
			first, second, quotes.Load())
	}
}
