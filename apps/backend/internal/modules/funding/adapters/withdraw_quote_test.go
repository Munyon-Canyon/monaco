package adapters_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const quoteTo = "0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed"

type quoteCall struct {
	path, user, pass, appID string
	body                    map[string]any
}

func quoteServer(t *testing.T, status int, reply string) (*httptest.Server, *[]quoteCall) {
	t.Helper()
	var calls []quoteCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, _ := r.BasicAuth()
		raw, _ := io.ReadAll(r.Body)
		c := quoteCall{path: r.URL.Path, user: user, pass: pass, appID: r.Header.Get("privy-app-id")}
		_ = json.Unmarshal(raw, &c.body)
		calls = append(calls, c)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func quotes(url string) adapters.PrivyTransferQuotes {
	return adapters.NewPrivyTransferQuotes(config.Config{
		Privy:    config.Privy{AppID: "app", AppSecret: "privy-app-5ecret", BaseURL: url},
		Timeouts: config.Timeouts{Privy: time.Second},
	}, clock.Real{})
}

func quoteReply(output string, expiresIn time.Duration) string {
	expires := clock.Real{}.Now().Add(expiresIn).Unix()
	return `{"estimated_output_amount":"` + output + `","expires_at":` + strconv.FormatInt(expires, 10) + `}`
}

func quoteSpec(micros uint64) multichain.TransferSpec {
	return multichain.TransferSpec{
		From: chain.NetworkSolana, To: chain.NetworkBase, Destination: quoteTo,
		AmountMicros: money.MicrosFromUint64(micros),
	}
}

func TestPrivyTransferQuotes_asksPrivyForAnExactInputQuote(t *testing.T) {
	t.Parallel()
	expires := clock.Real{}.Now().Add(time.Minute).Unix()
	srv, calls := quoteServer(t, http.StatusOK,
		`{"estimated_output_amount":"1.498875","estimated_input_amount":"1.5","expires_at":`+
			strconv.FormatInt(expires, 10)+`}`)
	got, err := quotes(srv.URL).QuoteTransfer(t.Context(), "wallet-1", quoteSpec(1_500_000))
	if err != nil || got.ReceiveMicros.String() != "1498875" || got.ExpiresAt.Unix() != expires {
		t.Fatalf("QuoteTransfer = %+v, %v", got, err)
	}
	c := (*calls)[0]
	wantBody := `{"amount":"1.5","amount_type":"exact_input","destination":{"address":"` + quoteTo +
		`","asset":"usdc","chain":"base"},"source":{"asset":"usdc","chain":"solana"}}`
	body, _ := json.Marshal(c.body)
	if c.path != "/v1/wallets/wallet-1/transfer/quote" || c.user != "app" || c.pass != "privy-app-5ecret" ||
		c.appID != "app" || string(body) != wantBody {
		t.Fatalf("call = %+v body %s", c, body)
	}
}

func TestPrivyTransferQuotes_sendsAWholeAmountWithoutAFraction(t *testing.T) {
	t.Parallel()
	srv, calls := quoteServer(t, http.StatusOK, quoteReply("2", 30*time.Second))
	got, err := quotes(srv.URL).QuoteTransfer(t.Context(), "wallet-1", quoteSpec(2_000_000))
	if err != nil || got.ReceiveMicros.String() != "2000000" || (*calls)[0].body["amount"] != "2" {
		t.Fatalf("QuoteTransfer = %+v, %v, calls %+v", got, err, *calls)
	}
}

func TestPrivyTransferQuotes_refusals(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		status int
		reply  string
		want   errs.Code
	}{
		"privy refuses the request": {http.StatusBadRequest, `{"error":"bad"}`, errs.CodeInvalidInput},
		"the wallet is short": {
			http.StatusBadRequest,
			`{"error":"Insufficient balance: wallet has insufficient funds for this transfer","code":"invalid_data"}`,
			errs.CodeInsufficientFunds,
		},
		"privy is down": {http.StatusServiceUnavailable, ``, errs.CodePrivyUnavailable},
		"not json":      {http.StatusOK, `nope`, errs.CodeDecodeFailed},
		"not a decimal": {http.StatusOK, quoteReply("x", 30*time.Second), errs.CodeDecodeFailed},
		"expired":       {http.StatusOK, quoteReply("1", -time.Second), errs.CodeDecodeFailed},
		"too far ahead": {http.StatusOK, quoteReply("1", 6*time.Minute), errs.CodeDecodeFailed},
		"milliseconds": {
			http.StatusOK,
			`{"estimated_output_amount":"1","expires_at":` +
				strconv.FormatInt(clock.Real{}.Now().Add(30*time.Second).UnixMilli(), 10) + `}`,
			errs.CodeDecodeFailed,
		},
		"no expiry": {http.StatusOK, `{"estimated_output_amount":"1"}`, errs.CodeDecodeFailed},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			srv, _ := quoteServer(t, c.status, c.reply)
			if _, err := quotes(
				srv.URL,
			).QuoteTransfer(t.Context(), "wallet-1", quoteSpec(1_000_000)); errs.CodeOf(
				err,
			) != c.want {
				t.Fatalf("QuoteTransfer err = %v, want %s", err, c.want)
			}
		})
	}
}

func TestPrivyTransferQuotes_unreachablePrivyIsUnavailable(t *testing.T) {
	t.Parallel()
	srv, _ := quoteServer(t, http.StatusOK, ``)
	srv.Close()
	if _, err := quotes(
		srv.URL,
	).QuoteTransfer(t.Context(), "wallet-1", quoteSpec(1_000_000)); errs.CodeOf(
		err,
	) != errs.CodePrivyUnavailable {
		t.Fatalf("QuoteTransfer err = %v, want privy_unavailable", err)
	}
}

func TestPrivyTransferQuotes_failsWithoutPrivyConfig(t *testing.T) {
	t.Parallel()
	q := adapters.NewPrivyTransferQuotes(config.Config{}, clock.Real{})
	if _, err := q.QuoteTransfer(t.Context(), "wallet-1", quoteSpec(1_000_000)); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("QuoteTransfer err = %v, want internal", err)
	}
}
