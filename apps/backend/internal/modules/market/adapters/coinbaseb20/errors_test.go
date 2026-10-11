package coinbaseb20_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/coinbaseb20"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func serving(t *testing.T, status int, body string) *coinbaseb20.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return coinbaseb20.New(httpclient.New("coinbase", httpclient.WithBaseURL(srv.URL),
		httpclient.WithTimeout(10*time.Second)))
}

func TestListings_refusesABodyItCannotRead(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		status int
		body   string
		want   errs.Code
	}{
		"down":     {http.StatusBadGateway, `{}`, errs.CodeUpstreamUnavailable},
		"refused":  {http.StatusForbidden, `{}`, errs.CodeUpstreamUnavailable},
		"bad json": {http.StatusOK, `{"tokens":`, errs.CodeDecodeFailed},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := serving(t, tc.status, tc.body).Listings(t.Context()); errs.CodeOf(err) != tc.want {
				t.Fatalf("err = %v, want %s", err, tc.want)
			}
		})
	}
}

func TestListings_skipsAnUnreadableTokenAndAlertsOnlyWhenItIsLive(t *testing.T) {
	t.Parallel()
	token := func(symbol, address, decimals, multiplier, live string) string {
		return `{"contract_address":"` + address + `","symbol":"` + symbol + `",` + decimals + `"multiplier":` +
			multiplier + `,"is_live":` + live + `}`
	}
	body := `{"tokens":[` + strings.Join([]string{
		token("BADc", "0x12", `"decimals":8,`, "1", "true"),
		token("ZEROc", aaplOnBase, `"decimals":8,`, "0", "true"),
		token("NODECc", aaplOnBase, "", "1", "true"),
		token("DEADc", "0x12", `"decimals":8,`, "1", "false"),
		token("GOOGLc", googlOnBase, `"decimals":8,`, "1", "false"),
		token("AAPLc", aaplOnBase, `"decimals":8,`, "1", "true"),
	}, ",") + `]}`
	var logs testkit.Logs
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, &logs))
	batch, err := serving(t, http.StatusOK, body).Listings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	kept := make([]string, 0, len(batch.Listings))
	for _, l := range batch.Listings {
		kept = append(kept, fmt.Sprint(l.Symbol, " ", l.Tradable))
	}
	if want := []string{"GOOGLc false", "AAPLc true"}; !slices.Equal(kept, want) {
		t.Errorf("kept = %q, want %q", kept, want)
	}
	alerts := logs.Lines("market.listings.asset_skipped", "level", "issuer", "symbol", "reason")
	want := []string{
		"WARN coinbase BADc bad address", "WARN coinbase ZEROc bad multiplier", "WARN coinbase NODECc missing decimals",
	}
	if !slices.Equal(alerts, want) {
		t.Errorf("alerts = %q, want %q", alerts, want)
	}
}

func TestListings_failsWhenCoinbaseIsUnreachable(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	c := coinbaseb20.New(httpclient.New("coinbase", httpclient.WithBaseURL(srv.URL),
		httpclient.WithTimeout(time.Second)))
	if _, err := c.Listings(t.Context()); err == nil {
		t.Fatal("an unreachable API returned listings")
	}
}
