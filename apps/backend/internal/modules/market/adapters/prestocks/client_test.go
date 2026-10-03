package prestocks_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/prestocks"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const route = "/prestocks/api/prestocks"

func fakePreStocks(t *testing.T, steps ...fakes.Step) *prestocks.Client {
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
	return prestocks.New(httpclient.New("prestocks", httpclient.WithBaseURL(srv.URL+"/prestocks"),
		httpclient.WithTimeout(10*time.Second)))
}

func preIPO(t *testing.T, symbol, mint, name, logo string, tradable bool) app.ProviderAsset {
	t.Helper()
	m, err := domain.ParseMint(mint)
	if err != nil {
		t.Fatal(err)
	}
	return app.ProviderAsset{
		Symbol: symbol, Mint: m, Decimals: 9, Kind: domain.KindPreIPO, DisplayName: name, LogoURL: logo,
		Tradable: tradable,
	}
}

func TestClient_listsEveryPreStocksTokenAsATradablePreIPOAsset(t *testing.T) {
	t.Parallel()
	c := fakePreStocks(t)
	if c.Issuer() != domain.IssuerPreStocks {
		t.Fatalf("Issuer = %s, want prestocks", c.Issuer())
	}
	got, err := c.Catalog(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	logo := "https://www.prestocks.com/logos/"
	want := []app.ProviderAsset{
		preIPO(
			t,
			"ANDURIL",
			"PresTj4Yc2bAR197Er7wz4UUKSfqt6FryBEdAriBoQB",
			"Anduril PreStocks",
			logo+"anduril.png",
			true,
		),
		preIPO(t, "ANTHROPIC", "Pren1FvFX6J3E4kXhJuCiAD5aDmGEb7qJRncwA8Lkhw", "Anthropic PreStocks",
			logo+"anthropic.png", true),
		preIPO(t, "FIGUREAI", "PreZad18qfPtbxNpMtMuAuX2zVpvkEU8DnJx56faCWd", "Figure AI PreStocks",
			logo+"figureai.png", true),
		preIPO(t, "KALSHI", "PreLWGkkeqG1s4HEfFZSy9moCrJ7btsHuUtfcCeoRua", "Kalshi PreStocks", logo+"kalshi.png", true),
		preIPO(t, "NEURALINK", "PrekqLJvJ3qVdXmBGDiexvwUTF4rLFDa6HWS4HJbw9S", "Neuralink PreStocks",
			logo+"neuralink.png", true),
		preIPO(t, "OPENAI", "PreweJYECqtQwBtpxHL171nL2K6umo692gTm7Q3rpgF", "OpenAI PreStocks", logo+"openai.png", true),
		preIPO(t, "POLYMARKET", "Pre8AREmFPtoJFT8mQSXQLh56cwJmM7CFDRuoGBZiUP", "Polymarket PreStocks",
			logo+"polymarket.png", true),
		preIPO(t, "SPACEX", "PreANxuXjsy2pvisWWMNB6YaJNzr7681wJJr2rHsfTh", "SpaceX PreStocks", logo+"spacex.png", true),
	}
	if !slices.Equal(got, want) {
		t.Fatalf("Catalog = %+v\nwant %+v", got, want)
	}
}

func TestClient_marksAPausedTokenUntradableAndSkipsOneWithNoMint(t *testing.T) {
	t.Parallel()
	c := fakePreStocks(t, fakes.Step{Route: route, Action: fakes.ActionSucceed, Fixture: route + "/_paused"})
	got, err := c.Catalog(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := []app.ProviderAsset{
		preIPO(t, "SPACEX", "PreANxuXjsy2pvisWWMNB6YaJNzr7681wJJr2rHsfTh", "SpaceX PreStocks", "", false),
	}
	if !slices.Equal(got, want) {
		t.Fatalf("Catalog = %+v, want only SPACEX, untradable, without its plain-HTTP logo", got)
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
			got, err := fakePreStocks(t, tc.step).Catalog(t.Context())
			if errs.CodeOf(err) != tc.want || got != nil {
				t.Fatalf("Catalog = %v, %v, want %s", got, err, tc.want)
			}
		})
	}
}
