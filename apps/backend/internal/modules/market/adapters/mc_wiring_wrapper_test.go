package adapters

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	executionQuoted   = 1_427_581
	executionReceived = 1_314_373
)

func TestMultichainRoutingQuoterGivesExecutionTheSimulatedOutput(t *testing.T) {
	t.Parallel()
	uni := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if req.URL.Path == "/v1/swap" {
			_, _ = io.WriteString(w, `{"requestId":"s1","swap":{"to":"0xd6145b2d3f379919e8cdeda7b97e37c4b2ca9c40",`+
				`"data":"0x3593564c00","value":"0x00","chainId":8453}}`)
			return
		}
		_, _ = io.WriteString(w, `{"requestId":"q1","routing":"CLASSIC","permitData":null,"permitTransaction":null,`+
			`"quote":{"quoteId":"q","input":{"amount":"4896325"},"output":{"amount":"1427581"},"gasFeeUSD":"0.01",`+
			`"route":[[{"fee":"10000"}]]}}`)
	}))
	t.Cleanup(uni.Close)
	rpc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(req.Body).Decode(&body)
		if body.Method == "eth_getCode" {
			_, _ = io.WriteString(
				w,
				`{"jsonrpc":"2.0","id":1,"result":"0xef0100d6cedde84be40893d153be9d467cd6ad37875b28"}`,
			)
			return
		}
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"error":{"code":3,"data":"0x8b063d73%064x%064x"}}`,
			executionQuoted, executionReceived)
	}))
	t.Cleanup(rpc.Close)
	deps := routingDeps("uni-key")
	deps.Config.Routing.UniswapBaseURL, deps.Config.EVM.RPCURLBase = uni.URL, rpc.URL
	listing := multichain.Listing{
		Network: chain.NetworkBase, Venue: multichain.VenueUniswap, Decimals: 8, SharesPerToken: big.NewRat(1, 1),
		TokenAddress: "0xb2000000000000000000002d0ba3164cc74f58b7",
	}

	q, err := MultichainRoutingQuoter(deps).Quote(t.Context(), listing, multichain.SideBuy,
		money.MicrosFromUint64(4_896_325))

	if err != nil || q.GrossOutput.Int64() != executionReceived {
		t.Fatalf("execution quote = %v, %v, want the %d the sponsored account receives, not the quoted %d",
			q.GrossOutput, err, executionReceived, executionQuoted)
	}
}
