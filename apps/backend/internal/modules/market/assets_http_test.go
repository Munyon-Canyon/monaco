package market_test

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/trace/noop"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/marketapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func TestAssets_SearchByName(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedFixtures(t)
	mint := marketfake.AAPLx().Mint.String()
	s.price(t, mint, -18*time.Hour-time.Minute, 100_000_000)
	s.price(t, mint, -30*time.Minute, 110_000_000)
	s.price(t, mint, -time.Minute, 200_000_000)
	page := pageOf(t, s.get(t, "/v1/assets?q=ApPle"))
	if len(page.Assets) != 1 || page.Assets[0].Symbol != "AAPLx" || page.NextCursor != nil {
		t.Fatalf("search = %+v", page)
	}
	assertAppleQuote(t, page.Assets[0])
	if hidden := pageOf(t, s.get(t, "/v1/assets?q=jpmorgan")); len(hidden.Assets) != 0 {
		t.Fatalf("untradable search = %+v", hidden.Assets)
	}
	prefix := pageOf(t, s.get(t, "/v1/assets?q=aap"))
	if len(prefix.Assets) != 1 || prefix.Assets[0].Symbol != "AAPLx" {
		t.Fatalf("prefix = %+v", prefix.Assets)
	}
}

func TestAssets_fixtureCatalogDoesNotExposeIssuerBranding(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedBrandedCatalogFixtures(t)
	for _, a := range pageOf(t, s.get(t, "/v1/assets")).Assets {
		if strings.Contains(strings.ToLower(a.DisplayName), "xstock") {
			t.Fatalf("%s display name = %q, contains issuer branding", a.Symbol, a.DisplayName)
		}
	}
}

func assertAppleQuote(t *testing.T, got api.AssetSummary) {
	t.Helper()
	if got.DisplayName != "Apple" || got.Issuer != api.Xstocks || got.Kind != api.AssetKindEquity ||
		got.Tradable == nil || !*got.Tradable {
		t.Fatalf("AAPLx identity = %+v", got)
	}
	assertApplePrice(t, got)
	if got.Session.State != api.MarketStateOpen || got.Session.Continuous || got.Session.NextState == nil ||
		*got.Session.NextState != api.MarketSessionNextStateAfterHours || got.Session.NextTransition == nil {
		t.Fatalf("session = %+v", got.Session)
	}
}

func assertApplePrice(t *testing.T, got api.AssetSummary) {
	t.Helper()
	wantAsOf := marketWhen().Add(-30 * time.Minute)
	if got.LogoUrl == nil || got.PriceMicros == nil || *got.PriceMicros != 110_000_000 {
		t.Fatalf("AAPLx price = %+v", got)
	}
	if got.PriceAsOf == nil || !got.PriceAsOf.Equal(wantAsOf) || got.ChangeBps == nil || *got.ChangeBps != 1000 {
		t.Fatalf("AAPLx change = %+v", got)
	}
	if got.SparklineMicros == nil || len(*got.SparklineMicros) != 2 {
		t.Fatalf("AAPLx sparkline = %+v", got.SparklineMicros)
	}
	if (*got.SparklineMicros)[0] != 100_000_000 || (*got.SparklineMicros)[1] != 110_000_000 {
		t.Fatalf("AAPLx sparkline = %+v", *got.SparklineMicros)
	}
}

func TestAssets_Popular(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedFixtures(t)
	first := pageOf(t, s.get(t, "/v1/assets?filter=popular&limit=1"))
	if len(first.Assets) != 1 || first.Assets[0].Symbol != "AAPLx" || first.NextCursor == nil {
		t.Fatalf("first popular page = %+v", first)
	}
	second := pageOf(t, s.get(t, "/v1/assets?filter=popular&limit=1&cursor="+url.QueryEscape(*first.NextCursor)))
	if len(second.Assets) != 1 || second.Assets[0].Symbol != "TSLAx" || second.NextCursor != nil {
		t.Fatalf("second popular page = %+v", second)
	}
}

func TestAssets_PreIPO(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedFixtures(t)
	space := s.insertClone(
		t,
		5,
		"SPACEx",
		"TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA",
		"SpaceX",
		domain.KindPreIPO,
		0,
	)
	s.price(t, space.Mint.String(), -30*time.Hour, 10_000_000)
	s.price(t, space.Mint.String(), -15*time.Hour+10*time.Minute, 50_000_000)
	s.price(t, space.Mint.String(), -time.Minute, 55_000_000)
	page := pageOf(t, s.get(t, "/v1/assets?filter=pre_ipo"))
	if len(page.Assets) != 1 || page.Assets[0].Symbol != "SPACEx" || page.Assets[0].Kind != api.AssetKindPreIpo {
		t.Fatalf("pre_ipo = %+v, want only SPACEx and not AAPLx", page.Assets)
	}
	got := page.Assets[0]
	if got.ChangeBps == nil || *got.ChangeBps != 0 || !got.Session.Continuous ||
		got.Session.NextState != nil || got.Session.NextTransition != nil || got.Session.State != api.MarketStateOpen {
		t.Fatalf("pre-IPO quote = %+v session %+v", got, got.Session)
	}
}

func TestAssets_PreIPOHoldsAnOpeningSpikeBeforeMeasuringTheChange(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	space := s.insertClone(
		t,
		5,
		"SPACEx",
		"TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA",
		"SpaceX",
		domain.KindPreIPO,
		0,
	)
	s.price(t, space.Mint.String(), -24*time.Hour-time.Minute, 100_000_000)
	s.price(t, space.Mint.String(), -14*time.Hour, 200_000_000)
	s.price(t, space.Mint.String(), -10*time.Minute, 100_000_000)
	s.price(t, space.Mint.String(), -5*time.Minute, 100_000_000)
	page := pageOf(t, s.get(t, "/v1/assets?filter=pre_ipo"))
	if len(page.Assets) != 1 || page.Assets[0].ChangeBps == nil || *page.Assets[0].ChangeBps != 0 {
		t.Fatalf("pre-IPO opening spike = %+v", page.Assets)
	}
}

