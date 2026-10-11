package coinbaseb20_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/coinbaseb20"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
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

func TestListings_refusesWhatItCannotRead(t *testing.T) {
	t.Parallel()
	token := func(address, multiplier string) string {
		return `{"tokens":[{"contract_address":"` + address + `","symbol":"AAPLc","decimals":8,"multiplier":` +
			multiplier + `,"is_live":true}]}`
	}
	for name, tc := range map[string]struct {
		status int
		body   string
		want   errs.Code
	}{
		"down":        {http.StatusBadGateway, `{}`, errs.CodeUpstreamUnavailable},
		"refused":     {http.StatusForbidden, `{}`, errs.CodeUpstreamUnavailable},
		"bad json":    {http.StatusOK, `{"tokens":`, errs.CodeDecodeFailed},
		"bad address": {http.StatusOK, token("0x12", "1"), errs.CodeDecodeFailed},
		"zero shares": {http.StatusOK, token(aaplOnBase, "0"), errs.CodeDecodeFailed},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := serving(t, tc.status, tc.body).Listings(t.Context()); errs.CodeOf(err) != tc.want {
				t.Fatalf("err = %v, want %s", err, tc.want)
			}
		})
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
