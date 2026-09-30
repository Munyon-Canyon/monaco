package xstocks_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/xstocks"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const listRoute = "/xstocks/api/v2/public/assets"

func fakeXStocks(t *testing.T, steps ...fakes.Step) *xstocks.Client {
	t.Helper()
	srv := httptest.NewServer(fakes.New())
	t.Cleanup(srv.Close)
	for _, step := range steps {
		body, err := json.Marshal(step)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/_script", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("script %+v: status %d", step, resp.StatusCode)
		}
	}
	return xstocks.New(httpclient.New("xstocks", httpclient.WithBaseURL(srv.URL+"/xstocks"),
		httpclient.WithTimeout(10*time.Second)))
}

func mint(t *testing.T, raw string) domain.Mint {
	t.Helper()
	m, err := domain.ParseMint(raw)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestClient_readsTheSolanaDeploymentOfEveryListedAsset(t *testing.T) {
	t.Parallel()
	c := fakeXStocks(t)
	if c.Issuer() != domain.IssuerXStocks {
		t.Fatalf("Issuer = %s, want xstocks", c.Issuer())
	}
	got, err := c.Catalog(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	logo := "https://xstocks-metadata.backed.fi/logos/tokens/"
	want := []app.ProviderAsset{
		{
			Symbol: "AAPLx", Mint: mint(t, "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp"), Decimals: 8,
			Kind: domain.KindEquity, DisplayName: "Apple xStock", LogoURL: logo + "AAPLx.png", Tradable: true,
		},
		{
			Symbol: "TSLAx", Mint: mint(t, "XsDoVfqeBukxuZHWhdvWHBhgEHjGNst4MLodqsJHzoB"), Decimals: 8,
			Kind: domain.KindEquity, DisplayName: "Tesla xStock", LogoURL: logo + "TSLAx.png", Tradable: true,
		},
		{
			Symbol: "JPSTx", Mint: mint(t, "XsCAXu7xTaZMG9b9KJhNWYapuvNjxPuE4SysZq8uvMq"), Decimals: 8,
			Kind: domain.KindEquity, DisplayName: "JPMorgan Ultra-Short Income xStock", LogoURL: logo + "JPSTx.png",
		},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("Catalog = %+v\nwant %+v", got, want)
	}
}

func TestClient_followsPagesAndSkipsAnAssetWithNoSolanaDeployment(t *testing.T) {
	t.Parallel()
	c := fakeXStocks(t, fakes.Step{Route: listRoute, Action: fakes.ActionSucceed, Fixture: listRoute + "/_page-0"})
	got, err := c.Catalog(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	symbols := make([]string, len(got))
	for i, a := range got {
		symbols[i] = a.Symbol
	}
	if !slices.Equal(symbols, []string{"NVDAx", "AAPLx", "TSLAx", "JPSTx"}) {
		t.Fatalf("symbols = %v, want NVDAx from page 0 then page 1, without XRXx", symbols)
	}
}

func TestClient_failsWhenTheUpstreamOrItsPayloadIsBroken(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		step fakes.Step
		want errs.Code
	}{
		"server error": {fakes.Step{Route: listRoute, Action: fakes.ActionFail, Status: 503}, errs.CodeUpstreamUnavailable},
		"not found":    {fakes.Step{Route: listRoute, Action: fakes.ActionFail, Status: 404}, errs.CodeUpstreamUnavailable},
		"malformed": {
			fakes.Step{Route: listRoute, Action: fakes.ActionSucceed, Fixture: listRoute + "/_malformed"},
			errs.CodeDecodeFailed,
		},
		"bad mint": {
			fakes.Step{Route: listRoute, Action: fakes.ActionSucceed, Fixture: listRoute + "/_bad-mint"},
			errs.CodeDecodeFailed,
		},
		"endless pages": {
			fakes.Step{Route: listRoute, Action: fakes.ActionSucceed, Fixture: listRoute + "/_page-0", Times: 64},
			errs.CodeDecodeFailed,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := fakeXStocks(t, tc.step).Catalog(t.Context())
			if errs.CodeOf(err) != tc.want || got != nil {
				t.Fatalf("Catalog = %v, %v, want %s", got, err, tc.want)
			}
		})
	}
}

func TestHTTPSOnly_dropsALogoTheAppCannotLoad(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]string{
		"https://x/a.png": "https://x/a.png", "http://x/a.png": "", "/logos/a.png": "", "": "",
	} {
		if got := xstocks.HTTPSOnly(raw); got != want {
			t.Fatalf("HTTPSOnly(%q) = %q, want %q", raw, got, want)
		}
	}
}
