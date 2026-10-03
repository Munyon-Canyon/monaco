package market_test

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
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

func assertAppleQuote(t *testing.T, got api.AssetSummary) {
	t.Helper()
	if got.DisplayName != "Apple xStock" || got.Issuer != api.Xstocks || got.Kind != api.AssetKindEquity {
		t.Fatalf("AAPLx identity = %+v", got)
	}
	assertAppleUnpriced(t, got)
	if got.Session.State != api.MarketStateOpen || got.Session.Continuous || got.Session.NextState == nil ||
		*got.Session.NextState != api.MarketSessionNextStateAfterHours || got.Session.NextTransition == nil {
		t.Fatalf("session = %+v", got.Session)
	}
}

func assertAppleUnpriced(t *testing.T, got api.AssetSummary) {
	t.Helper()
	if got.LogoUrl == nil || got.PriceMicros != nil || got.PriceAsOf != nil || got.ChangeBps != nil ||
		got.SparklineMicros != nil {
		t.Fatalf("AAPLx price = %+v", got)
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
	s.insertClone(t, 5, "SPACEx", "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA", "SpaceX", domain.KindPreIPO, 0)
	page := pageOf(t, s.get(t, "/v1/assets?filter=pre_ipo"))
	if len(page.Assets) != 1 || page.Assets[0].Symbol != "SPACEx" || page.Assets[0].Kind != api.AssetKindPreIpo {
		t.Fatalf("pre-IPO = %+v", page)
	}
	got := page.Assets[0]
	if got.PriceMicros != nil || got.ChangeBps != nil || !got.Session.Continuous ||
		got.Session.NextState != nil || got.Session.NextTransition != nil || got.Session.State != api.MarketStateOpen {
		t.Fatalf("pre-IPO quote = %+v session %+v", got, got.Session)
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
		code   api.ErrorCode
	}{
		{path: "/v1/assets", status: http.StatusUnauthorized, code: api.Unauthorized},
		{path: "/v1/assets?filter=nope", token: s.token, status: http.StatusBadRequest, code: api.InvalidInput},
		{path: "/v1/assets?limit=51", token: s.token, status: http.StatusBadRequest, code: api.InvalidInput},
		{
			path:   "/v1/assets?cursor=" + url.QueryEscape("%%%"),
			token:  s.token,
			status: http.StatusBadRequest,
			code:   api.InvalidInput,
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
	agent := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorAgent, ID: "agent"})
	if _, err := h.GetAssets(agent, api.GetAssetsRequestObject{}); errs.CodeOf(err) != errs.CodeForbidden {
		t.Fatalf("agent = %v", err)
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
	if rec.Code != http.StatusInternalServerError || problemCode(t, rec) != api.Internal {
		t.Fatalf("cancelled = %d %s", rec.Code, rec.Body)
	}
	if _, err := s.pool.Exec(t.Context(),
		`UPDATE assets SET mint = 'not-a-mint' WHERE symbol = 'AAPLx'`); err != nil {
		t.Fatal(err)
	}
	rec = s.get(t, "/v1/assets?q=aap")
	if rec.Code != http.StatusInternalServerError || problemCode(t, rec) != api.DecodeFailed {
		t.Fatalf("bad mint = %d %s", rec.Code, rec.Body)
	}
}

func TestAssets_theCalendarExpiresOutsideItsYears(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, time.Date(2030, 7, 1, 15, 0, 0, 0, time.UTC))
	s.seedFixtures(t)
	rec := s.get(t, "/v1/assets")
	if rec.Code != http.StatusInternalServerError || problemCode(t, rec) != api.CalendarExpired {
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
	}}}}}
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
	change := int32(0)
	h.List = hugeList{page: app.Page{Items: []app.Summary{{
		Asset: asset, Session: session, Priced: true, Change: &change,
		Price:     domain.Sample{Micros: money.MicrosFromUint64(1), ObservedAt: when},
		Sparkline: []money.Micros{money.MicrosFromUint64(1)},
	}}}}
	got, err := h.GetAssets(user, api.GetAssetsRequestObject{})
	body, ok := got.(api.GetAssets200JSONResponse)
	if err != nil || !ok || len(body.Assets) != 1 || body.Assets[0].PriceMicros == nil ||
		*body.Assets[0].PriceMicros != 1 {
		t.Fatalf("priced summary = %#v %v", got, err)
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
}

func newMarketAPI(t *testing.T, when time.Time) marketAPI {
	t.Helper()
	pool := testkit.DB(t)
	clk := testkit.NewClock(when)
	verifier, err := auth.NewDevVerifier(
		config.Config{Env: config.EnvTest, Auth: config.Auth{DevTokenKey: "test-only"}}, clk)
	if err != nil {
		t.Fatal(err)
	}
	var routes httpx.Routes
	market.New(module.Deps{Pool: pool, Clock: clk}).Routes(&routes)
	handler, err := httpx.Handler(httpx.Deps{
		Logger:       observability.NewLogger(config.Config{Env: config.EnvTest}, io.Discard),
		Tracer:       noop.NewTracerProvider(),
		Clock:        clk,
		IDs:          testkit.NewIDs(1),
		MaxBodyBytes: 1 << 20,
		Idempotency:  db.NewIdempotencyStore(pool, clk),
		Verifier:     verifier,
	}, routes, openapi.Spec)
	if err != nil {
		t.Fatal(err)
	}
	return marketAPI{
		pool: pool, handler: testkit.HTTP(t, handler), when: when,
		token: verifier.Mint(testkit.NewIDs(2).NewV7().String(), when.Add(time.Hour)),
	}
}

func (s marketAPI) seedFixtures(t *testing.T) {
	t.Helper()
	for _, asset := range marketfake.Fixtures() {
		insert(t, s.pool, stamped(asset, s.when), nil)
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
	asset.CompanyKey = domain.CompanyKey(name)
	insert(t, s.pool, stamped(asset, s.when), nil)
	return asset
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

func problemCode(t *testing.T, rec *httptest.ResponseRecorder) api.ErrorCode {
	t.Helper()
	var body api.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	return body.Code
}

func containsAll(body string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(body, part) {
			return false
		}
	}
	return true
}
