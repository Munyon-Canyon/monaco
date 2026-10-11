package jupiterquote_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/jupiterquote"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	mc "github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func quotedAt() time.Time { return time.Date(2026, 10, 11, 15, 0, 0, 0, time.UTC) }

const aaplxMint = "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp"

func newQuoter(t *testing.T) (jupiterquote.Quoter, string) {
	t.Helper()
	srv := httptest.NewServer(fakes.New())
	t.Cleanup(srv.Close)
	client := jupiter.New(config.Config{
		Jupiter: config.Jupiter{
			SwapBaseURL:  srv.URL + "/jupiter/swap/v2",
			PriceBaseURL: srv.URL + "/jupiter/price/v3",
		},
		Timeouts: config.Timeouts{JupiterQuote: 5 * time.Second, JupiterExecute: 5 * time.Second},
	}, testkit.NewClock(quotedAt()))
	return jupiterquote.NewQuoter(client, testkit.NewClock(quotedAt())), srv.URL
}

func aaplx(t *testing.T) mc.Listing {
	t.Helper()
	var id mc.ListingID
	if err := id.UnmarshalText([]byte("0199d2f4-5a10-7000-8000-000000000002")); err != nil {
		t.Fatal(err)
	}
	return mc.Listing{
		ID: id, Network: chain.NetworkSolana, TokenAddress: aaplxMint,
		Decimals: 8, Venue: mc.VenueJupiter,
	}
}

func TestQuoter_BuyQuotesUSDCIntoTheListingToken(t *testing.T) {
	t.Parallel()
	q, _ := newQuoter(t)
	l := aaplx(t)
	got, err := q.Quote(t.Context(), l, mc.SideBuy, money.MicrosFromUint64(100_000_000))
	if err != nil {
		t.Fatalf("Quote: %v", err)
	}
	if got.GrossOutput.String() != "44000000" || got.InputMicros.Uint64() != 100_000_000 {
		t.Errorf("GrossOutput = %s, InputMicros = %s, want 44000000 and 100000000", got.GrossOutput, got.InputMicros)
	}
	if got.ListingID != l.ID || got.Network != chain.NetworkSolana || got.Venue != mc.VenueJupiter ||
		got.Side != mc.SideBuy || len(got.Costs) != 0 {
		t.Errorf("quote = %+v", got)
	}
	if !got.QuotedAt.Equal(quotedAt()) || !got.ExpiresAt.Equal(quotedAt().Add(30*time.Second)) {
		t.Errorf("QuotedAt = %v, ExpiresAt = %v", got.QuotedAt, got.ExpiresAt)
	}
}

func TestQuoter_UnroutableIsNoRoute(t *testing.T) {
	t.Parallel()
	q, base := newQuoter(t)
	step := `{"route":"/jupiter/swap/v2/order","action":"succeed","fixture":"/jupiter/swap/v2/order/no-route"}`
	control := httpclient.New("fakes", httpclient.WithBaseURL(base), httpclient.WithTimeout(5*time.Second))
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, "/_script", strings.NewReader(step))
	resp, err := control.Do(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("script = %d", resp.StatusCode)
	}
	_, err = q.Quote(t.Context(), aaplx(t), mc.SideBuy, money.MicrosFromUint64(100_000_000))
	if errs.CodeOf(err) != errs.CodeNoRoute {
		t.Fatalf("err = %v, want %s", err, errs.CodeNoRoute)
	}
}

func TestQuoter_SellsAndEVMListingsAreInvalidInput(t *testing.T) {
	t.Parallel()
	q, _ := newQuoter(t)
	evm := aaplx(t)
	evm.Network = chain.NetworkEthereum
	for name, tc := range map[string]struct {
		l    mc.Listing
		side mc.Side
	}{"sell": {aaplx(t), mc.SideSell}, "evm": {evm, mc.SideBuy}} {
		_, err := q.Quote(t.Context(), tc.l, tc.side, money.MicrosFromUint64(100_000_000))
		if errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Errorf("%s: err = %v, want %s", name, err, errs.CodeInvalidInput)
		}
	}
}
