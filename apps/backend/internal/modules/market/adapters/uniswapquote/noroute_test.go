package uniswapquote_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/uniswapquote"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	mc "github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes/uniswap"
)

func clockedHarness(t *testing.T) (harness, *testkit.Clock) {
	t.Helper()
	fake := uniswap.New()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	swapper, _ := chain.ParseEVMAddress("0x000000000000000000000000000000000000dead")
	clk := testkit.NewClock(quotedAt())
	hc := httpclient.New("uniswap-noroute-test", uniswapquote.Options(srv.URL, 5*time.Second)...)
	return harness{fake: fake, client: uniswapquote.New(hc, "test-key", uniswapquote.Stored(swapper), clk)}, clk
}

func (h harness) buyAAPLxOnArbitrum(t *testing.T) error {
	t.Helper()
	_, err := h.client.Quote(t.Context(), listing(t, chain.NetworkArbitrum, aaplx, 18), mc.SideBuy,
		money.MicrosFromUint64(hundred))
	return err
}

func TestQuote_remembersANoRouteForThreeMinutes(t *testing.T) {
	t.Parallel()
	h, clk := clockedHarness(t)

	first, again := h.buyAAPLxOnArbitrum(t), h.buyAAPLxOnArbitrum(t)
	if errs.CodeOf(first) != errs.CodeNoRoute || errs.CodeOf(again) != errs.CodeNoRoute || len(h.fake.Requests()) != 1 {
		t.Fatalf("errs = %v, %v after %d calls, want no_route twice from one call", first, again,
			len(h.fake.Requests()))
	}
	clk.Advance(3*time.Minute - time.Second)
	if err := h.buyAAPLxOnArbitrum(t); errs.CodeOf(err) != errs.CodeNoRoute || len(h.fake.Requests()) != 1 {
		t.Fatalf("err = %v after %d calls, want the remembered no_route just inside three minutes", err,
			len(h.fake.Requests()))
	}
	clk.Advance(time.Second)
	if err := h.buyAAPLxOnArbitrum(t); errs.CodeOf(err) != errs.CodeNoRoute || len(h.fake.Requests()) != 2 {
		t.Fatalf("err = %v after %d calls, want Uniswap asked again after three minutes", err,
			len(h.fake.Requests()))
	}
}

func TestQuote_neverRemembersAThrottledOrFailedQuote(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			h, _ := clockedHarness(t)
			h.fake.FailNext(status)
			failed := h.buyAAPLxOnArbitrum(t)
			asked := len(h.fake.Requests())
			if err := h.buyAAPLxOnArbitrum(t); errs.CodeOf(failed) != errs.CodeUpstreamUnavailable ||
				errs.CodeOf(err) != errs.CodeNoRoute || len(h.fake.Requests()) != asked+1 {
				t.Fatalf("errs = %v, %v after %d then %d calls, want the failure forgotten and Uniswap asked again",
					failed, err, asked, len(h.fake.Requests()))
			}
		})
	}
}

func TestWrapper_asksNeitherRouteAgainAfterBothFoundNoRoute(t *testing.T) {
	t.Parallel()
	r := newWrapperRig(t, chain.NetworkBase)
	r.quote = `{"errorCode":"NoRouteFoundError"}`

	_, first := r.buy(t, chain.NetworkBase)
	_, again := r.buy(t, chain.NetworkBase)

	if errs.CodeOf(first) != errs.CodeNoRoute || errs.CodeOf(again) != errs.CodeNoRoute || len(r.quotes) != 2 {
		t.Fatalf("errs = %v, %v after %d quotes, want both routes asked once", first, again, len(r.quotes))
	}
}
