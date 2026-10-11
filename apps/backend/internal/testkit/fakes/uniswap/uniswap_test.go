package uniswap_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes/uniswap"
)

const (
	ethUSDC = "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48"
	aaplon  = "0x14c3abF95Cb9C93a8b82C1CdCB76D72Cb87b2d4c"
)

type reply struct {
	status int
	body   string
}

func call(t *testing.T, s *uniswap.Server, method, path, body string, header http.Header) reply {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	for k, v := range header {
		req.Header[k] = v
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	raw, err := io.ReadAll(rec.Result().Body)
	if err != nil {
		t.Fatal(err)
	}
	return reply{status: rec.Code, body: string(raw)}
}

func quoteBody(t *testing.T, tokenIn, tokenOut, amount string) string {
	t.Helper()
	raw, err := json.Marshal(uniswap.Request{
		Type: "EXACT_INPUT", Amount: amount, TokenInChainID: 1, TokenOutChainID: 1,
		TokenIn: tokenIn, TokenOut: tokenOut, Swapper: "0x000000000000000000000000000000000000dead",
		RoutingPreference: "BEST_PRICE", Protocols: []string{"V3"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestServer_RepliesWithTheRecordingMatchedCaseInsensitively(t *testing.T) {
	t.Parallel()
	s := uniswap.New()
	r := call(t, s, http.MethodPost, "/v1/quote", quoteBody(t, "0x"+strings.ToUpper(ethUSDC[2:]), aaplon, "100000000"),
		http.Header{"X-Api-Key": {"k-1"}})
	if r.status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", r.status, r.body)
	}
	var got struct {
		Routing string `json:"routing"`
		Quote   struct {
			QuoteID string `json:"quoteId"`
		} `json:"quote"`
	}
	if err := json.Unmarshal([]byte(r.body), &got); err != nil {
		t.Fatal(err)
	}
	if got.Routing != "CLASSIC" || got.Quote.QuoteID != "01a12a2f-23cd-8000-8bce-0d4e5bfb0100" {
		t.Errorf("reply = %+v", got)
	}
	reqs := s.Requests()
	if len(reqs) != 1 || reqs[0].APIKey != "k-1" || reqs[0].Protocols[0] != "V3" {
		t.Errorf("requests = %+v", reqs)
	}
}

func TestServer_ReplaysRecordedErrorStatus(t *testing.T) {
	t.Parallel()
	r := call(t, uniswap.New(), http.MethodPost, "/v1/quote",
		quoteBody(t, "0xaf88d065e77c8cc2239327c5edb3a432268e5831", "0x9d275685dc284c8eb1c79f6aba7a63dc75ec890a",
			"100000000"), nil)
	var got struct {
		ErrorCode string `json:"errorCode"`
	}
	if err := json.Unmarshal([]byte(r.body), &got); err != nil {
		t.Fatal(err)
	}
	if r.status != http.StatusNotFound || got.ErrorCode != "NoRouteFoundError" {
		t.Fatalf("reply = %+v", r)
	}
}

func TestServer_FailNextFailsQueuedCallsInOrderThenRecovers(t *testing.T) {
	t.Parallel()
	s := uniswap.New()
	s.FailNext(http.StatusTooManyRequests)
	s.FailNext(http.StatusBadGateway)
	body := quoteBody(t, ethUSDC, aaplon, "100000000")
	got := []reply{
		call(t, s, http.MethodPost, "/v1/quote", body, nil),
		call(t, s, http.MethodPost, "/v1/quote", body, nil),
	}
	want := []reply{
		{http.StatusTooManyRequests, `{"errorCode":"Too Many Requests"}`},
		{http.StatusBadGateway, `{"errorCode":"Bad Gateway"}`},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("replies = %+v, want %+v", got, want)
	}
	if r := call(t, s, http.MethodPost, "/v1/quote", body, nil); r.status != http.StatusOK {
		t.Fatalf("third status = %d", r.status)
	}
	if n := len(s.Requests()); n != 3 {
		t.Errorf("requests = %d, want 3", n)
	}
}

func TestServer_UnrecordedPairIsNotImplemented(t *testing.T) {
	t.Parallel()
	s := uniswap.New()
	r := call(t, s, http.MethodPost, "/v1/quote", quoteBody(t, ethUSDC, aaplon, "7"), nil)
	want := "no recording for " + ethUSDC + " -> " + aaplon + " 7"
	if r.status != http.StatusNotImplemented || !strings.Contains(r.body, want) {
		t.Fatalf("reply = %+v", r)
	}
	if len(s.Requests()) != 1 {
		t.Errorf("requests = %d, want 1", len(s.Requests()))
	}
}

func TestServer_MalformedBodyIsRequestValidationErrorAndNotRecorded(t *testing.T) {
	t.Parallel()
	s := uniswap.New()
	r := call(t, s, http.MethodPost, "/v1/quote", "{", nil)
	if r.status != http.StatusBadRequest || r.body != `{"errorCode":"RequestValidationError"}` {
		t.Fatalf("reply = %+v", r)
	}
	if len(s.Requests()) != 0 {
		t.Errorf("requests = %d, want 0", len(s.Requests()))
	}
}

func TestServer_OtherRoutesAreNotFound(t *testing.T) {
	t.Parallel()
	s := uniswap.New()
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/v1/quote"},
		{http.MethodPost, "/v1/swap"},
	} {
		if r := call(t, s, tc.method, tc.path, "{}", nil); r.status != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 404", tc.method, tc.path, r.status)
		}
	}
	if len(s.Requests()) != 0 {
		t.Errorf("requests = %d, want 0", len(s.Requests()))
	}
}
