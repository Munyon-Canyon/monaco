package adapters

import (
	"context"
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/marketapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type scriptedBestChain struct {
	best  app.BestChain
	err   error
	asked []string
}

func (s *scriptedBestChain) Get(_ context.Context, symbol string) (app.BestChain, error) {
	s.asked = append(s.asked, symbol)
	return s.best, s.err
}

func bestChainFixture() app.BestChain {
	g := testkit.NewIDs(5364)
	at := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	sol, base, arb := multichain.ListingIDFrom(g.NewV7()), multichain.ListingIDFrom(g.NewV7()),
		multichain.ListingIDFrom(g.NewV7())
	quote := func(id multichain.ListingID, issuer string, n chain.Network, v multichain.Venue, shares uint64,
		costs ...multichain.Cost,
	) multichain.Quote {
		return multichain.Quote{
			ListingID: id, Issuer: issuer, Network: n, Venue: v, Side: multichain.SideBuy,
			InputMicros: money.MicrosFromUint64(1_000_000), NetMicros: money.MicrosFromUint64(1_000_000),
			GrossOutput: big.NewInt(1), NetShares: multichain.ShareNanos(shares), Costs: costs,
			QuotedAt: at, ExpiresAt: at.Add(30 * time.Second),
		}
	}
	return app.BestChain{
		Symbol: "AAPLx", Amount: app.BestChainProbe(), QuotedAt: at,
		Comparison: multichain.Comparison{
			Side: multichain.SideBuy, Winner: sol,
			Ranked: []multichain.Quote{
				quote(sol, "xstocks", chain.NetworkSolana, multichain.VenueJupiter, 2_970_763),
				quote(base, "coinbase", chain.NetworkBase, multichain.VenueUniswap, 2_835_000,
					multichain.Cost{Kind: multichain.CostBridge, Micros: money.MicrosFromUint64(750)},
					multichain.Cost{Kind: multichain.CostGas, Micros: money.MicrosFromUint64(76_119)}),
			},
			Unavailable: []multichain.UnavailableVenue{{
				ListingID: arb, Network: chain.NetworkArbitrum, Venue: multichain.VenueUniswap,
				Reason: multichain.UnavailableSimulationFailed, Detail: app.NotDelegatedDetail,
			}},
		},
	}
}

func TestGetAssetBestChainAnswersTheWinnerAndEveryChain(t *testing.T) {
	t.Parallel()
	best := bestChainFixture()
	fake := &scriptedBestChain{best: best}
	user := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: testkit.NewIDs(1).NewV7().String()})
	got, err := HTTP{BestChain: fake, Clock: clock.Real{}}.GetAssetBestChain(user,
		api.GetAssetBestChainRequestObject{Symbol: "AAPLx"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	r := best.Comparison.Ranked
	u := best.Comparison.Unavailable[0]
	want := `{"amount_micros":"1000000","listing_id":"` + r[0].ListingID.String() + `","net_share_nanos":"2970763",` +
		`"network":"solana","price_comparison":{"quotes":[` +
		`{"costs":[],"expires_at":"2026-10-11T12:00:30Z","issuer":"xstocks","listing_id":"` + r[0].ListingID.String() +
		`","net_micros":"1000000","net_share_nanos":"2970763","network":"solana","quoted_at":"2026-10-11T12:00:00Z",` +
		`"venue":"jupiter"},` +
		`{"costs":[{"kind":"bridge","micros":"750"},{"kind":"gas","micros":"76119"}],` +
		`"expires_at":"2026-10-11T12:00:30Z","issuer":"coinbase","listing_id":"` + r[1].ListingID.String() +
		`","net_micros":"1000000","net_share_nanos":"2835000","network":"base","quoted_at":"2026-10-11T12:00:00Z",` +
		`"venue":"uniswap"}],` +
		`"unavailable":[{"listing_id":"` + u.ListingID.String() + `","network":"arbitrum",` +
		`"reason":"simulation_failed","reason_detail":"not set up on this chain yet","venue":"uniswap"}],` +
		`"winner_listing_id":"` + r[0].ListingID.String() + `"},` +
		`"quoted_at":"2026-10-11T12:00:00Z","stale":false,"symbol":"AAPLx","venue":"jupiter"}`
	if string(raw) != want {
		t.Fatalf("body =\n%s\nwant\n%s", raw, want)
	}
	if len(fake.asked) != 1 || fake.asked[0] != "AAPLx" {
		t.Fatalf("asked = %v", fake.asked)
	}
}

func TestGetAssetBestChainMarksTheLastGoodAnswerStale(t *testing.T) {
	t.Parallel()
	best := bestChainFixture()
	best.Stale = true
	user := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: testkit.NewIDs(1).NewV7().String()})
	got, err := HTTP{BestChain: &scriptedBestChain{best: best}, Clock: clock.Real{}}.GetAssetBestChain(user,
		api.GetAssetBestChainRequestObject{Symbol: "AAPLx"})
	if err != nil {
		t.Fatal(err)
	}
	body, ok := got.(api.GetAssetBestChain200JSONResponse)
	if !ok || body.Stale == nil || !*body.Stale {
		t.Fatalf("response = %+v, want stale true", got)
	}
}

