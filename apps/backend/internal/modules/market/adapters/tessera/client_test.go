package tessera_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/tessera"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const route = "/tessera/v1/public/token-details"

func fakeTessera(t *testing.T, steps ...fakes.Step) *tessera.Client {
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
	return tessera.New(httpclient.New("tessera", httpclient.WithBaseURL(srv.URL+"/tessera"),
		httpclient.WithTimeout(10*time.Second)))
}

func preIPO(t *testing.T, symbol, mint, name string, tradable bool) app.ProviderAsset {
	t.Helper()
	m, err := domain.ParseMint(mint)
	if err != nil {
		t.Fatal(err)
	}
	return app.ProviderAsset{
		Symbol: symbol, Mint: m, Decimals: 9, Kind: domain.KindPreIPO, DisplayName: name, Tradable: tradable,
	}
}

func TestClient_listsEveryTesseraTokenAsATradablePreIPOAsset(t *testing.T) {
	t.Parallel()
	c := fakeTessera(t)
	if c.Issuer() != domain.IssuerTessera {
		t.Fatalf("Issuer = %s, want tessera", c.Issuer())
	}
	got, err := c.Catalog(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := []app.ProviderAsset{
		preIPO(t, "tOpenAI", "oPAiAikWTaFj9RYoRFD35ccfwhnMcB3ThgBZRHSkjTZ", "T-OpenAI", true),
		preIPO(t, "tKalshi", "TKLSidmLVt3cqGaaodG8tyRzoANfQwoh67AccjmubeZ", "T-Kalshi", true),
		preIPO(t, "tSpaceX", "TSPXcLV76s6V2zDiZQ18kBfcbnjaE2ZzNT3ga2Pd99v", "T-SpaceX", true),
	}
	if !slices.Equal(got, want) {
		t.Fatalf("Catalog = %+v\nwant %+v", got, want)
	}
}

func TestClient_marksAPausedTokenUntradableAndSkipsOneWithNoMint(t *testing.T) {
	t.Parallel()
	c := fakeTessera(t, fakes.Step{Route: route, Action: fakes.ActionSucceed, Fixture: route + "/_paused"})
	got, err := c.Catalog(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := []app.ProviderAsset{preIPO(t, "tSpaceX", "TSPXcLV76s6V2zDiZQ18kBfcbnjaE2ZzNT3ga2Pd99v", "T-SpaceX", false)}
	if !slices.Equal(got, want) {
		t.Fatalf("Catalog = %+v, want only tSpaceX, untradable", got)
	}
}

func TestClient_failsWhenTheUpstreamOrItsPayloadIsBroken(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		step fakes.Step
		want errs.Code
	}{
		"server error": {fakes.Step{Route: route, Action: fakes.ActionFail, Status: 503}, errs.CodeUpstreamUnavailable},
		"not found":    {fakes.Step{Route: route, Action: fakes.ActionFail, Status: 404}, errs.CodeUpstreamUnavailable},
		"malformed": {
			fakes.Step{Route: route, Action: fakes.ActionSucceed, Fixture: route + "/_malformed"}, errs.CodeDecodeFailed,
		},
		"bad mint": {
			fakes.Step{Route: route, Action: fakes.ActionSucceed, Fixture: route + "/_bad-mint"}, errs.CodeDecodeFailed,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := fakeTessera(t, tc.step).Catalog(t.Context())
			if errs.CodeOf(err) != tc.want || got != nil {
				t.Fatalf("Catalog = %v, %v, want %s", got, err, tc.want)
			}
		})
	}
}
