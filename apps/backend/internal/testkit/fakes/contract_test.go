package fakes_test

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	chainprivy "github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const (
	aaplxMint      = chain.SolanaAddress("XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp")
	depositWatched = "7Moh9vxpdx8awiKHGpWLFze6sXVeZgBphdP2EopQ78UG"
	walletFlows    = "flows 01, 01a, 01b, 01c, 01d, 01e, 02, 03, 04, 14, 22, 22a, 22b, 22c, 22d, 22f, 23, 29 and 36"
	everyFlow      = "every flow from 00 to 39"
)

type contractWorld struct {
	srv   *fakes.Server
	http  *httpclient.Client
	rpc   *solana.Client
	privy *chainprivy.Client
}

func newContractWorld(t *testing.T) contractWorld {
	t.Helper()
	srv := fakes.New()
	transport := inProcess{srv}
	privy, err := chainprivy.New(config.Config{
		Privy: config.Privy{
			AppID: "contract-app", AppSecret: "contract-app-secret", BaseURL: "http://fakes.test/privy",
			VerificationKey:         fakes.PrivyVerificationKey(),
			AuthorizationPrivateKey: fakes.PrivyAuthorizationKeyConfig(),
			AuthorizationKeyID:      fakes.PrivyAuthorizationKeyID,
		},
		Timeouts: config.Timeouts{Privy: 10 * time.Second},
	}, clock.Real{}, httpclient.WithTransport(transport))
	if err != nil {
		t.Fatal(err)
	}
	return contractWorld{
		srv: srv,
		http: httpclient.New("fakes", httpclient.WithBaseURL("http://fakes.test"),
			httpclient.WithTimeout(time.Minute), httpclient.WithTransport(transport)),
		rpc: solana.New(config.Config{
			Solana:   config.Solana{RPCURL: "http://fakes.test/rpc"},
			Timeouts: config.Timeouts{RPC: 5 * time.Second},
		}, clock.Real{}, httpclient.WithTransport(transport)),
		privy: privy,
	}
}