func TestAssets_Cursor(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedFixtures(t)
	s.insertClone(t, 4, "MSFTx", "11111111111111111111111111111111", "Microsoft xStock", domain.KindEquity, 3)
	var got []string
	path := "/v1/assets?limit=1"
	for range 4 {
		page := pageOf(t, s.get(t, path))
		if len(page.Assets) != 1 {
			t.Fatalf("page from %s = %+v", path, page)
		}
		got = append(got, page.Assets[0].Symbol)
		if page.NextCursor == nil {
			break
		}
		path = "/v1/assets?limit=1&cursor=" + url.QueryEscape(*page.NextCursor)
	}
	want := []string{"AAPLx", "MSFTx", "TSLAx"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("cursor walk = %v, want %v", got, want)
	}
	if both := pageOf(t, s.get(t, "/v1/assets?limit=2")); len(both.Assets) != 2 {
		t.Fatalf("first page = %+v", both.Assets)
	}
}

func TestAssets_UnpricedIsNull(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	quiet := s.insertClone(
		t,
		6,
		"QUIEx",
		"EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
		"Quiet Holdings",
		domain.KindEquity,
		0,
	)
	quiet.LogoURL = ""
	if _, err := s.pool.Exec(t.Context(), `UPDATE assets SET logo_url = NULL WHERE symbol = 'QUIEx'`); err != nil {
		t.Fatal(err)
	}
	rec := s.get(t, "/v1/assets?q=quiet")
	page := pageOf(t, rec)
	if len(page.Assets) != 1 {
		t.Fatalf("unpriced = %+v", page)
	}
	got := page.Assets[0]
	if got.LogoUrl != nil || got.PriceMicros != nil || got.PriceAsOf != nil || got.ChangeBps != nil ||
		got.SparklineMicros != nil {
		t.Fatalf("unpriced fields = %+v", got)
	}
	if !json.Valid(rec.Body.Bytes()) ||
		!containsAll(rec.Body.String(), `"price_micros":null`, `"sparkline_micros":null`) {
		t.Fatalf("body = %s", rec.Body)
	}
}

func TestAssets_anOldPriceHasAnEmptySparkline(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedFixtures(t)
	s.price(t, marketfake.TSLAx().Mint.String(), -30*time.Hour, 80_000_000)
	page := pageOf(t, s.get(t, "/v1/assets?q=tesla"))
	if len(page.Assets) != 1 || page.Assets[0].PriceMicros == nil || *page.Assets[0].PriceMicros != 80_000_000 ||
		page.Assets[0].ChangeBps == nil || *page.Assets[0].ChangeBps != 0 ||
		page.Assets[0].SparklineMicros == nil || len(*page.Assets[0].SparklineMicros) != 0 {
		t.Fatalf("old price = %+v", page.Assets)
	}
}

func TestAssets_flagsADaysOldPriceAsStaleAndAFreshOneAsLive(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedFixtures(t)
	s.price(t, marketfake.TSLAx().Mint.String(), -30*time.Hour, 80_000_000)
	s.price(t, marketfake.AAPLx().Mint.String(), -time.Minute, 220_000_000)
	stale := pageOf(t, s.get(t, "/v1/assets?q=tesla")).Assets
	live := pageOf(t, s.get(t, "/v1/assets?q=apple")).Assets
	if len(stale) != 1 || stale[0].Stale == nil || !*stale[0].Stale || stale[0].PriceAsOf == nil {
		t.Fatalf("days-old price = %+v, want stale with its own price_as_of", stale)
	}
	if len(live) != 1 || live[0].Stale == nil || *live[0].Stale {
		t.Fatalf("fresh price = %+v, want live", live)
	}
	detail := s.get(t, "/v1/assets/TSLAx")
	if !containsAll(detail.Body.String(), `"stale":true`) {
		t.Fatalf("detail body = %s", detail.Body)
	}
}

func TestAssets_aClosedMarketKeepsItsCloseLiveAndDropsAReferenceOlderThanADay(t *testing.T) {
	t.Parallel()
	saturday := time.Date(2026, 3, 7, 15, 0, 0, 0, time.UTC)
	s := newMarketAPI(t, saturday)
	s.seedFixtures(t)
	s.price(t, marketfake.AAPLx().Mint.String(), -18*time.Hour-time.Minute, 220_000_000)
	s.price(t, marketfake.TSLAx().Mint.String(), -80*time.Hour, 80_000_000)
	closed := pageOf(t, s.get(t, "/v1/assets?q=apple")).Assets
	old := pageOf(t, s.get(t, "/v1/assets?q=tesla")).Assets
	if len(closed) != 1 || closed[0].Stale == nil || *closed[0].Stale {
		t.Fatalf("close sample = %+v, want live", closed)
	}
	if len(old) != 1 || old[0].Stale == nil || !*old[0].Stale || old[0].ChangeBps != nil {
		t.Fatalf("Wednesday sample on Saturday = %+v, want stale with no change", old)
	}
}

func TestAssets_omitsTheChangeWhenTheReferenceIsZero(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedFixtures(t)
	mint := marketfake.AAPLx().Mint.String()
	s.price(t, mint, -18*time.Hour-time.Minute, 0)
	s.price(t, mint, -10*time.Minute, 110_000_000)
	s.price(t, mint, -8*time.Minute, 110_000_000)
	s.price(t, mint, -6*time.Minute, 110_000_000)
	page := pageOf(t, s.get(t, "/v1/assets?q=aap"))
	if len(page.Assets) != 1 || page.Assets[0].PriceMicros == nil || *page.Assets[0].PriceMicros != 110_000_000 ||
		page.Assets[0].ChangeBps != nil {
		t.Fatalf("zero reference = %+v", page.Assets)
	}
}

func TestAssets_searchTreatsWildcardsAsText(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedFixtures(t)
	s.insertClone(t, 7, "PCTx", "So11111111111111111111111111111111111111112", "100% Fun_d", domain.KindEquity, 0)
	page := pageOf(t, s.get(t, "/v1/assets?q="+url.QueryEscape("100% Fun_d")))
	if len(page.Assets) != 1 || page.Assets[0].Symbol != "PCTx" {
		t.Fatalf("wildcard search = %+v", page.Assets)
	}
	if all := pageOf(
		t,
		s.get(t, "/v1/assets?q="+url.QueryEscape("%")),
	); len(all.Assets) != 1 ||
		all.Assets[0].Symbol != "PCTx" {
		t.Fatalf("a percent sign matched %+v", all.Assets)
	}
}

