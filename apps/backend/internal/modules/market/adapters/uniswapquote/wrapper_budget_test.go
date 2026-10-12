package uniswapquote_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/uniswapquote"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	mc "github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type hookless struct{}

func (hookless) RoundTrip(req *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	if bytes.Contains(body, []byte(mc.NoHooks)) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	return http.DefaultTransport.RoundTrip(req)
}

type spendOnSimulation struct{ spend context.CancelFunc }

type spentBody struct {
	io.ReadCloser
	spend context.CancelFunc
}

func (b spentBody) Close() error {
	defer b.spend()
	return b.ReadCloser.Close()
}

func (s spendOnSimulation) RoundTrip(req *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	req.Body = io.NopCloser(bytes.NewReader(body))
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err == nil && bytes.Contains(body, []byte(`"eth_call"`)) {
		resp.Body = spentBody{ReadCloser: resp.Body, spend: s.spend}
	}
	return resp, err
}

func TestWrapper_keepsTheFinishedRouteWhenTheOtherOutlivesTheBudget(t *testing.T) {
	t.Parallel()
	r := newWrapperRig(t, chain.NetworkBase)
	budget, spend := context.WithCancel(t.Context())
	defer spend()
	owner, _ := chain.ParseEVMAddress(liveOwner)
	delegate, _ := chain.ParseEVMAddress(uniswapquote.DefaultDelegate)
	rpc := httpclient.New("evm-rpc-test", httpclient.WithBaseURL(r.rpcURL), httpclient.WithTimeout(time.Second),
		httpclient.WithTransport(spendOnSimulation{spend: spend}))
	uni := httpclient.New("uniswap-wrapper-test",
		append(uniswapquote.Options(r.uniBase, 5*time.Second), httpclient.WithTransport(hookless{}))...)
	client := uniswapquote.New(uni, "test-key", uniswapquote.Stored(owner), r.clock).
		WithWrapper(uniswapquote.NewWrapper(map[chain.Network]*httpclient.Client{chain.NetworkBase: rpc}, delegate))

	q, err := client.Quote(budget, b20Listing(t, chain.NetworkBase), mc.SideBuy, money.MicrosFromUint64(liveInput))

	if err != nil || q.RouteID != "q-live" || q.Options.HooksOptions != "" || q.GrossOutput.Int64() != receivedLive {
		t.Fatalf("quote = %v %s %+v, %v, want the default route penalized to %d after the hookless one ran out",
			q.GrossOutput, q.RouteID, q.Options, err, receivedLive)
	}
}
