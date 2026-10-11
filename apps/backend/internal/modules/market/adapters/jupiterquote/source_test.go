package jupiterquote_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/jupiterquote"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	mc "github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func jupiterAt(t *testing.T, h http.Handler) *jupiter.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return jupiter.New(config.Config{
		Jupiter: config.Jupiter{
			SwapBaseURL:  srv.URL + "/jupiter/swap/v2",
			PriceBaseURL: srv.URL + "/jupiter/price/v3",
		},
		Timeouts: config.Timeouts{JupiterQuote: 5 * time.Second, JupiterExecute: 5 * time.Second},
	}, testkit.NewClock(quotedAt()))
}

func forbidden() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errorCode":403,"error":"forbidden"}`))
	})
}

func mints(t *testing.T) (domain.Mint, domain.Mint) {
	t.Helper()
	usdc, err := domain.ParseMint(chain.NetworkSolana.Cash().Address)
	if err != nil {
		t.Fatal(err)
	}
	stock, err := domain.ParseMint(aaplxMint)
	if err != nil {
		t.Fatal(err)
	}
	return usdc, stock
}

func TestQuoter_RejectedOrderPassesJupitersErrorThrough(t *testing.T) {
	t.Parallel()
	q := jupiterquote.NewQuoter(jupiterAt(t, forbidden()), testkit.NewClock(quotedAt()))
	_, err := q.Quote(t.Context(), aaplx(t), mc.SideBuy, money.MicrosFromUint64(100_000_000))
	if errs.CodeOf(err) != errs.CodeJupiterRejected {
		t.Fatalf("err = %v, want %s", err, errs.CodeJupiterRejected)
	}
}

func TestQuoter_ZeroAmountIsInvalidInput(t *testing.T) {
	t.Parallel()
	q, _ := newQuoter(t)
	_, err := q.Quote(t.Context(), aaplx(t), mc.SideBuy, money.MicrosFromUint64(0))
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("err = %v, want %s", err, errs.CodeInvalidInput)
	}
}

func TestSource_QuotesTheMintPairThroughJupiter(t *testing.T) {
	t.Parallel()
	usdc, stock := mints(t)
	got, err := jupiterquote.New(jupiterAt(t, fakes.New())).Quote(t.Context(), usdc, stock,
		money.NewBaseUnits(100_000_000, 6))
	if err != nil {
		t.Fatalf("Quote: %v", err)
	}
	want := app.Quote{
		InAmount: money.NewBaseUnits(100_000_000, 6), OutAmount: money.NewBaseUnits(44_000_000, 6),
		PriceImpactBps: 12, Routable: true,
	}
	if got != want {
		t.Errorf("quote = %+v, want %+v", got, want)
	}
}

func TestSource_RejectedOrderPassesJupitersErrorThrough(t *testing.T) {
	t.Parallel()
	usdc, stock := mints(t)
	_, err := jupiterquote.New(jupiterAt(t, forbidden())).Quote(t.Context(), usdc, stock,
		money.NewBaseUnits(100_000_000, 6))
	if errs.CodeOf(err) != errs.CodeJupiterRejected {
		t.Fatalf("err = %v, want %s", err, errs.CodeJupiterRejected)
	}
}
