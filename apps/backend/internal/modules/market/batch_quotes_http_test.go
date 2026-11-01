package market_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/marketapi"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func quotesOf(t *testing.T, rec *httptest.ResponseRecorder) api.AssetQuotes {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d %s", rec.Code, rec.Body)
	}
	var body api.AssetQuotes
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	return body
}

func seedQuotes(t *testing.T, s marketAPI) {
	t.Helper()
	s.seedFixtures(t)
	apple := marketfake.AAPLx().Mint.String()
	s.price(t, apple, -18*time.Hour-time.Minute, 100_000_000)
	s.price(t, apple, -30*time.Minute, 110_000_000)
	tesla := marketfake.TSLAx().Mint.String()
	s.price(t, tesla, -2*time.Hour, 250_000_000)
	s.price(t, tesla, -time.Minute, 260_000_000)
}

func TestAssetQuotes_matchTheSingleRoute(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	seedQuotes(t, s)
	got := quotesOf(t, s.get(t, "/v1/assets/quotes?symbols=AAPLx,TSLAx,JPSTx"))
	if len(got.Assets) != 3 {
		t.Fatalf("quotes = %+v", got.Assets)
	}
	for _, quote := range got.Assets {
		one := detailOf(t, s.get(t, "/v1/assets/"+quote.Symbol))
		want := api.AssetSummary{
			Id: one.Id, Symbol: one.Symbol, DisplayName: one.DisplayName, Issuer: one.Issuer, Kind: one.Kind,
			LogoUrl: one.LogoUrl, PriceMicros: one.PriceMicros, PriceAsOf: one.PriceAsOf, ChangeBps: one.ChangeBps,
			SparklineMicros: one.SparklineMicros, Volume24hMicros: one.Volume24hMicros,
			Volume24hAsOf: one.Volume24hAsOf, Quotable: one.Quotable, Session: one.Session,
			Tradable: &one.Tradable, Stale: one.Stale,
		}
		if !reflect.DeepEqual(quote, want) {
			t.Fatalf("%s batch = %+v, single = %+v", quote.Symbol, quote, want)
		}
	}
}

func TestAssetQuotes_unknownAndRepeatedSymbols(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	seedQuotes(t, s)
	got := quotesOf(t, s.get(t, "/v1/assets/quotes?symbols=AAPLx,NOPEx,AAPLx,,"))
	if len(got.Assets) != 1 || got.Assets[0].Symbol != "AAPLx" {
		t.Fatalf("quotes = %+v", got.Assets)
	}
	none := quotesOf(t, s.get(t, "/v1/assets/quotes?symbols=NOPEx"))
	if none.Assets == nil || len(none.Assets) != 0 {
		t.Fatalf("unknown only = %+v", none.Assets)
	}
}

func TestAssetQuotes_refusesTooManySymbols(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	s.seedFixtures(t)
	symbols := func(n int) string {
		out := make([]string, n)
		for i := range out {
			out[i] = "S" + strconv.Itoa(i)
		}
		return strings.Join(out, ",")
	}
	if rec := s.get(t, "/v1/assets/quotes?symbols="+symbols(50)); rec.Code != http.StatusOK {
		t.Fatalf("50 symbols = %d %s", rec.Code, rec.Body)
	}
	over := s.get(t, "/v1/assets/quotes?symbols="+symbols(51))
	if over.Code != http.StatusBadRequest || problemCode(t, over) != apibase.InvalidInput {
		t.Fatalf("51 symbols = %d %s", over.Code, over.Body)
	}
	blank := s.get(t, "/v1/assets/quotes?symbols=,")
	if blank.Code != http.StatusBadRequest || problemCode(t, blank) != apibase.InvalidInput {
		t.Fatalf("blank symbols = %d %s", blank.Code, blank.Body)
	}
}

func TestAssetQuotes_needsASignedInCaller(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	if rec := s.getAs(t, "/v1/assets/quotes?symbols=AAPLx", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d %s", rec.Code, rec.Body)
	}
}

func TestAssetQuotes_queryCount(t *testing.T) {
	t.Parallel()
	s := newMarketAPI(t, marketWhen())
	seedQuotes(t, s)
	testkit.AssertQueries(t, "market batch quotes for three assets", func() {
		if rec := s.get(t, "/v1/assets/quotes?symbols=AAPLx,TSLAx,JPSTx"); rec.Code != http.StatusOK {
			t.Fatalf("GET = %d %s", rec.Code, rec.Body)
		}
	})
}