func TestAssets_rejectsABadRequest(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	cases := []struct {
		path   string
		token  string
		status int
		code   apibase.ErrorCode
	}{
		{path: "/v1/assets", status: http.StatusUnauthorized, code: apibase.Unauthorized},
		{path: "/v1/assets?filter=nope", token: s.token, status: http.StatusBadRequest, code: apibase.InvalidInput},
		{path: "/v1/assets?limit=51", token: s.token, status: http.StatusBadRequest, code: apibase.InvalidInput},
		{
			path:   "/v1/assets?cursor=" + url.QueryEscape("%%%"),
			token:  s.token,
			status: http.StatusBadRequest,
			code:   apibase.InvalidInput,
		},
	}
	for _, tc := range cases {
		rec := s.getAs(t, tc.path, tc.token)
		if rec.Code != tc.status || problemCode(t, rec) != tc.code {
			t.Errorf("%s = %d %s, want %d %s", tc.path, rec.Code, rec.Body, tc.status, tc.code)
		}
	}
}

func TestAssets_rejectsACallerThatIsNotAUser(t *testing.T) {
	t.Parallel()
	h := adapters.HTTP{List: app.NewListAssets(nil, testkit.NewClock(marketWhen()))}
	if _, err := h.GetAssets(t.Context(), api.GetAssetsRequestObject{}); errs.CodeOf(err) != errs.CodeUnauthorized {
		t.Fatalf("no actor = %v", err)
	}
	service := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorService, ID: "service"})
	if _, err := h.GetAssets(service, api.GetAssetsRequestObject{}); errs.CodeOf(err) != errs.CodeForbidden {
		t.Fatalf("service = %v", err)
	}
	bad := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: "not-a-uuid"})
	if _, err := h.GetAssets(bad, api.GetAssetsRequestObject{}); errs.CodeOf(err) != errs.CodeUnauthorized {
		t.Fatalf("bad user = %v", err)
	}
}

func TestAssets_reportsABrokenCatalog(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedFixtures(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	rec := s.getCtx(ctx, t, "/v1/assets", s.token)
	if rec.Code != 499 || problemCode(t, rec) != apibase.ClientClosed {
		t.Fatalf("cancelled = %d %s", rec.Code, rec.Body)
	}
	if _, err := s.pool.Exec(t.Context(),
		`UPDATE assets SET mint = 'not-a-mint' WHERE symbol = 'AAPLx'`); err != nil {
		t.Fatal(err)
	}
	rec = s.get(t, "/v1/assets?q=aap")
	if rec.Code != http.StatusInternalServerError || problemCode(t, rec) != apibase.DecodeFailed {
		t.Fatalf("bad mint = %d %s", rec.Code, rec.Body)
	}
}

func TestAssets_theCalendarExpiresOutsideItsYears(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, time.Date(2030, 7, 1, 15, 0, 0, 0, time.UTC))
	s.seedFixtures(t)
	rec := s.get(t, "/v1/assets")
	if rec.Code != http.StatusInternalServerError || problemCode(t, rec) != apibase.CalendarExpired {
		t.Fatalf("expired calendar = %d %s", rec.Code, rec.Body)
	}
}

func TestAssets_aPricePastInt64DoesNotReachTheClient(t *testing.T) {
	t.Parallel()
	when := marketWhen()
	huge := money.MicrosFromUint64(uint64(math.MaxInt64) + 1)
	asset := domain.Asset{
		Symbol: "AAPLx", DisplayName: "Apple xStock", Issuer: domain.IssuerXStocks, Kind: domain.KindEquity,
	}
	session := domain.SessionInfo{State: domain.StateOpen, NextState: domain.StateAfterHours, NextTransition: when}
	h := adapters.HTTP{List: hugeList{page: app.Page{Items: []app.Summary{{
		Asset: asset, Session: session, Priced: true,
		Price:     domain.Sample{Micros: huge, ObservedAt: when},
		Sparkline: []money.Micros{money.MicrosFromUint64(1)},
	}}}}, Clock: testkit.NewClock(when)}
	user := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: testkit.NewIDs(1).NewV7().String()})
	if _, err := h.GetAssets(user, api.GetAssetsRequestObject{}); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("huge price = %v", err)
	}
	h.List = hugeList{page: app.Page{Items: []app.Summary{{
		Asset: asset, Session: session, Priced: true,
		Price:     domain.Sample{Micros: money.MicrosFromUint64(1), ObservedAt: when},
		Sparkline: []money.Micros{huge},
	}}}}
	if _, err := h.GetAssets(user, api.GetAssetsRequestObject{}); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("huge sparkline = %v", err)
	}
	h.List = hugeList{page: app.Page{Items: []app.Summary{{
		Asset: asset, Session: session, Volume: &app.Volume{At: when, USD: huge},
	}}}}
	if _, err := h.GetAssets(user, api.GetAssetsRequestObject{}); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("huge volume = %v", err)
	}
}

type hugeList struct{ page app.Page }

func (h hugeList) Handle(context.Context, app.ListRequest) (app.Page, error) { return h.page, nil }

func marketWhen() time.Time { return time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC) }

type marketAPI struct {
	pool    *pgxpool.Pool
	handler http.Handler
	token   string
	when    time.Time
	clock   *testkit.Clock
}

func newMarketAPI(t *testing.T, when time.Time) marketAPI {
	t.Helper()
	return newMarketAPIMounting(t, when, func(d module.Deps) func(apibase.Mount) { return market.New(d).Mount })
}

func newMarketAPIMounting(t *testing.T, when time.Time, mounting func(module.Deps) func(apibase.Mount)) marketAPI {
	t.Helper()
	pool := testkit.DB(t)
	clk := testkit.NewClock(when)
	verifier, err := auth.NewDevVerifier(
		config.Config{Env: config.EnvTest, Auth: config.Auth{DevTokenKey: "test-only"}}, clk)
	if err != nil {
		t.Fatal(err)
	}
	mount := mounting(module.Deps{
		Pool: pool, Clock: clk, Config: config.Config{Market: config.Market{PricePollInterval: 120 * time.Second}},
	})
	handler, err := httpx.Handler(httpx.Deps{
		Logger:       observability.NewLogger(config.Config{Env: config.EnvTest}, io.Discard),
		Tracer:       noop.NewTracerProvider(),
		Clock:        clk,
		IDs:          testkit.NewIDs(1),
		MaxBodyBytes: 1 << 20,
		Idempotency:  db.NewIdempotencyStore(pool, clk),
		Verifier:     verifier,
	}, mount, openapi.Spec)
	if err != nil {
		t.Fatal(err)
	}
	return marketAPI{
		pool: pool, handler: testkit.HTTP(t, handler), when: when, clock: clk,
		token: verifier.Mint(testkit.NewIDs(2).NewV7().String(), when.Add(time.Hour)),
	}
}

