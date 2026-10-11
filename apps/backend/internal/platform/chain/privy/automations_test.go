package privy_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const memberDest = chain.SolanaAddress("Dht9c9YfstFWkNYXgqr8HZbhqVn563bCpNU6zL32Ftqf")

func routes(byPath map[string]string) *upstream {
	return &upstream{handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := byPath[r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery]
		if !ok {
			body, ok = byPath[r.Method+" "+r.URL.Path]
		}
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, body)
	})}
}

func TestAttach_createsOneAutomationAndAttachesEachWalletOnce(t *testing.T) {
	t.Parallel()
	c, u, srv := overFakes(t)
	for range 2 {
		if err := c.Attach(t.Context(), "wallet-evm-a", memberDest); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Attach(t.Context(), "wallet-evm-b", memberDest); err != nil {
		t.Fatal(err)
	}
	if got := srv.PrivyAttachments(
		"wallet-evm-a",
	); len(got) != 1 ||
		got[0]["destination_address"] != string(memberDest) {
		t.Fatalf("attachments of a = %v, want one to the member wallet", got)
	}
	creates, attaches := automationWrites(u)
	assertDepositForwardCreate(t, creates, attaches)
}

func automationWrites(u *upstream) ([]sent, []sent) {
	var creates, attaches []sent
	for _, r := range u.requests() {
		switch {
		case r.method == http.MethodPost && r.path == "/privy/v1/wallet_automations":
			creates = append(creates, r)
		case r.method == http.MethodPost && strings.HasSuffix(r.path, "/automations/attach"):
			attaches = append(attaches, r)
		}
	}
	return creates, attaches
}

func assertDepositForwardCreate(t *testing.T, creates, attaches []sent) {
	t.Helper()
	if len(creates) != 1 || len(attaches) != 3 {
		t.Fatalf("%d creates and %d attaches, want 1 and 3", len(creates), len(attaches))
	}
	if key := creates[0].header.Get("privy-idempotency-key"); key != "automation:monaco-evm-deposit-forward" {
		t.Fatalf("create key = %q", key)
	}
	if key := attaches[0].header.Get("privy-idempotency-key"); key != "attach:wallet-evm-a:"+string(memberDest) {
		t.Fatalf("attach key = %q", key)
	}
	want := `{"config":{"action":{"destination_chain_asset":{"asset":"usdc","chain":"solana"},"type":"swap"},` +
		`"trigger":{"assets":{"mode":"include","values":[{"asset":"usdc","chain":"base"},` +
		`{"asset":"usdc","chain":"arbitrum"},{"asset":"usdc","chain":"ethereum"}]},"type":"deposit"}},` +
		`"name":"monaco-evm-deposit-forward","owner_id":"` + fakes.PrivyAuthorizationKeyID + `"}`
	var body map[string]any
	_ = json.Unmarshal([]byte(creates[0].body), &body)
	if got, _ := json.Marshal(body); string(got) != want {
		t.Fatalf("create = %s\nwant     %s", got, want)
	}
	if attaches[0].header.Get("privy-authorization-signature") == "" {
		t.Fatal("attach is not signed")
	}
}

func TestAttach_aWalletAttachedElsewhereIsAMismatchNeverKept(t *testing.T) {
	t.Parallel()
	c, _, srv := overFakes(t)
	if err := c.Attach(t.Context(), "wallet-evm-a", memberDest); err != nil {
		t.Fatal(err)
	}
	other := chain.SolanaAddress("4RpUTNtzuCs9xNvQnygP3dyyhH3NLh1NFhuFXeA8PDKU")
	wantCode(t, c.Attach(t.Context(), "wallet-evm-a", other), errs.CodeWalletMismatch)
	if got := srv.PrivyAttachments(
		"wallet-evm-a",
	); len(got) != 1 ||
		got[0]["destination_address"] != string(memberDest) {
		t.Fatalf("attachments = %v", got)
	}
}

func TestAttach_refusals(t *testing.T) {
	t.Parallel()
	c, u, _ := overFakes(t)
	err := c.Attach(t.Context(), "wallet-evm-a", "not-base58!")
	wantCode(t, err, errs.CodeInvalidInput)
	if len(u.requests()) != 0 {
		t.Fatal("a bad destination reached Privy")
	}
	cfg := testConfig()
	cfg.Privy.AuthorizationPrivateKey = ""
	wantCode(t, clientWith(cfg, nil, clock.Real{}).Attach(t.Context(), "w", memberDest), errs.CodeInternal)
	cfg = testConfig()
	cfg.Privy.AuthorizationKeyID = ""
	wantCode(t, clientWith(cfg, nil, clock.Real{}).Attach(t.Context(), "w", memberDest), errs.CodeInternal)
	listed := `{"data":[{"id":"auto-1","name":"monaco-evm-deposit-forward","status":"enabled"}],"next_cursor":null}`
	for name, tc := range map[string]struct {
		routes map[string]string
		want   errs.Code
	}{
		"list fails": {map[string]string{}, errs.CodeNotFound},
		"attach answers another wallet": {map[string]string{
			"GET /privy/v1/wallet_automations": listed,
			"POST /privy/v1/wallets/w/automations/attach": `{"data":[{"automation_id":"auto-1","wallet_id":"x",` +
				`"params":{"destination_address":"` + string(memberDest) + `"}}]}`,
		}, errs.CodeDecodeFailed},
		"attach fails": {map[string]string{
			"GET /privy/v1/wallet_automations":                       listed,
			"GET /privy/v1/wallet_automations?limit=100&wallet_id=w": `{"data":[],"next_cursor":null}`,
		}, errs.CodeNotFound},
		"create answers no id": {map[string]string{
			"GET /privy/v1/wallet_automations":  `{"data":[],"next_cursor":null}`,
			"POST /privy/v1/wallet_automations": `{}`,
		}, errs.CodeDecodeFailed},
		"create fails": {map[string]string{
			"GET /privy/v1/wallet_automations": `{"data":[{"id":"old","name":"monaco-evm-deposit-forward",` +
				`"status":"disabled"}],"next_cursor":null}`,
		}, errs.CodeNotFound},
	} {
		err := client(routes(tc.routes)).Attach(t.Context(), "w", memberDest)
		if errs.CodeOf(err) != tc.want {
			t.Fatalf("%s: err = %v, want %s", name, err, tc.want)
		}
	}
}

func TestCreateAutomation_pagesThroughAndReusesTheEnabledOne(t *testing.T) {
	t.Parallel()
	pages := 0
	u := &upstream{handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pages++
		if r.URL.Query().Get("cursor") == "" {
			_, _ = io.WriteString(w, `{"data":[{"id":"other","name":"x","status":"enabled"}],"next_cursor":"p2"}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":[{"id":"ours","name":"monaco-evm-deposit-forward","status":"enabled"}],`+
			`"next_cursor":null}`)
	})}
	id, err := client(u).CreateAutomation(t.Context())
	if err != nil || id != "ours" || pages != 2 {
		t.Fatalf("CreateAutomation = %q, %v after %d pages", id, err, pages)
	}
}

