package adapters

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
)

const (
	aaplC         = "0xb200000000000000000000c2e324d24d7eecd1fb"
	aaplOn        = "0x14c3abf95cb9c93a8b82c1cdcb76d72cb87b2d4c"
	aaplX         = "0x9d275685dc284c8eb1c79f6aba7a63dc75ec890a"
	aaplXWrapper  = "0x943bf64d566c32a2bcd41ac92fb63c111cc9de8f"
	aaplXSolana   = "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp"
	wrappedSOL    = "So11111111111111111111111111111111111111112"
	senderPolicy  = policyIDSelector + "b81736c875ab819dd97f59f2a6542cfb731ad52b4ae15a6f24df2fb02b0327f5"
	authorizedFor = isAuthorizedSelector +
		"0000000000000000000000000000000000000000000000000000000000000005" +
		"0000000000000000000000001111111111111111111111111111111111111111"
	word5    = "0x0000000000000000000000000000000000000000000000000000000000000005"
	word1    = "0x0000000000000000000000000000000000000000000000000000000000000001"
	word0    = "0x0000000000000000000000000000000000000000000000000000000000000000"
	word8    = "0x0000000000000000000000000000000000000000000000000000000000000008"
	word18   = "0x0000000000000000000000000000000000000000000000000000000000000012"
	word37   = "0x0000000000000000000000000000000000000000000000000000000000000025"
	aaplCSym = "0x0000000000000000000000000000000000000000000000000000000000000020" +
		"0000000000000000000000000000000000000000000000000000000000000005" +
		"4141504c63000000000000000000000000000000000000000000000000000000"
	aaplOnSym = "0x0000000000000000000000000000000000000000000000000000000000000020" +
		"0000000000000000000000000000000000000000000000000000000000000006" +
		"4141504c6f6e0000000000000000000000000000000000000000000000000000"
	aaplXSym = "0x0000000000000000000000000000000000000000000000000000000000000020" +
		"0000000000000000000000000000000000000000000000000000000000000005" +
		"4141504c78000000000000000000000000000000000000000000000000000000"
	wAAPLXSym = "0x0000000000000000000000000000000000000000000000000000000000000020" +
		"0000000000000000000000000000000000000000000000000000000000000006" +
		"774141504c780000000000000000000000000000000000000000000000000000"
)

func recordedCalls() map[string]string {
	return map[string]string{
		aaplC + " " + symbolSelector:   aaplCSym,
		aaplC + " " + decimalsSelector: word8,
		aaplC + " " + senderPolicy:     word5,
		aaplC + " " + policyIDSelector + "8a4b3fa2d8b921852bc0089c6ef0958aa6961897be36fd731330fe2cd23f8363": word5,
		aaplC + " " + policyIDSelector + "10be5173aff2a44e748bd9acd8b19fe34689581398a9db7ba2fb671e786ff7d8": word5,
		b20PolicyRegistry + " " + authorizedFor: word1,
		aaplOn + " " + symbolSelector:           aaplOnSym,
		aaplOn + " " + decimalsSelector:         word18,
		aaplX + " " + symbolSelector:            aaplXSym,
		aaplX + " " + decimalsSelector:          word18,
		aaplXWrapper + " " + symbolSelector:     wAAPLXSym,
	}
}

type rpcNode struct {
	calls  map[string]string
	status int
	raw    string
}

func (n *rpcNode) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req rpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Method != "eth_call" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	args, _ := req.Params[0].(map[string]any)
	if n.status != 0 {
		w.WriteHeader(n.status)
	}
	if n.raw != "" {
		_, _ = w.Write([]byte(n.raw))
		return
	}
	result, ok := n.calls[args["to"].(string)+" "+args["data"].(string)]
	if !ok {
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":3,"message":"execution reverted"}}`))
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": result})
}

type mints map[chain.SolanaAddress]uint8

func (m mints) MintConfig(_ context.Context, mint chain.SolanaAddress) (solana.MintConfig, error) {
	d, ok := m[mint]
	if !ok {
		return solana.MintConfig{}, errs.New(errs.CodeInvalidAddress, "test.MintConfig")
	}
	return solana.MintConfig{Mint: chain.Mint{Address: mint, Decimals: d}}, nil
}

func verifierFor(t *testing.T, node *rpcNode) *ListingVerifier {
	t.Helper()
	srv := httptest.NewServer(node)
	t.Cleanup(srv.Close)
	c := httpclient.New("evm-rpc", httpclient.WithTimeout(10*time.Second))
	return NewListingVerifier(c, func(chain.Network) string { return srv.URL },
		mints{aaplXSolana: 8})
}

func TestListingVerifier_passesTheRecordedLaunchTokens(t *testing.T) {
	t.Parallel()
	v := verifierFor(t, &rpcNode{calls: recordedCalls()})
	for _, tc := range []struct {
		listing app.ListingToVerify
		want    uint8
	}{
		{app.ListingToVerify{Issuer: "coinbase", Symbol: "AAPLc", Network: chain.NetworkBase, TokenAddress: aaplC}, 8},
		{app.ListingToVerify{Issuer: "ondo", Symbol: "AAPLon", Network: chain.NetworkEthereum, TokenAddress: aaplOn}, 18},
		{app.ListingToVerify{
			Issuer: "xstocks", Symbol: "AAPLx", Network: chain.NetworkEthereum, TokenAddress: aaplX,
			WrapperAddress: aaplXWrapper,
		}, 18},
		{app.ListingToVerify{
			Issuer: "xstocks", Symbol: "AAPLx", Network: chain.NetworkSolana,
			TokenAddress: aaplXSolana,
		}, 8},
	} {
		if got, err := v.Verify(t.Context(), tc.listing); err != nil || got != tc.want {
			t.Errorf("Verify(%s %s) = %d, %v, want %d", tc.listing.Symbol, tc.listing.Network, got, err, tc.want)
		}
	}
}