func (s marketAPI) seedFixtures(t *testing.T) {
	t.Helper()
	for _, asset := range marketfake.Fixtures() {
		insert(t, s.pool, stamped(asset, s.when), nil)
	}
}

func (s marketAPI) seedBrandedCatalogFixtures(t *testing.T) {
	t.Helper()
	fixtures := marketfake.Fixtures()
	assets := make([]app.ProviderAsset, len(fixtures))
	facts := &marketfake.MintFacts{}
	for i, asset := range fixtures {
		asset.DisplayName += " xStock"
		assets[i] = listed(asset)
		facts.Put(asset.Mint, asset.Decimals, 1, 1)
	}
	provider := &provider{issuer: domain.IssuerXStocks}
	provider.serve(nil, assets...)
	ids := testkit.NewIDs(3)
	poller := app.NewCatalogPoller(
		db.New(s.pool, ids, s.clock), s.pool, ids, s.clock, app.NewProviders(provider), facts,
	)
	if _, err := poller.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func (s marketAPI) insertClone(
	t *testing.T, seed uint64, symbol, mint, name string, kind domain.Kind, rank int16,
) market.Asset {
	t.Helper()
	asset := marketfake.AAPLx()
	asset.ID = domain.NewAssetID(testkit.NewIDs(seed))
	parsed, err := domain.ParseMint(mint)
	if err != nil {
		t.Fatal(err)
	}
	asset.Symbol, asset.Mint, asset.DisplayName, asset.Kind, asset.PopularRank = symbol, parsed, name, kind, rank
	asset.CompanyKey = domain.CompanyKey(asset.Issuer, name)
	insert(t, s.pool, stamped(asset, s.when), nil)
	return asset
}

func (s marketAPI) price(t *testing.T, mint string, offset time.Duration, micros int64) {
	t.Helper()
	_, err := s.pool.Exec(t.Context(),
		`INSERT INTO price_points (mint, ts, price_micros, source) VALUES ($1, $2, $3, 'jupiter')`,
		mint, s.when.Add(offset), micros)
	if err != nil {
		t.Fatal(err)
	}
}

func (s marketAPI) get(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	return s.getAs(t, path, s.token)
}

func (s marketAPI) getAs(t *testing.T, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	return s.getCtx(t.Context(), t, path, token)
}

func (s marketAPI) getCtx(ctx context.Context, t *testing.T, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)
	return rec
}

func pageOf(t *testing.T, rec *httptest.ResponseRecorder) api.AssetList {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d %s", rec.Code, rec.Body)
	}
	var page api.AssetList
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	return page
}

func problemCode(t *testing.T, rec *httptest.ResponseRecorder) apibase.ErrorCode {
	t.Helper()
	var body apibase.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	return body.Code
}

func TestAssetDetail_OtherListings(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedFixtures(t)
	mint := marketfake.AAPLx().Mint.String()
	s.price(t, mint, -18*time.Hour-time.Minute, 100_000_000)
	s.price(t, mint, -30*time.Minute, 110_000_000)
	s.price(t, mint, -time.Minute, 200_000_000)
	s.insertClone(t, 9, "AAPLy", "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA", "Apple Other", domain.KindEquity, 0)
	_, err := s.pool.Exec(t.Context(),
		`UPDATE assets SET company_key = $1, tradable_override = false WHERE symbol = 'AAPLy'`,
		marketfake.AAPLx().CompanyKey)
	if err != nil {
		t.Fatal(err)
	}
	detail := detailOf(t, s.get(t, "/v1/assets/AAPLx"))
	assertApplePrice(t, api.AssetSummary{
		LogoUrl: detail.LogoUrl, PriceMicros: detail.PriceMicros, PriceAsOf: detail.PriceAsOf,
		ChangeBps: detail.ChangeBps, SparklineMicros: detail.SparklineMicros,
	})
	assertOtherListing(t, detail)
	missing := s.get(t, "/v1/assets/NOPEx")
	if missing.Code != http.StatusNotFound || problemCode(t, missing) != apibase.AssetNotFound {
		t.Fatalf("missing = %d %s", missing.Code, missing.Body)
	}
}

func detailOf(t *testing.T, rec *httptest.ResponseRecorder) api.AssetDetail {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("detail = %d %s", rec.Code, rec.Body)
	}
	var detail api.AssetDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	return detail
}

func assertOtherListing(t *testing.T, detail api.AssetDetail) {
	t.Helper()
	if !detail.Tradable || detail.Decimals != 8 || detail.UiMultiplier.Num != 1 || detail.UiMultiplier.Den != 1 {
		t.Fatalf("facts = %+v", detail)
	}
	if detail.Attribution != domain.Attribution || len(detail.OtherListings) != 1 {
		t.Fatalf("listings = %+v %q", detail.OtherListings, detail.Attribution)
	}
	other := detail.OtherListings[0]
	if other.Symbol != "AAPLy" || other.Tradable || other.DisplayName != "Apple Other" {
		t.Fatalf("other = %+v", other)
	}
}

func TestAssetDetail_servesTheUIMultiplierInForceAtRequestTime(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedFixtures(t)
	step := s.when.Add(30 * time.Minute)
	_, err := s.pool.Exec(t.Context(), `UPDATE assets SET ui_multiplier_next_num = 101, ui_multiplier_next_den = 100,
		ui_multiplier_next_at = $1 WHERE symbol = 'AAPLx'`, step)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.pool.Exec(t.Context(),
		`UPDATE assets SET ui_multiplier_num = 3, ui_multiplier_den = 2 WHERE symbol = 'TSLAx'`)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		at       time.Time
		symbol   string
		num, den int64
	}{
		{step.Add(-time.Second), "AAPLx", 1, 1},
		{step, "AAPLx", 101, 100},
		{step.Add(time.Second), "AAPLx", 101, 100},
		{step.Add(time.Second), "TSLAx", 3, 2},
	} {
		s.clock.Set(tc.at)
		got := detailOf(t, s.get(t, "/v1/assets/"+tc.symbol)).UiMultiplier
		if got.Num != tc.num || got.Den != tc.den {
			t.Fatalf("%s at %s = %d/%d, want %d/%d", tc.symbol, tc.at, got.Num, got.Den, tc.num, tc.den)
		}
	}
}