func TestExecutions_readEachActionForItsAmountAndSolanaSignature(t *testing.T) {
	t.Parallel()
	page := `{"data":[` +
		`{"id":"e1","trigger_caip2":"eip155:8453","trigger_tx_hash":"0xsrc","wallet_action_id":"a1","status":"completed"},` +
		`{"id":"e2","trigger_caip2":"eip155:42161","trigger_tx_hash":"0xsrc2","wallet_action_id":null,"status":"pending"}` +
		`],"next_cursor":"next"}`
	action := `{"id":"a1","status":"succeeded","input_amount":"1000000","output_amount":"999250","steps":[` +
		`{"type":"evm_transaction","status":"confirmed","caip2":"eip155:8453","transaction_hash":"0xab"},` +
		`{"type":"svm_transaction","status":"confirmed","caip2":"solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp",` +
		`"transaction_signature":"sig"}]}`
	u := routes(map[string]string{
		"GET /privy/v1/wallet_automations/executions": page,
		"GET /privy/v1/wallets/w/actions/a1":          action,
	})
	got, err := client(u).Executions(t.Context(), "w", "c1")
	want := []multichain.Execution{
		{
			ID: "e1", ActionID: "a1", Network: chain.NetworkBase, SourceTx: "0xsrc",
			AmountMicros: money.MicrosFromUint64(999_250), State: multichain.ActionSucceeded, Signature: "sig",
		},
		{ID: "e2", Network: chain.NetworkArbitrum, SourceTx: "0xsrc2", State: multichain.ActionPending},
	}
	if err != nil || got.Next != "next" || len(got.Executions) != 2 || got.Executions[0] != want[0] ||
		got.Executions[1] != want[1] {
		t.Fatalf("Executions = %+v, %v", got, err)
	}
	if q := u.requests()[0].query; q != "cursor=c1&limit=100&wallet_id=w" {
		t.Fatalf("query = %q", q)
	}
	pending := `{"id":"a1","status":"pending","input_amount":"1000000","output_amount":null}`
	got, err = client(routes(map[string]string{
		"GET /privy/v1/wallet_automations/executions": page,
		"GET /privy/v1/wallets/w/actions/a1":          pending,
	})).Executions(t.Context(), "w", "")
	if err != nil || got.Executions[0].AmountMicros != money.MicrosFromUint64(1_000_000) {
		t.Fatalf("pending action amount = %+v, %v; want the input amount", got, err)
	}
	got, err = client(routes(map[string]string{
		"GET /privy/v1/wallet_automations/executions": page,
		"GET /privy/v1/wallets/w/actions/a1":          `{"id":"a1","status":"pending"}`,
	})).Executions(t.Context(), "w", "")
	if err != nil || !got.Executions[0].AmountMicros.IsZero() {
		t.Fatalf("action without amounts = %+v, %v; want zero", got, err)
	}
}

