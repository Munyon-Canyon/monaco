package evmsim_test

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/sha3"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/evmsim"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
)

const (
	owner  = "0xf15262a7ee9d20fa690185d58e0eb2aad475bf32"
	router = "0xd6145b2d3f379919e8cdeda7b97e37c4b2ca9c40"
)

type node struct {
	mu     sync.Mutex
	reply  string
	bodies []map[string]any
}

func newNode(t *testing.T, reply string) (*node, *httpclient.Client) {
	t.Helper()
	n := &node{reply: reply}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		n.mu.Lock()
		n.bodies = append(n.bodies, body)
		n.mu.Unlock()
		_, _ = io.WriteString(w, n.reply)
	}))
	t.Cleanup(srv.Close)
	return n, httpclient.New("evmsim-test", httpclient.WithBaseURL(srv.URL), httpclient.WithTimeout(time.Second))
}

func address(t *testing.T, raw string) chain.EVMAddress {
	t.Helper()
	a, err := chain.ParseEVMAddress(raw)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func swap(t *testing.T) evmsim.Swap {
	t.Helper()
	return evmsim.Swap{
		Owner:    address(t, owner),
		Router:   address(t, router),
		Calldata: []byte{0x35, 0x93, 0x56, 0x4c, 0},
	}
}

func revert(data string) string {
	return `{"jsonrpc":"2.0","id":1,"error":{"code":3,"message":"execution reverted","data":"` + data + `"}}`
}

func words(vs ...int64) string {
	var out strings.Builder
	for _, v := range vs {
		out.WriteString(hex.EncodeToString(big.NewInt(v).FillBytes(make([]byte, 32))))
	}
	return out.String()
}

func selector(signature string) string {
	h := sha3.NewLegacyKeccak256()
	h.Write([]byte(signature))
	return "0x" + hex.EncodeToString(h.Sum(nil)[:4])
}

func TestSimulate_callsExecuteFromTheEntryPointAt3MGasOnRealState(t *testing.T) {
	t.Parallel()
	n, client := newNode(t, `{"jsonrpc":"2.0","id":1,"result":"0x"}`)

	received, met, err := evmsim.Simulate(t.Context(), client, swap(t), nil, nil)

	if err != nil || !met || received != nil {
		t.Fatalf("Simulate = %v, %v, %v, want the quote met", received, met, err)
	}
	params, _ := n.bodies[0]["params"].([]any)
	call, _ := params[0].(map[string]any)
	wantData := "0xe9ae5c53" + words(0, 64, 57) + strings.TrimPrefix(router, "0x") + words(0) + "3593564c00" +
		strings.Repeat("0", 2*7)
	if n.bodies[0]["method"] != "eth_call" || len(params) != 2 || params[1] != "latest" ||
		call["from"] != evmsim.EntryPoint || call["to"] != owner || call["gas"] != "0x2dc6c0" ||
		call["data"] != wantData {
		t.Fatalf("eth_call = %+v, want execute(0, router||0||calldata) from EntryPoint at 3M gas, no overrides",
			n.bodies[0])
	}
}

func TestSimulate_sendsOverridesWhenGiven(t *testing.T) {
	t.Parallel()
	n, client := newNode(t, `{"jsonrpc":"2.0","id":1,"result":"0x"}`)
	diff := map[string]evmsim.Override{"0xcash": {StateDiff: map[string]string{"0x01": evmsim.Word(big.NewInt(5))}}}

	if _, _, err := evmsim.Simulate(t.Context(), client, swap(t), diff, nil); err != nil {
		t.Fatal(err)
	}
	params, _ := n.bodies[0]["params"].([]any)
	overrides, _ := params[2].(map[string]any)
	cash, _ := overrides["0xcash"].(map[string]any)
	state, _ := cash["stateDiff"].(map[string]any)
	if len(params) != 3 || state["0x01"] != "0x"+words(5) {
		t.Fatalf("params = %+v, want the state diff as the third param", params)
	}
}

func reason(err error) string {
	for _, a := range errs.Detail(err) {
		if a.Key == "reason" {
			return a.Value.String()
		}
	}
	return ""
}

func TestSimulate_readsWhatTheRevertSays(t *testing.T) {
	t.Parallel()
	v4 := selector("V4TooLittleReceived(uint256,uint256)")
	v3 := selector("V3TooLittleReceived()")
	for name, c := range map[string]struct {
		reply    string
		received int64
		want     errs.Code
		reason   string
	}{
		"v4 short":          {revert(v4 + words(1_427_581, 1_314_373)), 1_314_373, "", ""},
		"v4 truncated":      {revert(v4 + words(1)), 0, errs.CodeNoRoute, "simulation_reverted"},
		"v3 short":          {revert(v3), 0, errs.CodeNoRoute, "too_little_received_unmeasured"},
		"another revert":    {revert("0x08c379a0"), 0, errs.CodeNoRoute, "simulation_reverted"},
		"no data":           {`{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"invalid params"}}`, 0, errs.CodeNoRoute, "simulation_reverted"},
		"no result":         {`{"jsonrpc":"2.0","id":1}`, 0, errs.CodeNoRoute, "simulation_reverted"},
		"unreadable revert": {revert(v4 + "zz"), 0, errs.CodeNoRoute, "simulation_reverted"},
		"unreadable reply":  {`{`, 0, errs.CodeDecodeFailed, ""},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, client := newNode(t, c.reply)
			received, met, err := evmsim.Simulate(t.Context(), client, swap(t), nil, nil)
			if met || c.want == "" && (err != nil || received.Int64() != c.received) ||
				c.want != "" && errs.CodeOf(err) != c.want {
				t.Fatalf("Simulate = %v, %v, %v, want received %d and %q", received, met, err, c.received, c.want)
			}
			if got := reason(err); got != c.reason {
				t.Fatalf("reason = %q, want %q", got, c.reason)
			}
		})
	}
}

func TestSimulate_anUnreachableNodeIsRetryable(t *testing.T) {
	t.Parallel()
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	client := httpclient.New("evmsim-dead", httpclient.WithBaseURL(dead.URL), httpclient.WithTimeout(time.Second))

	_, _, err := evmsim.Simulate(t.Context(), client, swap(t), nil, nil)
	if errs.CodeOf(err) != errs.CodeRPCUnavailable {
		t.Fatalf("Simulate err = %v, want rpc_unavailable", err)
	}
}

func TestDelegation_readsAnyEIP7702Delegate(t *testing.T) {
	t.Parallel()
	code := func(v string) evmsim.Reply { return evmsim.Reply{Result: &v} }
	for name, c := range map[string]struct {
		reply evmsim.Reply
		want  string
	}{
		"privy delegate": {code("0xef0100D6CEDDE84BE40893D153BE9D467CD6AD37875B28"), "0xd6cedde84be40893d153be9d467cd6ad37875b28"},
		"other delegate": {code("0xef01000000000000000000000000000000000000000001"), "0x0000000000000000000000000000000000000001"},
		"no code":        {code("0x"), ""},
		"contract":       {code("0x6080604052"), ""},
		"truncated":      {code("0xef0100d6cedde8"), ""},
		"no result":      {evmsim.Reply{}, ""},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			delegate, ok := evmsim.Delegation(c.reply)
			if ok != (c.want != "") || ok && delegate != address(t, c.want) {
				t.Fatalf("Delegation = %s, %v, want %q", delegate, ok, c.want)
			}
		})
	}
}
