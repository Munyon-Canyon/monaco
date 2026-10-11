package adapters

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func bridgeServer(t *testing.T, status int, body string, seen *bridgeQuoteRequest) PrivyBridgeQuotes {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, pass, ok := r.BasicAuth(); !ok || user != "app" || pass != "secret" ||
			r.Header.Get("privy-app-id") != "app" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		if seen != nil {
			_ = json.Unmarshal(raw, seen)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return NewPrivyBridgeQuotes(httpclient.New("privy-bridge", httpclient.WithBaseURL(srv.URL),
		httpclient.WithTimeout(time.Second)), "app", "secret")
}

func TestBridgeOutReadsPrivysQuote(t *testing.T) {
	t.Parallel()
	var seen bridgeQuoteRequest
	b := bridgeServer(t, http.StatusOK,
		`{"input_amount":"100","estimated_output_amount":"99.925","created_at":"2026-10-11T09:02:20.834Z"}`, &seen)
	got, err := b.BridgeOut(t.Context(), chain.NetworkBase, money.MicrosFromUint64(100_000_000))
	if err != nil || got.Uint64() != 99_925_000 {
		t.Fatalf("BridgeOut = %s, %v, want 99925000", got, err)
	}
	if seen.Source.Chain != "solana" || seen.Destination.Chain != "base" || seen.InputAmount != "100.000000" ||
		seen.Source.Asset != "usdc" || seen.Destination.Asset != "usdc" {
		t.Fatalf("request = %+v", seen)
	}
}

func TestBridgeOutRefusesWhatItCannotUse(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		status int
		body   string
		want   errs.Code
	}{
		"unsupported": {http.StatusBadRequest, `{"error":"usdc is not available"}`, errs.CodeNoRoute},
		"bad json":    {http.StatusOK, `{"estimated`, errs.CodeDecodeFailed},
		"no amount":   {http.StatusOK, `{"estimated_output_amount":"x"}`, errs.CodeDecodeFailed},
		"zero":        {http.StatusOK, `{"estimated_output_amount":"0"}`, errs.CodeDecodeFailed},
		"more out":    {http.StatusOK, `{"estimated_output_amount":"100.5"}`, errs.CodeDecodeFailed},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := bridgeServer(t, tc.status, tc.body, nil).BridgeOut(t.Context(), chain.NetworkBase,
				money.MicrosFromUint64(100_000_000))
			if errs.CodeOf(err) != tc.want {
				t.Fatalf("err = %v, want %s", err, tc.want)
			}
		})
	}
}

func TestBridgeOutFailsWhenPrivyIsUnreachable(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	b := NewPrivyBridgeQuotes(httpclient.New("privy-bridge", httpclient.WithBaseURL(srv.URL),
		httpclient.WithTimeout(time.Second)), "app", "secret")
	if _, err := b.BridgeOut(t.Context(), chain.NetworkBase, money.MicrosFromUint64(1)); err == nil {
		t.Fatal("an unreachable Privy returned a quote")
	}
}