func TestListingVerifier_failsEveryListingThatDoesNotCheckOut(t *testing.T) {
	t.Parallel()
	with := func(key, result string) map[string]string {
		calls := recordedCalls()
		calls[key] = result
		return calls
	}
	without := func(key string) map[string]string {
		calls := recordedCalls()
		delete(calls, key)
		return calls
	}
	aaplOnBase := app.ListingToVerify{
		Issuer:       "coinbase",
		Symbol:       "AAPLc",
		Network:      chain.NetworkBase,
		TokenAddress: aaplC,
	}
	for name, tc := range map[string]struct {
		node    *rpcNode
		listing app.ListingToVerify
		reason  string
	}{
		"another symbol": {
			&rpcNode{calls: recordedCalls()},
			app.ListingToVerify{Symbol: "TSLAon", Network: chain.NetworkEthereum, TokenAddress: aaplOn},
			"symbol=AAPLon want=TSLAon",
		},
		"a wrapper of another token": {
			&rpcNode{calls: recordedCalls()},
			app.ListingToVerify{
				Symbol: "AAPLx", Network: chain.NetworkEthereum, TokenAddress: aaplX,
				WrapperAddress: aaplOn,
			},
			"symbol=AAPLon want=wAAPLx",
		},
		"a symbol that is not an ABI string": {
			&rpcNode{calls: with(aaplOn+" "+symbolSelector, word5)},
			app.ListingToVerify{Symbol: "AAPLon", Network: chain.NetworkEthereum, TokenAddress: aaplOn},
			"symbol= want=AAPLon",
		},
		"no decimals": {
			&rpcNode{calls: without(aaplOn + " " + decimalsSelector)},
			app.ListingToVerify{Symbol: "AAPLon", Network: chain.NetworkEthereum, TokenAddress: aaplOn},
			"reverted=execution reverted",
		},
		"more decimals than the catalog stores": {
			&rpcNode{calls: with(aaplOn+" "+decimalsSelector, word37)},
			app.ListingToVerify{Symbol: "AAPLon", Network: chain.NetworkEthereum, TokenAddress: aaplOn},
			"decimals=37",
		},
		"a short decimals word": {
			&rpcNode{calls: with(aaplOn+" "+decimalsSelector, "0x12")},
			app.ListingToVerify{Symbol: "AAPLon", Network: chain.NetworkEthereum, TokenAddress: aaplOn},
			"decode_failed",
		},
		"an allowlist": {
			&rpcNode{calls: with(b20PolicyRegistry+" "+authorizedFor, word0)}, aaplOnBase,
			"policy=5 reason=transfer policy is an allowlist",
		},
		"no policy": {&rpcNode{calls: without(aaplC + " " + senderPolicy)}, aaplOnBase, "execution reverted"},
		"no registry": {
			&rpcNode{calls: without(b20PolicyRegistry + " " + authorizedFor)}, aaplOnBase,
			"execution reverted",
		},
		"a node that is down":      {&rpcNode{status: http.StatusBadGateway}, aaplOnBase, "upstream"},
		"a node that refuses":      {&rpcNode{status: http.StatusForbidden}, aaplOnBase, "upstream_unavailable"},
		"a node that answers HTML": {&rpcNode{raw: "<html>"}, aaplOnBase, "decode_failed"},
		"a result that is not hex": {&rpcNode{raw: `{"result":"0xzz"}`}, aaplOnBase, "decode_failed"},
		"a mint that is not a token": {
			&rpcNode{},
			app.ListingToVerify{Symbol: "AAPLx", Network: chain.NetworkSolana, TokenAddress: wrappedSOL},
			"invalid_address",
		},
	} {
		got, err := verifierFor(t, tc.node).Verify(t.Context(), tc.listing)
		if err == nil || !strings.Contains(app.ListingCheck{Err: err}.Reason(), tc.reason) {
			t.Errorf("%s: Verify = %d, %v %v, want an error naming %q", name, got, err, errs.Detail(err), tc.reason)
		}
	}
}

func TestListingVerifier_refusesAnRPCURLItCannotRequest(t *testing.T) {
	t.Parallel()
	v := NewListingVerifier(httpclient.New("evm-rpc", httpclient.WithTimeout(time.Second)),
		func(chain.Network) string { return "://no-scheme" }, mints{})
	_, err := v.Verify(t.Context(), app.ListingToVerify{
		Symbol: "AAPLon", Network: chain.NetworkEthereum,
		TokenAddress: aaplOn,
	})
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("Verify = %v, want invalid_input", err)
	}
}

func TestABIString_refusesOffsetsAndLengthsPastTheData(t *testing.T) {
	t.Parallel()
	word := func(n byte) []byte {
		b := make([]byte, abiWordSize)
		b[abiWordSize-1] = n
		return b
	}
	huge := word(0)
	huge[0] = 1
	for name, data := range map[string][]byte{
		"empty":            nil,
		"offset past data": slices.Concat(word(64), word(1)),
		"offset too big":   slices.Concat(huge, word(1)),
		"length past data": slices.Concat(word(32), word(33)),
	} {
		if got, ok := abiString(data); ok {
			t.Errorf("%s: abiString = %q, want refused", name, got)
		}
	}
}