func TestAsset_rejectsACallerThatIsNotAUser(t *testing.T) {
	t.Parallel()
	h := adapters.HTTP{Clock: testkit.NewClock(marketWhen())}
	if _, err := h.GetAsset(
		t.Context(),
		api.GetAssetRequestObject{Symbol: "AAPLx"},
	); errs.CodeOf(
		err,
	) != errs.CodeUnauthorized {
		t.Fatalf("no actor = %v", err)
	}
	when := marketWhen()
	huge := money.MicrosFromUint64(uint64(math.MaxInt64) + 1)
	h.Detail = hugeDetail{view: app.AssetView{Summary: app.Summary{
		Asset:  domain.Asset{Symbol: "AAPLx", Decimals: 8, UIMultiplier: domain.Multiplier{Num: 1, Den: 1}},
		Priced: true, Price: domain.Sample{Micros: huge, ObservedAt: when},
	}}}
	user := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: testkit.NewIDs(1).NewV7().String()})
	if _, err := h.GetAsset(
		user,
		api.GetAssetRequestObject{Symbol: "AAPLx"},
	); errs.CodeOf(
		err,
	) != errs.CodeDecodeFailed {
		t.Fatalf("huge price = %v", err)
	}
}

type hugeDetail struct{ view app.AssetView }

func (h hugeDetail) Handle(context.Context, string) (app.AssetView, error) { return h.view, nil }

func TestChart_BucketsOHLC(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedFixtures(t)
	seedChartPrices(t, s, marketfake.AAPLx().Mint.String())
	assertDayChart(t, chartOf(t, s.get(t, "/v1/assets/AAPLx/chart?range=1D")))
	assertChartSpan(t, s, "1W", 3600, 2, 50_000_000, marketWhen().Add(-2*24*time.Hour))
	assertChartSpan(t, s, "1M", 3600, 3, 40_000_000, marketWhen().Add(-10*24*time.Hour))
	assertChartSpan(t, s, "3M", 3600, 4, 30_000_000, marketWhen().Add(-40*24*time.Hour))
	assertChartSpan(t, s, "1Y", 86400, 5, 20_000_000, marketWhen().Add(-100*24*time.Hour).Truncate(24*time.Hour))
	assertChartSpan(t, s, "ALL", 86400, 6, 10_000_000, marketWhen().Add(-400*24*time.Hour).Truncate(24*time.Hour))
}

func seedChartPrices(t *testing.T, s marketAPI, mint string) {
	t.Helper()
	s.price(t, mint, -400*24*time.Hour, 10_000_000)
	s.price(t, mint, -100*24*time.Hour, 20_000_000)
	s.price(t, mint, -40*24*time.Hour, 30_000_000)
	s.price(t, mint, -10*24*time.Hour, 40_000_000)
	s.price(t, mint, -2*24*time.Hour, 50_000_000)
	s.price(t, mint, -6*time.Minute, 100_000_000)
	s.price(t, mint, -4*time.Minute, 100_000_000)
	s.price(t, mint, -3*time.Minute, 130_000_000)
	s.price(t, mint, -2*time.Minute, 90_000_000)
	s.price(t, mint, -time.Minute, 100_000_000)
	s.price(t, mint, -40*time.Second, 110_000_000)
	s.price(t, mint, -20*time.Second, 400_000_000)
}

func assertDayChart(t *testing.T, got api.AssetChart) {
	t.Helper()
	if got.Range != api.AssetChartRangeN1D || got.BucketSeconds != 300 || got.Empty || len(got.Points) != 2 {
		t.Fatalf("day = %+v", got)
	}
	if got.Attribution != domain.Attribution {
		t.Fatalf("attribution = %q", got.Attribution)
	}
	if !got.Points[0].T.Equal(marketWhen().Add(-10 * time.Minute)) {
		t.Fatalf("open bucket = %s", got.Points[0].T)
	}
	assertOHLC(t, got.Points[0], 100_000_000, 100_000_000, 100_000_000, 100_000_000)
	if !got.Points[1].T.Equal(marketWhen().Add(-5 * time.Minute)) {
		t.Fatalf("close bucket = %s", got.Points[1].T)
	}
	assertOHLC(t, got.Points[1], 100_000_000, 130_000_000, 90_000_000, 110_000_000)
}

func assertChartSpan(t *testing.T, s marketAPI, raw string, bucket int64, n int, firstOpen int64, first time.Time) {
	t.Helper()
	got := chartOf(t, s.get(t, "/v1/assets/AAPLx/chart?range="+raw))
	if got.Empty || got.BucketSeconds != bucket || len(got.Points) != n || got.Points[0].OpenMicros != firstOpen {
		t.Fatalf("%s = %+v", raw, got)
	}
	if !got.Points[0].T.Equal(first) {
		t.Fatalf("%s first = %s, want %s", raw, got.Points[0].T, first)
	}
	last := got.Points[n-1]
	wantLast := marketWhen().Truncate(24 * time.Hour)
	if bucket == 3600 {
		wantLast = marketWhen().Add(-time.Hour)
	}
	if !last.T.Equal(wantLast) {
		t.Fatalf("%s last = %s, want %s", raw, last.T, wantLast)
	}
	assertOHLC(t, last, 100_000_000, 130_000_000, 90_000_000, 110_000_000)
}

func assertOHLC(t *testing.T, point api.ChartPoint, open, high, low, last int64) {
	t.Helper()
	if point.OpenMicros != open || point.HighMicros != high || point.LowMicros != low || point.CloseMicros != last {
		t.Fatalf("ohlc = %+v", point)
	}
}

func chartOf(t *testing.T, rec *httptest.ResponseRecorder) api.AssetChart {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("chart = %d %s", rec.Code, rec.Body)
	}
	var chart api.AssetChart
	if err := json.Unmarshal(rec.Body.Bytes(), &chart); err != nil {
		t.Fatal(err)
	}
	return chart
}

func TestChart_EmptyPreIPO(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.insertClone(t, 5, "SPACEx", "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA", "SpaceX", domain.KindPreIPO, 0)
	rec := s.get(t, "/v1/assets/SPACEx/chart?range=1D")
	got := chartOf(t, rec)
	if !got.Empty || len(got.Points) != 0 || got.BucketSeconds != 300 || got.Range != api.AssetChartRangeN1D {
		t.Fatalf("empty = %+v", got)
	}
	if got.Attribution != domain.Attribution || !strings.Contains(rec.Body.String(), `"points":[]`) {
		t.Fatalf("body = %s", rec.Body)
	}
}

func TestChart_rejectsABadRangeAndAnUnknownSymbol(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedFixtures(t)
	bad := s.get(t, "/v1/assets/AAPLx/chart?range=NOPE")
	if bad.Code != http.StatusBadRequest || problemCode(t, bad) != apibase.InvalidInput {
		t.Fatalf("bad range = %d %s", bad.Code, bad.Body)
	}
	missing := s.get(t, "/v1/assets/NOPEx/chart?range=1D")
	if missing.Code != http.StatusNotFound || problemCode(t, missing) != apibase.AssetNotFound {
		t.Fatalf("missing = %d %s", missing.Code, missing.Body)
	}
}