func TestGetAssetBestChainRefusesStrangersAndPassesErrorsThrough(t *testing.T) {
	t.Parallel()
	fake := &scriptedBestChain{err: errs.New(errs.CodeNoRoute, "test")}
	req := api.GetAssetBestChainRequestObject{Symbol: "AAPLx"}
	if _, err := (HTTP{BestChain: fake}).GetAssetBestChain(
		t.Context(),
		req,
	); errs.CodeOf(
		err,
	) != errs.CodeUnauthorized {
		t.Fatalf("anonymous err = %v, want unauthorized", err)
	}
	if len(fake.asked) != 0 {
		t.Fatalf("an anonymous caller reached the cache: %v", fake.asked)
	}
	user := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: testkit.NewIDs(1).NewV7().String()})
	if _, err := (HTTP{BestChain: fake}).GetAssetBestChain(user, req); errs.CodeOf(err) != errs.CodeNoRoute {
		t.Fatalf("err = %v, want no_route", err)
	}
}

func TestDeferRouterBuildsTheRouterOnceOnFirstUse(t *testing.T) {
	t.Parallel()
	builds := 0
	r := DeferRouter(func() *app.Router {
		builds++
		return MultichainRoutingRouter(routingDeps(""), nil)
	})
	if builds != 0 {
		t.Fatalf("built %d times before use", builds)
	}
	for range 2 {
		if _, err := r.Best(
			t.Context(),
			multichain.StockID{},
			multichain.SideSell,
			money.MicrosFromUint64(1),
		); errs.CodeOf(
			err,
		) != errs.CodeInvalidInput {
			t.Fatalf("Best err = %v, want invalid_input for a sell", err)
		}
	}
	if builds != 1 {
		t.Fatalf("built %d times, want 1", builds)
	}
}

type scriptedDetail struct{ view app.AssetView }

func (s scriptedDetail) Handle(context.Context, string) (app.AssetView, error) { return s.view, nil }

func detailBody(t *testing.T, view app.AssetView, now time.Time, poll time.Duration) api.AssetDetail {
	t.Helper()
	user := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: testkit.NewIDs(1).NewV7().String()})
	h := HTTP{Detail: scriptedDetail{view: view}, Clock: testkit.NewClock(now), PollInterval: poll}
	got, err := h.GetAsset(user, api.GetAssetRequestObject{Symbol: "NVDAx"})
	if err != nil {
		t.Fatal(err)
	}
	ok, isOK := got.(api.GetAsset200JSONResponse)
	if !isOK {
		t.Fatalf("response = %T", got)
	}
	return api.AssetDetail(ok)
}

func pricedView(at time.Time, source domain.Source) app.AssetView {
	return app.AssetView{Summary: app.Summary{
		Asset:  domain.Asset{Symbol: "NVDAx"},
		Priced: true,
		Price:  domain.Sample{Micros: money.MicrosFromUint64(110_000_000), ObservedAt: at, Source: source},
	}}
}

func TestAssetDetail_priceSourceFollowsTheSample(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 28, 14, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name string
		view app.AssetView
		want *api.AssetDetailPriceSource
	}{
		{"jupiter", pricedView(now, domain.SourceJupiter), ptr(api.AssetDetailPriceSourceJupiter)},
		{"coingecko", pricedView(now, domain.SourceCoinGecko), ptr(api.AssetDetailPriceSourceCoingecko)},
		{"unknown source", pricedView(now, ""), nil},
		{"unpriced", app.AssetView{Summary: app.Summary{Asset: domain.Asset{Symbol: "NVDAx"}}}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := detailBody(t, tc.view, now, 120*time.Second).PriceSource
			switch {
			case tc.want == nil && got != nil:
				t.Fatalf("price_source = %q, want null", *got)
			case tc.want != nil && (got == nil || *got != *tc.want):
				t.Fatalf("price_source = %v, want %q", got, *tc.want)
			}
		})
	}
}

func TestAssetDetail_pollIntervalFromConfig(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 28, 14, 0, 0, 0, time.UTC)
	view := pricedView(now, domain.SourceJupiter)
	for _, tc := range []struct {
		poll time.Duration
		want int
	}{{120 * time.Second, 120}, {45 * time.Second, 45}} {
		if got := detailBody(t, view, now, tc.poll).PricePollIntervalSeconds; got != tc.want {
			t.Fatalf("price_poll_interval_seconds for %s = %d, want %d", tc.poll, got, tc.want)
		}
	}
}

func TestAssetDetail_staleSampleKeepsItsTime(t *testing.T) {
	t.Parallel()
	sampled := time.Date(2026, 10, 28, 14, 0, 0, 0, time.UTC)
	now := sampled.Add(10 * 120 * time.Second)
	got := detailBody(t, pricedView(sampled, domain.SourceJupiter), now, 120*time.Second)
	if got.PriceAsOf == nil || !got.PriceAsOf.Equal(sampled) {
		t.Fatalf("price_as_of = %v, want %s", got.PriceAsOf, sampled)
	}
}

func ptr[T any](v T) *T { return &v }

func TestAsset_wiresTheAssetID(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 28, 14, 0, 0, 0, time.UTC)
	id := domain.NewAssetID(testkit.NewIDs(1))
	view := pricedView(now, domain.SourceJupiter)
	view.Summary.Asset.ID = id
	if got := detailBody(t, view, now, 120*time.Second).Id; got != id.UUID() {
		t.Fatalf("detail id = %s, want %s", got, id.UUID())
	}
	summary, err := wireSummary(view.Summary, now)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Id != id.UUID() {
		t.Fatalf("summary id = %s, want %s", summary.Id, id.UUID())
	}
}