func TestExecutions_decodeStrictly(t *testing.T) {
	t.Parallel()
	exec := func(caip2, status string) string {
		return `{"data":[{"id":"e1","trigger_caip2":"` + caip2 + `","trigger_tx_hash":"0x","wallet_action_id":"a1",` +
			`"status":"` + status + `"}],"next_cursor":null}`
	}
	for name, tc := range map[string]struct {
		page, action string
		want         errs.Code
	}{
		"unknown status":   {exec("eip155:8453", "teleported"), `{}`, errs.CodeDecodeFailed},
		"solana trigger":   {exec("solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp", "pending"), `{}`, errs.CodeDecodeFailed},
		"bad amount":       {exec("eip155:8453", "completed"), `{"id":"a1","status":"succeeded","output_amount":"1.5"}`, errs.CodeDecodeFailed},
		"bad action":       {exec("eip155:8453", "completed"), `{"id":"a1","status":"odd"}`, errs.CodeDecodeFailed},
		"action not found": {exec("eip155:8453", "skipped"), "", errs.CodeNotFound},
		"failed action":    {exec("eip155:1", "failed"), `{"id":"a1","status":"failed","output_amount":"x"}`, errs.CodeDecodeFailed},
	} {
		byPath := map[string]string{"GET /privy/v1/wallet_automations/executions": tc.page}
		if tc.action != "" {
			byPath["GET /privy/v1/wallets/w/actions/a1"] = tc.action
		}
		_, err := client(routes(byPath)).Executions(t.Context(), "w", "")
		if errs.CodeOf(err) != tc.want {
			t.Fatalf("%s: err = %v, want %s", name, err, tc.want)
		}
	}
	_, err := client(routes(map[string]string{})).Executions(t.Context(), "w", "")
	wantCode(t, err, errs.CodeNotFound)
}

func TestReindex_checksTheNetworksUSDC(t *testing.T) {
	t.Parallel()
	c, u, _ := overFakes(t)
	if err := c.Reindex(t.Context(), "wallet-evm-a", chain.NetworkArbitrum); err != nil {
		t.Fatal(err)
	}
	var body map[string]string
	_ = json.Unmarshal([]byte(u.requests()[0].body), &body)
	if body["caip2"] != "eip155:42161" || body["asset_address"] != chain.NetworkArbitrum.Cash().Address ||
		body["wallet_id"] != "wallet-evm-a" {
		t.Fatalf("reindex body = %v", body)
	}
	wantCode(t, c.Reindex(t.Context(), "w", chain.NetworkSolana), errs.CodeInvalidNetwork)
	for status, want := range map[string]errs.Code{"failed": errs.CodePrivyUnavailable, "odd": errs.CodeDecodeFailed} {
		u := replying(http.StatusOK, `{"wallet_id":"w","results":[{"status":"`+status+`"}]}`)
		wantCode(t, client(u).Reindex(t.Context(), "w", chain.NetworkBase), want)
	}
	wantCode(t, client(replying(http.StatusBadRequest, `{}`)).Reindex(t.Context(), "w", chain.NetworkBase),
		errs.CodeInvalidInput)
}