func TestChart_rejectsACallerThatIsNotAUser(t *testing.T) {
	t.Parallel()
	h := adapters.HTTP{}
	_, err := h.GetAssetChart(t.Context(), api.GetAssetChartRequestObject{Symbol: "AAPLx"})
	if errs.CodeOf(err) != errs.CodeUnauthorized {
		t.Fatalf("no actor = %v", err)
	}
	huge := money.MicrosFromUint64(uint64(math.MaxInt64) + 1)
	h.Chart = hugeChart{chart: app.Chart{
		Range: domain.Chart1D, Bucket: 5 * time.Minute,
		Points: []app.Point{{At: marketWhen(), Open: huge, High: huge, Low: huge, Close: huge}},
	}}
	user := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: testkit.NewIDs(1).NewV7().String()})
	_, hugeErr := h.GetAssetChart(user, api.GetAssetChartRequestObject{Symbol: "AAPLx"})
	if errs.CodeOf(hugeErr) != errs.CodeDecodeFailed {
		t.Fatalf("huge = %v", hugeErr)
	}
	h.Chart = hugeChart{chart: app.Chart{Range: domain.Chart1D, Bucket: 5 * time.Minute, PreviousClose: &huge}}
	_, closeErr := h.GetAssetChart(user, api.GetAssetChartRequestObject{Symbol: "AAPLx"})
	if errs.CodeOf(closeErr) != errs.CodeDecodeFailed {
		t.Fatalf("huge previous close = %v", closeErr)
	}
}

type hugeChart struct{ chart app.Chart }

func (h hugeChart) Handle(context.Context, string, app.ChartRequest) (app.Chart, error) {
	return h.chart, nil
}

func containsAll(body string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(body, part) {
			return false
		}
	}
	return true
}

func TestAssets_quotableIsTrueOnlyWithinFifteenMinutesOfTheLastJupiterPrice(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedFixtures(t)
	s.quotedAgo(t, marketfake.AAPLx(), 15*time.Minute)
	s.quotedAgo(t, marketfake.TSLAx(), 15*time.Minute+time.Second)
	got := map[string]bool{}
	for _, a := range pageOf(t, s.get(t, "/v1/assets")).Assets {
		got[a.Symbol] = a.Quotable
	}
	if !got["AAPLx"] || got["TSLAx"] {
		t.Fatalf("list quotable = %v, want AAPLx true and TSLAx false", got)
	}
	if !detailOf(t, s.get(t, "/v1/assets/AAPLx")).Quotable || detailOf(t, s.get(t, "/v1/assets/TSLAx")).Quotable {
		t.Fatal("detail quotable differs from the list")
	}
}

func TestAssets_neverQuotedIsNotQuotable(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedFixtures(t)
	if detailOf(t, s.get(t, "/v1/assets/AAPLx")).Quotable {
		t.Fatal("an asset Jupiter never priced must not be quotable")
	}
}

func (s marketAPI) quotedAgo(t *testing.T, a market.Asset, ago time.Duration) {
	t.Helper()
	if _, err := s.pool.Exec(t.Context(), `UPDATE assets SET last_quoted_at = $2 WHERE mint = $1`,
		a.Mint.String(), s.when.Add(-ago)); err != nil {
		t.Fatal(err)
	}
}

func seedDenseChartPrices(t *testing.T, s marketAPI) {
	t.Helper()
	mint := marketfake.AAPLx().Mint.String()
	for i := range 60 {
		s.price(t, mint, -time.Minute-time.Duration(i)*2*time.Minute, int64(100_000_000+i))
	}
}

func TestChart_intervalChoosesTheBarWidth(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedFixtures(t)
	seedDenseChartPrices(t, s)
	got := chartOf(t, s.get(t, "/v1/assets/AAPLx/chart?range=1D&interval=15m"))
	if got.BucketSeconds != 900 || got.Interval != api.AssetChartIntervalN15m || got.Empty {
		t.Fatalf("15m = %+v", got)
	}
	for _, point := range got.Points {
		if point.T.Unix()%900 != 0 {
			t.Fatalf("bar %s is not on a 15-minute boundary", point.T)
		}
	}
	def := chartOf(t, s.get(t, "/v1/assets/AAPLx/chart?range=1D"))
	if def.BucketSeconds != 300 || def.Interval != api.AssetChartIntervalN5m {
		t.Fatalf("default = %+v", def)
	}
}

func TestChart_pagesOlderBarsWithBefore(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedFixtures(t)
	seedDenseChartPrices(t, s)
	first := chartOf(t, s.get(t, "/v1/assets/AAPLx/chart?range=1D&limit=5"))
	if len(first.Points) != 5 || first.NextBefore == nil || !first.NextBefore.Equal(first.Points[0].T) {
		t.Fatalf("first page = %+v", first)
	}
	seen := pageBackToTheStart(t, s, first)
	oldest := marketWhen().Add(-119 * time.Minute).Truncate(5 * time.Minute)
	newest := marketWhen().Add(-time.Minute).Truncate(5 * time.Minute)
	want := int(newest.Sub(oldest)/(5*time.Minute)) + 1
	if !seen[0].T.Equal(oldest) || len(seen) != want {
		t.Fatalf("paged %d bars from %s, want %d from %s", len(seen), seen[0].T, want, oldest)
	}
	for i := 1; i < len(seen); i++ {
		if !seen[i].T.After(seen[i-1].T) {
			t.Fatalf("bars out of order at %d: %s then %s", i, seen[i-1].T, seen[i].T)
		}
	}
	rec := s.get(t, "/v1/assets/AAPLx/chart?range=1D&before="+url.QueryEscape(oldest.Format(time.RFC3339)))
	if !chartOf(t, rec).Empty || !strings.Contains(rec.Body.String(), `"next_before":null`) {
		t.Fatalf("before the first bar = %s", rec.Body)
	}
}

