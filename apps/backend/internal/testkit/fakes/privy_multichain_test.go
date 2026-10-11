package fakes_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func privyCall(
	t *testing.T,
	srv *fakes.Server,
	method, path, body string,
	headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, "http://privy.test"+path, strings.NewReader(body))
	req.Header.Set("privy-app-id", "app")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func signedHeaders(t *testing.T, path, body string) map[string]string {
	t.Helper()
	const key = "k"
	var parsed any
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{"privy-app-id": "app", "privy-idempotency-key": key}
	sig := privy.AuthorizationSignature(
		fakes.PrivyAuthorizationKey(),
		http.MethodPost,
		"http://privy.test"+path,
		parsed,
		headers,
	)
	return map[string]string{"privy-authorization-signature": sig, "privy-idempotency-key": key}
}

func TestPrivyMultichainFake_refusesMalformedRequests(t *testing.T) {
	t.Parallel()
	srv := fakes.New()
	transfer := "/privy/v1/wallets/w/transfer"
	attach := "/privy/v1/wallets/w/automations/attach"
	for name, tc := range map[string]struct {
		method, path, body string
		headers            map[string]string
		want               int
	}{
		"evm wallet without a key": {http.MethodPost, "/privy/v1/wallets", `{"chain_type":"ethereum","owner_id":"q"}`, nil, 400},
		"transfer not json":        {http.MethodPost, transfer, `nope`, map[string]string{"privy-idempotency-key": "k"}, 401},
		"transfer without a key":   {http.MethodPost, transfer, `{}`, nil, 401},
		"transfer without an asset": {
			http.MethodPost, transfer, `{"amount":"1","source":{"chain":"solana"},"destination":{"address":"x"}}`,
			signedHeaders(t, transfer, `{"amount":"1","source":{"chain":"solana"},"destination":{"address":"x"}}`), 400,
		},
		"unknown action":        {http.MethodGet, "/privy/v1/wallets/w/actions/none", ``, nil, 404},
		"automation no owner":   {http.MethodPost, "/privy/v1/wallet_automations", `{"name":"x","config":{}}`, nil, 400},
		"attach unsigned":       {http.MethodPost, attach, `{}`, nil, 401},
		"attach no params":      {http.MethodPost, attach, `{"automation_ids":["a"]}`, signedHeaders(t, attach, `{"automation_ids":["a"]}`), 400},
		"reindex without asset": {http.MethodPost, "/privy/v1/wallet_automations/reindex", `{"wallet_id":"w"}`, nil, 400},
	} {
		if rec := privyCall(t, srv, tc.method, tc.path, tc.body, tc.headers); rec.Code != tc.want {
			t.Fatalf("%s: %d %s, want %d", name, rec.Code, rec.Body.String(), tc.want)
		}
	}
	rec := privyCall(t, srv, http.MethodGet, "/privy/v1/wallet_automations/executions?wallet_id=w", ``, nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"data":[]`) {
		t.Fatalf("executions = %d %s", rec.Code, rec.Body.String())
	}
}

func TestPrivyMultichainFake_anEVMSourceTransferReportsAnEVMStep(t *testing.T) {
	t.Parallel()
	srv := fakes.New()
	path := "/privy/v1/wallets/w/transfer"
	body := `{"amount":"1","source":{"asset":"usdc","chain":"base"},` +
		`"destination":{"address":"x","asset":"usdc","chain":"solana"}}`
	rec := privyCall(t, srv, http.MethodPost, path, body, signedHeaders(t, path, body))
	var action struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &action); err != nil || rec.Code != 200 {
		t.Fatalf("transfer = %d %s", rec.Code, rec.Body.String())
	}
	rec = privyCall(t, srv, http.MethodGet, "/privy/v1/wallets/w/actions/"+action.ID, ``, nil)
	if !strings.Contains(rec.Body.String(), `"evm_transaction"`) ||
		!strings.Contains(rec.Body.String(), chain.NetworkBase.CAIP2()) {
		t.Fatalf("action = %s", rec.Body.String())
	}
}

func TestPrivyMultichainFake_bridgesToEachChainsCashAsset(t *testing.T) {
	t.Parallel()
	path := "/privy/v1/wallets/w/transfer"
	leg := func(asset, chain string) string { return `{"asset":"` + asset + `","chain":"` + chain + `"}` }
	for _, tc := range []struct{ source, dest, want string }{
		{leg("usdc", "solana"), leg("usdg", "robinhood"), `"status":"pending"`},
		{leg("usdg", "robinhood"), leg("usdc", "solana"), `"status":"pending"`},
		{leg("usdc", "solana"), leg("usdc", "robinhood"), "usdc is not available on robinhood"},
		{leg("usdc", "solana"), leg("usdg", "base"), "usdg is not available on base"},
	} {
		body := `{"amount":"1","source":` + tc.source + `,"destination":` +
			strings.Replace(tc.dest, "{", `{"address":"x",`, 1) + `}`
		rec := privyCall(t, fakes.New(), http.MethodPost, path, body, signedHeaders(t, path, body))
		if !strings.Contains(rec.Body.String(), tc.want) {
			t.Fatalf("transfer %s = %d %s, want %s", body, rec.Code, rec.Body.String(), tc.want)
		}
	}
}

func TestPrivyMultichainFake_pagesExecutionsAndRefusesAChangedAttachReplay(t *testing.T) {
	t.Parallel()
	srv := fakes.New()
	for _, id := range []string{"e1", "e2", "e3"} {
		srv.SeedPrivyExecution(map[string]any{"id": id, "wallet_id": "w"})
	}
	srv.SeedPrivyExecution(map[string]any{"id": "other", "wallet_id": "x"})
	var seen []string
	cursor := ""
	for range 3 {
		rec := privyCall(
			t,
			srv,
			http.MethodGet,
			"/privy/v1/wallet_automations/executions?wallet_id=w&limit=2&cursor="+cursor,
			``,
			nil,
		)
		var page struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
			Next *string `json:"next_cursor"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &page)
		for _, e := range page.Data {
			seen = append(seen, e.ID)
		}
		if page.Next == nil {
			break
		}
		cursor = *page.Next
	}
	if strings.Join(seen, ",") != "e1,e2,e3" {
		t.Fatalf("paged %v", seen)
	}
	attach := "/privy/v1/wallets/w/automations/attach"
	first := `{"automation_ids":["a"],"params":{"destination_address":"d1"}}`
	second := `{"automation_ids":["a"],"params":{"destination_address":"d2"}}`
	if rec := privyCall(t, srv, http.MethodPost, attach, first, signedHeaders(t, attach, first)); rec.Code != 200 {
		t.Fatalf("attach = %d", rec.Code)
	}
	if rec := privyCall(
		t,
		srv,
		http.MethodPost,
		attach,
		second,
		signedHeaders(t, attach, second),
	); rec.Code != 400 {
		t.Fatalf("changed replay = %d, want 400", rec.Code)
	}
	create := `{"name":"x","owner_id":"q","config":{"a":1}}`
	one := privyCall(
		t,
		srv,
		http.MethodPost,
		"/privy/v1/wallet_automations",
		create,
		map[string]string{"privy-idempotency-key": "c"},
	)
	two := privyCall(
		t,
		srv,
		http.MethodPost,
		"/privy/v1/wallet_automations",
		create,
		map[string]string{"privy-idempotency-key": "c"},
	)
	if one.Body.String() != two.Body.String() {
		t.Fatalf("keyed creates differ: %s / %s", one.Body.String(), two.Body.String())
	}
	rec := privyCall(t, srv, http.MethodGet, "/privy/v1/wallet_automations/executions?limit=0", ``, nil)
	if !strings.Contains(rec.Body.String(), `"other"`) {
		t.Fatalf("unfiltered executions = %s", rec.Body.String())
	}
}

func TestPrivyMultichainFake_ethereumRPCAndOutcomeControlRefuseMalformedRequests(t *testing.T) {
	t.Parallel()
	srv := fakes.New()
	rpc := "/privy/v1/wallets/w/rpc"
	noTo := `{"method":"eth_signTransaction","caip2":"eip155:8453","params":{"transaction":{}}}`
	for name, tc := range map[string]struct {
		path, body string
		headers    map[string]string
		want       int
	}{
		"send without a key": {rpc, `{"method":"eth_sendTransaction"}`, nil, 401},
		"sign without a to":  {rpc, noTo, signedHeaders(t, rpc, noTo), 400},
		"bad outcome":        {"/_privy_action", `{"status":"exploded"}`, nil, 400},
		"outcome not json":   {"/_privy_action", `nope`, nil, 400},
		"outcome reset":      {"/_privy_action", `{"status":""}`, nil, 204},
	} {
		if rec := privyCall(t, srv, http.MethodPost, tc.path, tc.body, tc.headers); rec.Code != tc.want {
			t.Fatalf("%s: %d %s, want %d", name, rec.Code, rec.Body.String(), tc.want)
		}
	}
}