func (w contractWorld) call(t *testing.T, method, path, body string, header ...string) reply {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, "http://fakes.test"+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	resp, err := w.http.Do(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return reply{status: resp.StatusCode, header: resp.Header, body: string(raw)}
}

func (w contractWorld) rpcCall(t *testing.T, method, params string) reply {
	t.Helper()
	return w.call(t, http.MethodPost, "/rpc", `{"jsonrpc":"2.0","id":1,"method":"`+method+`","params":`+params+`}`)
}

func at(body string, path ...string) any {
	var v any
	if json.Unmarshal([]byte(body), &v) != nil {
		return nil
	}
	for _, key := range path {
		switch node := v.(type) {
		case map[string]any:
			v = node[key]
		case []any:
			i, err := strconv.Atoi(key)
			if err != nil || i < 0 || i >= len(node) {
				return nil
			}
			v = node[i]
		default:
			return nil
		}
	}
	return v
}

func answered(r reply) string { return strconv.Itoa(r.status) + " " + r.body }

func (w contractWorld) usdc(t *testing.T, owner chain.SolanaAddress) (money.BaseUnits, string) {
	t.Helper()
	got, err := w.rpc.TokenBalance(t.Context(), owner, chain.Mint{Address: usdcMint, Decimals: 6})
	if err != nil {
		return money.BaseUnits{}, err.Error()
	}
	return got, ""
}

type defaultAnswer struct {
	name    string
	relies  string
	answers func(t *testing.T, w contractWorld) string
}

func defaultAnswers() []defaultAnswer {
	return []defaultAnswer{
		{
			name: "unseeded owner reads the USDC fixture",
			relies: "flow 01e AccountHasBalance relies on an unseeded member wallet reading the 25.50 USDC fixture, " +
				"and flow 09 (ok, AssetUntradable, NoRoute, AssetPaused, OverTradeCap, JupiterUnavailable), " +
				"flow 22e ok and flow 08's deposits (ok, Dust, UnknownAsset, BounceFailed) rely on an unseeded " +
				"treasury reading it (pot_cash_short otherwise)",
			answers: answersUnseededUSDC,
		},
		{
			name:    "unseeded owner holds no stock",
			relies:  "flow 10 relies on an unseeded owner holding no AAPLx",
			answers: answersUnseededStock,
		},
		{
			name: "recorded flow 01e wallets hold no USDC",
			relies: "flow 01e ok, AccountHasPositions, AccountHasArrivingDeposit and LeaveCreatorWithMembers rely on " +
				"their actors' recorded wallets holding no USDC",
			answers: answersRecordedWallets,
		},
		{
			name: "privy issues the recorded user and wallet ids",
			relies: walletFlows + " create Privy users and wallets, and flow 01e's owner fixtures are keyed by " +
				"the addresses the recorded ids did:privy:fake-N and wallet-N derive",
			answers: answersRecordedIDs,
		},
		{
			name:    "privy answers every recorded user",
			relies:  walletFlows + " sign in the recorded did:privy:qa-* users",
			answers: answersRecordedUsers,
		},
		{
			name: "privy lists, creates and attaches deposit wallets",
			relies: walletFlows + " rely on Privy listing a user's wallets, creating EVM deposit wallets and " +
				"attaching the deposit automation",
			answers: answersDepositWallets,
		},
		{
			name:    "jupiter quotes 25 USDC at 11 AAPLx",
			relies:  "flows 09, 11 and 12 rely on the default order fixture quoting 25 USDC at 11 AAPLx",
			answers: answersQuote,
		},
		{
			name: "jupiter orders, signs, executes and lands a treasury swap",
			relies: "flows 11 and 12 rely on a treasury's order coming back as req-N with a transaction its Privy " +
				"wallet signs, and on execute landing it for the quoted amounts",
			answers: answersTreasurySwap,
		},
		{
			name: "rpc sends and finalizes a relayer transaction",
			relies: "flows 07, 08, 14, 15, 25 and 29 rely on a blockhash, a block height, sendTransaction " +
				"answering the transaction's signature, and flows 07, 08, 15 and 25 on that signature reading finalized",
			answers: answersRelayerTransaction,
		},
		{
			name: "rpc answers flow 08's recorded deposits",
			relies: "flow 08's deposit poller relies on the recorded signatures for " + depositWatched +
				" and their recorded transactions",
			answers: answersRecordedDeposits,
		},
		{
			name: "rpc answers the AAPLx mint",
			relies: everyFlow + " relies on getEpochInfo and getMultipleAccounts answering the AAPLx mint, and " +
				"flows 07, 08, 09, 14, 15, 22e, 25 and 29 on getAccountInfo answering it",
			answers: answersMint,
		},
		{
			name: "market data lists AAPLx",
			relies: everyFlow + " relies on the xstocks, tessera and prestocks catalogs answering, with AAPLx " +
				"in xstocks, and flows 00, 01c, 01d, 10a, 18, 22, 22a, 24, 34 and 39 on Jupiter prices answering",
			answers: answersMarketData,
		},
		{
			name: "sinks accept what the flows send",
			relies: "most flows send PostHog batches, flows 02, 03, 04, 09, 11, 12, 14, 22-22f, 29, 36 and 38 " +
				"publish to Ably, flows 01e and 04 revoke Ably tokens, flows 24 and 33 push to APNs and flow 01e " +
				"lists avatars in storage",
			answers: answersSinks,
		},
	}
}

func answersUnseededUSDC(t *testing.T, w contractWorld) string {
	t.Helper()
	got, failed := w.usdc(t, unset)
	if failed != "" || got != money.NewBaseUnits(25_500_000, 6) {
		return "USDC balance " + got.String() + " " + failed
	}
	return ""
}

func answersUnseededStock(t *testing.T, w contractWorld) string {
	t.Helper()
	got, err := w.rpc.TokenBalance(t.Context(), unset, chain.Mint{Address: aaplxMint, Decimals: 8})
	if err != nil || got != money.NewBaseUnits(0, 8) {
		return "AAPLx balance " + got.String()
	}
	return ""
}

func answersRecordedWallets(t *testing.T, w contractWorld) string {
	t.Helper()
	for _, owner := range []chain.SolanaAddress{
		"32ykQ23PtuLRWEb2JtXVycRtQo1jXzGUBvLSmDWnPXtj", "3Ro1umCFbJiYFgnQv83FmuAe2qoYSPHnzsy77rcLtW6B",
		"8Nidxbbqw5bbQPpCj8LPhWnVwa6vhr8khZu2HwjvySen", "Ewxux2iqc2DVuvR8KFHoSGJQG5zW4Dm6KgqZ9VKtxBFM",
	} {
		if got, failed := w.usdc(t, owner); failed != "" || !got.IsZero() {
			return string(owner) + " USDC balance " + got.String() + " " + failed
		}
	}
	return ""
}

func answersRecordedIDs(t *testing.T, w contractWorld) string {
	t.Helper()
	user := w.call(t, http.MethodPost, "/privy/v1/users",
		`{"linked_accounts":[{"type":"email","address":"a@example.test"}]}`)
	wallet := w.call(t, http.MethodPost, "/privy/v1/wallets", `{"chain_type":"solana","owner_id":"u"}`)
	if at(user.body, "id") != "did:privy:fake-1" || at(wallet.body, "id") != "wallet-1" {
		return "first user " + answered(user) + ", first wallet " + answered(wallet)
	}
	return ""
}

func answersRecordedUsers(t *testing.T, w contractWorld) string {
	t.Helper()
	users := os.DirFS("testdata/fakes/privy/v1/users")
	paths, err := fs.Glob(users, "*.json")
	if err != nil || len(paths) == 0 {
		return "no recorded users"
	}
	for _, path := range paths {
		raw, _ := fs.ReadFile(users, path)
		id, _ := at(string(raw), "body", "id").(string)
		got, err := w.privy.GetUser(t.Context(), chainprivy.UserID(id))
		if err != nil || string(got.ID) != id {
			return path + ": " + string(got.ID)
		}
	}
	return ""
}

func answersDepositWallets(t *testing.T, w contractWorld) string {
	t.Helper()
	if _, err := w.privy.ListUserWallets(t.Context(), "did:privy:qa-del-ok"); err != nil {
		return "list wallets: " + err.Error()
	}
	evm, err := w.privy.CreateAppEVMWallet(t.Context(), "contract-evm")
	if err != nil {
		return "create EVM wallet: " + err.Error()
	}
	if _, err := w.privy.CreateAutomation(t.Context()); err != nil {
		return "create automation: " + err.Error()
	}
	if err := w.privy.Attach(t.Context(), evm.ID, unset); err != nil {
		return "attach: " + err.Error()
	}
	return ""
}

func answersQuote(t *testing.T, w contractWorld) string {
	t.Helper()
	got := w.call(t, http.MethodGet, "/jupiter/swap/v2/order?inputMint="+string(usdcMint)+
		"&outputMint="+string(aaplxMint)+"&amount=25000000", "")
	if got.status != http.StatusOK || at(got.body, "outAmount") != "11000000" ||
		at(got.body, "routePlan", "0") == nil {
		return answered(got)
	}
	return ""
}

func answersTreasurySwap(t *testing.T, w contractWorld) string {
	t.Helper()
	const treasury = "treasury-contract"
	w.call(t, http.MethodPost, "/_wallet", `{"id":"`+treasury+`"}`)
	order := w.call(t, http.MethodGet, "/jupiter/swap/v2/order?inputMint="+string(usdcMint)+
		"&outputMint="+string(aaplxMint)+"&amount=25000000&slippageBps=50&taker="+
		string(fakes.PrivyWalletAddress(treasury)), "")
	encoded, _ := at(order.body, "transaction").(string)
	unsigned, _ := base64.StdEncoding.DecodeString(encoded)
	if at(order.body, "requestId") != "req-1" || len(unsigned) == 0 {
		return "order " + answered(order)
	}
	signed, err := w.privy.SignTransaction(t.Context(), treasury, unsigned)
	if err != nil {
		return "sign: " + err.Error()
	}
	execute := w.call(t, http.MethodPost, "/jupiter/swap/v2/execute", `{"signedTransaction":"`+
		base64.StdEncoding.EncodeToString(signed)+`","requestId":"req-1"}`)
	signature, _ := at(execute.body, "signature").(string)
	if at(execute.body, "status") != "Success" || at(execute.body, "inputAmountResult") != "25000000" ||
		at(execute.body, "outputAmountResult") != "11000000" {
		return "execute " + answered(execute)
	}
	if landed := w.rpcCall(t, "getTransaction", `["`+signature+`",{"encoding":"jsonParsed"}]`); at(
		landed.body, "result") == nil {
		return "getTransaction " + answered(landed)
	}
	return ""
}

func answersRelayerTransaction(t *testing.T, w contractWorld) string {
	t.Helper()
	if got := w.rpcCall(
		t,
		"getLatestBlockhash",
		`[]`,
	); at(
		got.body,
		"result",
		"value",
		"blockhash",
	) == nil {
		return "getLatestBlockhash " + answered(got)
	}
	if got := w.rpcCall(t, "getBlockHeight", `[]`); at(got.body, "result") == nil {
		return "getBlockHeight " + answered(got)
	}
	tx := append([]byte{1}, []byte(strings.Repeat("\x07", 64)+"message")...)
	sent := w.rpcCall(t, "sendTransaction", `["`+base64.StdEncoding.EncodeToString(tx)+`"]`)
	signature := string(chain.SignatureOf(tx[1:65]))
	if at(sent.body, "result") != signature {
		return "sendTransaction " + answered(sent)
	}
	status := w.rpcCall(t, "getSignatureStatuses", `[["`+signature+`"]]`)
	if at(status.body, "result", "value", "0", "confirmationStatus") != "finalized" {
		return "getSignatureStatuses " + answered(status)
	}
	return ""
}

func answersRecordedDeposits(t *testing.T, w contractWorld) string {
	t.Helper()
	if got := w.rpcCall(t, "getSignaturesForAddress", `["`+depositWatched+`"]`); at(
		got.body, "result", "0") == nil {
		return "getSignaturesForAddress " + answered(got)
	}
	for _, signature := range []string{
		"3EWpx8usE7nk6a5xEYswYH2GQDcFJVtceYLWoptFi1dMZsyiWg4sShQrMKPce1vhq2ejTDSR8eE7JoJ8QwfuwnoC",
		"4ABXAAQc1LWqaGi5dMVZZEf12hykGXmHWU6zR3bkUruM6pnNaV78QHVRp2M53ojHEbvRpCWE2cKMZoQEugfoNi4h",
		"4cu1Tu8V1ZuQdTNo5qjiDtDfaFsoaKmRW6dXW8pVSsHMhPcV1wsCDzDLrCzTehfLfSTQSva26SS7uUUVB8nswqp9",
		"r3oLbEs3YRBcwXJUqrYiQiAU4xahpmcfwXZdMk77oxeyn7Y53tTxZ8yvySU1DNvMaaYqrodaEUoSshRQk67uYau",
		"sfLpFuMu7roifBc8qhCc2tnFUKtyscedzXgdRgNnm22w6YvDmYogC9UkjgYWZ6Md5A6a215gfE5PGcb6CAPrA1k",
	} {
		if got := w.rpcCall(t, "getTransaction", `["`+signature+`"]`); at(got.body, "result") == nil {
			return "getTransaction " + signature + " " + answered(got)
		}
	}
	return ""
}

func answersMint(t *testing.T, w contractWorld) string {
	t.Helper()
	if got := w.rpcCall(t, "getEpochInfo", `[]`); at(got.body, "result", "epoch") == nil {
		return "getEpochInfo " + answered(got)
	}
	mint := string(aaplxMint)
	if got := w.rpcCall(t, "getMultipleAccounts", `[["`+mint+`"]]`); at(
		got.body, "result", "value", "0") == nil {
		return "getMultipleAccounts " + answered(got)
	}
	if got := w.rpcCall(t, "getAccountInfo", `["`+mint+`"]`); at(got.body, "result", "value") == nil {
		return "getAccountInfo " + answered(got)
	}
	return ""
}

func answersMarketData(t *testing.T, w contractWorld) string {
	t.Helper()
	for _, path := range []string{
		"/xstocks/api/v2/public/assets", "/tessera/v1/public/token-details", "/prestocks/api/prestocks",
		"/jupiter/price/v3?ids=" + string(aaplxMint),
	} {
		if got := w.call(t, http.MethodGet, path, ""); got.status != http.StatusOK {
			return path + " " + answered(got)
		}
	}
	if got := w.call(t, http.MethodGet, "/xstocks/api/v2/public/assets", ""); !strings.Contains(
		got.body, string(aaplxMint)) {
		return "xstocks assets without AAPLx"
	}
	return ""
}

func answersSinks(t *testing.T, w contractWorld) string {
	t.Helper()
	for _, c := range []struct {
		path, body string
		header     []string
		want       int
	}{
		{"/posthog/batch/", batch(event(firstID, stamp(), "")), nil, http.StatusOK},
		{
			"/ably/channels/c/messages", `{"name":"n","data":"d"}`,
			[]string{"Authorization", "Basic a2V5"},
			http.StatusCreated,
		},
		{
			"/ably/keys/verify.key/revokeTokens", `{"targets":["clientId:u"]}`,
			[]string{"Authorization", "Basic a2V5"},
			http.StatusCreated,
		},
		{"/apns/3/device/abc", `{}`, []string{"apns-topic", "xyz.monacolabs.app"}, http.StatusOK},
		{
			"/storage/v1/object/list/avatars", `{"prefix":"u"}`,
			[]string{"Authorization", "Bearer k", "apikey", "k"},
			http.StatusOK,
		},
	} {
		if got := w.call(t, http.MethodPost, c.path, c.body, c.header...); got.status != c.want {
			return c.path + " " + answered(got)
		}
	}
	return ""
}

func TestContract_theDefaultFakesGiveTheAnswersTheFlowsRelyOn(t *testing.T) {
	t.Parallel()
	for _, c := range defaultAnswers() {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := c.answers(t, newContractWorld(t)); got != "" {
				t.Errorf("%s, but the default fakes answered: %s", c.relies, got)
			}
		})
	}
}