func pageBackToTheStart(t *testing.T, s marketAPI, first api.AssetChart) []api.ChartPoint {
	t.Helper()
	seen := first.Points
	page := first
	for range 20 {
		if page.NextBefore == nil {
			return seen
		}
		before := url.QueryEscape(page.NextBefore.UTC().Format(time.RFC3339))
		page = chartOf(t, s.get(t, "/v1/assets/AAPLx/chart?range=1D&limit=5&before="+before))
		if len(page.Points) == 0 || !page.Points[len(page.Points)-1].T.Before(seen[0].T) {
			t.Fatalf("page = %+v, want bars strictly before %s", page, seen[0].T)
		}
		if page.PreviousCloseMicros != nil {
			t.Fatalf("an older page carries the day-change base: %+v", page)
		}
		seen = append(append([]api.ChartPoint{}, page.Points...), seen...)
	}
	t.Fatalf("never reached the start: %+v", page)
	return nil
}

func TestChart_rejectsAnIntervalTheRangeDoesNotAllow(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedFixtures(t)
	for _, path := range []string{
		"/v1/assets/AAPLx/chart?range=1Y&interval=5m",
		"/v1/assets/AAPLx/chart?range=6M&interval=15m",
		"/v1/assets/AAPLx/chart?range=1D&interval=1m",
		"/v1/assets/AAPLx/chart?range=1D&limit=1001",
	} {
		rec := s.get(t, path)
		if rec.Code != http.StatusBadRequest || problemCode(t, rec) != apibase.InvalidInput {
			t.Fatalf("%s = %d %s", path, rec.Code, rec.Body)
		}
	}
}

func TestChart_sixMonths(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedFixtures(t)
	mint := marketfake.AAPLx().Mint.String()
	s.price(t, mint, -190*24*time.Hour, 98_000_000)
	s.price(t, mint, -170*24*time.Hour, 99_000_000)
	s.price(t, mint, -time.Minute, 100_000_000)
	got := chartOf(t, s.get(t, "/v1/assets/AAPLx/chart?range=6M"))
	if got.Range != api.AssetChartRangeN6M || got.BucketSeconds != 86400 || len(got.Points) != 2 {
		t.Fatalf("6M = %+v", got)
	}
	if !got.Points[0].T.Equal(marketWhen().Add(-170 * 24 * time.Hour).Truncate(24 * time.Hour)) {
		t.Fatalf("6M first = %s", got.Points[0].T)
	}
	if got.NextBefore == nil || !got.NextBefore.Equal(got.Points[0].T) {
		t.Fatalf("6M next_before = %v, want the first bar since an older sample exists", got.NextBefore)
	}
}

func TestChart_firstPageIsTheWholeWindowAndABeforePageIsCapped(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedFixtures(t)
	if _, err := s.pool.Exec(t.Context(), `INSERT INTO price_points (mint, ts, price_micros, source)
		SELECT $1, $2::timestamptz - (i * interval '1 hour') - interval '1 minute', 100000000 + (i % 7), 'jupiter'
		FROM generate_series(0, 1299) AS i`, marketfake.AAPLx().Mint.String(), marketWhen()); err != nil {
		t.Fatal(err)
	}
	whole := chartOf(t, s.get(t, "/v1/assets/AAPLx/chart?range=1M"))
	if n := len(whole.Points); n < 719 || n > 721 || whole.NextBefore == nil {
		t.Fatalf("1M without a limit = %d bars next_before %v, want the whole ~720-hour window", n, whole.NextBefore)
	}
	before := url.QueryEscape(marketWhen().Add(-24 * time.Hour).Format(time.RFC3339))
	page := chartOf(t, s.get(t, "/v1/assets/AAPLx/chart?range=1M&before="+before))
	if len(page.Points) != 500 || page.NextBefore == nil {
		t.Fatalf("before page without a limit = %d bars next_before %v, want 500", len(page.Points), page.NextBefore)
	}
	capped := chartOf(t, s.get(t, "/v1/assets/AAPLx/chart?range=1M&limit=10"))
	if len(capped.Points) != 10 {
		t.Fatalf("explicit limit = %d bars", len(capped.Points))
	}
}

func TestChart_weeklyBarsStartOnMondayAndPageBack(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedFixtures(t)
	if _, err := s.pool.Exec(t.Context(), `INSERT INTO price_points (mint, ts, price_micros, source)
		SELECT $1, $2::timestamptz - (i * interval '1 day') - interval '1 minute', 100000000 + (i % 7), 'jupiter'
		FROM generate_series(0, 299) AS i`, marketfake.AAPLx().Mint.String(), marketWhen()); err != nil {
		t.Fatal(err)
	}
	got := chartOf(t, s.get(t, "/v1/assets/AAPLx/chart?range=1Y&interval=1w"))
	if got.BucketSeconds != 604800 || got.Interval != api.AssetChartInterval("1w") || len(got.Points) < 40 {
		t.Fatalf("1w = %d s %s %d bars", got.BucketSeconds, got.Interval, len(got.Points))
	}
	for _, point := range got.Points {
		if point.T.Weekday() != time.Monday || point.T.Hour() != 0 || point.T.Minute() != 0 {
			t.Fatalf("weekly bar starts %s (%s), want Monday 00:00 UTC", point.T, point.T.Weekday())
		}
	}
	before := url.QueryEscape(got.Points[10].T.Add(3 * 24 * time.Hour).Format(time.RFC3339))
	page := chartOf(t, s.get(t, "/v1/assets/AAPLx/chart?range=1Y&interval=1w&before="+before))
	if len(page.Points) == 0 || !page.Points[len(page.Points)-1].T.Equal(got.Points[9].T) {
		t.Fatalf("before page last bar = %v, want %s", page.Points, got.Points[9].T)
	}
	if rec := s.get(t, "/v1/assets/AAPLx/chart?range=1M&interval=1w"); rec.Code != http.StatusBadRequest {
		t.Fatalf("1M weekly = %d", rec.Code)
	}
}

func (s marketAPI) volume(t *testing.T, mint string, offset time.Duration, micros int64) {
	t.Helper()
	_, err := s.pool.Exec(t.Context(),
		`INSERT INTO asset_volumes (mint, volume_24h_micros, as_of, updated_at) VALUES ($1, $2, $3, $3)`,
		mint, micros, s.when.Add(offset))
	if err != nil {
		t.Fatal(err)
	}
}

func assertVolume(t *testing.T, name string, micros *int64, asOf *time.Time, want int64, wantAsOf time.Time) {
	t.Helper()
	if micros == nil || *micros != want || asOf == nil || !asOf.Equal(wantAsOf) {
		t.Fatalf("%s volume = %v as of %v, want %d as of %s", name, micros, asOf, want, wantAsOf)
	}
}

func TestAssets_volumeIsServedOnTheListAndTheDetailForAFreshReading(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedFixtures(t)
	aapl := marketfake.AAPLx().Mint.String()
	s.price(t, aapl, -30*time.Minute, 110_000_000)
	s.volume(t, aapl, -35*time.Hour, 2_500_000_000_000)
	var apple api.AssetSummary
	for _, a := range pageOf(t, s.get(t, "/v1/assets")).Assets {
		if a.Symbol == "AAPLx" {
			apple = a
		}
	}
	wantAsOf := s.when.Add(-35 * time.Hour)
	assertVolume(t, "list", apple.Volume24hMicros, apple.Volume24hAsOf, 2_500_000_000_000, wantAsOf)
	detail := detailOf(t, s.get(t, "/v1/assets/AAPLx"))
	assertVolume(t, "detail", detail.Volume24hMicros, detail.Volume24hAsOf, 2_500_000_000_000, wantAsOf)
}

func TestAssets_aVolumeOlderThanThirtySixHoursIsNullAndSoIsAMintWithNoRow(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedFixtures(t)
	aapl, tsla := marketfake.AAPLx().Mint.String(), marketfake.TSLAx().Mint.String()
	s.price(t, aapl, -30*time.Minute, 110_000_000)
	s.volume(t, aapl, -36*time.Hour-time.Second, 2_500_000_000_000)
	for _, rec := range []*httptest.ResponseRecorder{s.get(t, "/v1/assets"), s.get(t, "/v1/assets/AAPLx")} {
		body := rec.Body.String()
		if strings.Count(body, `"volume_24h_micros":null`) == 0 || strings.Contains(body, `"volume_24h_micros":2`) ||
			strings.Contains(body, `"volume_24h_as_of":"`) {
			t.Fatalf("body = %s, want null volume and as_of for a stale reading", body)
		}
	}
	for _, a := range pageOf(t, s.get(t, "/v1/assets")).Assets {
		if a.Volume24hMicros != nil || a.Volume24hAsOf != nil {
			t.Fatalf("%s volume = %v as of %v, want null for both", a.Symbol, a.Volume24hMicros, a.Volume24hAsOf)
		}
	}
	s.volume(t, tsla, -time.Hour, 7_000_000)
	got := detailOf(t, s.get(t, "/v1/assets/AAPLx"))
	if got.Volume24hMicros != nil || got.Volume24hAsOf != nil {
		t.Fatalf("detail volume = %v as of %v, want null while another mint has a reading",
			got.Volume24hMicros, got.Volume24hAsOf)
	}
}

func TestAssetDetail_priceSourceComesFromTheStoredSample(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedFixtures(t)
	insert := func(mint, source string, offset time.Duration) {
		t.Helper()
		if _, err := s.pool.Exec(t.Context(),
			`INSERT INTO price_points (mint, ts, price_micros, source) VALUES ($1, $2, 100000000, $3)`,
			mint, s.when.Add(offset), source); err != nil {
			t.Fatal(err)
		}
	}
	insert(marketfake.AAPLx().Mint.String(), "jupiter", -time.Minute)
	insert(marketfake.TSLAx().Mint.String(), "coingecko", -time.Hour)

	aapl := detailOf(t, s.get(t, "/v1/assets/AAPLx"))
	if aapl.PriceSource == nil || *aapl.PriceSource != api.AssetDetailPriceSourceJupiter {
		t.Fatalf("AAPLx price_source = %v, want jupiter", aapl.PriceSource)
	}
	tsla := detailOf(t, s.get(t, "/v1/assets/TSLAx"))
	if tsla.PriceSource == nil || *tsla.PriceSource != api.AssetDetailPriceSourceCoingecko {
		t.Fatalf("TSLAx price_source = %v, want coingecko", tsla.PriceSource)
	}
	if tsla.PriceAsOf == nil || !tsla.PriceAsOf.Equal(s.when.Add(-time.Hour)) {
		t.Fatalf("TSLAx price_as_of = %v, want the sample's own time", tsla.PriceAsOf)
	}
	if aapl.PricePollIntervalSeconds != 120 {
		t.Fatalf("price_poll_interval_seconds = %d, want 120", aapl.PricePollIntervalSeconds)
	}
	s.clock.Advance(10 * 120 * time.Second)
	stale := detailOf(t, s.get(t, "/v1/assets/AAPLx"))
	if stale.PriceAsOf == nil || !stale.PriceAsOf.Equal(s.when.Add(-time.Minute)) {
		t.Fatalf("stale price_as_of = %v, want the old sample's time", stale.PriceAsOf)
	}
}

func TestAssets_aNeverPricedAssetIsOutOfTheListsAndSearchButReachableBySymbol(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedFixtures(t)
	if _, err := s.pool.Exec(t.Context(),
		`UPDATE assets SET last_quoted_at = NULL WHERE mint = $1`, marketfake.AAPLx().Mint.String()); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"/v1/assets?q=apple", "/v1/assets?q=aap", "/v1/assets?sort=name&q=apple", "/v1/assets?filter=popular&q=apple",
	} {
		if page := pageOf(t, s.get(t, path)); len(page.Assets) != 0 {
			t.Fatalf("%s = %+v, want nothing for an asset Jupiter never priced", path, page.Assets)
		}
	}
	for _, a := range pageOf(t, s.get(t, "/v1/assets")).Assets {
		if a.Symbol == "AAPLx" {
			t.Fatal("the plain list offers AAPLx, which Jupiter never priced")
		}
	}
	if got := detailOf(t, s.get(t, "/v1/assets/AAPLx")); got.Symbol != "AAPLx" {
		t.Fatalf("detail by symbol = %+v, want AAPLx still reachable for links", got)
	}
}

func TestCatalog_ListTradableLeavesOutAnAssetNeverPriced(t *testing.T) {
	t.Parallel()
	catalog, pool, _ := seededCatalog(t)
	if _, err := pool.Exec(t.Context(), `UPDATE assets SET last_quoted_at = NULL WHERE symbol = 'AAPLx'`); err != nil {
		t.Fatal(err)
	}
	for name, list := range map[string]func(context.Context) ([]market.Asset, error){
		"ListTradable": catalog.ListTradable, "ListPriceable": catalog.ListPriceable,
	} {
		got, err := list(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		listed := slices.ContainsFunc(got, func(a market.Asset) bool { return a.Symbol == "AAPLx" })
		if want := name == "ListPriceable"; listed != want {
			t.Fatalf("%s lists AAPLx = %v, want %v: the poller must keep pricing it", name, listed, want)
		}
	}
}
